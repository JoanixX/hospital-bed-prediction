// Package main implementa el analizador estadístico concurrente.
// Usa el patrón fan-out/fan-in para procesar el CSV gigante en paralelo.
//
// Para evitar la contención de memoria, los workers procesan lotes (batches)
// aislados y mantienen contadores/sumas locales sin compartir variables ni
// usar mutexes durante la fase de cómputo intensivo.
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ColStats almacena los agregados estadísticos intermedios para una columna.
type ColStats struct {
	Count float64 // float64 para evitar conversiones frecuentes
	Nulls int64
	Sum   float64
	SumSq float64
	Min   float64
	Max   float64
}

// NewColStats inicializa los límites estadísticos.
func NewColStats() ColStats {
	return ColStats{
		Min: math.MaxFloat64,
		Max: -math.MaxFloat64,
	}
}

// Merge une las estadísticas de otro lote.
func (s *ColStats) Merge(other ColStats) {
	if other.Count == 0 {
		s.Nulls += other.Nulls
		return
	}
	s.Count += other.Count
	s.Nulls += other.Nulls
	s.Sum += other.Sum
	s.SumSq += other.SumSq
	if other.Min < s.Min {
		s.Min = other.Min
	}
	if other.Max > s.Max {
		s.Max = other.Max
	}
}

// TargetColumns agrupa las columnas numéricas que analizamos.
type TargetColumns struct {
	Age          ColStats
	PSA          ColStats
	Cost         ColStats
	SurvivalDays ColStats
	Income       ColStats
}

func NewTargetColumns() TargetColumns {
	return TargetColumns{
		Age:          NewColStats(),
		PSA:          NewColStats(),
		Cost:         NewColStats(),
		SurvivalDays: NewColStats(),
		Income:       NewColStats(),
	}
}

func (tc *TargetColumns) Merge(other TargetColumns) {
	tc.Age.Merge(other.Age)
	tc.PSA.Merge(other.PSA)
	tc.Cost.Merge(other.Cost)
	tc.SurvivalDays.Merge(other.SurvivalDays)
	tc.Income.Merge(other.Income)
}

