package camera

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"orbo/internal/database"
)

// Camera represents a USB camera device with direct system access
type Camera struct {
	ID         string
	Name       string
	Device     string
	Resolution string
	FPS        int
	Status     string
	CreatedAt  time.Time
	
	// Internal fields for system access
	mu         sync.RWMutex
	isActive   bool
	stopCh     chan struct{}
}

// CameraManager manages multiple cameras
type CameraManager struct {
	cameras map[string]*Camera
	mu      sync.RWMutex
	db      *database.Database
}

// NewCameraManager creates a new camera manager
func NewCameraManager(db *database.Database) *CameraManager {
	cm := &CameraManager{
		cameras: make(map[string]*Camera),
		db:      db,
	}

	// Load cameras from database on startup
	if db != nil {
		if err := cm.loadCamerasFromDB(); err != nil {
			fmt.Printf("Warning: failed to load cameras from database: %v\n", err)
		}
	}

	return cm
}

// loadCamerasFromDB loads cameras from the database
func (cm *CameraManager) loadCamerasFromDB() error {
	records, err := cm.db.ListCameras()
	if err != nil {
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, record := range records {
		camera := &Camera{
			ID:         record.ID,
			Name:       record.Name,
			Device:     record.Device,
			Resolution: record.Resolution,
			FPS:        record.FPS,
			Status:     "inactive", // Always start inactive
			CreatedAt:  record.CreatedAt,
			stopCh:     make(chan struct{}),
		}
		cm.cameras[camera.ID] = camera
		fmt.Printf("Loaded camera from database: %s (%s)\n", camera.Name, camera.ID)
	}

	fmt.Printf("Loaded %d cameras from database\n", len(records))
	return nil
}

// NewCamera creates a new camera instance
func NewCamera(id, name, device, resolution string, fps int) *Camera {
	return &Camera{
		ID:         id,
		Name:       name,
		Device:     device,
		Resolution: resolution,
		FPS:        fps,
		Status:     "inactive",
		CreatedAt:  time.Now(),
		stopCh:     make(chan struct{}),
	}
}

// AddCamera adds a camera to the manager
func (cm *CameraManager) AddCamera(camera *Camera) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if device exists
	if !cm.deviceExists(camera.Device) {
		return fmt.Errorf("camera device %s does not exist", camera.Device)
	}

	cm.cameras[camera.ID] = camera

	// Persist to database
	if cm.db != nil {
		record := &database.CameraRecord{
			ID:         camera.ID,
			Name:       camera.Name,
			Device:     camera.Device,
			Resolution: camera.Resolution,
			FPS:        camera.FPS,
			Status:     camera.Status,
			CreatedAt:  camera.CreatedAt,
		}
		if err := cm.db.SaveCamera(record); err != nil {
			fmt.Printf("Warning: failed to persist camera to database: %v\n", err)
		}
	}

	return nil
}

// GetCamera retrieves a camera by ID
func (cm *CameraManager) GetCamera(id string) (*Camera, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	camera, exists := cm.cameras[id]
	if !exists {
		return nil, fmt.Errorf("camera with ID %s not found", id)
	}
	return camera, nil
}

// ListCameras returns all cameras
func (cm *CameraManager) ListCameras() []*Camera {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	cameras := make([]*Camera, 0, len(cm.cameras))
	for _, camera := range cm.cameras {
		cameras = append(cameras, camera)
	}
	return cameras
}

// RemoveCamera removes a camera from the manager
func (cm *CameraManager) RemoveCamera(id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	camera, exists := cm.cameras[id]
	if !exists {
		return fmt.Errorf("camera with ID %s not found", id)
	}

	// Stop the camera if it's active
	if camera.isActive {
		camera.stop()
	}

	delete(cm.cameras, id)

	// Delete from database
	if cm.db != nil {
		if err := cm.db.DeleteCamera(id); err != nil {
			fmt.Printf("Warning: failed to delete camera from database: %v\n", err)
		}
	}

	return nil
}

// ActivateCamera starts video capture for a camera
func (cm *CameraManager) ActivateCamera(id string) error {
	camera, err := cm.GetCamera(id)
	if err != nil {
		return err
	}

	if err := camera.activate(); err != nil {
		return err
	}

	// Update status in database
	if cm.db != nil {
		if err := cm.db.UpdateCameraStatus(id, "active"); err != nil {
			fmt.Printf("Warning: failed to update camera status in database: %v\n", err)
		}
	}

	return nil
}

