// Benchmarks comparativos para la sección 4.5 del informe.
// Ejecutar con:  go test -bench=. -benchmem -cpu=1,2,4,8 ./benchmarks/
package benchmarks

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/JoanixX/hospital-bed-prediction/internal/types"
	"github.com/JoanixX/hospital-bed-prediction/internal/worker"
)

func generate(n int) []types.Patient {
	races := []string{"white", "black", "hispanic", "asian", "other"}
	r := rand.New(rand.NewSource(1))
	patients := make([]types.Patient, n)
	for i := 0; i < n; i++ {
		patients[i] = types.Patient{
			ID:            fmt.Sprintf("BENCH-%d", i),
			Age:           45 + r.Intn(40),
			Race:          races[r.Intn(len(races))],
			Income:        20000 + r.Float64()*80000,
			Coverage:      0.4 + r.Float64()*0.6,
			PSALevel:      r.Float64() * 20,
			NumEncounters: 1 + r.Intn(30),
			NumDiagnoses:  1 + r.Intn(8),
		}
	}
	return patients
}

// BenchmarkSequential mide la línea base (1 worker = ejecución
// secuencial sin overhead de canales adicionales).
func BenchmarkSequential(b *testing.B) {
	patients := generate(100_000)
	pool := worker.Pool{NumWorkers: 1, Verbose: false}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Process(patients)
	}
}

func BenchmarkConcurrent2(b *testing.B) {
	patients := generate(100_000)
	pool := worker.Pool{NumWorkers: 2, Verbose: false}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Process(patients)
	}
}

func BenchmarkConcurrent4(b *testing.B) {
	patients := generate(100_000)
	pool := worker.Pool{NumWorkers: 4, Verbose: false}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Process(patients)
	}
}

func BenchmarkConcurrent8(b *testing.B) {
	patients := generate(100_000)
	pool := worker.Pool{NumWorkers: 8, Verbose: false}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Process(patients)
	}
}

func BenchmarkConcurrent16(b *testing.B) {
	patients := generate(100_000)
	pool := worker.Pool{NumWorkers: 16, Verbose: false}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Process(patients)
	}
}