func main() {
	datasetPath := flag.String("dataset", "data/patients.csv", "Ruta al archivo CSV de pacientes")
	numWorkers := flag.Int("workers", runtime.NumCPU(), "Número de goroutines trabajadoras (fan-out)")
	batchSize := flag.Int("batch", 5000, "Tamaño del lote para reducir la contención del canal")
	flag.Parse()

	startTime := time.Now()

	file, err := os.Open(*datasetPath)
	if err != nil {
		log.Fatalf("[analyzer] Error al abrir el archivo: %v", err)
	}
	defer file.Close()

	// Usamos un buffer de bufio de 1MB para acelerar la lectura física de I/O
	bufReader := bufio.NewReaderSize(file, 1*1024*1024)
	csvReader := csv.NewReader(bufReader)

	// Leer cabecera
	header, err := csvReader.Read()
	if err != nil {
		log.Fatalf("[analyzer] Error al leer la cabecera: %v", err)
	}

	// Mapear nombres de columnas a índices
	colIdx := map[string]int{
		"age":             -1,
		"psa":             -1,
		"healthcare_cost": -1,
		"survival_days":   -1,
		"income":          -1,
	}
	for i, colName := range header {
		cleanName := strings.ToLower(strings.TrimSpace(colName))
		if _, ok := colIdx[cleanName]; ok {
			colIdx[cleanName] = i
		}
	}

	// Validar que las columnas existan
	for name, idx := range colIdx {
		if idx == -1 {
			log.Fatalf("[analyzer] Columna requerida no encontrada: %s", name)
		}
	}

	fmt.Printf("[analyzer] Procesando dataset con %d workers y lotes de %d filas...\n", *numWorkers, *batchSize)

	// Canales con búfer para evitar el bloqueo del productor
	jobs := make(chan [][]string, *numWorkers*2)
	results := make(chan TargetColumns, *numWorkers)

	var wg sync.WaitGroup

	// Lanzar los Workers (Fan-Out)
	for i := 0; i < *numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Cada worker mantiene estadísticas locales aisladas en memoria caché L1/L2 de su CPU.
			// No hay locks ni variables compartidas (evitamos contención/falsas comparticiones).
			localStats := NewTargetColumns()

			for batch := range jobs {
				for _, row := range batch {
					// Procesar columna por columna
					parseVal(row, colIdx["age"], &localStats.Age)
					parseVal(row, colIdx["psa"], &localStats.PSA)
					parseVal(row, colIdx["healthcare_cost"], &localStats.Cost)
					parseVal(row, colIdx["survival_days"], &localStats.SurvivalDays)
					parseVal(row, colIdx["income"], &localStats.Income)
				}
			}

			// Al finalizar el canal, enviamos los agregados locales (Map-Reduce)
			results <- localStats
		}(i + 1)
	}

	// Productor: Lee del CSV y agrupa en lotes
	go func() {
		defer close(jobs)
		currentBatch := make([][]string, 0, *batchSize)

		for {
			row, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				// Ignorar filas corruptas menores para robustez del pipeline
				continue
			}

			currentBatch = append(currentBatch, row)
			if len(currentBatch) == *batchSize {
				jobs <- currentBatch
				currentBatch = make([][]string, 0, *batchSize)
			}
		}

		// Enviar lote remanente
		if len(currentBatch) > 0 {
			jobs <- currentBatch
		}
	}()

	// Cerrar el canal de resultados cuando terminen todos los workers
	go func() {
		wg.Wait()
		close(results)
	}()

	// Consumidor (Fan-In): Combina todos los agregados parciales
	globalStats := NewTargetColumns()
	for workerStats := range results {
		globalStats.Merge(workerStats)
	}

	elapsed := time.Since(startTime)

	// Imprimir resultados estadísticos agregados
	printReport("Edad", globalStats.Age)
	printReport("Nivel PSA", globalStats.PSA)
	printReport("Costos Médicos", globalStats.Cost)
	printReport("Días de supervivencia", globalStats.SurvivalDays)
	printReport("Ingresos Económicos (Income)", globalStats.Income)

	fmt.Printf("\n[analyzer] Procesamiento completado con éxito en: %v\n", elapsed)
}

// parseVal intenta parsear el valor a float64. Si falla o está vacío, cuenta como Nulo.
func parseVal(row []string, idx int, stats *ColStats) {
	if idx >= len(row) {
		stats.Nulls++
		return
	}
	valStr := strings.TrimSpace(row[idx])
	if valStr == "" || strings.EqualFold(valStr, "null") {
		stats.Nulls++
		return
	}

	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		stats.Nulls++
		return
	}

	stats.Count++
	stats.Sum += val
	stats.SumSq += val * val
	if val < stats.Min {
		stats.Min = val
	}
	if val > stats.Max {
		stats.Max = val
	}
}

// printReport calcula la media, varianza, desviación estándar y porcentaje de nulos final.
func printReport(name string, stats ColStats) {
	totalRows := int64(stats.Count) + stats.Nulls
	nullPercent := 0.0
	if totalRows > 0 {
		nullPercent = (float64(stats.Nulls) / float64(totalRows)) * 100.0
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Métrica para Columna: %s\n", name)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("  Registros Válidos : %d\n", int64(stats.Count))
	fmt.Printf("  Registros Nulos   : %d (%.4f%%)\n", stats.Nulls, nullPercent)

	if stats.Count > 0 {
		mean := stats.Sum / stats.Count
		// Varianza Muestral: s^2 = (SumSq - Sum^2/N) / (N - 1)
		variance := 0.0
		if stats.Count > 1 {
			variance = (stats.SumSq - (stats.Sum*stats.Sum)/stats.Count) / (stats.Count - 1)
		}
		stdDev := math.Sqrt(variance)

		fmt.Printf("  Media             : %.4f\n", mean)
		fmt.Printf("  Varianza (Muest.) : %.4f\n", variance)
		fmt.Printf("  Desv. Est. (Muest.): %.4f\n", stdDev)
		fmt.Printf("  Mínimo            : %.4f\n", stats.Min)
		fmt.Printf("  Máximo            : %.4f\n", stats.Max)
	} else {
		fmt.Println("  No hay datos numéricos válidos en esta columna para computar estadísticas.")
	}
}
