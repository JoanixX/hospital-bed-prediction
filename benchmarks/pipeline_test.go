// Benchmarks del entrenamiento paralelo (sección 4.5 del informe).
// Miden el speedup REAL del cálculo de gradiente — el núcleo del
// entrenamiento distribuido — al variar el número de goroutines.
//
// Ejecutar:  go test -bench=. -benchmem ./benchmarks/
package benchmarks

import (
	"math/rand"
	"testing"

	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
)

// makeData genera n muestras con dim features y etiqueta binaria.
func makeData(n, dim int) ([][]float64, []float64) {
	r := rand.New(rand.NewSource(1))
	X := make([][]float64, n)
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		x := make([]float64, dim)
		for j := 0; j < dim; j++ {
			x[j] = r.NormFloat64()
		}
		X[i] = x
		if r.Float64() > 0.5 {
			y[i] = 1
		}
	}
	return X, y
}

// benchGradient mide una iteración de gradiente (una época) sobre 200k
// muestras usando `workers` goroutines. Es exactamente el cómputo que un
// nodo del clúster ejecuta por época en el entrenamiento distribuido.
func benchGradient(b *testing.B, workers int) {
	dim := ml.NumFeatures()
	X, y := makeData(200_000, dim)
	m := ml.NewLogisticRegression("bench")
	w := make([]float64, dim+1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ml.RawGradientConcurrent(m, X, y, w, workers)
	}
}

func BenchmarkGradient1(b *testing.B)  { benchGradient(b, 1) }
func BenchmarkGradient2(b *testing.B)  { benchGradient(b, 2) }
func BenchmarkGradient4(b *testing.B)  { benchGradient(b, 4) }
func BenchmarkGradient8(b *testing.B)  { benchGradient(b, 8) }
func BenchmarkGradient16(b *testing.B) { benchGradient(b, 16) }
