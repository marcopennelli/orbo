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

// DINOv3Detector handles DINOv3-powered motion detection and scene analysis
type DINOv3Detector struct {
	endpoint    string
	client      *http.Client
	enabled     bool
	healthCheck time.Time
}

// DINOv3Detection represents a DINOv3 detection result
type DINOv3Detection struct {
	Class           string    `json:"class"`
	Confidence      float32   `json:"confidence"`
	BBox            []float32 `json:"bbox"`   // [x1, y1, x2, y2]
	MotionStrength  float32   `json:"motion_strength"`
	MotionType      string    `json:"motion_type"`
}

// DINOv3MotionResult represents DINOv3 motion detection response
type DINOv3MotionResult struct {
	MotionDetected      bool                `json:"motion_detected"`
	Confidence          float32             `json:"confidence"`
	FeatureSimilarity   float32             `json:"feature_similarity"`
	ChangeRegions       []DINOv3Detection   `json:"change_regions"`
	SceneAnalysis       SceneAnalysis       `json:"scene_analysis"`
	InferenceTimeMs     float32             `json:"inference_time_ms"`
	Device              string              `json:"device"`
	Model               string              `json:"model"`
	Threshold           float32             `json:"threshold"`
}

// SceneAnalysis represents scene understanding from DINOv3
type SceneAnalysis struct {
	SceneType           string                 `json:"scene_type"`
	ComplexityScore     float32                `json:"complexity_score"`
	FeatureStatistics   FeatureStatistics      `json:"feature_statistics"`
	MotionAnalysis      *MotionAnalysis        `json:"motion_analysis,omitempty"`
}

// FeatureStatistics contains DINOv3 feature statistics
type FeatureStatistics struct {
	Norm float32 `json:"norm"`
	Mean float32 `json:"mean"`
	Std  float32 `json:"std"`
}

// MotionAnalysis contains detailed motion analysis
type MotionAnalysis struct {
	MotionStrength       float32                 `json:"motion_strength"`
	MotionType           string                  `json:"motion_type"`
	AffectedRegions      []map[string]interface{} `json:"affected_regions"`
	TemporalConsistency  float32                 `json:"temporal_consistency"`
}

// DINOv3FeatureResult represents feature extraction response
type DINOv3FeatureResult struct {
	Features          []float32 `json:"features"`
	FeatureDimension  int       `json:"feature_dimension"`
	InferenceTimeMs   float32   `json:"inference_time_ms"`
	Device            string    `json:"device"`
}

// NewDINOv3Detector creates a new DINOv3-powered detector
func NewDINOv3Detector(endpoint string) *DINOv3Detector {
	return &DINOv3Detector{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 10 * time.Second, // Longer timeout for AI inference
		},
		enabled: true,
	}
}

// IsHealthy checks if the DINOv3 service is available
func (dd *DINOv3Detector) IsHealthy() bool {
	if !dd.enabled {
		return false
	}

	// Cache health check for 30 seconds
	if time.Since(dd.healthCheck) < 30*time.Second {
		return true
	}

	resp, err := dd.client.Get(dd.endpoint + "/health")
	if err != nil {
		dd.enabled = false
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		dd.healthCheck = time.Now()
		return true
	}

	dd.enabled = false
	return false
}

// DetectMotion performs DINOv3-powered motion detection
func (dd *DINOv3Detector) DetectMotion(imageData []byte, cameraID string, threshold float32) (*DINOv3MotionResult, error) {
	if !dd.IsHealthy() {
		return nil, fmt.Errorf("DINOv3 detection service unavailable")
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

	// Add camera ID and threshold
	w.WriteField("camera_id", cameraID)
	w.WriteField("threshold", fmt.Sprintf("%.3f", threshold))
	w.Close()

	// Make request
	req, err := http.NewRequest("POST", dd.endpoint+"/detect/motion", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := dd.client.Do(req)
	if err != nil {
		dd.enabled = false
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DINOv3 motion detection failed: %s", string(body))
	}

	var result DINOv3MotionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ExtractFeatures extracts DINOv3 feature embeddings from an image
func (dd *DINOv3Detector) ExtractFeatures(imageData []byte) (*DINOv3FeatureResult, error) {
	if !dd.IsHealthy() {
		return nil, fmt.Errorf("DINOv3 detection service unavailable")
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
	w.Close()

	// Make request
	req, err := http.NewRequest("POST", dd.endpoint+"/extract/features", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := dd.client.Do(req)
	if err != nil {
		dd.enabled = false
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DINOv3 feature extraction failed: %s", string(body))
	}

	var result DINOv3FeatureResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// AnalyzeScene performs DINOv3-powered scene analysis
func (dd *DINOv3Detector) AnalyzeScene(imageData []byte) (*SceneAnalysis, error) {
	if !dd.IsHealthy() {
		return nil, fmt.Errorf("DINOv3 detection service unavailable")
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
	w.Close()

	// Make request
	req, err := http.NewRequest("POST", dd.endpoint+"/analyze/scene", &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := dd.client.Do(req)
	if err != nil {
		dd.enabled = false
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DINOv3 scene analysis failed: %s", string(body))
	}

	var response struct {
		SceneAnalysis   SceneAnalysis `json:"scene_analysis"`
		InferenceTimeMs float32       `json:"inference_time_ms"`
		Device          string        `json:"device"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return &response.SceneAnalysis, nil
}

// ShouldAlert determines if a DINOv3 detection should trigger an alert
func (dd *DINOv3Detector) ShouldAlert(result *DINOv3MotionResult) bool {
	if !result.MotionDetected {
		return false
	}

	// High confidence motion detection
	if result.Confidence > 0.8 {
		return true
	}

	// Check motion analysis for significant changes
	if result.SceneAnalysis.MotionAnalysis != nil {
		motion := result.SceneAnalysis.MotionAnalysis
		
		// Alert on significant motion types
		switch motion.MotionType {
		case "significant_change", "object_motion":
			return motion.MotionStrength > 0.6
		case "subtle_motion":
			return motion.MotionStrength > 0.8
		}
	}

	// Alert on low feature similarity (indicating significant change)
	return result.FeatureSimilarity < 0.7
}

// GetThreatLevel returns threat level based on DINOv3 analysis
func (dd *DINOv3Detector) GetThreatLevel(result *DINOv3MotionResult) string {
	if !result.MotionDetected {
		return "none"
	}

	// Analyze motion characteristics
	if result.SceneAnalysis.MotionAnalysis != nil {
		motion := result.SceneAnalysis.MotionAnalysis
		
		switch motion.MotionType {
		case "significant_change":
			return "high"
		case "object_motion":
			if motion.MotionStrength > 0.8 {
				return "high"
			}
			return "medium"
		case "subtle_motion":
			return "low"
		case "environmental":
			return "none"
		}
	}

	// Fallback based on confidence
	if result.Confidence > 0.8 {
		return "high"
	} else if result.Confidence > 0.5 {
		return "medium"
	}
	
	return "low"
}

// GetMotionType returns a human-readable motion type
func (dd *DINOv3Detector) GetMotionType(result *DINOv3MotionResult) string {
	if result.SceneAnalysis.MotionAnalysis != nil {
		switch result.SceneAnalysis.MotionAnalysis.MotionType {
		case "significant_change":
			return "Major scene change"
		case "object_motion":
			return "Object movement"
		case "subtle_motion":
			return "Subtle movement"
		case "environmental":
			return "Environmental change"
		}
	}
	
	return "Unknown motion"
}