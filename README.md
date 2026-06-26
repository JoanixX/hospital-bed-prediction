# Hospital Bed Prediction — Sistema Concurrente y Distribuido

Pipeline concurrente y distribuido en Go para predicción de mortalidad, supervivencia y
costo de tratamiento del cáncer de próstata. Implementa dos modos de operación:

- **Local:** worker pool con goroutines y channels (`cmd/main.go`)
- **Distribuido:** clúster TCP/RPC con nodo Master coordinador y nodos Workers remotos (`cmd/master` + `cmd/worker_node`)

El Master también expone una **API REST con autenticación JWT**, caché en **Redis** y persistencia en **MongoDB**, además de telemetría en tiempo real por **WebSockets**. Instrumentado con `net/http/pprof` para auditar CPU, memoria y contención de bloqueos.

## Estructura del proyecto

```
PC4/
├── cmd/
│   ├── main.go               # entry point del pipeline concurrente + pprof
│   ├── api/
│   │   └── main.go           # entry point del servidor HTTP (Gin + Redis + JWT)
│   ├── analyzer/main.go      # analizador estadístico concurrente
│   ├── worker_node/main.go   # nodo worker del clúster TCP/RPC
│   └── master/main.go        # nodo coordinador del clúster TCP/RPC
├── internal/
│   ├── loader/loader.go      # carga concurrente del CSV (fan-out/fan-in)
│   ├── models/               # modelos predictivos
│   │   ├── mortality.go      # regresión logística (mortalidad)
│   │   ├── survival.go       # estimación de supervivencia
│   │   ├── cost.go           # estimación de costo de tratamiento
│   │   └── normalization.go  # normalización Min-Max concurrente del PSA
│   ├── worker/
│   │   ├── worker.go         # goroutine trabajadora
│   │   └── pool.go           # coordinador y particionado
│   ├── api/
│   │   ├── dto/dto.go        # structs JSON (request/response)
│   │   ├── handlers/
│   │   │   ├── predict.go    # POST /predict
│   │   │   └── stats.go      # GET /stats + POST /login
│   │   └── middleware/
│   │       ├── auth.go       # generación y validación JWT (HS256)
│   │       └── cache.go      # middleware Redis (SHA-256 key, TTL 5 min)
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
│   ├── plot_results.py       # gráficos de speedup vs workers
│   └── load_test.sh          # prueba de carga con hey (Tarea 9)
├── data/                     # CSVs y gráficos (no versionados)
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

## Requisitos

- Go 1.22 o superior
- Python 3.10+ con `numpy`, `pandas` y `matplotlib`
- Java 11+ (para `synthea-with-dependencies.jar`; `setup_synthea.ps1` puede descargarlo automáticamente)
- Docker 24+ (para despliegue contenedorizado con todos los servicios)

## Pasos para reproducir los resultados

### 1. Generar los datasets simulados (Synthea + oversampling)

Cada dataset se genera con un script independiente. El detalle completo está en [`scripts/README.md`](scripts/README.md).

```bash
pip install numpy pandas matplotlib

# 1.a  Descarga JAR de Synthea y produce la base cruda (una sola vez)
powershell -ExecutionPolicy Bypass -File scripts/setup_synthea.ps1   # Windows
# bash scripts/setup_synthea.sh                                       # Linux/Mac

# 1.b  Genera cada dataset (≥1.5M filas cada uno)
python scripts/generate_patients.py
python scripts/generate_encounters.py
python scripts/generate_observations.py
python scripts/generate_claims.py
python scripts/generate_claims_transactions.py
```

### 2. Análisis exploratorio

```bash
python scripts/eda.py --input data/patients.csv --output data/eda_plots
```

Genera `hist_age.png`, `hist_psa.png`, `hist_income.png`, `hist_survival.png`, `bar_race.png`, `bar_marital.png`, `heatmap_corr.png`.

### 3. Pipeline local (worker pool)

Ejecución por defecto (workers = NumCPU, dataset sintético en memoria):

```bash
go run ./cmd
```

Con CSV en disco y 8 workers:

```bash
go run ./cmd -workers=8 -dataset=data/patients.csv
```

Línea base secuencial para comparación:

```bash
go run ./cmd -sequential -dataset=data/patients.csv
```

Flag `-loop N` repite el procesamiento N veces, útil para capturar perfiles pprof con señal suficiente:

```bash
go run ./cmd -workers=8 -dataset=data/patients.csv -loop=5
```

### 4. Analizador estadístico concurrente

Procesa el CSV con el patrón fan-out/fan-in y calcula media, varianza, desviación estándar, mínimo y máximo de las columnas clave (`age`, `psa`, `healthcare_cost`, `survival_days`, `income`):

```bash
go run ./cmd/analyzer -dataset=data/patients.csv -workers=4 -batch=5000
```

### 5. Clúster distribuido TCP/RPC

#### 5.a  Modo CLI (procesamiento batch desde CSV)

Levanta los Workers y luego el Master en terminales separadas:

```bash
# Terminal 1 — Worker 1
go run ./cmd/worker_node -addr=localhost:8081 -pprof=localhost:6061 -id=1

