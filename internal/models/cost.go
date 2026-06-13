package models

import "github.com/JoanixX/hospital-bed-prediction/internal/types"

// PredictTreatmentCost implementa una aproximación heurística del
// modelo de regresión de costo acumulado de tratamiento (Modelo 3 del
// informe). El algoritmo formal de referencia es Gradient Boosting
// Regressor (Friedman, 2001).
//
// La función parte de un costo base y suma incrementos asociados al
// número de encuentros, severidad clínica y edad. El factor de
// ingreso modela la inequidad observada: pacientes de bajos ingresos
// reciben menor tratamiento (menor costo facturado, no mejor salud).
func PredictTreatmentCost(p types.Patient) float64 {
	base := 15000.0

	// Costo por encuentro promedio (consulta + estudios).
	base += float64(p.NumEncounters) * 800

	// Marcadores de severidad que disparan tratamientos costosos.
	if p.PSALevel > 20 {
		base += 35000
	} else if p.PSALevel > 10 {
		base += 20000
	}

	if p.Age > 75 {
		base += 12000
	} else if p.Age > 70 {
		base += 8000
	}

	// Comorbilidades incrementan el costo.
	base += float64(p.NumDiagnoses) * 1500

	// Inequidad: pacientes con bajo ingreso reciben menos tratamiento.
	if p.Income < 30000 {
		base *= 0.6
	} else if p.Income < 50000 {
		base *= 0.85
	}

	return base
}
