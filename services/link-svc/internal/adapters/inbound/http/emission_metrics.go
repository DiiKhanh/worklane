package http

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/duykhanh/worklane/pkg/platform/metrics"
	"github.com/duykhanh/worklane/services/link-svc/internal/app"
)

func emissionMetrics(clicks ClickEmitter) gin.HandlerFunc {
	observer, ok := clicks.(interface{ Stats() app.EmissionStats })
	if !ok {
		return metrics.Handler()
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "link_click_published_total", Help: "Click publications acknowledged by Kafka.",
	}, func() float64 { return float64(observer.Stats().Published) }))
	for reason, count := range map[string]func() uint64{
		"overflow": func() uint64 { return observer.Stats().Overflow },
		"publish":  func() uint64 { return observer.Stats().Failed },
		"shutdown": func() uint64 { return observer.Stats().Shutdown },
	} {
		registry.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "link_click_dropped_total", Help: "Clicks dropped or without a confirmed publication; delivery may be uncertain.",
			ConstLabels: prometheus.Labels{"reason": reason},
		}, func() float64 { return float64(count()) }))
	}
	return gin.WrapH(promhttp.HandlerFor(prometheus.Gatherers{prometheus.DefaultGatherer, registry}, promhttp.HandlerOpts{}))
}
