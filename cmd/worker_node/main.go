// Package main implementa el nodo Worker del clúster de ENTRENAMIENTO
// DISTRIBUIDO. Levanta un servidor RPC sobre TCP que:
//
//   - LoadShard:       recibe su partición del dataset (ya featurizada y
//                      estandarizada) y la conserva en memoria.
//   - ComputeGradient: dado un vector de pesos, calcula el gradiente
//                      PARCIAL del modelo sobre su shard usando fan-out de
//                      goroutines (concurrencia intra-nodo).
//   - SetModel/Predict: tras el entrenamiento, atiende inferencia con los
//                       modelos ya entrenados.
//
// El Master agrega los gradientes parciales de todos los Workers por época
// (data-parallel SGD / parameter server). Expone pprof para auditar CPU,
// memoria y contención de mutex.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/rpc"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

// WorkerService es el servicio RPC expuesto por el nodo. Conserva el
// estado del shard y el bundle de modelos entrenados entre llamadas.
type WorkerService struct {
	ID             int
	shard          atomic.Pointer[ml.ShardState]
	trained        atomic.Pointer[ml.TrainedModels]
	gradientsServed int64
}

// LoadShard almacena la partición del dataset asignada a este worker.
func (ws *WorkerService) LoadShard(args *types.LoadShardArgs, reply *types.LoadShardReply) error {
	state := &ml.ShardState{X: args.X, YMort: args.YMortality, YSurv: args.YSurvival, YCost: args.YCost}
	ws.shard.Store(state)
	reply.N = len(state.X)
	log.Printf("[worker-%d] Shard cargado: %d ejemplos, %d características", ws.ID, len(state.X), ml.NumFeatures())
	return nil
}

// ComputeGradient calcula el gradiente parcial del modelo `Kind` sobre el
// shard local con los pesos recibidos. Concurrencia intra-nodo: fan-out de
// runtime.NumCPU() goroutines.
func (ws *WorkerService) ComputeGradient(args *types.GradientArgs, reply *types.GradientReply) error {
	state := ws.shard.Load()
	if state == nil {
		reply.N = 0
		return nil
	}
	g := state.PartialGradient(ml.KindFromString(args.Kind), args.Weights, runtime.NumCPU())
	reply.GradSum, reply.Loss, reply.N = g.Sum, g.Loss, g.N
	atomic.AddInt64(&ws.gradientsServed, 1)
	return nil
}

// SetModel recibe del Master los modelos entrenados para inferencia.
func (ws *WorkerService) SetModel(args *types.ModelBundle, reply *types.LoadShardReply) error {
	ws.trained.Store(ml.TrainedFromBundle(*args))
	reply.N = 1
	log.Printf("[worker-%d] Modelos entrenados recibidos; listo para inferencia", ws.ID)
	return nil
}

// ProcessBatch atiende inferencia distribuida: aplica los modelos
// entrenados a un lote de pacientes usando fan-out de goroutines.
func (ws *WorkerService) ProcessBatch(args *types.ProcessArgs, reply *types.ProcessReply) error {
	start := time.Now()
	tm := ws.trained.Load()
	if tm == nil {
		reply.Results = []types.PatientResult{}
		reply.Stats = types.WorkerStats{WorkerID: ws.ID}
		return nil
	}
	n := len(args.Patients)
	results := make([]types.PatientResult, n)

	workers := runtime.NumCPU()
	chunk := (n + workers - 1) / workers
	if chunk == 0 {
		chunk = 1
	}
	done := make(chan struct{}, workers)
	launched := 0
	for w := 0; w < workers; w++ {
		s := w * chunk
		if s >= n {
			break
		}
		e := s + chunk
		if e > n {
			e = n
		}
		launched++
		go func(s, e int) {
			for i := s; i < e; i++ {
				results[i] = tm.PredictPatient(args.Patients[i], ws.ID)
			}
			done <- struct{}{}
		}(s, e)
	}
	for i := 0; i < launched; i++ {
		<-done
	}

	reply.Results = results
	reply.Stats = types.WorkerStats{WorkerID: ws.ID, PatientsHandled: n, ProcessingTime: time.Since(start)}
	return nil
}

func main() {
	addr := flag.String("addr", "localhost:8081", "Dirección TCP del servidor RPC")
	pprofAddr := flag.String("pprof", "localhost:6061", "Dirección del servidor pprof")
	workerID := flag.Int("id", 1, "Identificador único del nodo worker")
	flag.Parse()

	go func() {
		log.Printf("[worker-%d] pprof activo en http://%s/debug/pprof/", *workerID, *pprofAddr)
		if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
			log.Printf("[worker-%d] pprof error: %v", *workerID, err)
		}
	}()

	ws := &WorkerService{ID: *workerID}
	if err := rpc.Register(ws); err != nil {
		log.Fatalf("[worker-%d] error registrando RPC: %v", *workerID, err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("[worker-%d] error escuchando en %s: %v", *workerID, *addr, err)
	}
	defer listener.Close()
	log.Printf("[worker-%d] Servidor TCP/RPC de entrenamiento escuchando en %s", *workerID, *addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[worker-%d] error aceptando conexión: %v", *workerID, err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
