package models

import (
	"math"
	"testing"

	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// TestConcurrentlyNormalizePSA verifica que la normalización Min-Max concurrente
// escale adecuadamente los niveles de PSA en base a los límites poblacionales.
func TestConcurrentlyNormalizePSA(t *testing.T) {
	patients := []types.Patient{
		{ID: "P1", PSALevel: MinPSABound},                       // Debe normalizar a 0.0
		{ID: "P2", PSALevel: MaxPSABound},                       // Debe normalizar a 1.0
		{ID: "P3", PSALevel: (MinPSABound + MaxPSABound) / 2.0}, // Debe normalizar a 0.5
		{ID: "P4", PSALevel: 0.1},                               // Fuera de rango inferior, acotado a 0.0
		{ID: "P5", PSALevel: 100.0},                             // Fuera de rango superior, acotado a 1.0
	}

	normalized := ConcurrentlyNormalizePSA(patients)

	if len(normalized) != len(patients) {
		t.Fatalf("Se esperaban %d resultados de normalización, se obtuvieron %d", len(patients), len(normalized))
	}

	// Verificar límites exactos
	if math.Abs(normalized[0]-0.0) > 1e-6 {
		t.Errorf("Paciente 1 PSA %f debió normalizar a 0.0, obtenido %f", patients[0].PSALevel, normalized[0])
	}
	if math.Abs(normalized[1]-1.0) > 1e-6 {
		t.Errorf("Paciente 2 PSA %f debió normalizar a 1.0, obtenido %f", patients[1].PSALevel, normalized[1])
	}
	if math.Abs(normalized[2]-0.5) > 1e-6 {
		t.Errorf("Paciente 3 PSA %f debió normalizar a 0.5, obtenido %f", patients[2].PSALevel, normalized[2])
	}
	if math.Abs(normalized[3]-0.0) > 1e-6 {
		t.Errorf("Paciente 4 (out of bounds low) debió normalizar a 0.0, obtenido %f", normalized[3])
	}
	if math.Abs(normalized[4]-1.0) > 1e-6 {
		t.Errorf("Paciente 5 (out of bounds high) debió normalizar a 1.0, obtenido %f", normalized[4])
	}
}

// TestPredictiveModels verifica que los modelos predictivos vectorizados
// devuelvan valores numéricos lógicos y respeten restricciones de dominio.
func TestPredictiveModels(t *testing.T) {
	p := types.Patient{
		ID:            "P-TEST",
		Age:           65,
		Race:          "white",
		Income:        40000.0,
		Coverage:      0.80,
		NumEncounters: 10,
		NumDiagnoses:  3,
	}
	normalizedPSA := 0.45 // PSA normalizado simulado

	// 1. Validar predicción de mortalidad (debe estar en el rango de probabilidad [0.0, 1.0])
	mortality := PredictMortality(p, normalizedPSA)
	if mortality < 0.0 || mortality > 1.0 {
		t.Errorf("Riesgo de mortalidad fuera de rango [0, 1]: %f", mortality)
	}

	// 2. Validar supervivencia (debe ser mayor o igual a la cota mínima de 90 días)
	survival := PredictSurvival(p, normalizedPSA)
	if survival < 90.0 {
		t.Errorf("Días de supervivencia estimado inferior a la cota mínima (90 días): %f", survival)
	}

	// 3. Validar costo (debe ser un valor positivo mayor a la base de hospitalización)
	cost := PredictTreatmentCost(p, normalizedPSA)
	if cost <= 0.0 {
		t.Errorf("Costo de tratamiento estimado no puede ser cero o negativo: %f", cost)
	}
}
