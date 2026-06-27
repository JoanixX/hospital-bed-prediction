package worker

import (
	"fmt"
	"sync"

	"github.com/JoanixX/hospital-bed-prediction/internal/models"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// Pool coordina la ejecución paralela de N workers sobre el dataset
// completo. Aplica el patrón map-reduce: particiona los datos (map),
// despacha cada partición a una goroutine (parallel reduce) y agrega
// los resultados al cerrar los canales.
type Pool struct {
	NumWorkers int
	Verbose    bool
}

// Process ejecuta el pipeline paralelo sobre el slice de pacientes.
// Devuelve los resultados predictivos y las métricas por worker.
func (p Pool) Process(patients []types.Patient) ([]types.PatientResult, []types.WorkerStats) {
	if p.NumWorkers <= 0 {
		p.NumWorkers = 1
	}

	batches := make(chan []types.PatientResult, p.NumWorkers)
	stats := make(chan types.WorkerStats, p.NumWorkers)
	var wg sync.WaitGroup

	normalizedPSAs := models.ConcurrentlyNormalizePSA(patients)

	partitionSize := len(patients) / p.NumWorkers
	if partitionSize == 0 {
		partitionSize = 1
	}

	if p.Verbose {
		fmt.Println()
		fmt.Println("[pool] iniciando procesamiento paralelo")
		fmt.Printf("[pool]   pacientes totales : %d\n", len(patients))
		fmt.Printf("[pool]   workers (goroutines): %d\n", p.NumWorkers)
		fmt.Printf("[pool]   pacientes por worker: ~%d\n", partitionSize)
	}

	for i := 0; i < p.NumWorkers; i++ {
		start := i * partitionSize
		end := start + partitionSize
		if i == p.NumWorkers-1 {
			end = len(patients) // el último worker absorbe el remanente
		}
		wg.Add(1)
		go Run(i+1, patients[start:end], normalizedPSAs[start:end], batches, stats, &wg)
		if p.Verbose {
			fmt.Printf("[pool]   worker %d lanzado -> rango [%d, %d)\n", i+1, start, end)
		}
	}

	// Cerrar canales cuando todos los workers terminen.
	go func() {
		wg.Wait()
		close(batches)
		close(stats)
	}()

	// Recolectar lotes (uno por worker) y métricas.
	allResults := make([]types.PatientResult, 0, len(patients))
	var allStats []types.WorkerStats
	for batch := range batches {
		allResults = append(allResults, batch...)
	}
	for s := range stats {
		allStats = append(allStats, s)
	}
	return allResults, allStats
}

// ProcessSequential ejecuta el mismo pipeline pero sin paralelismo,
// como línea base para la comparación de rendimiento de la sección
// 4.5 del informe.
func (p Pool) ProcessSequential(patients []types.Patient) []types.PatientResult {
	results := make([]types.PatientResult, 0, len(patients))
	pool := Pool{NumWorkers: 1, Verbose: false}
	r, _ := pool.Process(patients)
	results = append(results, r...)
	return results
}
