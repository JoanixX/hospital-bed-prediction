package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JoanixX/hospital-bed-prediction/internal/api/dto"
	"github.com/JoanixX/hospital-bed-prediction/internal/api/middleware"
)

// startTime registra cuándo arrancó el servidor para calcular uptime.
var startTime = time.Now()

// Stats maneja GET /stats.
// Devuelve métricas agregadas del pipeline y de la caché Redis.
// No requiere autenticación para facilitar el monitoreo externo.
//
//	GET /stats
//
//	200 OK
//	{
//	  "total_processed": 1042,
//	  "cache_hits": 318,
//	  "cache_misses": 724,
//	  "avg_mortality_risk": 0.34,
//	  "avg_survival_days": 2150.7,
//	  "avg_treatment_cost": 48320.5,
//	  "uptime_seconds": 3600.0
//	}
func Stats(c *gin.Context) {
	total, mort, surv, cost := GetTotals()

	avg := func(sum float64) float64 {
		if total == 0 {
			return 0
		}
		return sum / float64(total)
	}

	c.JSON(http.StatusOK, dto.StatsResponse{
		TotalProcessed: int(total),
		CacheHits:      middleware.CacheHits.Load(),
		CacheMisses:    middleware.CacheMisses.Load(),
		AvgMortality:   avg(mort),
		AvgSurvival:    avg(surv),
		AvgCost:        avg(cost),
		UptimeSeconds:  time.Since(startTime).Seconds(),
	})
}

// ─────────────────────────────────────────────────────────────────────────────
//  /login  —  endpoint auxiliar para obtener un JWT de prueba.
//  En un sistema real se validaría contra una BD; aquí se usan credenciales
//  hardcodeadas para que los scripts de carga (hey/JMeter) puedan obtener
//  un token sin infraestructura extra.
// ─────────────────────────────────────────────────────────────────────────────

// LoginRequest contiene las credenciales del cliente.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse devuelve el token JWT.
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"` // segundos
}

// Login maneja POST /login (sin autenticación previa).
//
//	POST /login
//	{ "username": "admin", "password": "hospital2024" }
//
//	200 OK
//	{ "token": "eyJ...", "expires_in": 86400 }
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:   "invalid request body",
			Details: err.Error(),
		})
		return
	}

	// Validación simple (sustituir por consulta a BD en PC4 final)
	validUsers := map[string]string{
		"admin": "hospital2024",
		"demo":  "demo123",
	}
	expectedPass, ok := validUsers[req.Username]
	if !ok || req.Password != expectedPass {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{
			Error: "invalid credentials",
		})
		return
	}

	role := "user"
	if req.Username == "admin" {
		role = "admin"
	}

	token, err := middleware.GenerateToken(req.Username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error: "could not generate token",
		})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:     token,
		ExpiresIn: 86400,
	})
}
