// Package main implementa el nodo Master ("Coordinador") de nuestro clúster.
// Lee el CSV físico en streaming lote por lote y distribuye los registros
// uniformemente y de forma concurrente a los nodos Workers usando net/rpc.
//
// Para evitar contención de memoria en el Master:
// 1. No carga el CSV completo en memoria. Lee y procesa fila por fila en un búfer de streaming.
// 2. Agrupa los pacientes en lotes (ej., 5000) antes de enviarlos por red, disminuyendo el número
//    de llamadas RPC y reduciendo la contención en los canales de Go.
// 3. Usa un canal con búfer para los despachadores concurrentes (uno por worker de red).
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/report"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

func main() {
	datasetPath := flag.String("dataset", "data/patients.csv", "Ruta al archivo CSV de pacientes")
	workersStr := flag.String("workers", "localhost:8081,localhost:8082", "Direcciones TCP/RPC de los workers (separadas por comas)")
	batchSize := flag.Int("batch", 5000, "Tamaño del bloque de pacientes a enviar por llamada RPC")
	flag.Parse()

	startTime := time.Now()

	// Parsear direcciones de los workers
	workerAddrs := strings.Split(*workersStr, ",")
	for i := range workerAddrs {
		workerAddrs[i] = strings.TrimSpace(workerAddrs[i])
	}

	fmt.Println("====================================================")
	fmt.Println("   Master Coordinator - Distribución de Carga RPC   ")
	fmt.Println("====================================================")
	fmt.Printf("[master] Conectando a %d nodos workers...\n", len(workerAddrs))

	// Conectar a cada uno de los workers mediante RPC
	clients := make([]*rpc.Client, 0, len(workerAddrs))
	for _, addr := range workerAddrs {
		client, err := rpc.Dial("tcp", addr)
		if err != nil {
			log.Fatalf("[master] Error fatal: no se pudo conectar al worker en %s: %v", addr, err)
		}
		defer client.Close()
		fmt.Printf("[master]   - Worker en %s: CONECTADO\n", addr)
		clients = append(clients, client)
	}

	// Abrir el archivo CSV
	file, err := os.Open(*datasetPath)
	if err != nil {
		log.Fatalf("[master] Error al abrir el dataset: %v", err)
	}
	defer file.Close()

	// Buffer de lectura de 1MB para mitigar el cuello de botella de I/O de disco
	bufReader := bufio.NewReaderSize(file, 1*1024*1024)
	csvReader := csv.NewReader(bufReader)

	// Leer cabecera
	header, err := csvReader.Read()
	if err != nil {
		log.Fatalf("[master] Error leyendo cabecera: %v", err)
	}
	fmt.Printf("[master] Cabecera leída: %v\n", header)
	colIdx := indexColumns(header)
	fmt.Printf("[master] Mapa de columnas: %v\n", colIdx)

	// Canales de control
	batchesCh := make(chan []types.Patient, len(clients)*4)
	resultsCh := make(chan []types.PatientResult, len(clients)*4)
	statsCh := make(chan types.WorkerStats, len(clients)*4)

	var wgDispatchers sync.WaitGroup

	// Lanzar un despachador goroutine por cada conexión a worker
	// Esto implementa un balanceo de carga dinámico auto-regulado.
	// Si un worker es más rápido, procesará y vaciará el canal batchesCh más rápido.
	for i, client := range clients {
		wgDispatchers.Add(1)
		go func(workerID int, c *rpc.Client, addr string) {
			defer wgDispatchers.Done()
			for batch := range batchesCh {
				args := types.ProcessArgs{Patients: batch}
				var reply types.ProcessReply

				err := c.Call("WorkerService.ProcessBatch", &args, &reply)
				if err != nil {
					log.Printf("[master] Error en RPC worker %s: %v. Re-encolando lote...\n", addr, err)
					// Re-encolamos para garantizar tolerancia a fallos parciales
					batchesCh <- batch
					continue
				}

				resultsCh <- reply.Results
				statsCh <- reply.Stats
			}
		}(i+1, client, workerAddrs[i])
	}

	// Goroutine Productora: Lee el CSV en streaming y alimenta a los dispatchers
	var totalRecordsRead int64
	var discardedRecords int64
	go func() {
		defer close(batchesCh)
		fmt.Println("[master-producer] Iniciando lectura...")
		currentBatch := make([]types.Patient, 0, *batchSize)

		for {
			row, err := csvReader.Read()
			if err == io.EOF {
				fmt.Println("[master-producer] EOF alcanzado")
				break
			}
			if err != nil {
				fmt.Printf("[master-producer] Error de lectura: %v\n", err)
				discardedRecords++
				continue
			}

			p, ok := parsePatient(row, colIdx)
			if !ok {
				discardedRecords++
				continue
			}

			totalRecordsRead++
			currentBatch = append(currentBatch, p)

			if len(currentBatch) == *batchSize {
				batchesCh <- currentBatch
				currentBatch = make([]types.Patient, 0, *batchSize)
			}
		}

		// Enviar lote remanente
		if len(currentBatch) > 0 {
			batchesCh <- currentBatch
		}
		log.Printf("[master] Lectura del CSV finalizada. %d registros válidos, %d descartados.\n", totalRecordsRead, discardedRecords)
	}()

	// Goroutine para cerrar canales de resultados cuando los workers acaben
	go func() {
		wgDispatchers.Wait()
		close(resultsCh)
		close(statsCh)
	}()

	// Recolección y agregación final de datos (Map-Reduce consolidado)
	var allResults []types.PatientResult
	var allStats []types.WorkerStats

	// Hilo de consumo de resultados
	var wgAggregator sync.WaitGroup
	wgAggregator.Add(2)

	go func() {
		defer wgAggregator.Done()
		for r := range resultsCh {
			allResults = append(allResults, r...)
		}
	}()

	go func() {
		defer wgAggregator.Done()
		for s := range statsCh {
			allStats = append(allStats, s)
		}
	}()

	wgAggregator.Wait()
	elapsed := time.Since(startTime)

	// Imprimir el reporte unificado
	report.Print(allResults, allStats)

	fmt.Printf("\n[master] Procesamiento distribuido completado con éxito en: %v\n", elapsed)
	fmt.Printf("[master] Throughput del clúster: %.0f pacientes/s\n", float64(len(allResults))/elapsed.Seconds())
}

