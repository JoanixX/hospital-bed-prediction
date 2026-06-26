package models

import (
	"math"

	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// PredictMortality implementa un modelo real de Regresión Logística
// para predecir la probabilidad de mortalidad del paciente.
// Utiliza una combinación lineal vectorizada seguida de una función sigmoide.
func PredictMortality(p types.Patient, normalizedPSA float64) float64 {
	// Definición de pesos del modelo (Weights)
	wAge := 1.5        // Edad (a mayor edad, mayor riesgo)
	wPSA := 3.2        // PSA normalizado (fuerte indicador clínico)
	wIncome := -0.8    // Nivel socioeconómico (ingreso protege contra mortalidad)
	wCoverage := -0.5  // Cobertura de salud (a mayor seguro, menor mortalidad)
	wEncounters := 1.1 // Número de encuentros oncológicos (indica severidad)
	wDiagnoses := 0.7  // Comorbilidades (número de diagnósticos adicionales)
	bias := -2.4       // Sesgo / Intercepto

	// Normalización y escalado de variables de entrada a rangos uniformes [0, 1]
	xAge := float64(p.Age) / 100.0
	xIncome := p.Income / 100000.0
	xEncounters := float64(p.NumEncounters) / 20.0
	xDiagnoses := float64(p.NumDiagnoses) / 10.0
	xCoverage := p.Coverage

	// Cálculo del producto punto: z = w^T * x + b
	z := bias +
		(wAge * xAge) +
		(wPSA * normalizedPSA) +
		(wIncome * xIncome) +
		(wCoverage * xCoverage) +
		(wEncounters * xEncounters) +
		(wDiagnoses * xDiagnoses)

	// Ajuste demográfico por disparidad documentada
	if p.Race == "black" {
		z += 0.3
	}

	// Función de activación Logística/Sigmoide: P(y=1) = 1 / (1 + e^-z)
	prob := 1.0 / (1.0 + math.Exp(-z))

	// Control de bordes
	if prob < 0.0 {
		return 0.0
	}
	if prob > 1.0 {
		return 1.0
	}

	return prob
}
