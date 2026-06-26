package models

import (
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// PredictTreatmentCost implementa un modelo real de Regresión Lineal Múltiple
// para estimar los costos acumulados de tratamiento médico del paciente.
func PredictTreatmentCost(p types.Patient, normalizedPSA float64) float64 {
	// Definición de pesos del modelo (Weights en USD)
	wAge := 6000.0         // +$6000 USD para pacientes de avanzada edad (escala 0-1)
	wPSA := 28000.0        // +$28000 USD para niveles máximos de PSA (severidad)
	wIncome := 10000.0     // +$10000 USD por acceso a tratamientos costosos (escala 0-1)
	wCoverage := 6000.0    // +$6000 USD por acceso médico ampliado (escala 0-1)
	wEncounters := 14000.0 // +$14000 USD por alta frecuencia de encuentros clínicos (escala 0-1)
	wDiagnoses := 11000.0  // +$11000 USD por comorbilidades (escala 0-1)
	bias := 6000.0         // Costo clínico base de hospitalización

	// Escalado de variables de entrada a rangos uniformes
	xAge := float64(p.Age) / 100.0
	xIncome := p.Income / 100000.0
	xCoverage := p.Coverage
	xEncounters := float64(p.NumEncounters) / 20.0
	xDiagnoses := float64(p.NumDiagnoses) / 10.0

	// Cálculo del modelo lineal: y = w^T * x + b
	cost := bias +
		(wAge * xAge) +
		(wPSA * normalizedPSA) +
		(wIncome * xIncome) +
		(wCoverage * xCoverage) +
		(wEncounters * xEncounters) +
		(wDiagnoses * xDiagnoses)

	// Modelo de disparidad socioeconómica: si el paciente no tiene ingresos suficientes,
	// se reduce la facturación por falta de acceso a fármacos e inmunoterapia de alto costo.
	if p.Income < 30000.0 {
		cost *= 0.60
	} else if p.Income < 50000.0 {
		cost *= 0.85
	}

	return cost
}
