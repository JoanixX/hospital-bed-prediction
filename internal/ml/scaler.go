package ml

import (
	"math"
	"runtime"
	"sync"
)

// Scaler estandariza features a media 0 y desviación 1 (z-score).
// Imprescindible para que el gradiente converja cuando las features
// tienen escalas dispares (edad ~70 vs income ~50000). Las columnas
// one-hot también se estandarizan; es inofensivo y mantiene el código
// uniforme.
type Scaler struct {
	Mean []float64
	Std  []float64
}

// FitScaler calcula media y desviación por columna de forma concurrente
// (map-reduce sobre rangos de filas), patrón que reusa el resto del motor.
func FitScaler(X [][]float64) *Scaler {
	n := len(X)
	if n == 0 {
		return &Scaler{}
	}
	d := len(X[0])
	nw := runtime.NumCPU()
	if nw > n {
		nw = n
	}

	type acc struct {
		sum, sumSq []float64
	}
	partials := make([]acc, nw)
	var wg sync.WaitGroup
	chunk := (n + nw - 1) / nw
	for wkr := 0; wkr < nw; wkr++ {
		start := wkr * chunk
		if start >= n {
			break
		}
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(w, s, e int) {
			defer wg.Done()
			su := make([]float64, d)
			sq := make([]float64, d)
			for i := s; i < e; i++ {
				for j, v := range X[i] {
					su[j] += v
					sq[j] += v * v
				}
			}
			partials[w] = acc{su, sq}
		}(wkr, start, end)
	}
	wg.Wait()

	mean := make([]float64, d)
	std := make([]float64, d)
	sumSq := make([]float64, d)
	for _, p := range partials {
		for j := 0; j < d; j++ {
			mean[j] += p.sum[j]
			sumSq[j] += p.sumSq[j]
		}
	}
	fn := float64(n)
	for j := 0; j < d; j++ {
		mean[j] /= fn
		variance := sumSq[j]/fn - mean[j]*mean[j]
		if variance < 1e-12 {
			variance = 1e-12 // columna constante: evita división por 0
		}
		std[j] = math.Sqrt(variance)
	}
	return &Scaler{Mean: mean, Std: std}
}

// Transform estandariza una fila (no muta la entrada).
func (s *Scaler) Transform(x []float64) []float64 {
	out := make([]float64, len(x))
	for j, v := range x {
		out[j] = (v - s.Mean[j]) / s.Std[j]
	}
	return out
}

// TransformMatrix estandariza toda una matriz.
func (s *Scaler) TransformMatrix(X [][]float64) [][]float64 {
	out := make([][]float64, len(X))
	for i := range X {
		out[i] = s.Transform(X[i])
	}
	return out
}

// TargetScaler estandariza un objetivo continuo (supervivencia, costo)
// para estabilizar el entrenamiento; las predicciones se de-estandarizan
// con Inverse para volver a la escala original (días, USD).
type TargetScaler struct {
	Mean float64
	Std  float64
}

// FitTargetScaler ajusta sobre el vector objetivo.
func FitTargetScaler(y []float64) *TargetScaler {
	n := len(y)
	if n == 0 {
		return &TargetScaler{Mean: 0, Std: 1}
	}
	var sum, sumSq float64
	for _, v := range y {
		sum += v
		sumSq += v * v
	}
	mean := sum / float64(n)
	variance := sumSq/float64(n) - mean*mean
	if variance < 1e-12 {
		variance = 1e-12
	}
	return &TargetScaler{Mean: mean, Std: math.Sqrt(variance)}
}

func (t *TargetScaler) Forward(v float64) float64 { return (v - t.Mean) / t.Std }
func (t *TargetScaler) Inverse(v float64) float64 { return v*t.Std + t.Mean }

func (t *TargetScaler) ForwardAll(y []float64) []float64 {
	out := make([]float64, len(y))
	for i, v := range y {
		out[i] = t.Forward(v)
	}
	return out
}
