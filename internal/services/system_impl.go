package services

import (
	"context"
	"fmt"
	"time"

	config "orbo/gen/config"
	system_service "orbo/gen/system"
	"orbo/internal/camera"
	"orbo/internal/motion"
)

// PipelineConfigGetter interface for getting pipeline configuration
// This avoids circular imports by not depending on the full config service
type PipelineConfigGetter interface {
	GetPipelineConfig() *config.PipelineConfig
}

// SystemImplementation implements the system service
type SystemImplementation struct {
	cameraManager        *camera.CameraManager
	motionDetector       *motion.MotionDetector
	pipelineConfigGetter PipelineConfigGetter
	startTime            time.Time
}

// NewSystemService creates a new system service implementation
func NewSystemService(cameraManager *camera.CameraManager, motionDetector *motion.MotionDetector) *SystemImplementation {
	return &SystemImplementation{
		cameraManager:  cameraManager,
		motionDetector: motionDetector,
		startTime:      time.Now(),
	}
}

// SetPipelineConfigGetter sets the pipeline config getter for mode-aware detection control
func (s *SystemImplementation) SetPipelineConfigGetter(getter PipelineConfigGetter) {
	s.pipelineConfigGetter = getter
}

// Status returns the overall system status
func (s *SystemImplementation) Status(ctx context.Context) (*system_service.SystemStatus, error) {
	cameras := s.cameraManager.ListCameras()

	// Convert cameras to service format
	cameraInfos := make([]*system_service.CameraInfo, len(cameras))
	for i, cam := range cameras {
		createdAtStr := cam.CreatedAt.Format(time.RFC3339)
		detectionEnabled := cam.DetectionEnabled
		cameraInfos[i] = &system_service.CameraInfo{
			ID:               cam.ID,
			Name:             cam.Name,
			Device:           cam.Device,
			Status:           cam.Status,
			Resolution:       &cam.Resolution,
			Fps:              &cam.FPS,
			CreatedAt:        &createdAtStr,
			DetectionEnabled: &detectionEnabled,
		}
	}

	// Check if motion detection is active on any camera
	motionDetectionActive := false
	detectingCameras := 0
	for _, cam := range cameras {
		if s.motionDetector.IsDetectionRunning(cam.ID) {
			motionDetectionActive = true
			detectingCameras++
		}
	}

	// Get pipeline configuration state
	var pipelineMode *string
	var pipelineExecutionMode *string
	var pipelineDetectors []string
	pipelineDetectionEnabled := true // Default: detection enabled unless pipeline says otherwise

	if s.pipelineConfigGetter != nil {
		pipelineCfg := s.pipelineConfigGetter.GetPipelineConfig()
		if pipelineCfg != nil {
			pipelineMode = &pipelineCfg.Mode
			pipelineExecutionMode = &pipelineCfg.ExecutionMode
			pipelineDetectors = pipelineCfg.Detectors
			// Check if pipeline has detection disabled
			pipelineDetectionEnabled = pipelineCfg.Mode != "disabled"
		}
	}

	// Calculate uptime
	uptime := int(time.Since(s.startTime).Seconds())

	status := &system_service.SystemStatus{
		Cameras:               cameraInfos,
		MotionDetectionActive: motionDetectionActive,
		NotificationsActive:   false, // TODO: implement notification status check
		UptimeSeconds:         uptime,
		// Extended pipeline status fields
		PipelineMode:             pipelineMode,
		PipelineExecutionMode:    pipelineExecutionMode,
		PipelineDetectors:        pipelineDetectors,
		PipelineDetectionEnabled: &pipelineDetectionEnabled,
		DetectingCameras:         &detectingCameras,
	}

	return status, nil
}

// StartDetection starts motion detection on all active cameras
// NOTE: This respects the pipeline mode - if mode is "disabled", detection starts
// but YOLO/Face analysis will be skipped (streaming only). The UI should warn users
// about this state via the PipelineDetectionEnabled status field.
func (s *SystemImplementation) StartDetection(ctx context.Context) (*system_service.SystemStatus, error) {
	cameras := s.cameraManager.ListCameras()

	// Check pipeline mode and warn if detection is disabled
	pipelineMode := "motion_triggered" // Default
	if s.pipelineConfigGetter != nil {
		pipelineCfg := s.pipelineConfigGetter.GetPipelineConfig()
		if pipelineCfg != nil {
			pipelineMode = pipelineCfg.Mode
		}
	}

	// If pipeline mode is disabled, we still start streaming but no AI detection
	// This allows the user to view camera feeds without running detection
	if pipelineMode == "disabled" {
		fmt.Println("[SystemService] Starting detection with pipeline mode=disabled (streaming only, no AI detection)")
	}

	for _, cam := range cameras {
		if cam.Status == "active" {
			// Check if detection is enabled for this camera
			if !cam.DetectionEnabled {
				fmt.Printf("[SystemService] Skipping detection for camera %s (detection_enabled=false)\n", cam.ID)
				continue
			}

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
