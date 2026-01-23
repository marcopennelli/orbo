package pipeline

import (
	"context"
)

// Detector is the unified interface for all detection backends
// Implementations wrap existing detectors (GPUDetector, FaceRecognizer, etc.)
type Detector interface {
	// Name returns the detector identifier (e.g., "yolo", "face", "plate")
	Name() string

	// Type returns the detector type constant
	Type() DetectorType

	// IsHealthy returns true if the detector is operational
	IsHealthy() bool

	// Detect runs detection on a frame and returns results
	Detect(ctx context.Context, frame *FrameData) (*DetectionResult, error)

	// DetectAnnotated runs detection and returns results with an annotated image
	// Returns the same DetectionResult but with ImageData populated
	DetectAnnotated(ctx context.Context, frame *FrameData) (*DetectionResult, error)

	// SupportsAnnotation returns true if this detector can produce annotated images
	SupportsAnnotation() bool

	// Close releases detector resources
	Close() error
}

// ConditionalDetector extends Detector with conditional execution
// Used for detectors that should only run based on previous detection results
type ConditionalDetector interface {
	Detector

	// ShouldRun determines if this detector should run based on prior results
	// For example, FaceDetector should only run if YOLO detected persons
	ShouldRun(priorResults *DetectionResult) bool

	// GetTriggerClasses returns the classes that trigger this detector
	// For example, FaceDetector returns ["person"]
	GetTriggerClasses() []string
}

// FrameSubscription represents an active subscription to frame data
type FrameSubscription struct {
	CameraID string
	Channel  chan *FrameData
	Done     chan struct{} // Closed when subscription is cancelled
}

// FrameProvider captures frames from camera sources and broadcasts to subscribers
type FrameProvider interface {
	// Start begins capturing frames from the specified camera
	Start(cameraID string, device string, fps int, width int, height int) error

	// Stop halts frame capture for a camera
	Stop(cameraID string) error

	// Subscribe returns a channel that receives frames for a camera
	// Caller must call Unsubscribe when done to prevent resource leaks
	Subscribe(cameraID string, bufferSize int) (*FrameSubscription, error)

	// Unsubscribe removes a frame subscription
	Unsubscribe(sub *FrameSubscription)

	// IsRunning returns true if a camera is actively capturing
	IsRunning(cameraID string) bool

	// GetStats returns capture statistics for a camera
	GetStats(cameraID string) *CaptureStats
}

// CaptureStats contains frame capture statistics
type CaptureStats struct {
	CameraID          string
	FramesCaptured    uint64
	FramesDropped     uint64
	CurrentFPS        float32
	AvgLatencyMs      float32
	LastFrameTime     int64 // Unix timestamp
	ReconnectAttempts uint64
}

// FrameConsumer receives frames for processing
type FrameConsumer interface {
	// OnFrame is called for each captured frame
	OnFrame(frame *FrameData)
}

// DetectionStrategy decides when detection should run
// Different strategies implement different triggering logic
type DetectionStrategy interface {
	// Name returns the strategy identifier
	Name() string

	// ShouldDetect determines if detection should run for this frame
	ShouldDetect(frame *FrameData, lastDetection *DetectionResult) bool

	// OnDetectionComplete is called after detection completes
	// Allows strategy to update internal state
	OnDetectionComplete(result *DetectionResult)

	// Reset clears internal state (e.g., on camera switch)
	Reset()
}

// MotionDetector is a specialized interface for motion detection
// Used by motion-triggered and hybrid strategies
type MotionDetector interface {
	// DetectMotion analyzes a frame and returns true if motion is detected
	DetectMotion(frame *FrameData) (bool, float32, error) // (hasMotion, score, error)

	// SetSensitivity adjusts motion detection sensitivity
	SetSensitivity(sensitivity float32)

	// Reset clears motion detection state (e.g., new reference frame)
	Reset()
}

// DetectionResultHandler receives detection results
type DetectionResultHandler interface {
	// OnDetectionResult is called when detection completes
	OnDetectionResult(result *MergedDetectionResult)
}

// StreamOverlayProvider is the interface that streaming components implement
// to receive annotated frames for display
type StreamOverlayProvider interface {
	// SetAnnotatedFrame provides an annotated frame for streaming
	// seq is the frame sequence number for ordering validation
	// Implementations should drop frames with seq <= last received seq
	SetAnnotatedFrame(cameraID string, seq uint64, frameData []byte)

	// UpdateDetections provides detection metadata (for client-side rendering)
	UpdateDetections(cameraID string, detections []Detection, faces []FaceDetection)

	// GetCurrentFrameSeq returns the current frame sequence for freshness checks
	GetCurrentFrameSeq(cameraID string) uint64
}

// DetectorRegistry manages available detectors
type DetectorRegistry interface {
	// Register adds a detector to the registry
	Register(detector Detector) error

	// Get returns a detector by name
	Get(name string) (Detector, bool)

	// GetAll returns all registered detectors
	GetAll() []Detector

	// GetHealthy returns only healthy detectors
	GetHealthy() []Detector

	// GetHealthyByNames returns healthy detectors matching the given names, in order
	GetHealthyByNames(names []string) []Detector

	// Close releases all detector resources
	Close() error
}

// PipelineManager orchestrates the complete detection pipeline
type PipelineManager interface {
	// StartCamera initializes capture and detection for a camera
	// cameraConfig can be nil to use global defaults
	StartCamera(cameraID string, device string, cameraConfig *CameraDetectionConfig) error

	// StopCamera halts capture and detection for a camera
	StopCamera(cameraID string) error

	// UpdateConfig updates detection configuration for a camera
	// cameraConfig can be nil to use global defaults
	UpdateConfig(cameraID string, cameraConfig *CameraDetectionConfig) error

	// GetStats returns pipeline statistics
	GetStats(cameraID string) *PipelineStats

	// SubscribeResults registers a handler for detection results
	SubscribeResults(handler DetectionResultHandler) func() // Returns unsubscribe function

	// Close shuts down the pipeline manager
	Close() error
}

// PipelineStats contains pipeline performance metrics
type PipelineStats struct {
	CameraID           string
	CaptureStats       *CaptureStats
	DetectionsTotal    uint64
	DetectionsPerSec   float32
	AvgInferenceMs     float32
	LastDetectionTime  int64
	ActiveDetectors    []string
	CurrentMode        DetectionMode
}
