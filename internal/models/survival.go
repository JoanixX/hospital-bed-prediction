package models

import "github.com/JoanixX/hospital-bed-prediction/internal/types"

// PredictSurvival implementa una aproximación heurística del modelo
// de análisis de supervivencia (Modelo 2 del informe). Los algoritmos
// formales de referencia son Kaplan-Meier (Kaplan & Meier, 1958) y
// Cox Proportional Hazards (Cox, 1972).
//
// Toma como línea base una supervivencia teórica de 10 años (3650
// días) y aplica penalizaciones/bonificaciones en función del perfil
// clínico y socioeconómico del paciente.
func PredictSurvival(p types.Patient) float64 {
	baseline := 3650.0 // 10 años en días

	// Penalizaciones clínicas.
	if p.PSALevel > 20 {
		baseline -= 1095 // -3 años
	} else if p.PSALevel > 10 {
		baseline -= 730 // -2 años
	}

	if p.Age > 80 {
		baseline -= 730
	} else if p.Age > 75 {
		baseline -= 365
	} else if p.Age > 70 {
		baseline -= 180
	}

	// Bonificaciones socioeconómicas y de seguimiento.
	if p.Income > 60000 {
		baseline += 180 // mejor acceso a tratamiento oportuno
	}
	if p.Coverage > 0.8 {
		baseline += 120
	}
	if p.NumEncounters > 10 {
		baseline += 90 // seguimiento frecuente mejora pronóstico
	}

	if baseline < 90 {
		baseline = 90 // cota mínima de seguridad
	}
	return baseline
}
