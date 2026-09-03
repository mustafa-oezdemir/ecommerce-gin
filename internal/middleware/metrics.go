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
		defer m.HTTPRequestsInFlight.Dec()

		started := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())
		labels := []string{c.Request.Method, route, status}
		m.HTTPRequestsTotal.WithLabelValues(labels...).Inc()
		m.HTTPRequestDuration.WithLabelValues(labels...).Observe(time.Since(started).Seconds())
		size := c.Writer.Size()
		if size < 0 {
			size = 0
		}
		m.HTTPResponseSize.WithLabelValues(labels...).Observe(float64(size))
	}
}
