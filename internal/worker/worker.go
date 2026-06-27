// Package worker implementa el worker pool de INFERENCIA. Tras el
// entrenamiento, cada worker (goroutine) aplica los modelos ya entrenados
// a una partición de pacientes sin contención: acumula sus resultados en
// un slice local y envía el lote completo al canal al terminar.
package worker

import (
	"sync"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// Run aplica los modelos entrenados a una partición de pacientes.
func Run(
	id int,
	partition []types.Patient,
	models *ml.TrainedModels,
	batches chan<- []types.PatientResult,
	stats chan<- types.WorkerStats,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	start := time.Now()
	batch := make([]types.PatientResult, len(partition))
	for i, p := range partition {
		batch[i] = models.PredictPatient(p, id)
	}

	batches <- batch
	stats <- types.WorkerStats{
		WorkerID:        id,
		PatientsHandled: len(partition),
		ProcessingTime:  time.Since(start),
	}
}
