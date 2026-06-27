// Comando del pipeline LOCAL (un proceso): carga concurrente del dataset,
// ENTRENAMIENTO real de los tres modelos por descenso de gradiente
// paralelizado con goroutines, e inferencia paralela con el worker pool.
// Sirve como línea base monolítica frente al clúster distribuido
// (cmd/master + cmd/worker_node), que usa exactamente las mismas
// primitivas de ml pero reparte el gradiente entre nodos.
//
// Uso:
//
//	go run ./cmd -dataset=data/patients.csv -workers=8 -epochs=100
//	go run ./cmd -synthetic=200000 -workers=4
//	go run ./cmd -dataset=data/patients.csv -sequential   (línea base)
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/loader"
	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
	"github.com/JoanixX/hospital-bed-prediction/internal/report"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
	"github.com/JoanixX/hospital-bed-prediction/internal/worker"
)

func main() {
	workers := flag.Int("workers", runtime.NumCPU(), "goroutines para entrenamiento e inferencia")
	dataset := flag.String("dataset", "", "ruta al CSV de pacientes (vacío => sintético)")
	synthetic := flag.Int("synthetic", 200_000, "tamaño del dataset sintético si dataset=''")
	epochs := flag.Int("epochs", 100, "épocas de entrenamiento")
	lr := flag.Float64("lr", 0.5, "tasa de aprendizaje")
	l2 := flag.Float64("l2", 1e-4, "regularización L2")
	sequential := flag.Bool("sequential", false, "forzar 1 worker (línea base)")
	pprofAddr := flag.String("pprof", "localhost:6060", "dirección del servidor pprof")
	flag.Parse()

	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	go func() {
		log.Printf("[pprof] activo en http://%s/debug/pprof/", *pprofAddr)
		if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
			log.Printf("[pprof] no se pudo iniciar: %v", err)
		}
	}()
	time.Sleep(120 * time.Millisecond)

	numWorkers := *workers
	if *sequential {
		numWorkers = 1
	}

	fmt.Println("====================================================")
	fmt.Println("  Cáncer de Próstata — Entrenamiento + Inferencia")
	fmt.Println("  Modelos: Mortalidad (logística) | Supervivencia | Costo (lineales)")
	fmt.Println("====================================================")

	// 1. Cargar dataset.
	var patients []types.Patient
	if *dataset != "" {
		fmt.Printf("\n[loader] cargando %s (concurrente, %d workers)...\n", *dataset, numWorkers)
		ps, discarded, err := loader.LoadConcurrent(loader.LoadConfig{
			Path: *dataset, NumWorkers: numWorkers, BufferSize: 1024,
		})
		if err != nil {
			log.Fatalf("[loader] error: %v", err)
		}
		patients = ps
		fmt.Printf("[loader] %d válidos, %d descartados\n", len(patients), discarded)
	} else {
		fmt.Printf("\n[loader] generando %d pacientes sintéticos...\n", *synthetic)
		patients = generateSyntheticPatients(*synthetic)
	}

	// 2. ENTRENAMIENTO (gradiente descendente paralelo con goroutines).
	cfg := ml.TrainConfig{Epochs: *epochs, LR: *lr, L2: *l2, NumWorkers: numWorkers}
	fmt.Printf("\n[train] entrenando 3 modelos: %d épocas, lr=%.3g, L2=%.1g, %d workers\n",
		cfg.Epochs, cfg.LR, cfg.L2, numWorkers)
	tStart := time.Now()
	models, rep := ml.TrainAll(patients, cfg, 0.2)
	fmt.Printf("[train] entrenamiento completado en %v\n", time.Since(tStart).Round(time.Millisecond))
	report.PrintTraining(rep)

	// 3. INFERENCIA paralela sobre todos los pacientes.
	fmt.Printf("\n[infer] inferencia paralela (%d workers) sobre %d pacientes\n",
		numWorkers, len(patients))
	pool := worker.Pool{NumWorkers: numWorkers, Models: models, Verbose: true}
	iStart := time.Now()
	results, stats := pool.Process(patients)
	iElapsed := time.Since(iStart)

	report.Print(results, stats)
	fmt.Printf("\n[timing] inferencia: %v (%.0f pacientes/s)\n",
		iElapsed.Round(time.Millisecond), float64(len(patients))/iElapsed.Seconds())
}

// generateSyntheticPatients produce datos con SEÑAL real (no ruido puro)
// para que los modelos tengan algo que aprender cuando no hay CSV.
func generateSyntheticPatients(n int) []types.Patient {
	races := []string{"white", "black", "hispanic", "asian", "other"}
	r := rand.New(rand.NewSource(42))
	patients := make([]types.Patient, n)
	for i := 0; i < n; i++ {
		age := 45 + r.Intn(40)
		psa := r.Float64() * 20
		income := 20000 + r.Float64()*80000
		coverage := 0.4 + r.Float64()*0.6
		enc := 1 + r.Intn(30)
		diag := 1 + r.Intn(8)

		// Mortalidad: probabilidad creciente con edad, PSA, diagnósticos.
		z := -6.0 + 0.05*float64(age) + 0.12*psa + 0.20*float64(diag) - 0.00002*income
		pDie := 1.0 / (1.0 + math.Exp(-z))
		died := r.Float64() < pDie

		// Supervivencia (días): decrece con edad/PSA, crece con cobertura.
		surv := 4200 - 18*float64(age) - 90*psa + 1500*coverage + r.NormFloat64()*200
		if surv < 90 {
			surv = 90
		}
		// Costo (USD): crece con PSA, encuentros, diagnósticos.
		cost := 6000 + 1500*psa + 600*float64(enc) + 2200*float64(diag) +
			0.05*income + r.NormFloat64()*1500
		if cost < 0 {
			cost = 0
		}

		patients[i] = types.Patient{
			ID:             fmt.Sprintf("PAT-%07d", i+1),
			Age:            age,
			Race:           races[r.Intn(len(races))],
			Income:         income,
			HealthcareCost: cost,
			Coverage:       coverage,
			PSALevel:       psa,
			NumEncounters:  enc,
			NumDiagnoses:   diag,
			HasDied:        died,
			SurvivalDays:   int(surv),
		}
	}
	return patients
}

