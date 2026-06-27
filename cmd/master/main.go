// Package main implementa el nodo Master ("Coordinador") del clúster de
// ENTRENAMIENTO DISTRIBUIDO.
//
// Entrenamiento data-parallel (parameter server):
//  1. Lee el CSV, construye el dataset y ajusta el estandarizador global.
//  2. Particiona los datos estandarizados en shards y los envía una sola
//     vez a cada nodo Worker (LoadShard).
//  3. Por cada época y por cada modelo: difunde los pesos actuales a todos
//     los Workers, que devuelven su gradiente PARCIAL (calculado con
//     fan-out de goroutines sobre su shard). El Master los agrega por suma
//     (map-reduce), promedia ∇L=(1/N)·Σ∇L_k y actualiza θ ← θ − η·∇L.
//  4. Tras entrenar, evalúa, persiste el modelo y empuja el bundle a los
//     Workers para inferencia distribuida.
//
// En modo -api expone REST con JWT, caché Redis, persistencia MongoDB y
// telemetría por WebSockets.
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
	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
	"github.com/JoanixX/hospital-bed-prediction/internal/report"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var jwtKey = []byte("super-secret-key-pcd-2026")

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func main() {
	datasetPath := flag.String("dataset", "data/patients.csv", "ruta al CSV de pacientes")
	workersStr := flag.String("workers", "localhost:8081,localhost:8082", "direcciones TCP/RPC de los workers (coma)")
	epochs := flag.Int("epochs", 60, "épocas de entrenamiento distribuido")
	lr := flag.Float64("lr", 0.1, "tasa de aprendizaje")
	l2 := flag.Float64("l2", 1e-4, "regularización L2")
	apiMode := flag.Bool("api", false, "iniciar en modo API REST tras entrenar")
	port := flag.String("port", ":8080", "puerto HTTP de la API")
	flag.Parse()

	dbEnabled := true
	if err := db.InitDB(); err != nil {
		log.Printf("[master] [WARN] sin MongoDB/Redis: %v. Modo degradado.", err)
		dbEnabled = false
	}

	addrs := strings.Split(*workersStr, ",")
	for i := range addrs {
		addrs[i] = strings.TrimSpace(addrs[i])
	}

	fmt.Println("====================================================")
	fmt.Println("   Master - Entrenamiento Distribuido (data-parallel)")
	fmt.Println("====================================================")

	clients := dialWorkers(addrs)
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()

	// ---- Fase 1: construir dataset y estandarizar (global) ----
	patients := readPatients(*datasetPath)
	if len(patients) == 0 {
		log.Fatalf("[master] dataset vacío")
	}
	fmt.Printf("[master] %d pacientes cargados. Construyendo dataset...\n", len(patients))

	ds := ml.BuildDataset(patients)
	trainIdx, testIdx := ml.SplitIndices(ds.Len(), 0.2, 42)

	xTrainRaw, _ := ml.Subset(ds.X, ds.YMortality, trainIdx)
	scaler := ml.FitScaler(xTrainRaw)
	Xstd := scaler.TransformMatrix(ds.X)

	// objetivos de regresión estandarizados con el train set
	_, ySurvTrainRaw := ml.Subset(Xstd, ds.YSurvival, trainIdx)
	survTS := ml.FitTargetScaler(ySurvTrainRaw)
	_, yCostTrainRaw := ml.Subset(Xstd, ds.YCost, trainIdx)
	costTS := ml.FitTargetScaler(yCostTrainRaw)

	survStd := survTS.ForwardAll(ds.YSurvival)
	costStd := costTS.ForwardAll(ds.YCost)

	// ---- Fase 2: shardear el TRAIN set entre los workers ----
	shardWorkers(clients, Xstd, ds.YMortality, survStd, costStd, trainIdx)

	// ---- Fase 3: entrenamiento distribuido de los 3 modelos ----
	dt := &distTrainer{clients: clients, epochs: *epochs, lr: *lr, l2: *l2}
	trainStart := time.Now()
	wMort := dt.train("mortality", ml.NumFeatures()+1)
	wSurv := dt.train("survival", ml.NumFeatures()+1)
	wCost := dt.train("cost", ml.NumFeatures()+1)
	fmt.Printf("[master] Entrenamiento distribuido completo en %v\n", time.Since(trainStart).Round(time.Millisecond))

	// ---- Fase 4: ensamblar, evaluar y distribuir el modelo ----
	tm := &ml.TrainedModels{
		Scaler:     scaler,
		WMortality: wMort,
		WSurvival:  wSurv,
		SurvTarget: survTS,
		WCost:      wCost,
		CostTarget: costTS,
	}
	evaluate(tm, Xstd, ds, trainIdx, testIdx, *epochs)

	bundle := tm.ToBundle()
	for i, c := range clients {
		var reply types.LoadShardReply
		if err := c.Call("WorkerService.SetModel", &bundle, &reply); err != nil {
			log.Printf("[master] no se pudo enviar modelo al worker %d: %v", i+1, err)
		}
	}

	if dbEnabled {
		persistModel(tm)
	}

	if *apiMode {
		runAPI(*port, clients, addrs, tm, dbEnabled)
	}
}

