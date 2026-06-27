// cmd/api/main.go — Servidor HTTP de la API REST del sistema de predicción
// de cáncer de próstata.  Usa Gin como framework HTTP, Go-Redis como caché
// y golang-jwt para autenticación.
//
// Uso:
//
//	go run ./cmd/api -addr=:8080 -redis=localhost:6379
//	JWT_SECRET=mi-secreto go run ./cmd/api
//
// Variables de entorno:
//
//	JWT_SECRET   clave HMAC para firmar tokens (default: dev-secret)
//	REDIS_ADDR   host:port de Redis   (override de -redis flag)
//	REDIS_PASS   contraseña de Redis  (vacío por defecto)
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/JoanixX/hospital-bed-prediction/internal/api/handlers"
	"github.com/JoanixX/hospital-bed-prediction/internal/api/middleware"
	"github.com/JoanixX/hospital-bed-prediction/internal/loader"
	"github.com/JoanixX/hospital-bed-prediction/internal/ml"
	"github.com/JoanixX/hospital-bed-prediction/internal/types"
)

func main() {
	addr := flag.String("addr", ":8080", "dirección donde escucha la API")
	redisAddr := flag.String("redis", "localhost:6379", "host:port de Redis")
	ginMode := flag.String("mode", "debug", "modo Gin: debug | release")
	dataset := flag.String("dataset", "", "CSV de pacientes para entrenar al arrancar (vacío => sintético)")
	synthetic := flag.Int("synthetic", 50_000, "tamaño del dataset sintético si dataset=''")
	epochs := flag.Int("epochs", 60, "épocas de entrenamiento al arrancar")
	flag.Parse()

	// Variables de entorno tienen prioridad sobre flags
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		*redisAddr = v
	}
	if v := os.Getenv("DATASET"); v != "" {
		*dataset = v
	}

	// ── Entrenamiento de los modelos ML al arrancar ───────────────────────
	// La API no puede inferir sin modelos entrenados: los entrena una sola
	// vez (descenso de gradiente paralelo con goroutines) y los inyecta a
	// los handlers. Esto reemplaza las antiguas heurísticas escritas a mano.
	trainModelsAtStartup(*dataset, *synthetic, *epochs)

	// ── Gin ──────────────────────────────────────────────────────────────
	gin.SetMode(*ginMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// ── Redis ─────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: os.Getenv("REDIS_PASS"),
		DB:       0,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[redis] advertencia: no se pudo conectar a %s: %v", *redisAddr, err)
		log.Println("[redis] la API seguirá funcionando sin caché")
		rdb = nil // nil seguro; el middleware lo comprueba
	} else {
		log.Printf("[redis] conectado a %s", *redisAddr)
	}

	// ── Rutas públicas ────────────────────────────────────────────────────
	router.POST("/login", handlers.Login)
	router.GET("/stats", handlers.Stats)

	// ── Rutas protegidas con JWT ──────────────────────────────────────────
	protected := router.Group("/")
	protected.Use(middleware.RequireJWT())
	{
		// Si Redis está disponible, añadir caché antes del handler
		predictHandlers := gin.HandlersChain{handlers.Predict}
		if rdb != nil {
			predictHandlers = gin.HandlersChain{
				rawBodyReader(),            // guarda el body crudo en el contexto
				middleware.RedisCache(rdb), // intenta hit; pasa si miss
				handlers.Predict,           // procesa y cachea la respuesta
			}
		}
		protected.POST("/predict", predictHandlers...)
	}

	// ── Servidor HTTP con graceful shutdown ───────────────────────────────
	srv := &http.Server{
		Addr:         *addr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Arrancar en goroutine para poder capturar señales
	go func() {
		fmt.Println("════════════════════════════════════════════")
		fmt.Println("  Hospital Bed Prediction — API REST")
		fmt.Printf("  Escuchando en http://localhost%s\n", *addr)
		fmt.Println("  POST /login     → obtener JWT")
		fmt.Println("  POST /predict   → predicción (JWT requerido)")
		fmt.Println("  GET  /stats     → métricas del sistema")
		fmt.Println("════════════════════════════════════════════")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[api] error fatal: %v", err)
		}
	}()

	// Graceful shutdown al recibir SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[api] apagando servidor...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Fatalf("[api] shutdown forzado: %v", err)
	}
	log.Println("[api] servidor detenido correctamente")
}

// trainModelsAtStartup entrena los tres modelos de ML una sola vez al
// iniciar la API y los inyecta a los handlers de predicción. Si se indica
// un CSV lo usa; en caso contrario genera un dataset sintético con señal
// real para que los modelos converjan.
func trainModelsAtStartup(datasetPath string, syntheticN, epochs int) {
	numWorkers := runtime.NumCPU()

	var patients []types.Patient
	if datasetPath != "" {
		log.Printf("[api] entrenando con dataset %s (%d workers)...", datasetPath, numWorkers)
		ps, discarded, err := loader.LoadConcurrent(loader.LoadConfig{
			Path: datasetPath, NumWorkers: numWorkers, BufferSize: 1024,
		})
		if err != nil {
			log.Printf("[api] no se pudo cargar %s (%v); usando datos sintéticos", datasetPath, err)
		} else {
			patients = ps
			log.Printf("[api] %d pacientes válidos, %d descartados", len(patients), discarded)
		}
	}
	if len(patients) == 0 {
		log.Printf("[api] generando %d pacientes sintéticos para entrenar...", syntheticN)
		patients = generateSyntheticPatients(syntheticN)
	}

	cfg := ml.TrainConfig{Epochs: epochs, LR: 0.5, L2: 1e-4, NumWorkers: numWorkers}
	start := time.Now()
	models, rep := ml.TrainAll(patients, cfg, 0.2)
	handlers.SetModels(models)
	log.Printf("[api] modelos entrenados en %v | mortalidad AUC=%.3f | supervivencia R²=%.3f | costo R²=%.3f",
		time.Since(start).Round(time.Millisecond), rep.Mortality.AUC, rep.Survival.R2, rep.Cost.R2)
}

// generateSyntheticPatients produce datos con señal real (no ruido puro)
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

		z := -6.0 + 0.05*float64(age) + 0.12*psa + 0.20*float64(diag) - 0.00002*income
		pDie := 1.0 / (1.0 + math.Exp(-z))
		died := r.Float64() < pDie

		surv := 4200 - 18*float64(age) - 90*psa + 1500*coverage + r.NormFloat64()*200
		if surv < 90 {
			surv = 90
		}
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

// rawBodyReader es un middleware ligero que lee el body completo y
// lo almacena en el contexto de Gin bajo la clave "rawBody".
// Permite que RedisCache genere la clave de caché sin consumir el body,
// y que ShouldBindJSON en el handler lo lea de nuevo desde el mismo buffer.
func rawBodyReader() gin.HandlerFunc {
	return func(c *gin.Context) {
		var raw []byte
		if c.Request.Body != nil {
			var err error
			raw, err = io.ReadAll(c.Request.Body)
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewReader(raw))
			}
		}
		c.Set("rawBody", raw)
		c.Next()
	}
}
