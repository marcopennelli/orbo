package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	camera_service "orbo/gen/camera"
	"orbo/internal/camera"
	"orbo/internal/motion"
)

// CameraImplementation implements the camera service
type CameraImplementation struct {
	cameraManager  *camera.CameraManager
	motionDetector *motion.MotionDetector
}

// NewCameraService creates a new camera service implementation
func NewCameraService(cameraManager *camera.CameraManager, motionDetector *motion.MotionDetector) camera_service.Service {
	return &CameraImplementation{
		cameraManager:  cameraManager,
		motionDetector: motionDetector,
	}
}

// List returns all configured cameras
func (c *CameraImplementation) List(ctx context.Context) ([]*camera_service.CameraInfo, error) {
	cameras := c.cameraManager.ListCameras()
	result := make([]*camera_service.CameraInfo, len(cameras))

	for i, cam := range cameras {
		resolution := cam.Resolution
		fps := cam.FPS
		createdAt := cam.CreatedAt.Format(time.RFC3339)

		alertsEnabled := cam.AlertsEnabled
		result[i] = &camera_service.CameraInfo{
			ID:            cam.ID,
			Name:          cam.Name,
			Device:        cam.Device,
			Status:        cam.GetStatus(),
			Resolution:    &resolution,
			Fps:           &fps,
			CreatedAt:     &createdAt,
			AlertsEnabled: &alertsEnabled,
		}
	}

	return result, nil
}

// Get returns camera information by ID
func (c *CameraImplementation) Get(ctx context.Context, p *camera_service.GetPayload) (*camera_service.CameraInfo, error) {
	cam, err := c.cameraManager.GetCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	resolution := cam.Resolution
	fps := cam.FPS
	createdAt := cam.CreatedAt.Format(time.RFC3339)
	alertsEnabled := cam.AlertsEnabled

	return &camera_service.CameraInfo{
		ID:            cam.ID,
		Name:          cam.Name,
		Device:        cam.Device,
		Status:        cam.GetStatus(),
		Resolution:    &resolution,
		Fps:           &fps,
		CreatedAt:     &createdAt,
		AlertsEnabled: &alertsEnabled,
	}, nil
}

// Create adds a new camera
func (c *CameraImplementation) Create(ctx context.Context, p *camera_service.CreatePayload) (*camera_service.CameraInfo, error) {
	// Generate new camera ID
	id := uuid.New().String()

	// Use payload values (defaults are handled by Goa)
	resolution := p.Resolution
	fps := p.Fps

	// Create camera
	cam := camera.NewCamera(id, p.Name, p.Device, resolution, fps)

	// Set alerts_enabled from payload (defaults to true via Goa)
	cam.AlertsEnabled = p.AlertsEnabled

	// Add to manager
	err := c.cameraManager.AddCamera(cam)
	if err != nil {
		details := err.Error()
		return nil, &camera_service.BadRequestError{
			Message: "Failed to add camera",
			Details: &details,
		}
	}

	resolutionPtr := cam.Resolution
	fpsPtr := cam.FPS
	createdAt := cam.CreatedAt.Format(time.RFC3339)
	alertsEnabled := cam.AlertsEnabled

	return &camera_service.CameraInfo{
		ID:            cam.ID,
		Name:          cam.Name,
		Device:        cam.Device,
		Status:        cam.GetStatus(),
		Resolution:    &resolutionPtr,
		Fps:           &fpsPtr,
		CreatedAt:     &createdAt,
		AlertsEnabled: &alertsEnabled,
	}, nil
}

// Update modifies camera configuration
func (c *CameraImplementation) Update(ctx context.Context, p *camera_service.UpdatePayload) (*camera_service.CameraInfo, error) {
	cam, err := c.cameraManager.GetCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	// Check if device change is requested
	if p.Device != nil && *p.Device != cam.Device {
		// Device can only be changed when camera is inactive
		if cam.GetStatus() == "active" {
			details := "Camera must be deactivated before changing device/URL. Please deactivate the camera first."
			return nil, &camera_service.BadRequestError{
				Message: "Cannot change device while camera is active",
				Details: &details,
			}
		}
		// Update device
		cam.Device = *p.Device
	}

	// Update camera configuration
	name := cam.Name
	if p.Name != nil {
		name = *p.Name
	}

	resolution := cam.Resolution
	if p.Resolution != nil {
		resolution = *p.Resolution
	}

	fps := cam.FPS
	if p.Fps != nil {
		fps = *p.Fps
	}

	err = cam.UpdateConfiguration(name, resolution, fps)
	if err != nil {
		details := err.Error()
		return nil, &camera_service.BadRequestError{
			Message: "Failed to update camera",
			Details: &details,
		}
	}

	// Update alerts_enabled if provided
	if p.AlertsEnabled != nil {
		c.cameraManager.SetAlertsEnabled(p.ID, *p.AlertsEnabled)
	}

	resolutionPtr := cam.Resolution
	fpsPtr := cam.FPS
	createdAt := cam.CreatedAt.Format(time.RFC3339)
	alertsEnabled := cam.AlertsEnabled

	return &camera_service.CameraInfo{
		ID:            cam.ID,
		Name:          cam.Name,
		Device:        cam.Device,
		Status:        cam.GetStatus(),
		Resolution:    &resolutionPtr,
		Fps:           &fpsPtr,
		CreatedAt:     &createdAt,
		AlertsEnabled: &alertsEnabled,
	}, nil
}