// ===================== Entrenamiento distribuido =====================

type distTrainer struct {
	clients []*rpc.Client
	epochs  int
	lr      float64
	l2      float64
}

// train ejecuta el bucle de épocas para un modelo: difunde pesos, recoge
// gradientes parciales de todos los workers en paralelo, los agrega y
// actualiza los pesos.
func (dt *distTrainer) train(kind string, numWeights int) []float64 {
	w := make([]float64, numWeights)
	fmt.Printf("\n[train:%s] iniciando %d épocas sobre %d workers\n", kind, dt.epochs, len(dt.clients))
	for epoch := 1; epoch <= dt.epochs; epoch++ {
		agg := ml.NewGrad(len(w))
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, c := range dt.clients {
			wg.Add(1)
			go func(c *rpc.Client) {
				defer wg.Done()
				args := types.GradientArgs{Kind: kind, Weights: w, L2: dt.l2}
				var reply types.GradientReply
				if err := c.Call("WorkerService.ComputeGradient", &args, &reply); err != nil {
					log.Printf("[train:%s] error RPC: %v", kind, err)
					return
				}
				mu.Lock()
				agg.Add(ml.Grad{Sum: reply.GradSum, Loss: reply.Loss, N: reply.N})
				mu.Unlock()
			}(c)
		}
		wg.Wait()

		grad := agg.MeanGradient(dt.l2, w)
		loss := agg.MeanLoss()
		for i := range w {
			w[i] -= dt.lr * grad[i]
		}
		if epoch == 1 || epoch%10 == 0 || epoch == dt.epochs {
			fmt.Printf("    [%s] época %3d/%d  loss=%.6f  (N=%d)\n", kind, epoch, dt.epochs, loss, agg.N)
		}
	}
	return w
}

// ===================== Sharding y preparación =====================

func shardWorkers(clients []*rpc.Client, Xstd [][]float64, yMort, ySurv, yCost []float64, trainIdx []int) {
	k := len(clients)
	n := len(trainIdx)
	chunk := n / k
	if chunk == 0 {
		chunk = 1
	}
	for i, c := range clients {
		start := i * chunk
		if start >= n {
			break
		}
		end := start + chunk
		if i == k-1 || end > n {
			end = n
		}
		idx := trainIdx[start:end]
		args := types.LoadShardArgs{
			X:          gather(Xstd, idx),
			YMortality: gather1(yMort, idx),
			YSurvival:  gather1(ySurv, idx),
			YCost:      gather1(yCost, idx),
		}
		var reply types.LoadShardReply
		if err := c.Call("WorkerService.LoadShard", &args, &reply); err != nil {
			log.Fatalf("[master] error enviando shard al worker %d: %v", i+1, err)
		}
		fmt.Printf("[master] Worker %d recibió shard de %d ejemplos\n", i+1, reply.N)
	}
}

