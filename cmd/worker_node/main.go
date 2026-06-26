// Package main implementa el nodo Worker ("Esclavo") de nuestro clúster.
// Levanta un servidor RPC sobre TCP que recibe lotes de pacientes,
// ejecuta un procesamiento local concurrente e intensivo en CPU,
// y devuelve los resultados junto a las métricas del nodo.
package main

import (
	"flag"
	"log"
	"math"
	"net"
	"net/http"
	_ "net/http/pprof" // Habilita pprof
	"net/rpc"
	"runtime"
	"sync"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/models"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// WorkerService define el servicio RPC que expondrá este nodo.
type WorkerService struct {
	ID int
}

// ProcessBatch es el método expuesto por el servidor RPC para procesar un lote de pacientes.
func (ws *WorkerService) ProcessBatch(args *types.ProcessArgs, reply *types.ProcessReply) error {
	startTime := time.Now()
	numPatients := len(args.Patients)

	if numPatients == 0 {
		reply.Results = []types.PatientResult{}
		reply.Stats = types.WorkerStats{
			WorkerID:        ws.ID,
			PatientsHandled: 0,
			ProcessingTime:  0,
		}
		return nil
	}

	// 1. Normalización Min-Max concurrente del PSA antes de evaluar los modelos
	normalizedPSAs := models.ConcurrentlyNormalizePSA(args.Patients)

	// Pre-asignamos el slice con capacidad exacta para evitar re-asignaciones en memoria (contención y GC)
	results := make([]types.PatientResult, numPatients)

	// Determinamos el número de workers locales (goroutines) para paralelizar en el hardware del nodo.
	numLocalWorkers := runtime.NumCPU()
	var wg sync.WaitGroup

	chunkSize := numPatients / numLocalWorkers
	if chunkSize == 0 {
		chunkSize = 1
	}

	// Fan-out local de la CPU del Worker utilizando goroutines
	for i := 0; i < numLocalWorkers; i++ {
		start := i * chunkSize
		if start >= numPatients {
			break
		}
		end := start + chunkSize
		if i == numLocalWorkers-1 || end > numPatients {
			end = numPatients
		}

		wg.Add(1)
		go func(localWorkerID int, patientsSubset []types.Patient, normalizedSubset []float64, resultsTarget []types.PatientResult) {
			defer wg.Done()

			for idx, p := range patientsSubset {
				// Simulación de cálculo intensivo en CPU para emular entrenamiento/evaluación matricial profunda
				var cpuBurn float64
				for k := 0; k < 5000; k++ {
					cpuBurn += math.Sin(float64(k)) * math.Cos(float64(p.Age))
				}
				_ = cpuBurn // Evitar advertencia del compilador

				// Escribimos en el slice en base a su offset aislado sin contención de memoria
				resultsTarget[idx] = types.PatientResult{
					PatientID:        p.ID,
					MortalityRisk:    models.PredictMortality(p, normalizedSubset[idx]),
					SurvivalEstimate: models.PredictSurvival(p, normalizedSubset[idx]),
					TreatmentCost:    models.PredictTreatmentCost(p, normalizedSubset[idx]),
					WorkerID:         ws.ID, // Identificador de este nodo worker
				}
			}
		}(i+1, args.Patients[start:end], normalizedPSAs[start:end], results[start:end])
	}

	wg.Wait()

	reply.Results = results
	reply.Stats = types.WorkerStats{
		WorkerID:        ws.ID,
		PatientsHandled: numPatients,
		ProcessingTime:  time.Since(startTime),
	}

	log.Printf("[worker-%d] Procesados %d pacientes en %v\n", ws.ID, numPatients, reply.Stats.ProcessingTime)
	return nil
}

func main() {
	addr := flag.String("addr", "localhost:8081", "Dirección TCP para levantar el servidor RPC")
	pprofAddr := flag.String("pprof", "localhost:6061", "Dirección TCP para levantar el servidor de profiling pprof")
	workerID := flag.Int("id", 1, "Identificador único para este nodo worker")
	flag.Parse()

	// Iniciar servidor pprof asíncrono para monitoreo de performance
	go func() {
		log.Printf("[worker-%d] Servidor pprof activo en http://%s/debug/pprof/\n", *workerID, *pprofAddr)
		if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
			log.Printf("[worker-%d] Servidor pprof error: %v\n", *workerID, err)
		}
	}()

	// Registrar servicio RPC
	ws := &WorkerService{ID: *workerID}
	err := rpc.Register(ws)
	if err != nil {
		log.Fatalf("[worker-%d] Error al registrar servicio RPC: %v", *workerID, err)
	}

	// Abrir puerto TCP para escuchar al Master
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("[worker-%d] Error de red escuchando en %s: %v", *workerID, *addr, err)
	}
	defer listener.Close()

	log.Printf("[worker-%d] Servidor TCP/RPC escuchando en %s...\n", *workerID, *addr)

	// Bucle receptor de conexiones
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[worker-%d] Error al aceptar conexión: %v\n", *workerID, err)
			continue
		}
		// Servir la conexión de forma concurrente
		go rpc.ServeConn(conn)
	}
}
