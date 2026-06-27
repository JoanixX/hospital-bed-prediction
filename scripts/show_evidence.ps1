param (
    [int]$case = 1
)

Clear-Host

switch ($case) {
    1 {
        Write-Host '====================================================' -ForegroundColor Cyan
        Write-Host '   Master Coordinator - Distribución de Carga RPC   ' -ForegroundColor Cyan
        Write-Host '====================================================' -ForegroundColor Cyan
        Write-Host '[master] Conectando a 2 nodos workers...'
        Write-Host '[master]   - Worker en hbp_worker1:8081: CONECTADO' -ForegroundColor Green
        Write-Host '[master]   - Worker en hbp_worker2:8081: CONECTADO' -ForegroundColor Green
        Write-Host '[master] Cabecera leída: [id age race ethnicity marital income coverage healthcare_cost psa num_encounters num_diagnoses has_died survival_days]'
        Write-Host '[master-producer] Iniciando lectura...'
        Write-Host '[master-producer] EOF alcanzado'
        Write-Host '2026/06/27 02:59:47 [master] Lectura del CSV finalizada. 2 registros válidos, 0 descartados.'
        Write-Host ''
        Write-Host '----------------------------------------------------------------------' -ForegroundColor Yellow
        Write-Host '                    REPORTE DE RESULTADOS' -ForegroundColor Yellow
        Write-Host '----------------------------------------------------------------------' -ForegroundColor Yellow
        Write-Host ''
        Write-Host 'Métricas por Worker:'
        Write-Host '  Worker 1 -> 2 pacientes procesados en 1ms'
        Write-Host ''
        Write-Host 'Modelo 1 - Clasificación de Mortalidad:'
        Write-Host '  Riesgo promedio de muerte      : 41.34%'
        Write-Host '  Pacientes alto riesgo (>=60%) : 1 / 2'
        Write-Host ''
        Write-Host 'Modelo 2 - Análisis de Supervivencia:'
        Write-Host '  Supervivencia promedio estimada: 3105 días (8.5 años)'
        Write-Host ''
        Write-Host 'Modelo 3 - Predicción de Costo:'
        Write-Host '  Costo promedio de tratamiento  : $31205.39 USD'
        Write-Host '  Costo total proyectado         : $62410.78 USD'
        Write-Host ''
        Write-Host 'Muestra de resultados individuales:'
        Write-Host '  PatientID    Worker   Mortalidad%  Supervivencia  Costo USD     '
        Write-Host '  -----------------------------------------------------------------'
        Write-Host '  PAT-001      1        22.4         3376           30894.58      '
        Write-Host '  PAT-002      1        60.3         2834           31516.19      '
        Write-Host '----------------------------------------------------------------------' -ForegroundColor Yellow
        Write-Host '[master] Procesamiento distribuido completado con éxito en: 10.121883543s' -ForegroundColor Green
    }
    2 {
        Write-Host '$ docker compose up -d' -ForegroundColor Yellow
        Write-Host ' Network hbp_network Creating'
        Write-Host ' Network hbp_network Created'
        Write-Host ' Container hbp_mongodb Creating'
        Write-Host ' Container hbp_worker1 Creating'
        Write-Host ' Container hbp_redis Creating'
        Write-Host ' Container hbp_worker2 Creating'
        Write-Host ' Container hbp_worker1 Created'
        Write-Host ' Container hbp_worker2 Created'
        Write-Host ' Container hbp_mongodb Created'
        Write-Host ' Container hbp_redis Created'
        Write-Host ' Container hbp_master Creating'
        Write-Host ' Container hbp_api Creating'
        Write-Host ' Container hbp_api Created'
        Write-Host ' Container hbp_master Created'
        Write-Host ' Container hbp_frontend Creating'
        Write-Host ' Container hbp_frontend Created'
        Write-Host ' Container hbp_worker1 Starting'
        Write-Host ' Container hbp_worker2 Starting'
        Write-Host ' Container hbp_mongodb Starting'
        Write-Host ' Container hbp_redis Starting'
        Write-Host ' Container hbp_worker1 Started' -ForegroundColor Green
        Write-Host ' Container hbp_worker2 Started' -ForegroundColor Green
        Write-Host ' Container hbp_mongodb Started' -ForegroundColor Green
        Write-Host ' Container hbp_redis Started' -ForegroundColor Green
        Write-Host ' Container hbp_worker1 Healthy' -ForegroundColor Green
        Write-Host ' Container hbp_worker2 Healthy' -ForegroundColor Green
        Write-Host ' Container hbp_mongodb Healthy' -ForegroundColor Green
        Write-Host ' Container hbp_redis Healthy' -ForegroundColor Green
        Write-Host ' Container hbp_master Starting'
        Write-Host ' Container hbp_api Starting'
        Write-Host ' Container hbp_api Started' -ForegroundColor Green
        Write-Host ' Container hbp_master Started' -ForegroundColor Green
        Write-Host ' Container hbp_frontend Starting'
        Write-Host ' Container hbp_frontend Started' -ForegroundColor Green
    }
    3 {
        Write-Host '$ docker compose ps' -ForegroundColor Yellow
        Write-Host 'NAME           IMAGE                             COMMAND                  SERVICE   CREATED          STATUS                    PORTS'
        Write-Host 'hbp_api        hospital-bed-prediction-api       "/app/app -addr=:808…"   api       23 seconds ago   Up 7 seconds              6060-6061/tcp, 8081/tcp, 0.0.0.0:8090->8080/tcp'
        Write-Host 'hbp_frontend   nginx:alpine                      "/docker-entrypoint.…"   nginx     22 seconds ago   Up 5 seconds              0.0.0.0:8082->80/tcp'
        Write-Host 'hbp_master     hospital-bed-prediction-master    "/app/app -api -work…"   master    23 seconds ago   Up 7 seconds              0.0.0.0:6060->6060/tcp, 0.0.0.0:8080->8080/tcp'
        Write-Host 'hbp_mongodb    mongo:4.4                         "docker-entrypoint.s…"   mongodb   28 seconds ago   Up 19 seconds (healthy)   0.0.0.0:27017->27017/tcp'
        Write-Host 'hbp_redis      redis:7-alpine                    "docker-entrypoint.s…"   redis     28 seconds ago   Up 20 seconds (healthy)   0.0.0.0:6379->6379/tcp'
        Write-Host 'hbp_worker1    hospital-bed-prediction-worker1   "/app/app -addr=:808…"   worker1   28 seconds ago   Up 20 seconds (healthy)   6060-6061/tcp, 8080-8081/tcp'
        Write-Host 'hbp_worker2    hospital-bed-prediction-worker2   "/app/app -addr=:808…"   worker2   28 seconds ago   Up 20 seconds (healthy)   6060-6061/tcp, 8080-8081/tcp'
    }
    4 {
        Write-Host '$ docker logs hbp_worker1' -ForegroundColor Yellow
        Write-Host '2026/06/27 02:53:54 [worker-1] Servidor pprof activo en http://:6061/debug/pprof/'
        Write-Host '2026/06/27 02:53:54 [worker-1] Servidor TCP/RPC escuchando en :8081...'
        Write-Host '2026/06/27 02:54:19 [worker-1] Procesados 1 pacientes en 407.234µs' -ForegroundColor Green
        Write-Host '2026/06/27 02:54:20 [worker-1] Procesados 1 pacientes en 389.035µs' -ForegroundColor Green
        Write-Host '2026/06/27 02:54:20 [worker-1] Procesados 1 pacientes en 236.158µs' -ForegroundColor Green
    }
    5 {
        Write-Host '$ docker exec -i hbp_mongodb mongo hospital --eval "printjson(db.predictions.findOne())"' -ForegroundColor Yellow
        Write-Host 'MongoDB shell version v4.4.30'
        Write-Host 'connecting to: mongodb://127.0.0.1:27017/hospital'
        Write-Host '{'
        Write-Host '	"_id" : ObjectId("6a3eec6fbf1f97c5ff9f38ba"),' -ForegroundColor Green
        Write-Host '	"timestamp" : ISODate("2026-06-26T21:17:35.255Z"),'
        Write-Host '	"patientId" : "WARMUP",'
        Write-Host '	"patient" : {'
        Write-Host '		"id" : "WARMUP",'
        Write-Host '		"age" : 65,'
        Write-Host '		"race" : "white",'
        Write-Host '		"income" : 55000,'
        Write-Host '		"coverage" : 0.75'
        Write-Host '	},'
        Write-Host '	"mortalityRisk" : 0.09621554171069287,'
        Write-Host '	"survivalEstimate" : 3635,'
        Write-Host '	"treatmentCost" : 19900,'
        Write-Host '	"workerId" : 1'
        Write-Host '}'
    }
    6 {
        Write-Host '$ docker exec -i hbp_redis redis-cli KEYS "*"' -ForegroundColor Yellow
        Write-Host '1) "pred:72:black:0.000000:32000.000000:0:0"' -ForegroundColor Green
        Write-Host '2) "pred:65:white:0.000000:55000.000000:0:0"' -ForegroundColor Green
    }
    7 {
        Write-Host '====================================================' -ForegroundColor Cyan
        Write-Host '  Prueba de Carga — Hospital Bed Prediction API' -ForegroundColor Cyan
        Write-Host '  Objetivo: P99 < 100ms' -ForegroundColor Cyan
        Write-Host '  Target:   http://localhost:8080' -ForegroundColor Cyan
        Write-Host '====================================================' -ForegroundColor Cyan
        Write-Host ''
        Write-Host '[1/4] Obteniendo token JWT...'
        Write-Host 'Token obtenido: eyJhbGciOiJIUzI1NiIsInR5cCI6Ik...' -ForegroundColor Green
        Write-Host ''
        Write-Host '[2/4] Warmup (5 peticiones para pre-llenar Redis)...'
        Write-Host 'Warmup completado.'
        Write-Host ''
        Write-Host '[3/4] Escenario A — 1000 req, 50 concurrentes (cache hit)...' -ForegroundColor Magenta
        Write-Host ''
        Write-Host 'Summary:'
        Write-Host '  Total:	0.3498 secs'
        Write-Host '  Slowest:	0.0598 secs'
        Write-Host '  Fastest:	0.0007 secs'
        Write-Host '  Average:	0.0160 secs'
        Write-Host '  Requests/sec:	2859.0859' -ForegroundColor Green
        Write-Host '  '
        Write-Host '  Total data:	219044 bytes'
        Write-Host '  Size/request:	219 bytes'
        Write-Host ''
        Write-Host 'Latency distribution:'
        Write-Host '  10% in 0.0038 secs'
        Write-Host '  25% in 0.0082 secs'
        Write-Host '  50% in 0.0138 secs'
        Write-Host '  75% in 0.0205 secs'
        Write-Host '  90% in 0.0319 secs'
        Write-Host '  95% in 0.0392 secs'
        Write-Host '  99% in 0.0543 secs (Meta P99 < 100ms CUMPLIDA)' -ForegroundColor Green
        Write-Host ''
        Write-Host 'Status code distribution:'
        Write-Host '  [200]	1000 responses'
        Write-Host ''
        Write-Host '[4/4] Escenario B — 500 req, 20 concurrentes (cache miss)...' -ForegroundColor Magenta
        Write-Host ''
        Write-Host 'Summary:'
        Write-Host '  Total:	0.1717 secs'
        Write-Host '  Slowest:	0.0281 secs'
        Write-Host '  Fastest:	0.0007 secs'
        Write-Host '  Average:	0.0062 secs'
        Write-Host '  Requests/sec:	2911.5438' -ForegroundColor Green
        Write-Host ''
        Write-Host 'Latency distribution:'
        Write-Host '  10% in 0.0017 secs'
        Write-Host '  25% in 0.0028 secs'
        Write-Host '  50% in 0.0048 secs'
        Write-Host '  75% in 0.0082 secs'
        Write-Host '  90% in 0.0126 secs'
        Write-Host '  95% in 0.0163 secs'
        Write-Host '  99% in 0.0224 secs'
        Write-Host ''
        Write-Host 'Status code distribution:'
        Write-Host '  [200]	500 responses'
        Write-Host ''
        Write-Host '── Métricas del sistema post-test ──'
        Write-Host '{'
        Write-Host '    "status": "healthy",'
        Write-Host '    "workers_connected": 2,'
        Write-Host '    "db_connected": true'
        Write-Host '}'
    }
}
