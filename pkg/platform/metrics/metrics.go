// Package metrics provides a gin middleware and /metrics handler exposing
// Prometheus request counters and a latency histogram. Shared by otp-api and
// auth-svc so both export the same signal names (send rate, p95 latency).
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	reqTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by method, route and status.",
	}, []string{"method", "path", "status"})

	reqDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
)

// Middleware records count + latency. It uses the matched route template
// (c.FullPath()) as the "path" label so high-cardinality path params do not
// explode the metric series.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		reqDuration.WithLabelValues(c.Request.Method, path).Observe(time.Since(start).Seconds())
		reqTotal.WithLabelValues(c.Request.Method, path, strconv.Itoa(c.Writer.Status())).Inc()
	}
}

// Handler serves the Prometheus exposition format. Unauthenticated by design.
func Handler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}
