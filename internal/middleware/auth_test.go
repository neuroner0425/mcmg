package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAuthMiddleware(t *testing.T) {
	secret := "test-secret-key"

	userToken, err := GenerateToken("Steve", RoleUser, secret, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate user token: %v", err)
	}

	adminToken, err := GenerateToken("AdminSteve", RoleAdmin, secret, time.Hour)
	if err != nil {
		t.Fatalf("failed to generate admin token: %v", err)
	}

	tests := []struct {
		name         string
		minRole      string
		token        string
		expectedCode int
	}{
		{
			name:         "No token on user route",
			minRole:      RoleUser,
			token:        "",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "User token on user route",
			minRole:      RoleUser,
			token:        userToken,
			expectedCode: http.StatusOK,
		},
		{
			name:         "User token on admin route (Forbidden)",
			minRole:      RoleAdmin,
			token:        userToken,
			expectedCode: http.StatusForbidden,
		},
		{
			name:         "Admin token on admin route",
			minRole:      RoleAdmin,
			token:        adminToken,
			expectedCode: http.StatusOK,
		},
		{
			name:         "Admin token on user route",
			minRole:      RoleUser,
			token:        adminToken,
			expectedCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/api/test", RequireRole(secret, tt.minRole), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req, _ := http.NewRequest(http.MethodGet, "/api/test", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}