// indexColumns mapea los nombres de cabecera a su índice
func indexColumns(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

// parsePatient parsea y valida un registro individual
func parsePatient(row []string, idx map[string]int) (types.Patient, bool) {
	get := func(key string) string {
		i, ok := idx[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	id := get("id")
	if id == "" {
		return types.Patient{}, false
	}

	age, err := strconv.Atoi(get("age"))
	if err != nil || age < 0 || age > 120 {
		return types.Patient{}, false
	}

	psa, err := strconv.ParseFloat(get("psa"), 64)
	if err != nil || psa < 0 || psa > 200 {
		return types.Patient{}, false
	}

	income, _ := strconv.ParseFloat(get("income"), 64)
	cov, _ := strconv.ParseFloat(get("coverage"), 64)
	cost, _ := strconv.ParseFloat(get("healthcare_cost"), 64)
	enc, _ := strconv.Atoi(get("num_encounters"))
	diag, _ := strconv.Atoi(get("num_diagnoses"))
	died := strings.EqualFold(get("has_died"), "true") || get("has_died") == "1"
	sd, _ := strconv.Atoi(get("survival_days"))

	return types.Patient{
		ID:             id,
		Age:            age,
		Race:           strings.ToLower(get("race")),
		Ethnicity:      strings.ToLower(get("ethnicity")),
		MaritalStatus:  strings.ToLower(get("marital")),
		Income:         income,
		Coverage:       cov,
		HealthcareCost: cost,
		PSALevel:       psa,
		NumEncounters:  enc,
		NumDiagnoses:   diag,
		HasDied:        died,
		SurvivalDays:   sd,
	}, true
}
