package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "messenger_http_requests_total",
		Help: "Total HTTP requests",
	}, []string{"method", "path", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "messenger_http_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	wsConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "messenger_ws_connections_active",
		Help: "Active WebSocket connections",
	})

	messagesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "messenger_messages_total",
		Help: "Total messages sent",
	})
)

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		httpRequests.WithLabelValues(c.Request.Method, path, status).Inc()
		httpDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

func IncWSConnections() { wsConnections.Inc() }
func DecWSConnections() { wsConnections.Dec() }
func IncMessagesTotal() { messagesTotal.Inc() }
