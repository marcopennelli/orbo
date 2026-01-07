package services

import (
	"context"
	"time"

	system_service "orbo/gen/system"
	"orbo/internal/camera"
	"orbo/internal/motion"
)

// SystemImplementation implements the system service
type SystemImplementation struct {
	cameraManager  *camera.CameraManager
	motionDetector *motion.MotionDetector
	startTime      time.Time
}

// NewSystemService creates a new system service implementation
func NewSystemService(cameraManager *camera.CameraManager, motionDetector *motion.MotionDetector) *SystemImplementation {
	return &SystemImplementation{
		cameraManager:  cameraManager,
		motionDetector: motionDetector,
		startTime:      time.Now(),
	}
}

// Status returns the overall system status
func (s *SystemImplementation) Status(ctx context.Context) (*system_service.SystemStatus, error) {
	cameras := s.cameraManager.ListCameras()

	// Convert cameras to service format
	cameraInfos := make([]*system_service.CameraInfo, len(cameras))
	for i, cam := range cameras {
		createdAtStr := cam.CreatedAt.Format(time.RFC3339)
		cameraInfos[i] = &system_service.CameraInfo{
			ID:         cam.ID,
			Name:       cam.Name,
			Device:     cam.Device,
			Status:     cam.Status,
			Resolution: &cam.Resolution,
			Fps:        &cam.FPS,
			CreatedAt:  &createdAtStr,
		}
	}

	// Check if motion detection is active on any camera
	motionDetectionActive := false
	for _, cam := range cameras {
		if s.motionDetector.IsDetectionRunning(cam.ID) {
			motionDetectionActive = true
			break
		}
	}

	// Calculate uptime
	uptime := int(time.Since(s.startTime).Seconds())

	return &system_service.SystemStatus{
		Cameras:               cameraInfos,
		MotionDetectionActive: motionDetectionActive,
		NotificationsActive:   false, // TODO: implement notification status check
		UptimeSeconds:         uptime,
	}, nil
}

// StartDetection starts motion detection on all active cameras
func (s *SystemImplementation) StartDetection(ctx context.Context) (*system_service.SystemStatus, error) {
	cameras := s.cameraManager.ListCameras()

	for _, cam := range cameras {
		if cam.Status == "active" {
			// Start motion detection for this camera
			if !s.motionDetector.IsDetectionRunning(cam.ID) {
				err := s.motionDetector.StartDetection(cam.ID, cam.Device)
				if err != nil {
					return nil, &system_service.InternalError{
						Message: "Failed to start motion detection: " + err.Error(),
					}
				}
			}
		}
	}

	// Return updated status
	return s.Status(ctx)
}

// StopDetection stops motion detection on all cameras
func (s *SystemImplementation) StopDetection(ctx context.Context) (*system_service.SystemStatus, error) {
	cameras := s.cameraManager.ListCameras()

	for _, cam := range cameras {
		// Stop motion detection for this camera
		if s.motionDetector.IsDetectionRunning(cam.ID) {
			s.motionDetector.StopDetection(cam.ID)
		}
	}

	// Return updated status
	return s.Status(ctx)
}