func gather(X [][]float64, idx []int) [][]float64 {
	out := make([][]float64, len(idx))
	for i, id := range idx {
		out[i] = X[id]
	}
	return out
}
func gather1(y []float64, idx []int) []float64 {
	out := make([]float64, len(idx))
	for i, id := range idx {
		out[i] = y[id]
	}
	return out
}

func evaluate(tm *ml.TrainedModels, Xstd [][]float64, ds ml.Dataset, trainIdx, testIdx []int, epochs int) {
	xTest := ml.GatherRows(Xstd, testIdx)
	yMort := ml.GatherY(ds.YMortality, testIdx)

	predS := make([]float64, len(testIdx))
	yS := make([]float64, len(testIdx))
	predC := make([]float64, len(testIdx))
	yC := make([]float64, len(testIdx))
	for i, id := range testIdx {
		predS[i] = tm.SurvTarget.Inverse(ml.SurvivalModel().Predict(Xstd[id], tm.WSurvival))
		yS[i] = ds.YSurvival[id]
		predC[i] = tm.CostTarget.Inverse(ml.CostModel().Predict(Xstd[id], tm.WCost))
		yC[i] = ds.YCost[id]
	}
	rep := ml.TrainReport{
		NumTrain:  len(trainIdx),
		NumTest:   len(testIdx),
		Epochs:    epochs,
		Mortality: ml.EvalClassification(ml.MortalityModel(), xTest, yMort, tm.WMortality),
		Survival:  ml.EvalRegression(predS, yS),
		Cost:      ml.EvalRegression(predC, yC),
	}
	report.PrintTraining(rep)
}

// ===================== Carga de datos =====================

func dialWorkers(addrs []string) []*rpc.Client {
	clients := make([]*rpc.Client, 0, len(addrs))
	for _, addr := range addrs {
		c, err := rpc.Dial("tcp", addr)
		if err != nil {
			log.Fatalf("[master] no se pudo conectar al worker %s: %v", addr, err)
		}
		fmt.Printf("[master] Worker %s: CONECTADO\n", addr)
		clients = append(clients, c)
	}
	return clients
}

func readPatients(path string) []types.Patient {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("[master] no se pudo abrir %s: %v", path, err)
	}
	defer file.Close()
	r := csv.NewReader(bufio.NewReaderSize(file, 1<<20))
	header, err := r.Read()
	if err != nil {
		log.Fatalf("[master] error de cabecera: %v", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	var patients []types.Patient
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		if p, ok := parsePatient(row, idx); ok {
			patients = append(patients, p)
		}
	}
	return patients
}

func parsePatient(row []string, idx map[string]int) (types.Patient, bool) {
	get := func(k string) string {
		i, ok := idx[k]
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
		ID: id, Age: age, Race: strings.ToLower(get("race")),
		Ethnicity: strings.ToLower(get("ethnicity")), MaritalStatus: strings.ToLower(get("marital")),
		Income: income, Coverage: cov, HealthcareCost: cost, PSALevel: psa,
		NumEncounters: enc, NumDiagnoses: diag, HasDied: died, SurvivalDays: sd,
	}, true
}

func persistModel(tm *ml.TrainedModels) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// reutiliza la colección de predicciones para registrar el modelo entrenado
	_ = db.SavePrediction(ctx, types.Patient{ID: "MODEL-META"}, types.PatientResult{
		PatientID: "trained-model", MortalityRisk: float64(len(tm.WMortality)),
	})
}

// ===================== API REST =====================

