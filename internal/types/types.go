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


// ===================================================================
// Tipos del protocolo RPC para el ENTRENAMIENTO DISTRIBUIDO
// (master coordina, workers calculan gradientes parciales por shard).
// ===================================================================

// ScalerParams transporta los parámetros del estandarizador global
// (media/desviación por característica) que el Master calcula una sola
// vez y reparte a todos los Workers para que featuricen su shard de
// forma idéntica. Sin esto, los gradientes de distintos nodos no serían
// promediables.
type ScalerParams struct {
	Mean []float64
	Std  []float64
}

// LoadShardArgs envía a un Worker su partición del dataset ya featurizada
// y estandarizada, junto con los tres objetivos. El Worker la conserva en
// memoria entre épocas para no retransmitir datos en cada iteración.
type LoadShardArgs struct {
	X          [][]float64
	YMortality []float64
	YSurvival  []float64 // estandarizado (escala del modelo)
	YCost      []float64 // estandarizado (escala del modelo)
}

// LoadShardReply confirma la recepción del shard.
type LoadShardReply struct {
	N int // número de ejemplos almacenados en el shard
}

// GradientArgs pide al Worker el gradiente parcial de un modelo para un
// vector de pesos dado. Kind ∈ {"mortality","survival","cost"}.
type GradientArgs struct {
	Kind    string
	Weights []float64
	L2      float64
}

// GradientReply devuelve la suma (no promediada) del gradiente parcial,
// la pérdida acumulada y el número de ejemplos del shard.
type GradientReply struct {
	GradSum []float64
	Loss    float64
	N       int
}

// ModelBundle transporta los pesos entrenados de los tres modelos más el
// estandarizador, para que el Worker pueda atender inferencia distribuida
// tras el entrenamiento.
type ModelBundle struct {
	Scaler       ScalerParams
	WMortality   []float64
	WSurvival    []float64
	WCost        []float64
	SurvMean     float64
	SurvStd      float64
	CostMean     float64
	CostStd      float64
}

// SetModelArgs difunde a un Worker el modelo entrenado completo para que
// pueda atender inferencia distribuida tras el entrenamiento.
type SetModelArgs struct {
	Bundle ModelBundle
}

type SetModelReply struct {
	OK bool
}
