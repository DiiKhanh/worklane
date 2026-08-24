package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMetricsExposedAndUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/metrics", Handler())
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })

	// generate one request so the counter is non-zero
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ping", nil))

	rr := httptest.NewRecorder()
	// deliberately no Authorization header
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 on /metrics, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "http_requests_total") {
		t.Fatalf("want http_requests_total in metrics body, got:\n%s", rr.Body.String())
	}
}
