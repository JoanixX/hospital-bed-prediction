// Package main implementa el nodo Master ("Coordinador") de nuestro clúster.
// Lee el CSV físico en streaming lote por lote y distribuye los registros
// uniformemente y de forma concurrente a los nodos Workers usando net/rpc.
//
// También expone una API REST segura mediante JWT, gestiona almacenamiento persistente
// en MongoDB y caché en Redis, y transmite telemetría en tiempo real por WebSockets.
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/db"
	"github.com/JoanixX/hospital-bed-prediction/internal/report"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// Clave secreta para firmar los tokens JWT
var jwtKey = []byte("super-secret-key-pcd-2026")

// Upgrader para WebSockets (permitir orígenes de desarrollo CORS)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// APIServer contiene los clientes RPC de los workers para procesar peticiones web.
type APIServer struct {
	clients        []*rpc.Client
	workerAddrs    []string
	mu             sync.Mutex
	totalRequests  int64
	totalLatencyNs int64
	wsClients      map[*websocket.Conn]bool
	dbEnabled      bool
}

// Estructuras de login
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

func main() {
	datasetPath := flag.String("dataset", "data/patients.csv", "Ruta al archivo CSV de pacientes")
	workersStr := flag.String("workers", "localhost:8081,localhost:8082", "Direcciones TCP/RPC de los workers (separadas por comas)")
	batchSize := flag.Int("batch", 5000, "Tamaño del bloque de pacientes a enviar por llamada RPC")
	apiMode := flag.Bool("api", false, "Iniciar el Master en modo API REST")
	port := flag.String("port", ":8080", "Puerto para el servidor HTTP REST API")
	flag.Parse()

	startTime := time.Now()

	// 1. Inicialización de base de datos y caché
	dbEnabled := true
	log.Println("[master] Inicializando bases de datos...")
	if err := db.InitDB(); err != nil {
		log.Printf("[master] [WARN] No se pudo conectar a MongoDB o Redis: %v. Continuando en modo degradado (sin base de datos/caché).", err)
		dbEnabled = false
	}

	// Parsear direcciones de los workers
	workerAddrs := strings.Split(*workersStr, ",")
	for i := range workerAddrs {
		workerAddrs[i] = strings.TrimSpace(workerAddrs[i])
	}

	fmt.Println("====================================================")
	if *apiMode {
		fmt.Println("   Master Coordinator - Modo API REST Activo        ")
	} else {
		fmt.Println("   Master Coordinator - Distribución de Carga RPC   ")
	}
	fmt.Println("====================================================")
	fmt.Printf("[master] Conectando a %d nodos workers...\n", len(workerAddrs))

	// Conectar a cada uno de los workers mediante RPC
	clients := make([]*rpc.Client, 0, len(workerAddrs))
	for _, addr := range workerAddrs {
		client, err := rpc.Dial("tcp", addr)
		if err != nil {
			log.Fatalf("[master] Error fatal: no se pudo conectar al worker en %s: %v", addr, err)
		}
		defer client.Close()
		fmt.Printf("[master]   - Worker en %s: CONECTADO\n", addr)
		clients = append(clients, client)
	}

	if *apiMode {
		runAPIServer(*port, clients, workerAddrs, dbEnabled)
		return
	}

	// Modo CLI - Lectura física y distribución
	file, err := os.Open(*datasetPath)
	if err != nil {
		log.Fatalf("[master] Error al abrir el dataset: %v", err)
	}
	defer file.Close()

	bufReader := bufio.NewReaderSize(file, 1*1024*1024)
	csvReader := csv.NewReader(bufReader)

	// Leer cabecera
	header, err := csvReader.Read()
	if err != nil {
		log.Fatalf("[master] Error leyendo cabecera: %v", err)
	}
	fmt.Printf("[master] Cabecera leída: %v\n", header)
	colIdx := indexColumns(header)

	// Canales de control
	batchesCh := make(chan []types.Patient, len(clients)*4)
	resultsCh := make(chan []types.PatientResult, len(clients)*4)
	statsCh := make(chan types.WorkerStats, len(clients)*4)

	var wgDispatchers sync.WaitGroup

	// Lanzar dispatchers
	for i, client := range clients {
		wgDispatchers.Add(1)
		go func(workerID int, c *rpc.Client, addr string) {
			defer wgDispatchers.Done()
			for batch := range batchesCh {
				// Para propósitos de este script batch, procesamos directamente
				args := types.ProcessArgs{Patients: batch}
				var reply types.ProcessReply

				err := c.Call("WorkerService.ProcessBatch", &args, &reply)
				if err != nil {
					log.Printf("[master] Error en RPC worker %s: %v. Re-encolando lote...\n", addr, err)
					batchesCh <- batch
					continue
				}

				// Asíncronamente guardar en MongoDB/Redis si están disponibles
				if dbEnabled {
					go func(pts []types.Patient, rts []types.PatientResult) {
						ctx := context.Background()
						for k, result := range rts {
							patient := pts[k]
							key := db.GenerateCacheKey(patient)
							_ = db.SetCachedPrediction(ctx, key, result, 15*time.Minute)
							_ = db.SavePrediction(ctx, patient, result)
						}
					}(batch, reply.Results)
				}

				resultsCh <- reply.Results
				statsCh <- reply.Stats
			}
		}(i+1, client, workerAddrs[i])
	}

	// Productora
	var totalRecordsRead int64
	var discardedRecords int64
	go func() {
		defer close(batchesCh)
		fmt.Println("[master-producer] Iniciando lectura...")
		currentBatch := make([]types.Patient, 0, *batchSize)

		for {
			row, err := csvReader.Read()
			if err == io.EOF {
				fmt.Println("[master-producer] EOF alcanzado")
				break
			}
			if err != nil {
				fmt.Printf("[master-producer] Error de lectura: %v\n", err)
				discardedRecords++
				continue
			}

			p, ok := parsePatient(row, colIdx)
			if !ok {
				discardedRecords++
				continue
			}

			totalRecordsRead++
			currentBatch = append(currentBatch, p)

			if len(currentBatch) == *batchSize {
				batchesCh <- currentBatch
				currentBatch = make([]types.Patient, 0, *batchSize)
			}
		}

		if len(currentBatch) > 0 {
			batchesCh <- currentBatch
		}
		log.Printf("[master] Lectura del CSV finalizada. %d registros válidos, %d descartados.\n", totalRecordsRead, discardedRecords)
	}()

	go func() {
		wgDispatchers.Wait()
		close(resultsCh)
		close(statsCh)
	}()

	var allResults []types.PatientResult
	var allStats []types.WorkerStats

	var wgAggregator sync.WaitGroup
	wgAggregator.Add(2)

	go func() {
		defer wgAggregator.Done()
		for r := range resultsCh {
			allResults = append(allResults, r...)
		}
	}()

	go func() {
		defer wgAggregator.Done()
		for s := range statsCh {
			allStats = append(allStats, s)
		}
	}()

	wgAggregator.Wait()
	elapsed := time.Since(startTime)

	report.Print(allResults, allStats)
	fmt.Printf("\n[master] Procesamiento distribuido completado con éxito en: %v\n", elapsed)
}

