// Package handlers implementa los handlers HTTP de la API REST.
package handlers

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/JoanixX/hospital-bed-prediction/internal/api/dto"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
	"github.com/JoanixX/hospital-bed-prediction/internal/worker"
)

// Métricas agregadas en memoria para /stats.
var (
	totalProcessed  atomic.Int64
	sumMortality    atomic.Value // float64
	sumSurvival     atomic.Value // float64
	sumCost         atomic.Value // float64
)

func init() {
	sumMortality.Store(float64(0))
	sumSurvival.Store(float64(0))
	sumCost.Store(float64(0))
}

// Predict maneja POST /predict.
//
// Recibe un JSON con los datos del paciente, los pasa por el worker
// pool existente (1 worker para una sola predicción) y devuelve los
// tres resultados del modelo.
//
//	POST /predict
//	Authorization: Bearer <token>
//	Content-Type: application/json
//
//	{
//	  "id": "PAT-0000001",
//	  "age": 65,
//	  "race": "white",
//	  "income": 55000,
//	  "psa_level": 8.5,
//	  "coverage": 0.75,
//	  "num_encounters": 6,
//	  "num_diagnoses": 2
//	}
func Predict(c *gin.Context) {
	var req dto.PredictRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid request body",
			Details: err.Error(),
		})
		return
	}

	// Convertir DTO → types.Patient (el pipeline no conoce los DTOs)
	patient := types.Patient{
		ID:             req.ID,
		Age:            req.Age,
		Race:           req.Race,
		Income:         req.Income,
		HealthcareCost: req.HealthcareCost,
		Coverage:       req.Coverage,
		PSALevel:       req.PSALevel,
		NumEncounters:  req.NumEncounters,
		NumDiagnoses:   req.NumDiagnoses,
	}
	if patient.ID == "" {
		patient.ID = "API-REQUEST"
	}

	// Reutilizamos el pool existente con 1 worker para una sola predicción.
	// Para batch, se puede recibir []PredictRequest y ajustar NumWorkers.
	pool := worker.Pool{NumWorkers: 1, Verbose: false}
	results, _ := pool.Process([]types.Patient{patient})

	if len(results) == 0 {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "pipeline returned no results",
		})
		return
	}

	r := results[0]

	// Actualizar métricas globales (operaciones atómicas para thread-safety)
	totalProcessed.Add(1)
	addFloat64(&sumMortality, r.MortalityRisk)
	addFloat64(&sumSurvival, r.SurvivalEstimate)
	addFloat64(&sumCost, r.TreatmentCost)

	c.JSON(http.StatusOK, dto.PredictResponse{
		PatientID:        r.PatientID,
		MortalityRisk:    r.MortalityRisk,
		SurvivalEstimate: r.SurvivalEstimate,
		TreatmentCost:    r.TreatmentCost,
		Cached:           false, // el middleware de caché lo sobreescribe a true si aplica
	})
}

// addFloat64 suma de forma atómica sobre un atomic.Value que guarda float64.
func addFloat64(v *atomic.Value, delta float64) {
	for {
		old := v.Load().(float64)
		if v.CompareAndSwap(old, old+delta) {
			return
		}
	}
}

// GetTotals expone las métricas acumuladas para el handler de /stats.
func GetTotals() (total int64, mort, surv, cost float64) {
	total = totalProcessed.Load()
	mort = sumMortality.Load().(float64)
	surv = sumSurvival.Load().(float64)
	cost = sumCost.Load().(float64)
	return
}