# Terminal 2 — Worker 2
go run ./cmd/worker_node -addr=localhost:8082 -pprof=localhost:6062 -id=2

# Terminal 3 — Master (coordinador)
go run ./cmd/master -dataset=data/patients.csv -workers="localhost:8081,localhost:8082" -batch=5000
```

El Master lee el CSV en streaming, distribuye lotes de 5000 pacientes con balanceo dinámico entre los Workers, recolecta resultados y muestra el reporte agregado.

#### 5.b  Modo API REST

Añade el flag `-api` al Master para exponer los endpoints HTTP:

```bash
go run ./cmd/master -api -workers="localhost:8081,localhost:8082" -port=:8080
```

**Endpoints disponibles:**

| Método | Ruta                        | Descripción                                        |
|--------|-----------------------------|----------------------------------------------------|
| POST   | `/api/v1/login`             | Obtiene un token JWT (`admin` / `admin123`)        |
| GET    | `/health`                   | Estado del clúster y conexión a bases de datos     |
| POST   | `/api/v1/predict`           | Predice para uno o varios pacientes (requiere JWT) |
| WS     | `/api/v1/ws/metrics`        | Stream de telemetría en tiempo real (WebSockets)   |

**Ejemplo de flujo:**

```bash
# 1. Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | jq -r '.token')

# 2. Predicción
curl -s -X POST http://localhost:8080/api/v1/predict \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '[{"ID":"P001","Age":65,"PSALevel":8.5,"Income":45000,"Coverage":0.7,"NumEncounters":5,"NumDiagnoses":2}]'
```

La respuesta incluye los resultados de los tres modelos, el tiempo de procesamiento y la fuente (caché Redis o cómputo en workers).

### 6. Profiling con pprof

Mientras el binario mantiene activo `:6060` (modo local) o `:6061`/`:6062` (workers):

```bash
# CPU profile (20 s)
go tool pprof -http=:7070 "http://localhost:6060/debug/pprof/profile?seconds=20"

# Heap profile
go tool pprof -http=:7071 http://localhost:6060/debug/pprof/heap

# Contención de mutex
go tool pprof -http=:7072 http://localhost:6060/debug/pprof/mutex

# Goroutines en vivo
curl "http://localhost:6060/debug/pprof/goroutine?debug=1"
```

Captura el flame graph desde la pestaña *VIEW → Flame Graph*.

### 7. Benchmarks formales

```bash
go test -bench=. -benchmem -cpu=1,2,4,8 ./benchmarks/
```

Anota los tiempos en `data/bench_results.csv` y genera los gráficos:

```bash
python scripts/plot_results.py --input data/bench_results.csv --output data/eda_plots
```

### 8. Despliegue con Docker Compose

Levanta los 6 servicios del clúster completo (MongoDB, Redis, Master API, Worker 1, Worker 2, Nginx frontend):

```bash
docker compose build
docker compose up
```

| Servicio  | Puerto expuesto | Descripción                              |
|-----------|-----------------|------------------------------------------|
| mongodb   | 27017           | Persistencia de predicciones             |
| redis     | 6379            | Caché de resultados por parámetros       |
| master    | 8080            | API REST + WebSockets + coordinador RPC  |
| worker1   | —               | Cálculo concurrente (red interna)        |
| worker2   | —               | Cálculo concurrente (red interna)        |
| nginx     | 80              | Frontend SPA estático                    |

## Arquitectura del clúster

```
[Cliente HTTP/WS]
       │
       ▼
  [Master :8080]  ◄──── JWT auth ────►  MongoDB :27017
       │                               Redis :6379
       │  net/rpc (TCP)
       ├──────────────────────► [Worker 1 :8081]
       └──────────────────────► [Worker 2 :8082]
                                    │
                                    └── fan-out local (NumCPU goroutines)
                                        ├── PredictMortality (Regresión Logística)
                                        ├── PredictSurvival  (Regresión Lineal)
                                        └── PredictTreatmentCost (Regresión Lineal)
