package detection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// GPUDetector handles AMD GPU-accelerated object detection
type GPUDetector struct {
	endpoint    string
	client      *http.Client
	enabled     bool
	healthCheck time.Time
}

// Detection represents a detected object
type Detection struct {
	Class      string    `json:"class"`
	ClassID    int       `json:"class_id"`
	Confidence float32   `json:"confidence"`
	BBox       []float32 `json:"bbox"`   // [x1, y1, x2, y2]
	Center     []float32 `json:"center"` // [center_x, center_y]
	Area       float32   `json:"area"`
}

// DetectionResult represents the full detection response
type DetectionResult struct {
	Detections      []Detection `json:"detections"`
	Count           int         `json:"count"`
	InferenceTimeMs float32     `json:"inference_time_ms"`
	Device          string      `json:"device"`
	ModelSize       string      `json:"model_size"`
	ConfThreshold   float32     `json:"conf_threshold"`
}

// SecurityDetectionResult represents security-focused detection response
type SecurityDetectionResult struct {
	Detections      []Detection    `json:"detections"`
	Count           int            `json:"count"`
	ThreatAnalysis  ThreatAnalysis `json:"threat_analysis"`
	InferenceTimeMs float32        `json:"inference_time_ms"`
	Device          string         `json:"device"`
	SecurityFilter  []string       `json:"security_filter"`
}

// ThreatAnalysis categorizes detections by priority
type ThreatAnalysis struct {
	HighPriority   []Detection `json:"high_priority"`   // persons
	MediumPriority []Detection `json:"medium_priority"` // vehicles
	LowPriority    []Detection `json:"low_priority"`    // bikes, motorcycles
}

// NewGPUDetector creates a new GPU-accelerated object detector
func NewGPUDetector(endpoint string) *GPUDetector {
	return &GPUDetector{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		enabled: true,
	}
}

// IsHealthy checks if the GPU detection service is available
func (gd *GPUDetector) IsHealthy() bool {
	if !gd.enabled {
		return false
	}

	// Cache health check for 30 seconds
	if time.Since(gd.healthCheck) < 30*time.Second {
		return true
	}

	resp, err := gd.client.Get(gd.endpoint + "/health")
	if err != nil {
		gd.enabled = false
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		gd.healthCheck = time.Now()
		return true
	}

	gd.enabled = false
	return false
}

// DetectObjects performs general object detection
func (gd *GPUDetector) DetectObjects(imageData []byte, confThreshold float32) (*DetectionResult, error) {
	if !gd.IsHealthy() {
		return nil, fmt.Errorf("GPU detection service unavailable")
	}

	// Create multipart form data
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add image file
	fw, err := w.CreateFormFile("file", "frame.jpg")
	if err != nil {
		return nil, err
	}
	fw.Write(imageData)

	// Add confidence threshold
	w.WriteField("conf_threshold", fmt.Sprintf("%.2f", confThreshold))
	w.Close()

	// Make request
	req, err := http.NewRequest("POST", gd.endpoint+"/detect", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := gd.client.Do(req)
	if err != nil {
		gd.enabled = false
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("detection failed: %s", string(body))
	}

	var result DetectionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DetectSecurityObjects performs security-focused object detection
func (gd *GPUDetector) DetectSecurityObjects(imageData []byte, confThreshold float32) (*SecurityDetectionResult, error) {
	if !gd.IsHealthy() {
		return nil, fmt.Errorf("GPU detection service unavailable")
	}

	// Create multipart form data
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add image file
	fw, err := w.CreateFormFile("file", "frame.jpg")
	if err != nil {
		return nil, err
	}
	fw.Write(imageData)

	// Add confidence threshold
	w.WriteField("conf_threshold", fmt.Sprintf("%.2f", confThreshold))
	w.Close()

	// Make request to security endpoint
	req, err := http.NewRequest("POST", gd.endpoint+"/detect/security", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := gd.client.Do(req)
	if err != nil {
		gd.enabled = false
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("security detection failed: %s", string(body))
	}

	var result SecurityDetectionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ShouldAlert determines if a detection should trigger an alert
func (gd *GPUDetector) ShouldAlert(detection Detection) bool {
	switch detection.Class {
	case "person":
		return detection.Confidence > 0.6
	case "car", "truck", "bus":
		return detection.Confidence > 0.7
	case "bicycle", "motorcycle":
		return detection.Confidence > 0.5
	case "cat", "dog":
		return detection.Confidence > 0.8 // Only very confident animal detections
	default:
		return false
	}
}

// GetThreatLevel returns the threat level for a detection
func (gd *GPUDetector) GetThreatLevel(detection Detection) string {
	switch detection.Class {
	case "person":
		return "high"
	case "car", "truck", "bus":
		return "medium"
	case "bicycle", "motorcycle":
		return "low"
	default:
		return "none"
	}
}