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
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/JoanixX/hospital-bed-prediction/internal/api/handlers"
	"github.com/JoanixX/hospital-bed-prediction/internal/api/middleware"
)

func main() {
	addr      := flag.String("addr", ":8080", "dirección donde escucha la API")
	redisAddr := flag.String("redis", "localhost:6379", "host:port de Redis")
	ginMode   := flag.String("mode", "debug", "modo Gin: debug | release")
	flag.Parse()

	// Variables de entorno tienen prioridad sobre flags
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		*redisAddr = v
	}

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
				rawBodyReader(),                 // guarda el body crudo en el contexto
				middleware.RedisCache(rdb),      // intenta hit; pasa si miss
				handlers.Predict,                // procesa y cachea la respuesta
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

// rawBodyReader es un middleware ligero que lee el body completo y
// lo almacena en el contexto de Gin bajo la clave "rawBody".
// Permite que RedisCache genere la clave de caché sin consumir el body,
// y que ShouldBindJSON en el handler lo lea de nuevo desde el mismo buffer.
func rawBodyReader() gin.HandlerFunc {
	return func(c *gin.Context) {
		var raw []byte
		if c.Request.Body != nil {
			// Leer manualmente sin cerrar el body
			buf := make([]byte, 0, 512)
			tmp := make([]byte, 128)
			for {
				n, err := c.Request.Body.Read(tmp)
				buf = append(buf, tmp[:n]...)
				if err != nil {
					break
				}
			}
			raw = buf
			// Restaurar el body para que ShouldBindJSON funcione en el handler
			c.Request.Body = noopCloser{reader: raw}
		}
		c.Set("rawBody", raw)
		c.Next()
	}
}

// noopCloser implementa io.ReadCloser sobre un slice de bytes.
type noopCloser struct {
	reader []byte
	pos    int
}

func (n noopCloser) Read(p []byte) (int, error) {
	if n.pos >= len(n.reader) {
		return 0, fmt.Errorf("EOF")
	}
	copied := copy(p, n.reader[n.pos:])
	n.pos += copied
	return copied, nil
}
func (noopCloser) Close() error { return nil }
