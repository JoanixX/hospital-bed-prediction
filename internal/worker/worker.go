// Package worker implementa el patrón Worker Pool del clúster ML.
// Cada worker es una goroutine independiente que recibe una partición
// del dataset y aplica los tres modelos predictivos en secuencia.
package worker

import (
	"sync"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/models"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// Run procesa una partición de pacientes y envía los resultados al
// canal compartido. Es función entry-point de la goroutine y debe
// ejecutarse con `go Run(...)`.
//
// Parámetros:
//   - id:        identificador del worker (1..N).
//   - partition: subconjunto del dataset asignado a este worker.
//   - results:   canal de salida con los PatientResult agregados.
//   - stats:     canal de salida con las métricas operativas.
//   - wg:        WaitGroup que coordina el cierre del pool.
func Run(
	id int,
	partition []types.Patient,
	results chan<- types.PatientResult,
	stats chan<- types.WorkerStats,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	start := time.Now()
	count := 0

	for _, p := range partition {
		// Ejecutar los tres modelos heurísticos sobre el paciente.
		results <- types.PatientResult{
			PatientID:        p.ID,
			MortalityRisk:    models.PredictMortality(p),
			SurvivalEstimate: models.PredictSurvival(p),
			TreatmentCost:    models.PredictTreatmentCost(p),
			WorkerID:         id,
		}
		count++
	}

	stats <- types.WorkerStats{
		WorkerID:        id,
		PatientsHandled: count,
		ProcessingTime:  time.Since(start),
	}
}