// Delete removes a camera
func (c *CameraImplementation) Delete(ctx context.Context, p *camera_service.DeletePayload) error {
	err := c.cameraManager.RemoveCamera(p.ID)
	if err != nil {
		return &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	return nil
}

// Activate starts video capture for a camera
func (c *CameraImplementation) Activate(ctx context.Context, p *camera_service.ActivatePayload) (*camera_service.CameraInfo, error) {
	cam, err := c.cameraManager.GetCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	// Activate camera - this also creates the MJPEG stream
	err = c.cameraManager.ActivateCamera(p.ID)
	if err != nil {
		return nil, &camera_service.InternalError{
			Message: fmt.Sprintf("Failed to activate camera: %s", err.Error()),
		}
	}

	resolutionPtr := cam.Resolution
	fpsPtr := cam.FPS
	createdAt := cam.CreatedAt.Format(time.RFC3339)
	alertsEnabled := cam.AlertsEnabled

	return &camera_service.CameraInfo{
		ID:            cam.ID,
		Name:          cam.Name,
		Device:        cam.Device,
		Status:        cam.GetStatus(),
		Resolution:    &resolutionPtr,
		Fps:           &fpsPtr,
		CreatedAt:     &createdAt,
		AlertsEnabled: &alertsEnabled,
	}, nil
}

// Deactivate stops video capture for a camera
func (c *CameraImplementation) Deactivate(ctx context.Context, p *camera_service.DeactivatePayload) (*camera_service.CameraInfo, error) {
	cam, err := c.cameraManager.GetCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	// Stop detection for this camera if it's running
	if c.motionDetector.IsDetectionRunning(p.ID) {
		c.motionDetector.StopDetection(p.ID)
		fmt.Printf("[CameraService] Stopped detection for camera %s (camera deactivated)\n", p.ID)
	}

	// Deactivate camera - this also deletes the MJPEG stream
	err = c.cameraManager.DeactivateCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	resolutionPtr := cam.Resolution
	fpsPtr := cam.FPS
	createdAt := cam.CreatedAt.Format(time.RFC3339)
	alertsEnabled := cam.AlertsEnabled

	return &camera_service.CameraInfo{
		ID:            cam.ID,
		Name:          cam.Name,
		Device:        cam.Device,
		Status:        cam.GetStatus(),
		Resolution:    &resolutionPtr,
		Fps:           &fpsPtr,
		CreatedAt:     &createdAt,
		AlertsEnabled: &alertsEnabled,
	}, nil
}

// Capture captures a single frame from camera as base64-encoded JPEG
func (c *CameraImplementation) Capture(ctx context.Context, p *camera_service.CapturePayload) (*camera_service.FrameResponse, error) {
	cam, err := c.cameraManager.GetCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	frameBytes, err := cam.CaptureFrame()
	if err != nil {
		return nil, &camera_service.InternalError{
			Message: fmt.Sprintf("Failed to capture frame: %s", err.Error()),
		}
	}

	// Encode frame as base64
	base64Data := base64.StdEncoding.EncodeToString(frameBytes)

	return &camera_service.FrameResponse{
		Data:        base64Data,
		ContentType: "image/jpeg",
	}, nil
}

// EnableAlerts enables alerts (events and notifications) for this camera
// Detection pipeline continues running for bounding boxes on the stream
func (c *CameraImplementation) EnableAlerts(ctx context.Context, p *camera_service.EnableAlertsPayload) (*camera_service.CameraInfo, error) {
	cam, err := c.cameraManager.GetCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	// Enable alerts for this camera
	err = c.cameraManager.SetAlertsEnabled(p.ID, true)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	fmt.Printf("[CameraService] Enabled alerts for camera %s\n", p.ID)

	resolutionPtr := cam.Resolution
	fpsPtr := cam.FPS
	createdAt := cam.CreatedAt.Format(time.RFC3339)
	alertsEnabled := true // Just set to true since we enabled it

	return &camera_service.CameraInfo{
		ID:            cam.ID,
		Name:          cam.Name,
		Device:        cam.Device,
		Status:        cam.GetStatus(),
		Resolution:    &resolutionPtr,
		Fps:           &fpsPtr,
		CreatedAt:     &createdAt,
		AlertsEnabled: &alertsEnabled,
	}, nil
}

// DisableAlerts disables alerts (events and notifications) for this camera
// Detection pipeline continues running for bounding boxes on the stream
func (c *CameraImplementation) DisableAlerts(ctx context.Context, p *camera_service.DisableAlertsPayload) (*camera_service.CameraInfo, error) {
	cam, err := c.cameraManager.GetCamera(p.ID)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	// Disable alerts for this camera
	err = c.cameraManager.SetAlertsEnabled(p.ID, false)
	if err != nil {
		return nil, &camera_service.NotFoundError{
			Message: "Camera not found",
			ID:      p.ID,
		}
	}

	fmt.Printf("[CameraService] Disabled alerts for camera %s (detection continues for bounding boxes)\n", p.ID)

	resolutionPtr := cam.Resolution
	fpsPtr := cam.FPS
	createdAt := cam.CreatedAt.Format(time.RFC3339)
	alertsEnabled := false // Just set to false since we disabled it

	return &camera_service.CameraInfo{
		ID:            cam.ID,
		Name:          cam.Name,
		Device:        cam.Device,
		Status:        cam.GetStatus(),
		Resolution:    &resolutionPtr,
		Fps:           &fpsPtr,
		CreatedAt:     &createdAt,
		AlertsEnabled: &alertsEnabled,
	}, nil
}