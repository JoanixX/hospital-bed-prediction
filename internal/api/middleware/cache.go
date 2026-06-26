package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/JoanixX/hospital-bed-prediction/internal/api/dto"
)

const cacheTTL = 5 * time.Minute

// Contadores globales de hits/misses para el endpoint /stats.
var (
	CacheHits   atomic.Int64
	CacheMisses atomic.Int64
)

// RedisCache devuelve un middleware Gin que:
//  1. Genera una clave SHA-256 del body JSON de la petición.
//  2. Consulta Redis; si hay hit, responde 200 con Cached=true.
//  3. Si hay miss, deja pasar al handler y cachea la respuesta resultante.
//
// Solo aplica a rutas POST que devuelven dto.PredictResponse.
func RedisCache(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Leer el body para generar la clave (Gin permite leerlo varias veces
		// mediante c.Request.Body; usamos ShouldBindJSON en el handler, así que
		// necesitamos un body copiado — usamos el raw guardado por Gin).
		rawBody, exists := c.Get("rawBody")
		if !exists {
			// Sin rawBody no podemos cachear; dejar pasar sin error.
			c.Next()
			return
		}

		cacheKey := cacheKeyFromBody(rawBody.([]byte))
		ctx := context.Background()

		// ── Cache HIT ─────────────────────────────────────────────────────
		if val, err := rdb.Get(ctx, cacheKey).Result(); err == nil {
			CacheHits.Add(1)
			var cached dto.PredictResponse
			if jsonErr := json.Unmarshal([]byte(val), &cached); jsonErr == nil {
				cached.Cached = true
				c.AbortWithStatusJSON(http.StatusOK, cached)
				return
			}
		}

		// ── Cache MISS ────────────────────────────────────────────────────
		CacheMisses.Add(1)

		// Usamos un responseWriter personalizado para capturar lo que
		// escriba el handler y almacenarlo en Redis.
		rw := &responseCapture{ResponseWriter: c.Writer, body: []byte{}}
		c.Writer = rw

		c.Next()

		// Solo cachear respuestas 200 OK
		if rw.status == http.StatusOK || rw.status == 0 {
			_ = rdb.Set(ctx, cacheKey, rw.body, cacheTTL).Err()
		}
	}
}

// BodyReader es un middleware previo que lee y guarda el body en el
// contexto para que RedisCache pueda acceder a él sin consumirlo.
// Debe registrarse ANTES de RedisCache en la cadena de middlewares.
func BodyReader() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		body := make([]byte, 0)
		if err := c.ShouldBindBodyWithJSON(&body); err == nil {
			c.Set("rawBody", body)
		}
		c.Next()
	}
}

// cacheKeyFromBody genera una clave Redis determinista a partir del
// contenido del body. SHA-256 evita colisiones en datasets grandes.
func cacheKeyFromBody(body []byte) string {
	h := sha256.Sum256(body)
	return fmt.Sprintf("predict:%x", h)
}

// responseCapture envuelve el ResponseWriter de Gin para capturar
// el body escrito por el handler sin modificar la respuesta al cliente.
type responseCapture struct {
	gin.ResponseWriter
	status int
	body   []byte
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
