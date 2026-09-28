package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHealthHandler(nil)
	r := gin.New()
	r.GET("/healthz", h.Healthz)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}

func TestReadyz_Unavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// gorm.Open pings automatically by default, which would make Open itself
	// fail (and error) for an unreachable address. DisableAutomaticPing skips
	// that so we get back a *gorm.DB whose *sql.DB is still unconnected, and
	// Readyz's own PingContext call is what fails instead - fast, since port
	// 1 gets an immediate connection-refused rather than a timeout.
	db, err := gorm.Open(
		postgres.Open("host=127.0.0.1 port=1 user=x password=x dbname=x sslmode=disable connect_timeout=1"),
		&gorm.Config{DisableAutomaticPing: true},
	)
	require.NoError(t, err)

	h := NewHealthHandler(db)
	r := gin.New()
	r.GET("/readyz", h.Readyz)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.JSONEq(t, `{"status":"unavailable"}`, w.Body.String())
}
