package motion

import (
	"fmt"
	"os"
	"time"

	"orbo/internal/database"
)

// MotionEvent represents a detected motion event
type MotionEvent struct {
	ID                string        `json:"id"`
	CameraID          string        `json:"camera_id"`
	Timestamp         time.Time     `json:"timestamp"`
	Confidence        float32       `json:"confidence"`
	BoundingBoxes     []BoundingBox `json:"bounding_boxes"`
	FramePath         string        `json:"frame_path"`
	NotificationSent  bool          `json:"notification_sent"`
	// NEW: AI-enhanced fields
	ObjectClass       string        `json:"object_class,omitempty"`        // "person", "car", etc.
	ObjectConfidence  float32       `json:"object_confidence,omitempty"`   // AI confidence
	ThreatLevel       string        `json:"threat_level,omitempty"`        // "high", "medium", "low"
	InferenceTimeMs   float32       `json:"inference_time_ms,omitempty"`   // GPU inference time
	DetectionDevice   string        `json:"detection_device,omitempty"`    // "cuda", "cpu"
}

// BoundingBox represents detected motion area coordinates
type BoundingBox struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// MotionDetector handles motion detection for cameras
type MotionDetector struct {
	streamDetector *StreamDetector // Use streaming detector by default
}

// NewMotionDetector creates a new motion detector
func NewMotionDetector(frameDir string, db *database.Database) *MotionDetector {
	if err := os.MkdirAll(frameDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create frame directory %s: %v\n", frameDir, err)
	}

	return &MotionDetector{
		streamDetector: NewStreamDetector(frameDir, db),
	}
}

// StartDetection starts streaming motion detection for a camera
func (md *MotionDetector) StartDetection(cameraID, cameraDevice string) error {
	return md.streamDetector.StartStreamingDetection(cameraID, cameraDevice)
}

// StopDetection stops streaming motion detection for a camera
func (md *MotionDetector) StopDetection(cameraID string) {
	md.streamDetector.StopStreamingDetection(cameraID)
}

// GetEvents returns motion events with optional filtering
func (md *MotionDetector) GetEvents(cameraID string, since *time.Time, limit int) []*MotionEvent {
	return md.streamDetector.GetEvents(cameraID, since, limit)
}

// GetEvent returns a specific motion event by ID
func (md *MotionDetector) GetEvent(eventID string) (*MotionEvent, error) {
	return md.streamDetector.GetEvent(eventID)
}

// GetEventFrame returns the frame data for a motion event
func (md *MotionDetector) GetEventFrame(eventID string) ([]byte, error) {
	return md.streamDetector.GetEventFrame(eventID)
}

// IsDetectionRunning checks if motion detection is running for a camera
func (md *MotionDetector) IsDetectionRunning(cameraID string) bool {
	return md.streamDetector.IsDetectionRunning(cameraID)
}