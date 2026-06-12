# Hospital Bed Prediction — Sistema concurrente PC3

Pipeline concurrente en Go para predicción de mortalidad, supervivencia y
costo de tratamiento del cáncer de próstata, basado en el patrón **worker
pool** con goroutines y `channels`. Instrumentado con `net/http/pprof` para
auditar CPU, memoria y contención de bloqueos.

## Estructura

```
PC3/
├── cmd/main.go               # entry point con pprof asíncrono
├── internal/
│   ├── loader/loader.go      # carga concurrente del CSV (fan-out/fan-in)
│   ├── models/               # modelos heurísticos (sustituidos en PC4)
│   │   ├── mortality.go
│   │   ├── survival.go
│   │   └── cost.go
│   ├── worker/
│   │   ├── worker.go         # goroutine trabajadora
│   │   └── pool.go           # coordinador y particionado
│   ├── types/types.go        # Patient, PatientResult, WorkerStats
│   └── report/report.go      # reporte agregado
├── benchmarks/pipeline_test.go  # go test -bench
├── scripts/
│   ├── setup_synthea.ps1     # descarga JAR de Synthea + ejecuta el módulo prostate_cancer
│   ├── setup_synthea.sh      #   (variante bash)
│   ├── _common.py            # utilidades compartidas (oversampling, I/O)
│   ├── generate_patients.py            # data/patients.csv            (>=1.5M)
│   ├── generate_encounters.py          # data/encounters.csv          (>=1.5M)
│   ├── generate_observations.py        # data/observations.csv        (>=1.5M)
│   ├── generate_claims.py              # data/claims.csv              (>=1.5M)
│   ├── generate_claims_transactions.py # data/claims_transactions.csv (>=1.5M)
│   ├── eda.py                # análisis exploratorio + gráficos PNG
│   └── plot_results.py       # gráficos de speedup vs workers
├── data/                     # CSVs y gráficos (no versionados)
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

## Requisitos

- Go 1.22 o superior
- Python 3.10+ con `numpy`, `pandas` y `matplotlib`
- Java 11+ (para `synthea-with-dependencies.jar`)
- Docker 24+ (opcional, sólo para despliegue contenedorizado)

## Pasos para reproducir los resultados de la PC3

### 1. Generar los datasets simulados (Synthea + oversampling)

Cada dataset se genera con un script independiente. El detalle está en
[`scripts/README.md`](scripts/README.md).

```bash
pip install numpy pandas matplotlib

# 1.a Descarga JAR de Synthea y produce la base cruda (una sola vez)
powershell -ExecutionPolicy Bypass -File scripts/setup_synthea.ps1   # Windows
# bash scripts/setup_synthea.sh                                       # Linux/Mac

# 1.b Genera cada dataset (>=1.5M filas cada uno) — un script por dataset
python scripts/generate_patients.py
python scripts/generate_encounters.py
python scripts/generate_observations.py
python scripts/generate_claims.py
python scripts/generate_claims_transactions.py
```

### 2. Análisis exploratorio (sección 4.1 del informe)

```bash
python scripts/eda.py --input data/patients.csv --output data/eda_plots
```

Genera `hist_age.png`, `hist_psa.png`, `hist_income.png`,
`hist_survival.png`, `bar_race.png`, `bar_marital.png`,
`heatmap_corr.png`.

### 3. Ejecución del pipeline concurrente

Ejecución por defecto (workers = NumCPU, dataset sintético en memoria):

```bash
go run ./cmd
```

Con CSV en disco y 8 workers explícitos:

```bash
go run ./cmd -workers=8 -dataset=data/patients.csv
```

Línea base secuencial para la comparación de la sección 4.5:

```bash
go run ./cmd -sequential -dataset=data/patients.csv
```

### 4. Profiling con pprof (sección 4.5 del informe)

Mientras el binario corre y mantiene activo `:6060`, abrir otra terminal:

```bash
# CPU profile (20 s)
go tool pprof -http=:7070 http://localhost:6060/debug/pprof/profile?seconds=20

# Heap profile
go tool pprof -http=:7071 http://localhost:6060/debug/pprof/heap

# Mutex contention
go tool pprof -http=:7072 http://localhost:6060/debug/pprof/mutex

# Goroutines en vivo
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

Capturar el flame graph desde la pestaña *VIEW → Flame Graph*.

### 5. Benchmark formal Go

```bash
go test -bench=. -benchmem -cpu=1,2,4,8 ./benchmarks/
```

Anotar los tiempos en `data/bench_results.csv` y generar los gráficos:

```bash
python scripts/plot_results.py --input data/bench_results.csv --output data/eda_plots
```

### 6. Despliegue Docker (validación de portabilidad)

```bash
docker compose build
docker compose up cluster
# pprof expuesto en http://localhost:6060/debug/pprof/
```

## Git Flow

El repositorio sigue el modelo de Driessen (2010):

- `main`     → versiones liberadas (tags por entrega: `pc3`, `pc4`, `tb2`).
- `develop`  → integración continua.
- `feature/*` → cada funcionalidad (ej. `feature/worker-pool`, `feature/pprof`).
- `release/*` → preparación de cada entrega.
- `hotfix/*`  → arreglos urgentes sobre `main`.

## Roadmap

- **PC3 (este entregable):** cargador concurrente, worker pool,
  modelos heurísticos, profiling, benchmark.
- **PC4:** sustitución de modelos heurísticos por XGBoost / Cox /
  Gradient Boosting reales; API REST con JWT; MongoDB + Redis;
  comunicación TCP nodo-coordinador.
- **TB2:** Frontend SPA en React; dashboards de impacto social;
  evaluación experimental final.
