package orbo

import (
	"context"
	"log"
	camera "orbo/gen/camera"
)

// camera service example implementation.
// The example methods log the requests and return zero values.
type camerasrvc struct {
	logger *log.Logger
}

// NewCamera returns the camera service implementation.
func NewCamera(logger *log.Logger) camera.Service {
	return &camerasrvc{logger}
}

// List all configured cameras
func (s *camerasrvc) List(ctx context.Context) (res []*camera.CameraInfo, err error) {
	s.logger.Print("camera.list")
	return
}

// Get camera information by ID
func (s *camerasrvc) Get(ctx context.Context, p *camera.GetPayload) (res *camera.CameraInfo, err error) {
	res = &camera.CameraInfo{}
	s.logger.Print("camera.get")
	return
}

// Add a new camera
func (s *camerasrvc) Create(ctx context.Context, p *camera.CreatePayload) (res *camera.CameraInfo, err error) {
	res = &camera.CameraInfo{}
	s.logger.Print("camera.create")
	return
}

// Update camera configuration
func (s *camerasrvc) Update(ctx context.Context, p *camera.UpdatePayload) (res *camera.CameraInfo, err error) {
	res = &camera.CameraInfo{}
	s.logger.Print("camera.update")
	return
}

// Remove a camera
func (s *camerasrvc) Delete(ctx context.Context, p *camera.DeletePayload) (err error) {
	s.logger.Print("camera.delete")
	return
}

// Activate camera for motion detection
func (s *camerasrvc) Activate(ctx context.Context, p *camera.ActivatePayload) (res *camera.CameraInfo, err error) {
	res = &camera.CameraInfo{}
	s.logger.Print("camera.activate")
	return
}

// Deactivate camera
func (s *camerasrvc) Deactivate(ctx context.Context, p *camera.DeactivatePayload) (res *camera.CameraInfo, err error) {
	res = &camera.CameraInfo{}
	s.logger.Print("camera.deactivate")
	return
}

// Capture a single frame from camera as base64
func (s *camerasrvc) Capture(ctx context.Context, p *camera.CapturePayload) (res *camera.FrameResponse, err error) {
	res = &camera.FrameResponse{}
	s.logger.Print("camera.capture")
	return
}
