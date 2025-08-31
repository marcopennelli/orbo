package orbo

import (
	"context"
	"log"
	motion "orbo/gen/motion"
)

// motion service example implementation.
// The example methods log the requests and return zero values.
type motionsrvc struct {
	logger *log.Logger
}

// NewMotion returns the motion service implementation.
func NewMotion(logger *log.Logger) motion.Service {
	return &motionsrvc{logger}
}

// List motion detection events
func (s *motionsrvc) Events(ctx context.Context, p *motion.EventsPayload) (res []*motion.MotionEvent, err error) {
	s.logger.Print("motion.events")
	return
}

// Get motion event by ID
func (s *motionsrvc) Event(ctx context.Context, p *motion.EventPayload) (res *motion.MotionEvent, err error) {
	res = &motion.MotionEvent{}
	s.logger.Print("motion.event")
	return
}

// Get captured frame for motion event
func (s *motionsrvc) Frame(ctx context.Context, p *motion.FramePayload) (res []byte, err error) {
	s.logger.Print("motion.frame")
	return
}
