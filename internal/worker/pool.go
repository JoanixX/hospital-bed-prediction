package worker

import (
	"fmt"
	"sync"

	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// Pool coordina la inferencia paralela de N workers (map-reduce):
// particiona los pacientes, despacha cada partición a una goroutine y
// agrega los resultados.
type Pool struct {
	NumWorkers int
	Models     *ml.TrainedModels
	Verbose    bool
}

// Process ejecuta la inferencia paralela sobre el slice de pacientes.
func (p Pool) Process(patients []types.Patient) ([]types.PatientResult, []types.WorkerStats) {
	if p.NumWorkers <= 0 {
		p.NumWorkers = 1
	}

	batches := make(chan []types.PatientResult, p.NumWorkers)
	stats := make(chan types.WorkerStats, p.NumWorkers)
	var wg sync.WaitGroup

	partitionSize := len(patients) / p.NumWorkers
	if partitionSize == 0 {
		partitionSize = 1
	}

	if p.Verbose {
		fmt.Println()
		fmt.Println("[pool] iniciando inferencia paralela")
		fmt.Printf("[pool]   pacientes totales : %d\n", len(patients))
		fmt.Printf("[pool]   workers (goroutines): %d\n", p.NumWorkers)
		fmt.Printf("[pool]   pacientes por worker: ~%d\n", partitionSize)
	}

	for i := 0; i < p.NumWorkers; i++ {
		start := i * partitionSize
		end := start + partitionSize
		if i == p.NumWorkers-1 {
			end = len(patients)
		}
		if start > len(patients) {
			start = len(patients)
		}
		if end > len(patients) {
			end = len(patients)
		}
		wg.Add(1)
		go Run(i+1, patients[start:end], p.Models, batches, stats, &wg)
	}

	go func() {
		wg.Wait()
		close(batches)
		close(stats)
	}()

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
