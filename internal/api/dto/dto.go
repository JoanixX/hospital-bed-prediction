// Package dto define las estructuras de transferencia de datos (Data
// Transfer Objects) que la API REST serializa/deserializa como JSON.
// Son independientes de los tipos internos del pipeline para que
// el contrato HTTP pueda evolucionar sin tocar la lógica de negocio.
package dto

// ─────────────────────────────────────────────
//  /predict  —  POST
// ─────────────────────────────────────────────

// PredictRequest es el cuerpo JSON que el cliente envía a POST /predict.
// Todos los campos reflejan los atributos clínicos de types.Patient.
type PredictRequest struct {
	ID             string  `json:"id"`
	Age            int     `json:"age"            binding:"required,min=0,max=120"`
	Race           string  `json:"race"`
	Income         float64 `json:"income"         binding:"required,min=0"`
	HealthcareCost float64 `json:"healthcare_cost"`
	Coverage       float64 `json:"coverage"       binding:"min=0,max=1"`
	PSALevel       float64 `json:"psa_level"      binding:"required,min=0"`
	NumEncounters  int     `json:"num_encounters"`
	NumDiagnoses   int     `json:"num_diagnoses"`
}

// PredictResponse es la respuesta JSON de POST /predict.
type PredictResponse struct {
	PatientID        string  `json:"patient_id"`
	MortalityRisk    float64 `json:"mortality_risk"`    // [0, 1]
	SurvivalEstimate float64 `json:"survival_estimate"` // días
	TreatmentCost    float64 `json:"treatment_cost"`    // USD
	Cached           bool    `json:"cached"`            // true si vino de Redis
}

// ─────────────────────────────────────────────
//  /stats  —  GET
// ─────────────────────────────────────────────

// StatsResponse agrega métricas operativas del pipeline y Redis.
type StatsResponse struct {
	TotalProcessed int     `json:"total_processed"`
	CacheHits      int64   `json:"cache_hits"`
	CacheMisses    int64   `json:"cache_misses"`
	AvgMortality   float64 `json:"avg_mortality_risk"`
	AvgSurvival    float64 `json:"avg_survival_days"`
	AvgCost        float64 `json:"avg_treatment_cost"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
}

// ─────────────────────────────────────────────
//  Errores genéricos
// ─────────────────────────────────────────────

// ErrorResponse es el sobre JSON de cualquier respuesta de error.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}
