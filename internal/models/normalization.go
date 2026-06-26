package models

import (
	"runtime"
	"sync"

	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

const (
	// MinPSABound y MaxPSABound definen los límites globales del PSA
	// obtenidos en el EDA clínico (0.32 y 50.17 ng/mL).
	MinPSABound = 0.3200
	MaxPSABound = 50.1700
)

// ConcurrentlyNormalizePSA ejecuta una normalización Min-Max sobre la columna PSA
// de forma concurrente, dividiendo el slice de pacientes en chunks balanceados
// según la cantidad de CPUs disponibles en el sistema.
func ConcurrentlyNormalizePSA(patients []types.Patient) []float64 {
	n := len(patients)
	normalized := make([]float64, n)
	if n == 0 {
		return normalized
	}

	numWorkers := runtime.NumCPU()
	if n < numWorkers {
		numWorkers = n
	}

	chunkSize := n / numWorkers
	if chunkSize == 0 {
		chunkSize = 1
	}

	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		start := i * chunkSize
		if start >= n {
			break
		}
		end := start + chunkSize
		if i == numWorkers-1 || end > n {
			end = n
		}

		wg.Add(1)
		go func(startIndex, endIndex int) {
			defer wg.Done()
			for idx := startIndex; idx < endIndex; idx++ {
				psa := patients[idx].PSALevel
				// Acotar el valor por seguridad para evitar división fuera de rango
				if psa < MinPSABound {
					psa = MinPSABound
				}
				if psa > MaxPSABound {
					psa = MaxPSABound
				}
				
				// Normalización Min-Max: escalar a [0.0, 1.0]
				normalizedVal := (psa - MinPSABound) / (MaxPSABound - MinPSABound)
				normalized[idx] = normalizedVal
			}
		}(start, end)
	}

	wg.Wait()
	return normalized
}