// DeactivateCamera stops video capture for a camera
func (cm *CameraManager) DeactivateCamera(id string) error {
	camera, err := cm.GetCamera(id)
	if err != nil {
		return err
	}

	camera.deactivate()

	// Update status in database
	if cm.db != nil {
		if err := cm.db.UpdateCameraStatus(id, "inactive"); err != nil {
			fmt.Printf("Warning: failed to update camera status in database: %v\n", err)
		}
	}

	return nil
}


// activate starts the camera - THIS WILL TURN ON THE LED!
func (c *Camera) activate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.isActive {
		return fmt.Errorf("camera %s is already active", c.ID)
	}
	
	// Check if device exists and is accessible
	if !c.deviceAccessible() {
		c.Status = "error"
		return fmt.Errorf("camera device %s is not accessible", c.Device)
	}
	
	// Test camera access by taking a quick frame (this turns on the LED)
	_, err := c.captureFrameWithFfmpeg()
	if err != nil {
		c.Status = "error"
		return fmt.Errorf("failed to access camera device %s: %w", c.Device, err)
	}
	
	c.isActive = true
	c.Status = "active"
	c.stopCh = make(chan struct{})
	
	return nil
}

// deactivate stops the camera capture
func (c *Camera) deactivate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.isActive {
		return
	}
	
	c.stop()
}

// stop internal method to stop capture (must be called with lock held)
func (c *Camera) stop() {
	if c.stopCh != nil {
		close(c.stopCh)
	}
	
	c.isActive = false
	c.Status = "inactive"
}


// CaptureFrame captures a single frame and returns it as JPEG bytes
func (c *Camera) CaptureFrame() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if !c.isActive {
		return nil, fmt.Errorf("camera %s is not active", c.ID)
	}
	
	// Use ffmpeg to capture a frame from the camera
	return c.captureFrameWithFfmpeg()
}

// isNetworkSource checks if device is an HTTP/RTSP URL
func isNetworkSource(device string) bool {
	return strings.HasPrefix(device, "http://") ||
		strings.HasPrefix(device, "https://") ||
		strings.HasPrefix(device, "rtsp://")
}

// deviceExists checks if a camera device exists
func (cm *CameraManager) deviceExists(device string) bool {
	// Network sources (HTTP/RTSP) are always considered to exist
	// Actual connectivity is checked when activating
	if isNetworkSource(device) {
		return true
	}

	if _, err := os.Stat(device); os.IsNotExist(err) {
		return false
	}

	// Try to access the device
	file, err := os.OpenFile(device, os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer file.Close()

	return true
}

// captureFrameWithFfmpeg captures a frame using ffmpeg command
func (c *Camera) captureFrameWithFfmpeg() ([]byte, error) {
	var args []string

	if isNetworkSource(c.Device) {
		// For HTTP/RTSP sources, use appropriate input format
		args = []string{
			"-y",                   // Overwrite output
			"-i", c.Device,         // Input URL
			"-vframes", "1",        // Capture 1 frame
			"-f", "mjpeg",          // Output format
			"-q:v", "2",            // High quality JPEG
			"-",                    // Output to stdout
		}
	} else {
		// For V4L2 devices
		if c.Resolution != "" {
			args = []string{
				"-f", "v4l2",
				"-video_size", c.Resolution,
				"-i", c.Device,
				"-vframes", "1",
				"-f", "mjpeg",
				"-q:v", "2",
				"-",
			}
		} else {
			args = []string{
				"-f", "v4l2",
				"-i", c.Device,
				"-vframes", "1",
				"-f", "mjpeg",
				"-q:v", "2",
				"-",
			}
		}
	}

	// Execute ffmpeg command
	cmd := exec.Command("ffmpeg", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// deviceAccessible checks if device is accessible
func (c *Camera) deviceAccessible() bool {
	// Network sources are checked by actually connecting
	if isNetworkSource(c.Device) {
		return true // Will be verified when capturing
	}

	if _, err := os.Stat(c.Device); os.IsNotExist(err) {
		return false
	}

	// Try to open for read to check permissions
	file, err := os.OpenFile(c.Device, os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer file.Close()

	return true
}

// IsActive returns whether the camera is currently active
func (c *Camera) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isActive
}

// GetStatus returns the current camera status
func (c *Camera) GetStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Status
}

// UpdateConfiguration updates camera settings
func (c *Camera) UpdateConfiguration(name, resolution string, fps int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	wasActive := c.isActive
	
	// Stop camera if active
	if c.isActive {
		c.stop()
	}
	
	// Update configuration
	if name != "" {
		c.Name = name
	}
	if resolution != "" {
		c.Resolution = resolution
	}
	if fps > 0 {
		c.FPS = fps
	}
	
	// Restart camera if it was active
	if wasActive {
		c.mu.Unlock() // Unlock before calling activate which needs the lock
		return c.activate()
	}
	
	return nil
}