package detector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MotionEvent represents a detected motion event
type MotionEvent struct {
	ID            string
	CameraID      string
	Timestamp     time.Time
	Confidence    float32
	BoundingBoxes []BoundingBox
	FramePath     string
}

// BoundingBox represents a detected motion area
type BoundingBox struct {
	X      int
	Y      int
	Width  int
	Height int
}

// MotionDetector handles motion detection for cameras (basic implementation)
type MotionDetector struct {
	mu              sync.RWMutex
	isRunning       bool
	cameras         map[string]*CameraDetector
	eventsChan      chan MotionEvent
	stopCh          chan struct{}
	frameStorePath  string
	minConfidence   float32
}

// CameraDetector handles motion detection for a single camera (basic implementation)
type CameraDetector struct {
	cameraID        string
	isRunning       bool
	stopCh          chan struct{}
	minContourArea  float64
}

// MotionDetectorConfig holds configuration for motion detection
type MotionDetectorConfig struct {
	MinConfidence    float32
	MinContourArea   float64
	FrameStorePath   string
	BackgroundHistory int
	VarThreshold     float64
}

// NewMotionDetector creates a new motion detector (basic implementation)
func NewMotionDetector(config MotionDetectorConfig) *MotionDetector {
	if config.MinConfidence == 0 {
		config.MinConfidence = 0.5
	}
	if config.MinContourArea == 0 {
		config.MinContourArea = 1000
	}
	if config.FrameStorePath == "" {
		config.FrameStorePath = "./frames"
	}

	// Ensure frame storage directory exists
	os.MkdirAll(config.FrameStorePath, 0755)

	return &MotionDetector{
		cameras:        make(map[string]*CameraDetector),
		eventsChan:     make(chan MotionEvent, 100),
		frameStorePath: config.FrameStorePath,
		minConfidence:  config.MinConfidence,
	}
}

// Start begins motion detection
func (md *MotionDetector) Start(ctx context.Context) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	if md.isRunning {
		return fmt.Errorf("motion detector is already running")
	}

	md.isRunning = true
	md.stopCh = make(chan struct{})

	return nil
}

// Stop ends motion detection
func (md *MotionDetector) Stop() {
	md.mu.Lock()
	defer md.mu.Unlock()

	if !md.isRunning {
		return
	}

	close(md.stopCh)
	md.isRunning = false

	// Stop all camera detectors
	for _, detector := range md.cameras {
		detector.stop()
	}
}

// AddCamera adds a camera for motion detection (basic implementation)
func (md *MotionDetector) AddCamera(cameraID string, framesChan interface{}) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	if _, exists := md.cameras[cameraID]; exists {
		return fmt.Errorf("camera %s already added for motion detection", cameraID)
	}

	detector := &CameraDetector{
		cameraID:       cameraID,
		minContourArea: 1000,
	}

	md.cameras[cameraID] = detector

	// Start basic detection simulation for this camera
	go md.simulateMotionDetection(cameraID)

	return nil
}

// RemoveCamera removes a camera from motion detection
func (md *MotionDetector) RemoveCamera(cameraID string) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	detector, exists := md.cameras[cameraID]
	if !exists {
		return fmt.Errorf("camera %s not found in motion detection", cameraID)
	}

	detector.stop()
	delete(md.cameras, cameraID)

	return nil
}

// GetEventsChannel returns the channel for receiving motion events
func (md *MotionDetector) GetEventsChannel() <-chan MotionEvent {
	return md.eventsChan
}

// IsRunning returns whether motion detection is active
func (md *MotionDetector) IsRunning() bool {
	md.mu.RLock()
	defer md.mu.RUnlock()
	return md.isRunning
}

// simulateMotionDetection simulates motion detection for basic implementation
func (md *MotionDetector) simulateMotionDetection(cameraID string) {
	detector := md.cameras[cameraID]
	detector.isRunning = true
	detector.stopCh = make(chan struct{})

	defer func() {
		detector.isRunning = false
	}()

	// Simulate periodic motion events (for testing)
	ticker := time.NewTicker(30 * time.Second) // Simulate motion every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-detector.stopCh:
			return
		case <-md.stopCh:
			return
		case <-ticker.C:
			// Create a simulated motion event
			event := MotionEvent{
				ID:         generateEventID(),
				CameraID:   cameraID,
				Timestamp:  time.Now(),
				Confidence: 0.75, // Simulated confidence
				BoundingBoxes: []BoundingBox{
					{X: 100, Y: 100, Width: 200, Height: 150}, // Simulated bounding box
				},
				FramePath: md.createPlaceholderFrame(cameraID),
			}

			// Send event to channel (non-blocking)
			select {
			case md.eventsChan <- event:
			default:
				// Event channel is full, could log this
			}
		}
	}
}

// createPlaceholderFrame creates a placeholder frame file
func (md *MotionDetector) createPlaceholderFrame(cameraID string) string {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s_placeholder.txt", cameraID, timestamp)
	filepath := filepath.Join(md.frameStorePath, filename)

	// Create a placeholder text file instead of an image
	content := fmt.Sprintf("Placeholder frame from camera %s at %s\nOpenCV integration needed for actual frame capture", 
		cameraID, time.Now().Format(time.RFC3339))
	
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		return ""
	}

	return filepath
}

// stop stops the camera detector
func (cd *CameraDetector) stop() {
	if cd.isRunning && cd.stopCh != nil {
		close(cd.stopCh)
	}
	cd.isRunning = false
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("event_%d", time.Now().UnixNano())
}

// GetFrameBytes reads and returns frame bytes from disk
func GetFrameBytes(framePath string) ([]byte, error) {
	if framePath == "" {
		return nil, fmt.Errorf("frame path is empty")
	}

	data, err := os.ReadFile(framePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read frame file: %w", err)
	}

	return data, nil
}

// CleanupOldFrames removes frame files older than the specified duration
func (md *MotionDetector) CleanupOldFrames(maxAge time.Duration) error {
	entries, err := os.ReadDir(md.frameStorePath)
	if err != nil {
		return fmt.Errorf("failed to read frames directory: %w", err)
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > maxAge {
			filepath := filepath.Join(md.frameStorePath, entry.Name())
			if err := os.Remove(filepath); err != nil {
				// Log error but continue cleanup
				continue
			}
		}
	}

	return nil
}

// GetCameraDetectorStatus returns status information for all camera detectors
func (md *MotionDetector) GetCameraDetectorStatus() map[string]bool {
	md.mu.RLock()
	defer md.mu.RUnlock()

	status := make(map[string]bool)
	for cameraID, detector := range md.cameras {
		status[cameraID] = detector.isRunning
	}

	return status
}