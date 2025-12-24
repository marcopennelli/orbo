package detection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"time"
)

// YOLODetector handles YOLO-powered object detection
type YOLODetector struct {
	endpoint      string
	client        *http.Client
	enabled       bool
	securityMode  bool
	confThreshold float32
	classesFilter string
	drawBoxes     bool
	healthCheck   time.Time
	mu            sync.RWMutex
}

// YOLODetection represents a single YOLO detection result
type YOLODetection struct {
	Class      string    `json:"class"`
	ClassID    int       `json:"class_id"`
	Confidence float32   `json:"confidence"`
	BBox       []float32 `json:"bbox"` // [x1, y1, x2, y2]
	Center     []float32 `json:"center"`
	Area       float32   `json:"area"`
}

// YOLOResult represents YOLO detection response
type YOLOResult struct {
	Detections      []YOLODetection `json:"detections"`
	Count           int             `json:"count"`
	InferenceTimeMs float32         `json:"inference_time_ms"`
	Device          string          `json:"device"`
	ModelSize       string          `json:"model_size"`
	ConfThreshold   float32         `json:"conf_threshold"`
	FilterApplied   string          `json:"filter_applied,omitempty"`
}

// YOLOSecurityResult represents security-focused detection response
type YOLOSecurityResult struct {
	Detections      []YOLODetection       `json:"detections"`
	Count           int                   `json:"count"`
	ThreatAnalysis  YOLOThreatAnalysis    `json:"threat_analysis"`
	InferenceTimeMs float32               `json:"inference_time_ms"`
	Device          string                `json:"device"`
	SecurityFilter  []string              `json:"security_filter"`
}

// YOLOThreatAnalysis contains threat categorization
type YOLOThreatAnalysis struct {
	HighPriority   []YOLODetection `json:"high_priority"`   // persons
	MediumPriority []YOLODetection `json:"medium_priority"` // vehicles
	LowPriority    []YOLODetection `json:"low_priority"`    // bikes
}

// YOLOHealthResponse represents health check response
type YOLOHealthResponse struct {
	Status      string `json:"status"`
	Device      string `json:"device"`
	GPUAvailable bool  `json:"gpu_available"`
	ModelLoaded bool   `json:"model_loaded"`
}

// YOLOConfig holds configuration for the detector
type YOLOConfig struct {
	Enabled             bool
	ServiceEndpoint     string
	ConfidenceThreshold float32
	SecurityMode        bool
	ClassesFilter       string
	DrawBoxes           bool
}

// YOLOAnnotatedResult represents annotated detection response with image
type YOLOAnnotatedResult struct {
	ImageData       []byte              `json:"-"`
	Detections      []YOLODetection     `json:"detections"`
	Count           int                 `json:"count"`
	InferenceTimeMs float32             `json:"inference_time_ms"`
	Device          string              `json:"device"`
	ThreatAnalysis  *YOLOThreatAnalysis `json:"threat_analysis,omitempty"`
}

// NewYOLODetector creates a new YOLO-powered detector
func NewYOLODetector(endpoint string) *YOLODetector {
	return &YOLODetector{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 15 * time.Second, // Longer timeout for GPU inference
		},
		enabled:       true,
		securityMode:  true,
		confThreshold: 0.5,
	}
}

// NewYOLODetectorWithConfig creates a new YOLO detector with configuration
func NewYOLODetectorWithConfig(cfg YOLOConfig) *YOLODetector {
	d := NewYOLODetector(cfg.ServiceEndpoint)
	d.enabled = cfg.Enabled
	d.securityMode = cfg.SecurityMode
	d.confThreshold = cfg.ConfidenceThreshold
	d.classesFilter = cfg.ClassesFilter
	d.drawBoxes = cfg.DrawBoxes
	return d
}

// IsHealthy checks if the YOLO service is available
func (yd *YOLODetector) IsHealthy() bool {
	yd.mu.RLock()
	if !yd.enabled {
		yd.mu.RUnlock()
		return false
	}

	// Cache health check for 30 seconds
	if time.Since(yd.healthCheck) < 30*time.Second {
		yd.mu.RUnlock()
		return true
	}
	yd.mu.RUnlock()

	resp, err := yd.client.Get(yd.endpoint + "/health")
	if err != nil {
		yd.mu.Lock()
		yd.enabled = false
		yd.mu.Unlock()
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var health YOLOHealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
			if health.ModelLoaded {
				yd.mu.Lock()
				yd.healthCheck = time.Now()
				yd.mu.Unlock()
				return true
			}
		}
	}

	yd.mu.Lock()
	yd.enabled = false
	yd.mu.Unlock()
	return false
}

