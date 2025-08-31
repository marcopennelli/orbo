package orbo

import (
	"context"
	"log"
	health "orbo/gen/health"
)

// health service example implementation.
// The example methods log the requests and return zero values.
type healthsrvc struct {
	logger *log.Logger
}

// NewHealth returns the health service implementation.
func NewHealth(logger *log.Logger) health.Service {
	return &healthsrvc{logger}
}

// Liveness probe endpoint - indicates if the service is alive
func (s *healthsrvc) Healthz(ctx context.Context) (err error) {
	s.logger.Print("health.healthz")
	return
}

// Readiness probe endpoint - indicates if the service is ready to serve traffic
func (s *healthsrvc) Readyz(ctx context.Context) (err error) {
	s.logger.Print("health.readyz")
	return
}
