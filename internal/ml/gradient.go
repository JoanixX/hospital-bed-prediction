package ml

import (
	"runtime"
	"sync"
)

// rawGradientRange acumula secuencialmente el gradiente (sin promediar)
// del modelo m sobre las filas [start,end) de X con objetivos y.
func rawGradientRange(m Model, X [][]float64, y, w []float64, start, end int) Grad {
	g := NewGrad(len(w))
	for i := start; i < end; i++ {
		g.Loss += m.AccumSample(X[i], y[i], w, g.Sum)
	}
	g.N = end - start
	return g
}

// RawGradientConcurrent calcula la SUMA del gradiente (no promediada) y
// la pérdida sobre todo (X,y) usando numWorkers goroutines en fan-out y
// agregando los parciales (fan-in / reduce). Es la primitiva paralela
// que se ejecuta dentro de cada nodo; el promedio y la L2 se aplican
// después con Grad.MeanGradient.
func RawGradientConcurrent(m Model, X [][]float64, y, w []float64, numWorkers int) Grad {
	n := len(X)
	total := NewGrad(len(w))
	if n == 0 {
		return total
	}
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	if numWorkers > n {
		numWorkers = n
	}

	partials := make([]Grad, numWorkers)
	var wg sync.WaitGroup
	chunk := (n + numWorkers - 1) / numWorkers
	for wkr := 0; wkr < numWorkers; wkr++ {
		start := wkr * chunk
		if start >= n {
			break
		}
		end := start + chunk
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(idx, s, e int) {
			defer wg.Done()
			partials[idx] = rawGradientRange(m, X, y, w, s, e)
		}(wkr, start, end)
	}
	wg.Wait()

	for _, p := range partials {
		if p.N > 0 {
			total.Add(p)
		}
	}
	return total
}
