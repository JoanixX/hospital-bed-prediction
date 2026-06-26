// Package middleware contiene los middlewares de Gin reutilizables:
// autenticación JWT y caché Redis.
package middleware

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/JoanixX/hospital-bed-prediction/internal/api/dto"
)

// jwtSecret lee la clave secreta de la variable de entorno JWT_SECRET.
// En producción, sustituir por un secret manager.
func jwtSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("hospital-bed-dev-secret-change-in-prod")
}

// Claims define el payload del token JWT.
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken crea un JWT firmado con HS256 válido por 24 h.
// Llamar desde el handler de /login (fuera de esta PC4, pero incluido
// para que el sistema sea completo y testeable con hey/JMeter).
func GenerateToken(userID, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "hospital-bed-api",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

// RequireJWT es el middleware Gin que valida el token Bearer en cada
// petición. Si el token es válido, inyecta los claims en el contexto
// bajo la clave "claims" para que los handlers los consuman.
//
// Header esperado:
//
//	Authorization: Bearer <token>
func RequireJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{
				Error: "authorization header missing",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{
				Error: "authorization header format must be: Bearer <token>",
			})
			return
		}

		tokenStr := parts[1]
		claims := &Claims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			// Verificar que el algoritmo sea el esperado (evita alg confusion attacks)
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret(), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{
				Error:   "invalid or expired token",
				Details: err.Error(),
			})
			return
		}

		// Inyectar claims para uso en handlers
		c.Set("claims", claims)
		c.Next()
	}
}
