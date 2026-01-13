package pipeline

import (
	"time"
)

// DetectionMode defines when detection should run for a camera
type DetectionMode string

const (
	// DetectionModeDisabled - no detection, streaming only
	DetectionModeDisabled DetectionMode = "disabled"
	// DetectionModeVisualOnly - run detection for bounding boxes but don't send alerts
	DetectionModeVisualOnly DetectionMode = "visual_only"
	// DetectionModeContinuous - run detection on every frame
	DetectionModeContinuous DetectionMode = "continuous"
	// DetectionModeMotionTriggered - detect only when motion is detected (current behavior)
	DetectionModeMotionTriggered DetectionMode = "motion_triggered"
	// DetectionModeScheduled - run detection at fixed intervals
	DetectionModeScheduled DetectionMode = "scheduled"
	// DetectionModeHybrid - run on motion OR at scheduled intervals
	DetectionModeHybrid DetectionMode = "hybrid"
)

// ExecutionMode defines how detectors are executed
type ExecutionMode string

const (
	// ExecutionModeSequential - chain detectors: YOLO → Face (if person) → Plate (if vehicle)
	// This is the only supported mode. Parallel mode was removed because it causes
	// time-travel effects (mixed latencies) and cannot properly merge annotations.
	ExecutionModeSequential ExecutionMode = "sequential"
)

// FrameData represents a captured video frame
type FrameData struct {
	CameraID  string    // Camera identifier
	Data      []byte    // JPEG frame data
	Seq       uint64    // Frame sequence number
	Timestamp time.Time // Capture timestamp
	Width     int       // Frame width (if known)
	Height    int       // Frame height (if known)
}

// DetectorType identifies the type of detector
type DetectorType string

const (
	DetectorTypeYOLO  DetectorType = "yolo"
	DetectorTypeFace  DetectorType = "face"
	DetectorTypePlate DetectorType = "plate"
	DetectorTypeDINO  DetectorType = "dino"
)

// BBox represents a bounding box in normalized or pixel coordinates
type BBox struct {
	X1 float32 `json:"x1"` // Left
	Y1 float32 `json:"y1"` // Top
	X2 float32 `json:"x2"` // Right
	Y2 float32 `json:"y2"` // Bottom
}

// Detection represents a single object detection result
type Detection struct {
	Class      string                 `json:"class"`      // Detection class (person, car, etc.)
	Confidence float32                `json:"confidence"` // Detection confidence [0-1]
	BBox       BBox                   `json:"bbox"`       // Bounding box
	TrackID    *int                   `json:"track_id"`   // Optional tracking ID
	Metadata   map[string]interface{} `json:"metadata"`   // Additional detector-specific data
}

// FaceDetection represents a face detection/recognition result
type FaceDetection struct {
	BBox              BBox      `json:"bbox"`                // Face bounding box
	Confidence        float32   `json:"confidence"`          // Detection confidence
	PersonID          *string   `json:"person_id"`           // Recognized person ID (nil if unknown)
	PersonName        *string   `json:"person_name"`         // Recognized person name (nil if unknown)
	Similarity        float32   `json:"similarity"`          // Similarity score to matched face
	Age               *int      `json:"age"`                 // Estimated age (if available)
	Gender            *string   `json:"gender"`              // Estimated gender (if available)
	Embedding         []float32 `json:"-"`                   // Face embedding (not serialized)
	AssociatedTrackID int       `json:"associated_track_id"` // YOLO person track ID this face belongs to
}

// PlateDetection represents a license plate detection result
type PlateDetection struct {
	BBox       BBox    `json:"bbox"`       // Plate bounding box
	Confidence float32 `json:"confidence"` // Detection confidence
	PlateText  string  `json:"plate_text"` // Recognized plate text
	Country    *string `json:"country"`    // Detected country/region
}

// DetectionResult contains aggregated results from one or more detectors
type DetectionResult struct {
	CameraID     string           `json:"camera_id"`
	FrameSeq     uint64           `json:"frame_seq"`
	Timestamp    time.Time        `json:"timestamp"`
	DetectorType DetectorType     `json:"detector_type"` // Primary detector that produced this result
	Detections   []Detection      `json:"detections"`    // Object detections (YOLO/DINO)
	Faces        []FaceDetection  `json:"faces"`         // Face detections
	Plates       []PlateDetection `json:"plates"`        // Plate detections
	ImageData    []byte           `json:"-"`             // Annotated image (optional, not serialized)
	InferenceMs  float32          `json:"inference_ms"`  // Inference time in milliseconds
	HasMotion    bool             `json:"has_motion"`    // Motion was detected in frame
}

// MergedDetectionResult combines results from multiple detectors
type MergedDetectionResult struct {
	CameraID      string             `json:"camera_id"`
	FrameSeq      uint64             `json:"frame_seq"`
	Timestamp     time.Time          `json:"timestamp"`
	Results       []*DetectionResult `json:"results"`       // Individual detector results
	Detections    []Detection        `json:"detections"`    // Merged object detections
	Faces         []FaceDetection    `json:"faces"`         // Merged face detections
	Plates        []PlateDetection   `json:"plates"`        // Merged plate detections
	ImageData     []byte             `json:"-"`             // Annotated image with all overlays
	TotalInferenceMs float32         `json:"total_inference_ms"`
	HasMotion     bool               `json:"has_motion"`
}