// GetHealthInfo returns detailed health information
func (yd *YOLODetector) GetHealthInfo() (*YOLOHealthResponse, error) {
	resp, err := yd.client.Get(yd.endpoint + "/health")
	if err != nil {
		return nil, fmt.Errorf("failed to check YOLO health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YOLO health check returned status %d", resp.StatusCode)
	}

	var health YOLOHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode health response: %w", err)
	}

	return &health, nil
}

// DetectObjects performs YOLO object detection on an image
func (yd *YOLODetector) DetectObjects(imageData []byte, confThreshold float32) (*YOLOResult, error) {
	if !yd.IsHealthy() {
		return nil, fmt.Errorf("YOLO detection service unavailable")
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
	if confThreshold <= 0 {
		confThreshold = yd.confThreshold
	}
	w.WriteField("conf_threshold", fmt.Sprintf("%.3f", confThreshold))

	// Add classes filter if set
	yd.mu.RLock()
	if yd.classesFilter != "" {
		w.WriteField("classes_filter", yd.classesFilter)
	}
	yd.mu.RUnlock()

	w.Close()

	// Make request
	req, err := http.NewRequest("POST", yd.endpoint+"/detect", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := yd.client.Do(req)
	if err != nil {
		yd.mu.Lock()
		yd.enabled = false
		yd.mu.Unlock()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("YOLO detection failed: %s", string(body))
	}

	var result YOLOResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DetectSecurity performs security-focused detection (person, car, etc.)
func (yd *YOLODetector) DetectSecurity(imageData []byte, confThreshold float32) (*YOLOSecurityResult, error) {
	if !yd.IsHealthy() {
		return nil, fmt.Errorf("YOLO detection service unavailable")
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
	if confThreshold <= 0 {
		confThreshold = yd.confThreshold
	}
	w.WriteField("conf_threshold", fmt.Sprintf("%.3f", confThreshold))
	w.Close()

	// Make request to security endpoint
	req, err := http.NewRequest("POST", yd.endpoint+"/detect/security", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := yd.client.Do(req)
	if err != nil {
		yd.mu.Lock()
		yd.enabled = false
		yd.mu.Unlock()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("YOLO security detection failed: %s", string(body))
	}

	var result YOLOSecurityResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DetectSecurityAnnotated performs security detection and returns annotated image with bounding boxes
func (yd *YOLODetector) DetectSecurityAnnotated(imageData []byte, confThreshold float32) (*YOLOAnnotatedResult, error) {
	if !yd.IsHealthy() {
		return nil, fmt.Errorf("YOLO detection service unavailable")
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
	if confThreshold <= 0 {
		confThreshold = yd.confThreshold
	}
	w.WriteField("conf_threshold", fmt.Sprintf("%.3f", confThreshold))
	w.Close()

	// Make request to security annotated endpoint
	req, err := http.NewRequest("POST", yd.endpoint+"/detect/security/annotated?format=image", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := yd.client.Do(req)
	if err != nil {
		yd.mu.Lock()
		yd.enabled = false
		yd.mu.Unlock()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("YOLO security annotated detection failed: %s", string(body))
	}

	// Read annotated image
	annotatedImage, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read annotated image: %w", err)
	}

	// Parse detection metadata from headers
	count := 0
	if countStr := resp.Header.Get("X-Detection-Count"); countStr != "" {
		fmt.Sscanf(countStr, "%d", &count)
	}

	var inferenceTime float32
	if timeStr := resp.Header.Get("X-Inference-Time-Ms"); timeStr != "" {
		fmt.Sscanf(timeStr, "%f", &inferenceTime)
	}

	device := resp.Header.Get("X-Device")

	// Parse threat analysis from headers
	var highCount, mediumCount, lowCount int
	if h := resp.Header.Get("X-High-Priority-Count"); h != "" {
		fmt.Sscanf(h, "%d", &highCount)
	}
	if m := resp.Header.Get("X-Medium-Priority-Count"); m != "" {
		fmt.Sscanf(m, "%d", &mediumCount)
	}
	if l := resp.Header.Get("X-Low-Priority-Count"); l != "" {
		fmt.Sscanf(l, "%d", &lowCount)
	}

	result := &YOLOAnnotatedResult{
		ImageData:       annotatedImage,
		Count:           count,
		InferenceTimeMs: inferenceTime,
		Device:          device,
	}

	// Add threat analysis if any counts are present
	if highCount > 0 || mediumCount > 0 || lowCount > 0 {
		result.ThreatAnalysis = &YOLOThreatAnalysis{
			HighPriority:   make([]YOLODetection, highCount),
			MediumPriority: make([]YOLODetection, mediumCount),
			LowPriority:    make([]YOLODetection, lowCount),
		}
	}

	return result, nil
}

// Detect performs detection based on configured mode (security or general)
func (yd *YOLODetector) Detect(imageData []byte) (*YOLOResult, error) {
	yd.mu.RLock()
	securityMode := yd.securityMode
	confThreshold := yd.confThreshold
	yd.mu.RUnlock()

	if securityMode {
		secResult, err := yd.DetectSecurity(imageData, confThreshold)
		if err != nil {
			return nil, err
		}
		// Convert security result to standard result
		return &YOLOResult{
			Detections:      secResult.Detections,
			Count:           secResult.Count,
			InferenceTimeMs: secResult.InferenceTimeMs,
			Device:          secResult.Device,
			FilterApplied:   "security",
		}, nil
	}

	return yd.DetectObjects(imageData, confThreshold)
}

// UpdateConfig updates the detector configuration
func (yd *YOLODetector) UpdateConfig(cfg YOLOConfig) {
	yd.mu.Lock()
	defer yd.mu.Unlock()

	yd.enabled = cfg.Enabled
	if cfg.ServiceEndpoint != "" {
		yd.endpoint = cfg.ServiceEndpoint
	}
	yd.confThreshold = cfg.ConfidenceThreshold
	yd.securityMode = cfg.SecurityMode
	yd.classesFilter = cfg.ClassesFilter
	yd.drawBoxes = cfg.DrawBoxes

	// Reset health check to force re-validation
	yd.healthCheck = time.Time{}
}

// GetConfig returns current configuration
func (yd *YOLODetector) GetConfig() YOLOConfig {
	yd.mu.RLock()
	defer yd.mu.RUnlock()

	return YOLOConfig{
		Enabled:             yd.enabled,
		ServiceEndpoint:     yd.endpoint,
		ConfidenceThreshold: yd.confThreshold,
		SecurityMode:        yd.securityMode,
		ClassesFilter:       yd.classesFilter,
		DrawBoxes:           yd.drawBoxes,
	}
}

// DrawBoxesEnabled returns whether bounding boxes should be drawn
func (yd *YOLODetector) DrawBoxesEnabled() bool {
	yd.mu.RLock()
	defer yd.mu.RUnlock()
	return yd.drawBoxes
}

// GetEndpoint returns the service endpoint
func (yd *YOLODetector) GetEndpoint() string {
	yd.mu.RLock()
	defer yd.mu.RUnlock()
	return yd.endpoint
}

// SetEnabled enables or disables the detector
func (yd *YOLODetector) SetEnabled(enabled bool) {
	yd.mu.Lock()
	defer yd.mu.Unlock()
	yd.enabled = enabled
}

// IsEnabled returns whether the detector is enabled
func (yd *YOLODetector) IsEnabled() bool {
	yd.mu.RLock()
	defer yd.mu.RUnlock()
	return yd.enabled
}

// ShouldAlert determines if a YOLO detection should trigger an alert
func (yd *YOLODetector) ShouldAlert(result *YOLOResult) bool {
	if result.Count == 0 {
		return false
	}

	// Alert if any high-confidence person detected
	for _, det := range result.Detections {
		if det.Class == "person" && det.Confidence > 0.7 {
			return true
		}
	}

	// Alert if multiple security-relevant objects
	securityCount := 0
	for _, det := range result.Detections {
		switch det.Class {
		case "person", "car", "truck", "bus", "motorcycle", "bicycle":
			securityCount++
		}
	}

	return securityCount >= 2
}

// GetThreatLevel returns threat level based on YOLO detection
func (yd *YOLODetector) GetThreatLevel(result *YOLOResult) string {
	if result.Count == 0 {
		return "none"
	}

	personCount := 0
	vehicleCount := 0

	for _, det := range result.Detections {
		switch det.Class {
		case "person":
			if det.Confidence > 0.7 {
				personCount++
			}
		case "car", "truck", "bus":
			vehicleCount++
		}
	}

	if personCount >= 2 {
		return "high"
	}
	if personCount >= 1 {
		return "medium"
	}
	if vehicleCount >= 1 {
		return "low"
	}

	return "none"
}
