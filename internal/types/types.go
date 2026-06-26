// Package types contiene las estructuras de datos compartidas por todo
// el pipeline concurrente de predicción de cáncer de próstata.
package types

import "time"

// Patient representa un registro clínico simplificado de un paciente
// oncológico extraído del dataset UCI/Synthea.
type Patient struct {
	ID             string
	Age            int
	Race           string
	Ethnicity      string
	MaritalStatus  string
	Income         float64
	HealthcareCost float64
	Coverage       float64
	PSALevel       float64 // Antígeno Prostático Específico (ng/mL)
	NumEncounters  int
	NumDiagnoses   int
	HasDied        bool
	SurvivalDays   int // días desde diagnóstico hasta muerte o corte
}

// PatientResult almacena los resultados de los tres modelos predictivos
// ejecutados sobre un paciente por un worker del clúster.
type PatientResult struct {
	PatientID        string
	MortalityRisk    float64 // Modelo 1: probabilidad de muerte [0,1]
	SurvivalEstimate float64 // Modelo 2: días estimados de supervivencia
	TreatmentCost    float64 // Modelo 3: costo estimado en USD
	WorkerID         int     // qué goroutine procesó este paciente
}

// WorkerStats guarda métricas operativas por goroutine, utilizadas
// para auditar el balanceo de carga del worker pool.
type WorkerStats struct {
	WorkerID        int
	PatientsHandled int
	ProcessingTime  time.Duration
}

// PipelineConfig agrupa los parámetros configurables del pipeline.
type PipelineConfig struct {
	NumWorkers     int
	DatasetPath    string
	ChannelBuffer  int
	EnableProfile  bool
	ProfileAddress string
}

// ProcessArgs representa los argumentos de entrada para el RPC de procesamiento
type ProcessArgs struct {
	Patients []Patient
}

// ProcessReply representa la respuesta del RPC de procesamiento
type ProcessReply struct {
	Results []PatientResult
	Stats   WorkerStats
}
