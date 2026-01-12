package motion

import (
	"fmt"
	"os"
	"time"

	"orbo/internal/database"
	"orbo/internal/detection"
	"orbo/internal/stream"
	"orbo/internal/telegram"
	"orbo/internal/ws"
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
	// AI-enhanced fields
	ObjectClass      string  `json:"object_class,omitempty"`      // "person", "car", etc.
	ObjectConfidence float32 `json:"object_confidence,omitempty"` // AI confidence
	ThreatLevel      string  `json:"threat_level,omitempty"`      // "high", "medium", "low"
	InferenceTimeMs  float32 `json:"inference_time_ms,omitempty"` // GPU inference time
	DetectionDevice  string  `json:"detection_device,omitempty"`  // "cuda", "cpu"
	// Face recognition fields
	FacesDetected       int      `json:"faces_detected,omitempty"`        // Number of faces detected
	KnownIdentities     []string `json:"known_identities,omitempty"`      // Recognized face names
	UnknownFacesCount   int      `json:"unknown_faces_count,omitempty"`   // Number of unrecognized faces
	ForensicThumbnails  []string `json:"forensic_thumbnails,omitempty"`   // Paths to forensic face analysis images
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

// HasAnyDetectionRunning returns true if detection is running on any camera
func (md *MotionDetector) HasAnyDetectionRunning() bool {
	return md.streamDetector.HasAnyDetectionRunning()
}

// SetDrawBoxes enables or disables bounding box drawing on detection images
func (md *MotionDetector) SetDrawBoxes(enabled bool) {
	md.streamDetector.SetDrawBoxes(enabled)
}

// DrawBoxesEnabled returns whether bounding boxes are drawn on detection images
func (md *MotionDetector) DrawBoxesEnabled() bool {
	return md.streamDetector.DrawBoxesEnabled()
}

// SetTelegramBot sets the Telegram bot for notifications
func (md *MotionDetector) SetTelegramBot(bot *telegram.TelegramBot) {
	md.streamDetector.SetTelegramBot(bot)
}

// GetEventsForTelegram returns events in a format suitable for Telegram command handler
// This avoids circular imports by returning a simpler struct
func (md *MotionDetector) GetEventsForTelegram(cameraID string, since *time.Time, limit int) []telegram.MotionEventInfo {
	events := md.streamDetector.GetEvents(cameraID, since, limit)
	result := make([]telegram.MotionEventInfo, len(events))
	for i, e := range events {
		result[i] = telegram.MotionEventInfo{
			ID:          e.ID,
			CameraID:    e.CameraID,
			Timestamp:   e.Timestamp,
			ObjectClass: e.ObjectClass,
		}
	}
	return result
}

// SetWebSocketHub sets the WebSocket hub for real-time detection broadcasting
func (md *MotionDetector) SetWebSocketHub(hub *ws.DetectionHub) {
	md.streamDetector.SetWebSocketHub(hub)
}

// SetStreamOverlay sets the stream overlay provider for drawing bounding boxes on MJPEG streams
func (md *MotionDetector) SetStreamOverlay(overlay stream.StreamOverlayProvider) {
	md.streamDetector.SetStreamOverlay(overlay)
}

// SetPipelineConfig sets the function that provides detection configuration for mode gating
// When detection mode is "disabled", YOLO detection will be skipped (streaming only)
func (md *MotionDetector) SetPipelineConfig(provider PipelineConfigProvider) {
	md.streamDetector.SetPipelineConfig(provider)
}

// SetGRPCDetector sets the gRPC-based YOLO detector for low-latency streaming detection
// If set, the StreamDetector will prefer gRPC over HTTP for object detection
func (md *MotionDetector) SetGRPCDetector(grpcDet *detection.GRPCDetector) {
	md.streamDetector.SetGRPCDetector(grpcDet)
}

// SetGRPCFaceRecognizer sets the gRPC-based face recognizer for low-latency streaming
// If set, the StreamDetector will prefer gRPC over HTTP for face recognition
func (md *MotionDetector) SetGRPCFaceRecognizer(grpcFace *detection.GRPCFaceRecognizer) {
	md.streamDetector.SetGRPCFaceRecognizer(grpcFace)
}

// ConfigureGRPCYOLO sends configuration to the gRPC YOLO service
// This is called when YOLO settings are changed via the config API
func (md *MotionDetector) ConfigureGRPCYOLO(confThreshold float32, classes []string) error {
	return md.streamDetector.ConfigureGRPCYOLO(confThreshold, classes)
}

// SetYOLOConfig sets the function that provides YOLO configuration (confidence threshold)
// This allows the stream detector to use the current configured threshold for HTTP fallback
func (md *MotionDetector) SetYOLOConfig(provider YOLOConfigProvider) {
	md.streamDetector.SetYOLOConfig(provider)
}