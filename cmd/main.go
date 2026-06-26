// Comando principal del pipeline concurrente de predicción de cáncer
// de próstata. Lanza un servidor pprof asíncrono en :6060 para
// auditar CPU, memoria, goroutines y contención de mutex; carga el
// dataset desde CSV; ejecuta el worker pool; e imprime el reporte.
//
// Uso:
//
//	go run ./cmd -workers=8 -dataset=./data/patients.csv
//	go run ./cmd -sequential -dataset=./data/patients.csv   (línea base)
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // registra handlers en /debug/pprof/
	"os"
	"runtime"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/loader"
	"github.com/JoanixX/hospital-bed-prediction/internal/report"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
	"github.com/JoanixX/hospital-bed-prediction/internal/worker"
)

func main() {
	workers := flag.Int("workers", runtime.NumCPU(), "número de goroutines del worker pool")
	dataset := flag.String("dataset", "./data/patients.csv", "ruta al CSV de pacientes")
	sequential := flag.Bool("sequential", false, "ejecutar en modo secuencial (línea base)")
	pprofAddr := flag.String("pprof", "localhost:6060", "dirección del servidor pprof")
	loop := flag.Int("loop", 1, "repetir el procesamiento N veces (útil para capturar perfiles pprof significativos)")
	flag.Parse()

	// Habilita los perfiles de contención de mutex y de bloqueo (por defecto
	// están desactivados, lo que hace que /debug/pprof/mutex devuelva un
	// perfil vacío). Con fracción/tasa = 1 se registra cada evento.
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	// 1. Servidor pprof asíncrono.
	go func() {
		log.Printf("[pprof] servidor de profiling activo en %s", *pprofAddr)
		log.Printf("[pprof] endpoints disponibles en http://%s/debug/pprof/", *pprofAddr)
		if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
			log.Printf("[pprof] no se pudo iniciar el servidor: %v", err)
		}
	}()
	time.Sleep(150 * time.Millisecond) // dar tiempo al servidor a registrarse

	fmt.Println("====================================================")
	fmt.Println("  Sistema Distribuido - Cáncer de Próstata (Integrado)")
	fmt.Println("  Modelos: Mortalidad | Supervivencia | Costo")
	fmt.Println("====================================================")

	// 2. Cargar dataset.
	if _, err := os.Stat(*dataset); os.IsNotExist(err) {
		log.Fatalf("[loader] Error: No se encontró el dataset en '%s'. Asegúrate de generar los CSVs reales primero.\n", *dataset)
	}

	var patients []types.Patient
	var err error
	var discarded int
	fmt.Printf("\n[loader] cargando dataset concurrente desde %s ...\n", *dataset)
	patients, discarded, err = loader.LoadConcurrent(loader.LoadConfig{
		Path:       *dataset,
		NumWorkers: *workers,
		BufferSize: 1024,
	})
	if err != nil {
		log.Fatalf("[loader] error: %v", err)
	}
	fmt.Printf("[loader] %d registros válidos cargados, %d descartados\n",
		len(patients), discarded)

	// 3. Procesamiento (paralelo o secuencial).
	numWorkers := *workers
	if *sequential {
		numWorkers = 1
		fmt.Println("\n[mode] EJECUCIÓN SECUENCIAL (línea base de comparación)")
	} else {
		fmt.Printf("\n[mode] EJECUCIÓN CONCURRENTE con %d workers\n", numWorkers)
	}

	pool := worker.Pool{NumWorkers: numWorkers, Verbose: *loop == 1}
	if *loop > 1 {
		fmt.Printf("[loop] repitiendo procesamiento %d veces para profiling\n", *loop)
	}
	start := time.Now()
	var results []types.PatientResult
	var stats []types.WorkerStats
	for i := 0; i < *loop; i++ {
		results, stats = pool.Process(patients)
	}
	elapsed := time.Since(start)

	// 4. Reporte.
	report.Print(results, stats)
	fmt.Printf("\n[timing] tiempo total de procesamiento: %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("[timing] throughput aproximado: %.0f pacientes/s\n",
		float64(len(patients))/elapsed.Seconds())

	// 5. Mantener vivo el servidor pprof para captura post-mortem.
	fmt.Println("\n[pprof] manteniendo servidor activo 30s para captura de perfiles...")
	fmt.Printf("[pprof]   CPU    : go tool pprof http://%s/debug/pprof/profile?seconds=20\n", *pprofAddr)
	fmt.Printf("[pprof]   Heap   : go tool pprof http://%s/debug/pprof/heap\n", *pprofAddr)
	fmt.Printf("[pprof]   Mutex  : go tool pprof http://%s/debug/pprof/mutex\n", *pprofAddr)
	fmt.Printf("[pprof]   Goroutines: http://%s/debug/pprof/goroutine?debug=1\n", *pprofAddr)
	time.Sleep(30 * time.Second)
}
