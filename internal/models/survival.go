package models

import (
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// PredictSurvival implementa un modelo real de Regresión Lineal Múltiple
// para estimar los días de supervivencia del paciente a partir de su diagnóstico.
func PredictSurvival(p types.Patient, normalizedPSA float64) float64 {
	// Definición de pesos del modelo (Weights en días)
	wAge := -15.0        // -15 días por cada año de edad adicional
	wPSA := -1600.0      // -1600 días para el máximo nivel de PSA
	wIncome := 450.0     // +450 días para el máximo nivel de ingresos (escala 0-1)
	wCoverage := 350.0   // +350 días para 100% de cobertura médica
	wEncounters := 200.0 // +200 días para visitas frecuentes (control clínico preventivo)
	wDiagnoses := -280.0 // -280 días por cada diagnóstico comórbido (escala 0-1)
	bias := 4100.0       // Intercepto base (~11 años de supervivencia teórica)

	// Escalado de variables de entrada a rangos uniformes
	xAge := float64(p.Age)
	xIncome := p.Income / 100000.0
	xCoverage := p.Coverage
	xEncounters := float64(p.NumEncounters) / 20.0
	xDiagnoses := float64(p.NumDiagnoses) / 10.0

	// Cálculo del modelo lineal: y = w^T * x + b
	survivalDays := bias +
		(wAge * xAge) +
		(wPSA * normalizedPSA) +
		(wIncome * xIncome) +
		(wCoverage * xCoverage) +
		(wEncounters * xEncounters) +
		(wDiagnoses * xDiagnoses)

	// Cota inferior de seguridad (90 días mínimos)
	if survivalDays < 90.0 {
		return 90.0
	}

	return survivalDays
}