// CameraDetectionConfig contains per-camera detection configuration
// Nil/zero values mean "inherit from global config"
type CameraDetectionConfig struct {
	Mode              *DetectionMode  `json:"mode,omitempty"`               // Detection mode
	ExecutionMode     *ExecutionMode  `json:"execution_mode,omitempty"`     // Sequential or parallel
	Detectors         []string        `json:"detectors,omitempty"`          // Enabled detectors
	ScheduleInterval  *time.Duration  `json:"schedule_interval,omitempty"`  // For scheduled/hybrid mode
	MotionSensitivity *float32        `json:"motion_sensitivity,omitempty"` // Motion detection threshold
	MotionCooldownMs  *int            `json:"motion_cooldown_ms,omitempty"` // Cooldown after motion stops (milliseconds)
	YOLOConfidence    *float32        `json:"yolo_confidence,omitempty"`    // YOLO detection threshold
	EnableFaceRecog   *bool           `json:"enable_face_recog,omitempty"`  // Enable face recognition
	EnablePlateRecog  *bool           `json:"enable_plate_recog,omitempty"` // Enable plate recognition
}

// GlobalDetectionConfig contains global default detection settings
type GlobalDetectionConfig struct {
	Mode              DetectionMode `json:"mode"`
	ExecutionMode     ExecutionMode `json:"execution_mode"`
	Detectors         []string      `json:"detectors"`
	ScheduleInterval  time.Duration `json:"schedule_interval"`
	MotionSensitivity float32       `json:"motion_sensitivity"`
	MotionCooldownMs  int           `json:"motion_cooldown_ms"` // Cooldown in milliseconds after motion stops
	YOLOConfidence    float32       `json:"yolo_confidence"`
	EnableFaceRecog   bool          `json:"enable_face_recog"`
	EnablePlateRecog  bool          `json:"enable_plate_recog"`
}

// EffectiveConfig represents the merged configuration for a camera
// (camera overrides applied to global defaults)
type EffectiveConfig struct {
	CameraID          string
	Mode              DetectionMode
	ExecutionMode     ExecutionMode
	Detectors         []string
	ScheduleInterval  time.Duration
	MotionSensitivity float32
	MotionCooldownMs  int // Cooldown in milliseconds after motion stops
	YOLOConfidence    float32
	EnableFaceRecog   bool
	EnablePlateRecog  bool
}

// DefaultGlobalConfig returns sensible defaults for global detection config
func DefaultGlobalConfig() *GlobalDetectionConfig {
	return &GlobalDetectionConfig{
		Mode:              DetectionModeMotionTriggered,
		ExecutionMode:     ExecutionModeSequential,
		Detectors:         []string{"yolo", "face"},
		ScheduleInterval:  5 * time.Second,
		MotionSensitivity: 0.1,
		MotionCooldownMs:  2000, // 2 second default cooldown
		YOLOConfidence:    0.5,
		EnableFaceRecog:   true,
		EnablePlateRecog:  false,
	}
}

// MergeWithGlobal merges camera-specific config with global defaults
func (c *CameraDetectionConfig) MergeWithGlobal(cameraID string, global *GlobalDetectionConfig) *EffectiveConfig {
	if global == nil {
		global = DefaultGlobalConfig()
	}

	effective := &EffectiveConfig{
		CameraID:          cameraID,
		Mode:              global.Mode,
		ExecutionMode:     global.ExecutionMode,
		Detectors:         global.Detectors,
		ScheduleInterval:  global.ScheduleInterval,
		MotionSensitivity: global.MotionSensitivity,
		MotionCooldownMs:  global.MotionCooldownMs,
		YOLOConfidence:    global.YOLOConfidence,
		EnableFaceRecog:   global.EnableFaceRecog,
		EnablePlateRecog:  global.EnablePlateRecog,
	}

	if c == nil {
		return effective
	}

	// Apply camera-specific overrides
	if c.Mode != nil {
		effective.Mode = *c.Mode
	}
	if c.ExecutionMode != nil {
		effective.ExecutionMode = *c.ExecutionMode
	}
	if len(c.Detectors) > 0 {
		effective.Detectors = c.Detectors
	}
	if c.ScheduleInterval != nil {
		effective.ScheduleInterval = *c.ScheduleInterval
	}
	if c.MotionSensitivity != nil {
		effective.MotionSensitivity = *c.MotionSensitivity
	}
	if c.MotionCooldownMs != nil {
		effective.MotionCooldownMs = *c.MotionCooldownMs
	}
	if c.YOLOConfidence != nil {
		effective.YOLOConfidence = *c.YOLOConfidence
	}
	if c.EnableFaceRecog != nil {
		effective.EnableFaceRecog = *c.EnableFaceRecog
	}
	if c.EnablePlateRecog != nil {
		effective.EnablePlateRecog = *c.EnablePlateRecog
	}

	return effective
}

// HasDetector checks if a detector type is enabled in the effective config
func (e *EffectiveConfig) HasDetector(detectorType string) bool {
	for _, d := range e.Detectors {
		if d == detectorType {
			return true
		}
	}
	return false
}
