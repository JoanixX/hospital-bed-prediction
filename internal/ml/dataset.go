package ml

import "github.com/JoanixX/hospital-bed-prediction/internal/types"

// raceCategories define el orden fijo del one-hot encoding de la raza.
// Mantener el orden estable es crítico: los pesos entrenados dependen de
// la posición de cada columna.
var raceCategories = []string{"white", "black", "hispanic", "asian", "other"}

// numContinuous features continuas + len(raceCategories) one-hot.
const numContinuous = 6

// NumFeatures es la dimensión D del vector de features.
func NumFeatures() int { return numContinuous + len(raceCategories) }

// FeatureNames devuelve los nombres de las D features, en orden.
func FeatureNames() []string {
	names := []string{"age", "psa", "income", "coverage", "num_encounters", "num_diagnoses"}
	for _, r := range raceCategories {
		names = append(names, "race_"+r)
	}
	return names
}

// raceIndex devuelve el índice one-hot de una raza ("other" si no matchea).
func raceIndex(race string) int {
	for i, r := range raceCategories {
		if r == race {
			return i
		}
	}
	return len(raceCategories) - 1 // other
}

// FeatureVector construye el vector de features crudas (sin estandarizar)
// de un paciente. El costo (HealthcareCost) NO se incluye como feature
// porque es el objetivo del modelo de costo (evita fuga de información).
func FeatureVector(p types.Patient) []float64 {
	x := make([]float64, NumFeatures())
	x[0] = float64(p.Age)
	x[1] = p.PSALevel
	x[2] = p.Income
	x[3] = p.Coverage
	x[4] = float64(p.NumEncounters)
	x[5] = float64(p.NumDiagnoses)
	x[numContinuous+raceIndex(p.Race)] = 1.0
	return x
}

// Dataset agrupa la matriz de features y los tres objetivos alineados
// por índice de fila.
type Dataset struct {
	X          [][]float64 // N×D features crudas
	YMortality []float64   // 0/1
	YSurvival  []float64   // días
	YCost      []float64   // USD
}

// Len devuelve el número de muestras.
func (d Dataset) Len() int { return len(d.X) }

// BuildDataset transforma []Patient en matrices numéricas listas para
// estandarizar y entrenar.
func BuildDataset(patients []types.Patient) Dataset {
	n := len(patients)
	d := Dataset{
		X:          make([][]float64, n),
		YMortality: make([]float64, n),
		YSurvival:  make([]float64, n),
		YCost:      make([]float64, n),
	}
	for i, p := range patients {
		d.X[i] = FeatureVector(p)
		if p.HasDied {
			d.YMortality[i] = 1.0
		}
		d.YSurvival[i] = float64(p.SurvivalDays)
		d.YCost[i] = p.HealthcareCost
	}
	return d
}