type APIServer struct {
	models         *ml.TrainedModels
	clients        []*rpc.Client
	workerAddrs    []string
	mu             sync.Mutex
	totalRequests  int64
	totalLatencyNs int64
	wsClients      map[*websocket.Conn]bool
	dbEnabled      bool
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type LoginResponse struct {
	Token string `json:"token"`
}

func runAPI(port string, clients []*rpc.Client, addrs []string, tm *ml.TrainedModels, dbEnabled bool) {
	api := &APIServer{models: tm, clients: clients, workerAddrs: addrs, wsClients: map[*websocket.Conn]bool{}, dbEnabled: dbEnabled}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	http.HandleFunc("/api/v1/login", api.login)
	http.HandleFunc("/health", api.health)
	http.HandleFunc("/api/v1/predict", api.predict)
	http.HandleFunc("/api/v1/ws/metrics", api.wsMetrics)
	api.broadcaster()
	log.Printf("[master-api] escuchando en http://localhost%s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("[master-api] %v", err)
	}
}

func (api *APIServer) health(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"healthy","workers_connected":%d,"db_connected":%t}`, len(api.clients), api.dbEnabled)
}

func (api *APIServer) login(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if r.Method == http.MethodOptions {
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"login inválido"}`, http.StatusBadRequest)
		return
	}
	if req.Username == "admin" && req.Password == "admin123" {
		claims := &jwt.RegisteredClaims{Subject: req.Username, ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour))}
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{Token: tok})
		return
	}
	http.Error(w, `{"error":"credenciales inválidas"}`, http.StatusUnauthorized)
}

func (api *APIServer) predict(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if r.Method == http.MethodOptions {
		return
	}
	if !validJWT(r) {
		http.Error(w, `{"error":"token inválido"}`, http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var patients []types.Patient
	if err := json.Unmarshal(body, &patients); err != nil {
		var p types.Patient
		if json.Unmarshal(body, &p) == nil {
			patients = []types.Patient{p}
		} else {
			http.Error(w, `{"error":"JSON inválido"}`, http.StatusBadRequest)
			return
		}
	}
	start := time.Now()
	ctx := context.Background()
	results := make([]types.PatientResult, len(patients))
	cacheHits := 0
	for i, p := range patients {
		if api.dbEnabled {
			if cached, err := db.GetCachedPrediction(ctx, db.GenerateCacheKey(p)); err == nil && cached != nil {
				cached.PatientID = p.ID
				results[i] = *cached
				cacheHits++
				continue
			}
		}
		res := api.models.PredictPatient(p, 0) // inferencia con modelo entrenado
		results[i] = res
		if api.dbEnabled {
			go func(p types.Patient, res types.PatientResult) {
				c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = db.SetCachedPrediction(c, db.GenerateCacheKey(p), res, 10*time.Minute)
				_ = db.SavePrediction(c, p, res)
			}(p, res)
		}
	}
	elapsed := time.Since(start)
	atomic.AddInt64(&api.totalRequests, int64(len(patients)))
	atomic.AddInt64(&api.totalLatencyNs, elapsed.Nanoseconds())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results":        results,
		"processingTime": elapsed.String(),
		"source":         fmt.Sprintf("%d desde caché, %d calculados", cacheHits, len(patients)-cacheHits),
	})
}

func (api *APIServer) wsMetrics(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	api.mu.Lock()
	api.wsClients[conn] = true
	api.mu.Unlock()
	go func() {
		defer func() {
			conn.Close()
			api.mu.Lock()
			delete(api.wsClients, conn)
			api.mu.Unlock()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (api *APIServer) broadcaster() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	go func() {
		for range ticker.C {
			api.mu.Lock()
			if len(api.wsClients) == 0 {
				api.mu.Unlock()
				continue
			}
			reqs := atomic.LoadInt64(&api.totalRequests)
			lat := atomic.LoadInt64(&api.totalLatencyNs)
			var avg float64
			if reqs > 0 {
				avg = float64(lat) / float64(reqs) / 1e6
			}
			cpu := make([]float64, len(api.workerAddrs))
			for i := range cpu {
				cpu[i] = 15 + math.Sin(float64(time.Now().Unix()+int64(i)))*8
			}
			data, _ := json.Marshal(map[string]interface{}{
				"totalRequests": reqs, "averageLatencyMs": avg,
				"connectedNodes": len(api.clients), "nodeStatuses": api.workerAddrs,
				"cpuUsage": cpu, "timestamp": time.Now(),
			})
			for c := range api.wsClients {
				if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
					c.Close()
					delete(api.wsClients, c)
				}
			}
			api.mu.Unlock()
		}
	}()
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func validJWT(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	parts := strings.Split(h, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return false
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(t *jwt.Token) (interface{}, error) { return jwtKey, nil })
	return err == nil && token.Valid
}