```

El Master aplica **balanceo de carga dinámico**: cada goroutine despachadora consume del canal `batchesCh` a su propio ritmo; los workers más rápidos procesan más lotes automáticamente. En modo API, el Master también consulta Redis antes de despachar al clúster, reduciendo latencia en predicciones repetidas.

## Modelos predictivos

Todos los modelos usan normalización Min-Max concurrente del PSA antes de evaluarse:

| Modelo              | Tipo                  | Salida                        |
|---------------------|-----------------------|-------------------------------|
| `PredictMortality`  | Regresión logística   | Probabilidad de muerte [0, 1] |
| `PredictSurvival`   | Regresión lineal múlt.| Días estimados de supervivencia |
| `PredictTreatmentCost` | Regresión lineal múlt. | Costo estimado en USD      |

## API REST (PC4)

### Iniciar el servidor

```bash
# Requisito: Redis corriendo en localhost:6379
docker run -p 6379:6379 redis:alpine

# Levantar la API
go run ./cmd/api
```

Variables de entorno opcionales:

| Variable     | Default                        | Descripción                     |
|--------------|--------------------------------|---------------------------------|
| `JWT_SECRET` | `hospital-bed-dev-secret-...`  | Clave HMAC para firmar tokens   |
| `REDIS_ADDR` | `localhost:6379`               | Dirección del servidor Redis    |
| `REDIS_PASS` | _(vacío)_                      | Contraseña de Redis             |

### Endpoints

| Método | Ruta       | Auth | Descripción                          |
|--------|------------|------|--------------------------------------|
| POST   | `/login`   | No   | Obtiene un JWT (válido 24 h)         |
| POST   | `/predict` | JWT  | Predicción de mortalidad/supervivencia/costo |
| GET    | `/stats`   | No   | Métricas agregadas del sistema       |

### Uso desde PowerShell (Windows)

**1. Obtener token:**
```powershell
$response = Invoke-RestMethod -Method POST -Uri "http://localhost:8080/login" -ContentType "application/json" -Body '{"username":"admin","password":"hospital2024"}'
$token = $response.token
```

**2. Hacer una predicción:**
```powershell
Invoke-RestMethod -Method POST -Uri "http://localhost:8080/predict" -ContentType "application/json" -Headers @{ Authorization = "Bearer $token" } -Body '{"age":65,"race":"white","income":55000,"psa_level":8.5,"coverage":0.75,"num_encounters":6,"num_diagnoses":2}'
```

**3. Ver métricas del sistema:**
```powershell
Invoke-RestMethod -Uri "http://localhost:8080/stats"
```

### Respuesta de `/predict`

```json
{
  "patient_id":        "API-REQUEST",
  "mortality_risk":    0.399,
  "survival_estimate": 2959,
  "treatment_cost":    38200,
  "cached":            false
}
```

`cached: true` indica que la respuesta fue servida desde Redis sin invocar el pipeline.

### Prueba de carga (Tarea 9)

```bash
go install github.com/rakyll/hey@latest
bash scripts/load_test.sh
```

El script ejecuta dos escenarios: 1000 peticiones con 50 concurrentes (cache hit) y 500 peticiones con 20 concurrentes (cache miss). El objetivo es P99 < 100 ms.

---

## Git Flow

El repositorio sigue el modelo de Driessen (2010):

- `main`     → versiones liberadas (tags por entrega: `pc3`, `pc4`, `tb2`).
- `develop`  → integración continua.
- `feature/*` → cada funcionalidad (ej. `feature/worker-pool`, `feature/pprof`, `feature/API`).
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