// indexColumns mapea los nombres de cabecera a su índice
func indexColumns(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

// parsePatient parsea y valida un registro clínico
func parsePatient(row []string, idx map[string]int) (types.Patient, bool) {
	get := func(key string) string {
		i, ok := idx[key]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	id := get("id")
	if id == "" {
		return types.Patient{}, false
	}

	age, err := strconv.Atoi(get("age"))
	if err != nil || age < 0 || age > 120 {
		return types.Patient{}, false
	}

	psa, err := strconv.ParseFloat(get("psa"), 64)
	if err != nil || psa < 0 || psa > 200 {
		return types.Patient{}, false
	}

	income, _ := strconv.ParseFloat(get("income"), 64)
	cov, _ := strconv.ParseFloat(get("coverage"), 64)
	cost, _ := strconv.ParseFloat(get("healthcare_cost"), 64)
	enc, _ := strconv.Atoi(get("num_encounters"))
	diag, _ := strconv.Atoi(get("num_diagnoses"))
	died := strings.EqualFold(get("has_died"), "true") || get("has_died") == "1"
	sd, _ := strconv.Atoi(get("survival_days"))

	return types.Patient{
		ID:             id,
		Age:            age,
		Race:           strings.ToLower(get("race")),
		Ethnicity:      strings.ToLower(get("ethnicity")),
		MaritalStatus:  strings.ToLower(get("marital")),
		Income:         income,
		Coverage:       cov,
		HealthcareCost: cost,
		PSALevel:       psa,
		NumEncounters:  enc,
		NumDiagnoses:   diag,
		HasDied:        died,
		SurvivalDays:   sd,
	}, true
}

// runAPIServer inicia el servidor REST HTTP y WebSockets
func runAPIServer(port string, clients []*rpc.Client, workerAddrs []string, dbEnabled bool) {
	api := &APIServer{
		clients:     clients,
		workerAddrs: workerAddrs,
		wsClients:   make(map[*websocket.Conn]bool),
		dbEnabled:   dbEnabled,
	}

	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	// Endpoints públicos
	http.HandleFunc("/api/v1/login", api.LoginHandler)
	http.HandleFunc("/health", api.HealthHandler)

	// Endpoints protegidos por JWT (verificación interna en el Handler)
	http.HandleFunc("/api/v1/predict", api.PredictHandler)

	// WebSockets para monitoreo en tiempo real
	http.HandleFunc("/api/v1/ws/metrics", api.WSMetricsHandler)

	// Iniciar broadcaster de métricas en tiempo real
	api.startMetricsBroadcaster()

	log.Printf("[master-api] Servidor HTTP REST API escuchando en http://localhost%s ...\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("[master-api] Error levantando el servidor HTTP: %v", err)
	}
}

// HealthHandler verifica la disponibilidad del clúster
func (api *APIServer) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "healthy", "workers_connected": ` + strconv.Itoa(len(api.clients)) + `, "db_connected": ` + strconv.FormatBool(api.dbEnabled) + `}`))
}

// LoginHandler autentica al usuario y le expide un token JWT
func (api *APIServer) LoginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "JSON de login inválido"}`))
		return
	}

	// Validación simple
	if req.Username == "admin" && req.Password == "admin123" {
		expirationTime := time.Now().Add(24 * time.Hour)
		claims := &jwt.RegisteredClaims{
			Subject:   req.Username,
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString(jwtKey)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Error al generar JWT"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{Token: tokenString})
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error": "Usuario o contraseña inválidos"}`))
}

// PredictHandler procesa las solicitudes de predicción aplicando seguridad JWT,
// consulta a caché de Redis y almacenamiento asíncrono en MongoDB.
func (api *APIServer) PredictHandler(w http.ResponseWriter, r *http.Request) {
	// Cabeceras CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(`{"error": "Método no permitido. Use POST"}`))
		return
	}

	// 1. Verificación de JWT Token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Token de autorización requerido"}`))
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Formato de token inválido. Use 'Bearer <token>'"}`))
		return
	}

	// Validar el token
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(t *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Token inválido o expirado"}`))
		return
	}

	// 2. Lectura del cuerpo del paciente
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Error al leer el cuerpo"}`))
		return
	}

	var patients []types.Patient
	err = json.Unmarshal(bodyBytes, &patients)
	if err != nil {
		// Intentar como un único paciente
		var p types.Patient
		err2 := json.Unmarshal(bodyBytes, &p)
		if err2 == nil {
			patients = []types.Patient{p}
		} else {
			var wrapper struct {
				Patients []types.Patient `json:"patients"`
			}
			err3 := json.Unmarshal(bodyBytes, &wrapper)
			if err3 == nil {
				patients = wrapper.Patients
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error": "JSON no válido para Patient"}`))
				return
			}
		}
	}

	if len(patients) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "No se enviaron pacientes"}`))
		return
	}

	// Llenar IDs automáticos por seguridad
	for i := range patients {
		if patients[i].ID == "" {
			patients[i].ID = fmt.Sprintf("PAT-API-%d-%d", time.Now().UnixNano(), i+1)
		}
	}

	predictStart := time.Now()
	ctx := context.Background()

	// 3. Resolución en Caché de Redis (si está activo)
	finalResults := make([]types.PatientResult, len(patients))
	var cacheMissIndices []int
	var cacheMissPatients []types.Patient

	for i, p := range patients {
		if api.dbEnabled {
			key := db.GenerateCacheKey(p)
			cachedRes, err := db.GetCachedPrediction(ctx, key)
			if err == nil && cachedRes != nil {
				// Cache Hit!
				cachedRes.PatientID = p.ID // Mantener ID de la solicitud actual
				finalResults[i] = *cachedRes
				continue
			}
		}
		// Cache Miss o DB apagada
		cacheMissIndices = append(cacheMissIndices, i)
		cacheMissPatients = append(cacheMissPatients, p)
	}

	// 4. Despachar Cache Misses al clúster de workers RPC
	numMissed := len(cacheMissPatients)
	if numMissed > 0 {
		numWorkers := len(api.clients)
		if numWorkers == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "No hay workers de cálculo conectados"}`))
			return
		}

		var wg sync.WaitGroup
		workerResults := make([][]types.PatientResult, numWorkers)
		errChan := make(chan error, numWorkers)

		chunkSize := numMissed / numWorkers
		if chunkSize == 0 {
			chunkSize = 1
		}

		for i := 0; i < numWorkers; i++ {
			start := i * chunkSize
			if start >= numMissed {
				break
			}
			end := start + chunkSize
			if i == numWorkers-1 || end > numMissed {
				end = numMissed
			}

			wg.Add(1)
			go func(workerIndex int, subset []types.Patient) {
				defer wg.Done()

				args := types.ProcessArgs{Patients: subset}
				var reply types.ProcessReply

				client := api.clients[workerIndex]
				errCall := client.Call("WorkerService.ProcessBatch", &args, &reply)
				if errCall != nil {
					// Fallback simple a otro worker
					fbIdx := (workerIndex + 1) % numWorkers
					errCall = api.clients[fbIdx].Call("WorkerService.ProcessBatch", &args, &reply)
					if errCall != nil {
						errChan <- fmt.Errorf("error llamando al worker %d y fallback: %v", workerIndex, errCall)
						return
					}
				}

				workerResults[workerIndex] = reply.Results
			}(i, cacheMissPatients[start:end])
		}

		wg.Wait()
		close(errChan)

		// Verificar si alguna partición requerida falló
		anyFailed := false
		for i := 0; i < numWorkers; i++ {
			start := i * chunkSize
			if start < numMissed && workerResults[i] == nil {
				anyFailed = true
				break
			}
		}

		if anyFailed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "Fallo de ejecución en los workers de cálculo"}`))
			return
		}

		// Consolidar resultados en el orden original secuencial
		var computedResults []types.PatientResult
		for i := 0; i < numWorkers; i++ {
			if workerResults[i] != nil {
				computedResults = append(computedResults, workerResults[i]...)
			}
		}

		// Guardar en base de datos de manera asíncrona para maximizar throughput y caching en Redis
		if api.dbEnabled && len(computedResults) > 0 {
			go func(pts []types.Patient, rts []types.PatientResult) {
				dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer dbCancel()
				for idx, result := range rts {
					patient := pts[idx]
					key := db.GenerateCacheKey(patient)
					_ = db.SetCachedPrediction(dbCtx, key, result, 10*time.Minute)
					_ = db.SavePrediction(dbCtx, patient, result)
				}
			}(cacheMissPatients, computedResults)
		}

		// Mapear de vuelta los resultados calculados a sus posiciones originales
		for idx, result := range computedResults {
			origIdx := cacheMissIndices[idx]
			finalResults[origIdx] = result
		}
	}

	elapsed := time.Since(predictStart)

	// Registrar métricas de latencia de forma segura
	atomic.AddInt64(&api.totalRequests, int64(len(patients)))
	atomic.AddInt64(&api.totalLatencyNs, elapsed.Nanoseconds())

	// Responder al cliente
	response := struct {
		Results        []types.PatientResult `json:"results"`
		ProcessingTime string                `json:"processingTime"`
		Source         string                `json:"source"` // Indica si fue resuelto de cache o red
	}{
		Results:        finalResults,
		ProcessingTime: elapsed.String(),
		Source:         fmt.Sprintf("Resolved %d from cache, %d from worker cluster", len(patients)-numMissed, numMissed),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// WSMetricsHandler maneja las conexiones WebSockets del panel de administrador
func (api *APIServer) WSMetricsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[master-ws] Error al actualizar a WebSocket: %v", err)
		return
	}

	api.mu.Lock()
	api.wsClients[conn] = true
	api.mu.Unlock()

	log.Printf("[master-ws] Nuevo cliente registrado. Total: %d", len(api.wsClients))

	// Hilo de lectura para detectar cierres
	go func(c *websocket.Conn) {
		defer func() {
			c.Close()
			api.mu.Lock()
			delete(api.wsClients, c)
			api.mu.Unlock()
			log.Printf("[master-ws] Cliente desconectado.")
		}()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
	}(conn)
}

// startMetricsBroadcaster lanza un loop que difunde métricas cada 1.5s
func (api *APIServer) startMetricsBroadcaster() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	go func() {
		for range ticker.C {
			api.mu.Lock()
			clientsCount := len(api.wsClients)
			if clientsCount == 0 {
				api.mu.Unlock()
				continue
			}

			// Calcular latencia promedio
			var avgLatency float64
			reqs := atomic.LoadInt64(&api.totalRequests)
			lat := atomic.LoadInt64(&api.totalLatencyNs)
			if reqs > 0 {
				avgLatency = float64(lat) / float64(reqs) / 1e6 // Convertir a milisegundos
			}

			// Simular telemetría dinámica de nodos Workers para visualización en el dashboard
			cpuUsage := make([]float64, len(api.workerAddrs))
			for i := range cpuUsage {
				cpuUsage[i] = 15.0 + math.Sin(float64(time.Now().Unix()+int64(i)))*8.0 + float64(time.Now().UnixNano()%10)
			}

			metrics := struct {
				TotalRequests  int64     `json:"totalRequests"`
				AverageLatency float64   `json:"averageLatencyMs"`
				ConnectedNodes int       `json:"connectedNodes"`
				NodeStatuses   []string  `json:"nodeStatuses"`
				CPUUsage       []float64 `json:"cpuUsage"`
				Timestamp      time.Time `json:"timestamp"`
			}{
				TotalRequests:  reqs,
				AverageLatency: avgLatency,
				ConnectedNodes: len(api.clients),
				NodeStatuses:   api.workerAddrs,
				CPUUsage:       cpuUsage,
				Timestamp:      time.Now(),
			}
			api.mu.Unlock()

			data, err := json.Marshal(metrics)
			if err != nil {
				continue
			}

			// Broadcast
			api.mu.Lock()
			for conn := range api.wsClients {
				err := conn.WriteMessage(websocket.TextMessage, data)
				if err != nil {
					conn.Close()
					delete(api.wsClients, conn)
				}
			}
			api.mu.Unlock()
		}
	}()
}
