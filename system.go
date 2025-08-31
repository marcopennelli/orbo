package orbo

import (
	"context"
	"log"
	system "orbo/gen/system"
)

// system service example implementation.
// The example methods log the requests and return zero values.
type systemsrvc struct {
	logger *log.Logger
}

// NewSystem returns the system service implementation.
func NewSystem(logger *log.Logger) system.Service {
	return &systemsrvc{logger}
}

// Get overall system status
func (s *systemsrvc) Status(ctx context.Context) (res *system.SystemStatus, err error) {
	res = &system.SystemStatus{}
	s.logger.Print("system.status")
	return
}

// Start motion detection on all active cameras
func (s *systemsrvc) StartDetection(ctx context.Context) (res *system.SystemStatus, err error) {
	res = &system.SystemStatus{}
	s.logger.Print("system.start_detection")
	return
}

// Stop motion detection
func (s *systemsrvc) StopDetection(ctx context.Context) (res *system.SystemStatus, err error) {
	res = &system.SystemStatus{}
	s.logger.Print("system.stop_detection")
	return
}
