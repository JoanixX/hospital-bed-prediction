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

// Run procesa una partición de pacientes acumulando los resultados en
// un slice local (sin contención con otros workers) y enviando el lote
// completo al canal `batches` cuando termina. Esto evita que el canal
// de resultados se convierta en un serializador con N productores y 1
// consumidor.
//
// Parámetros:
//   - id:        identificador del worker (1..N).
//   - partition: subconjunto del dataset asignado a este worker.
//   - batches:   canal de salida con el slice completo de resultados.
//   - stats:     canal de salida con las métricas operativas.
//   - wg:        WaitGroup que coordina el cierre del pool.
func Run(
	id int,
	partition []types.Patient,
	normalizedPSAs []float64,
	batches chan<- []types.PatientResult,
	stats chan<- types.WorkerStats,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	start := time.Now()
	batch := make([]types.PatientResult, len(partition))

	for i, p := range partition {
		var normPSA float64
		if i < len(normalizedPSAs) {
			normPSA = normalizedPSAs[i]
		}
		batch[i] = types.PatientResult{
			PatientID:        p.ID,
			MortalityRisk:    models.PredictMortality(p, normPSA),
			SurvivalEstimate: models.PredictSurvival(p, normPSA),
			TreatmentCost:    models.PredictTreatmentCost(p, normPSA),
			WorkerID:         id,
		}
	}

	batches <- batch
	stats <- types.WorkerStats{
		WorkerID:        id,
		PatientsHandled: len(partition),
		ProcessingTime:  time.Since(start),
	}
}
