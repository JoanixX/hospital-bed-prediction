// Package models implementa las funciones de puntuación heurística
// que reemplazan provisionalmente a los algoritmos formales de ML
// durante la PC3. En PC4 estas funciones serán sustituidas por
// implementaciones reales basadas en gonum/golearn/xgboost, sin
// alterar la firma func(types.Patient) float64 que consume el worker.
package models

import "github.com/JoanixX/hospital-bed-prediction/internal/types"

// PredictMortality implementa una aproximación heurística del modelo
// de clasificación binaria de mortalidad (Modelo 1 del informe). El
// algoritmo formal de referencia es XGBoost (Chen & Guestrin, 2016).
//
// La función pondera factores clínicos (edad, PSA, número de
// encuentros oncológicos) y socioeconómicos (ingreso, raza) con pesos
// derivados de la literatura de cáncer de próstata, devolviendo una
// probabilidad acotada al intervalo [0, 1].
func PredictMortality(p types.Patient) float64 {
	score := 0.0

	// Factor edad: el riesgo crece de forma marcada a partir de los 70.
	switch {
	case p.Age >= 80:
		score += 0.40
	case p.Age >= 70:
		score += 0.30
	case p.Age >= 60:
		score += 0.15
	}

	// PSA: valores > 10 ng/mL se asocian con alto riesgo oncológico.
	switch {
	case p.PSALevel > 20:
		score += 0.40
	case p.PSALevel > 10:
		score += 0.30
	case p.PSALevel > 4:
		score += 0.10
	}

	// Bajo ingreso reduce el acceso a tratamiento y eleva la mortalidad.
	if p.Income < 30000 {
		score += 0.15
	}

	// Número alto de encuentros oncológicos indica enfermedad avanzada.
	if p.NumEncounters > 15 {
		score += 0.10
	}

	// Disparidad racial documentada en la literatura oncológica.
	if p.Race == "black" {
		score += 0.10
	}

	// Comorbilidad: muchos diagnósticos comórbidos elevan el riesgo.
	if p.NumDiagnoses > 5 {
		score += 0.05
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}
