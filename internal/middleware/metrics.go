package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
)

func Metrics(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		m.HTTPRequestsInFlight.Inc()
		started := time.Now()
		defer func() {
			m.HTTPRequestsInFlight.Dec()
			status := strconv.Itoa(c.Writer.Status())
			labels := []string{c.Request.Method, routeLabel(c), status}
			m.HTTPRequestsTotal.WithLabelValues(labels...).Inc()
			m.HTTPRequestDuration.WithLabelValues(labels...).Observe(time.Since(started).Seconds())
			m.HTTPResponseSize.WithLabelValues(labels...).Observe(float64(max(c.Writer.Size(), 0)))
		}()
		c.Next()
	}
}
