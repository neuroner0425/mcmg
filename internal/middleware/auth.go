package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
	CookieKey = "mc_token"
)

// Claims represents custom JWT claims with nickname and role.
type Claims struct {
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT token for the specified nickname and role.
func GenerateToken(nickname, role, secret string, duration time.Duration) (string, error) {
	claims := Claims{
		Nickname: nickname,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken validates and extracts claims from a raw JWT token string.
func ParseToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// RequireRole enforces role-based access control.
func RequireRole(secret string, minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""

		if cookie, err := c.Cookie(CookieKey); err == nil && cookie != "" {
			tokenStr = cookie
		}

		if tokenStr == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		isAPI := strings.HasPrefix(c.Request.URL.Path, "/api")

		if tokenStr == "" {
			if isAPI {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			} else {
				c.Redirect(http.StatusFound, "/login")
				c.Abort()
			}
			return
		}

		claims, err := ParseToken(tokenStr, secret)
		if err != nil {
			if isAPI {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			} else {
				c.Redirect(http.StatusFound, "/login")
				c.Abort()
			}
			return
		}

		if minRole == RoleAdmin && claims.Role != RoleAdmin {
			if isAPI {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "administrator privilege required"})
			} else {
				c.AbortWithStatus(http.StatusForbidden)
			}
			return
		}

		// Session auto-renewal (sliding window): if less than 12 hours remaining, reissue token for 24h
		if claims.ExpiresAt != nil && time.Until(claims.ExpiresAt.Time) < 12*time.Hour {
			if newToken, err := GenerateToken(claims.Nickname, claims.Role, secret, 24*time.Hour); err == nil {
				c.SetCookie(CookieKey, newToken, 86400, "/", "", false, true)
			}
		}

		c.Set("role", claims.Role)
		c.Set("nickname", claims.Nickname)
		c.Next()
	}
}
