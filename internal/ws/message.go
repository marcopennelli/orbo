package ws

import "time"

// DetectionMessage represents an object detection broadcast
type DetectionMessage struct {
	Type        string            `json:"type"` // "detection"
	CameraID    string            `json:"camera_id"`
	Timestamp   time.Time         `json:"timestamp"`
	FrameWidth  int               `json:"frame_width"`
	FrameHeight int               `json:"frame_height"`
	Objects     []ObjectDetection `json:"objects"`
	Frame       string            `json:"frame,omitempty"` // Base64 encoded JPEG frame
}

// ObjectDetection represents a single detected object
type ObjectDetection struct {
	Class       string    `json:"class"`                  // "person", "car", etc.
	Confidence  float32   `json:"confidence"`             // 0.0-1.0
	BBox        []float32 `json:"bbox"`                   // [x, y, w, h] in pixels
	ThreatLevel string    `json:"threat_level,omitempty"` // "low", "medium", "high"
}

// FaceMessage represents a face recognition broadcast
type FaceMessage struct {
	Type      string          `json:"type"` // "face"
	CameraID  string          `json:"camera_id"`
	Timestamp time.Time       `json:"timestamp"`
	Faces     []FaceDetection `json:"faces"`
}

// FaceDetection represents a single detected/recognized face
type FaceDetection struct {
	BBox       []float32 `json:"bbox"`                 // [x, y, w, h] in pixels
	Confidence float32   `json:"confidence"`           // Detection confidence 0.0-1.0
	Identity   *string   `json:"identity,omitempty"`   // Name if known, nil if unknown
	IsKnown    bool      `json:"is_known"`             // Whether face was recognized
	Similarity *float32  `json:"similarity,omitempty"` // Match similarity score
	Age        *int      `json:"age,omitempty"`        // Estimated age
	Gender     *string   `json:"gender,omitempty"`     // "male", "female", "unknown"
}

// NewDetectionMessage creates a new detection message
func NewDetectionMessage(cameraID string, frameWidth, frameHeight int) *DetectionMessage {
	return &DetectionMessage{
		Type:        "detection",
		CameraID:    cameraID,
		Timestamp:   time.Now(),
		FrameWidth:  frameWidth,
		FrameHeight: frameHeight,
		Objects:     make([]ObjectDetection, 0),
	}
}

// SetFrame sets the base64-encoded frame data
func (m *DetectionMessage) SetFrame(frameBase64 string) {
	m.Frame = frameBase64
}

// FrameMessage represents a video frame broadcast (no detections)
type FrameMessage struct {
	Type        string    `json:"type"` // "frame"
	CameraID    string    `json:"camera_id"`
	Timestamp   time.Time `json:"timestamp"`
	FrameWidth  int       `json:"frame_width"`
	FrameHeight int       `json:"frame_height"`
	Frame       string    `json:"frame"` // Base64 encoded JPEG frame
}

// NewFrameMessage creates a new frame message for live streaming
func NewFrameMessage(cameraID string, frameWidth, frameHeight int, frameBase64 string) *FrameMessage {
	return &FrameMessage{
		Type:        "frame",
		CameraID:    cameraID,
		Timestamp:   time.Now(),
		FrameWidth:  frameWidth,
		FrameHeight: frameHeight,
		Frame:       frameBase64,
	}
}

// NewFaceMessage creates a new face message
func NewFaceMessage(cameraID string) *FaceMessage {
	return &FaceMessage{
		Type:      "face",
		CameraID:  cameraID,
		Timestamp: time.Now(),
		Faces:     make([]FaceDetection, 0),
	}
}

// AddObject adds an object detection to the message
func (m *DetectionMessage) AddObject(class string, confidence float32, bbox []float32, threatLevel string) {
	m.Objects = append(m.Objects, ObjectDetection{
		Class:       class,
		Confidence:  confidence,
		BBox:        bbox,
		ThreatLevel: threatLevel,
	})
}

// AddFace adds a face detection to the message
func (m *FaceMessage) AddFace(bbox []float32, confidence float32, identity *string, isKnown bool, similarity *float32, age *int, gender *string) {
	m.Faces = append(m.Faces, FaceDetection{
		BBox:       bbox,
		Confidence: confidence,
		Identity:   identity,
		IsKnown:    isKnown,
		Similarity: similarity,
		Age:        age,
		Gender:     gender,
	})
}
