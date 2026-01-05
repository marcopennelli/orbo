package detection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sync"
	"time"
)

// FaceRecognizer handles face detection and recognition via the recognition service
type FaceRecognizer struct {
	endpoint   string
	client     *http.Client
	enabled    bool
	threshold  float32 // similarity threshold for matching
	mu         sync.RWMutex
	healthy    bool
	lastHealth time.Time
}

// FaceRecognizerConfig holds configuration for the face recognition service
type FaceRecognizerConfig struct {
	Enabled             bool
	ServiceEndpoint     string
	SimilarityThreshold float32
}

// FaceDetection represents a detected face
type FaceDetection struct {
	BBox       []float32 `json:"bbox"`
	Confidence float32   `json:"confidence"`
	Center     []float32 `json:"center,omitempty"`
	Area       float32   `json:"area,omitempty"`
	Age        int       `json:"age,omitempty"`
	Gender     string    `json:"gender,omitempty"`
}

// FaceRecognition represents a recognized face
type FaceRecognition struct {
	BBox       []float32 `json:"bbox"`
	Confidence float32   `json:"confidence"`
	Identity   *string   `json:"identity"`
	Similarity float32   `json:"similarity"`
	IsKnown    bool      `json:"is_known"`
	Age        int       `json:"age,omitempty"`
	Gender     string    `json:"gender,omitempty"`
}

// FaceRecognitionResult represents the result of face recognition
type FaceRecognitionResult struct {
	Recognitions        []FaceRecognition `json:"recognitions"`
	Count               int               `json:"count"`
	KnownCount          int               `json:"known_count"`
	UnknownCount        int               `json:"unknown_count"`
	InferenceTimeMs     float32           `json:"inference_time_ms"`
	Device              string            `json:"device"`
	SimilarityThreshold float32           `json:"similarity_threshold"`
}

// FaceDetectResult represents the result of face detection (without recognition)
type FaceDetectResult struct {
	Faces           []FaceDetection `json:"faces"`
	Count           int             `json:"count"`
	InferenceTimeMs float32         `json:"inference_time_ms"`
	Device          string          `json:"device"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status          string `json:"status"`
	Device          string `json:"device"`
	ModelLoaded     bool   `json:"model_loaded"`
	KnownFacesCount int    `json:"known_faces_count"`
}

// ForensicFaceAnalysis represents a single face with forensic analysis
type ForensicFaceAnalysis struct {
	Index        int      `json:"index"`
	BBox         []int    `json:"bbox"`
	Confidence   float32  `json:"confidence"`
	Identity     *string  `json:"identity"`
	Similarity   *float32 `json:"similarity"`
	IsKnown      bool     `json:"is_known"`
	ImageBase64  string   `json:"image_base64"`
	HasLandmarks bool     `json:"has_landmarks"`
	Age          int      `json:"age,omitempty"`
	Gender       string   `json:"gender,omitempty"`
}

// ForensicAnalysisResult represents the forensic face analysis result
type ForensicAnalysisResult struct {
	Faces           []ForensicFaceAnalysis `json:"faces"`
	Count           int                    `json:"count"`
	KnownCount      int                    `json:"known_count"`
	UnknownCount    int                    `json:"unknown_count"`
	InferenceTimeMs float32                `json:"inference_time_ms"`
	Device          string                 `json:"device"`
}

// NewFaceRecognizer creates a new face recognition client
func NewFaceRecognizer(config FaceRecognizerConfig) *FaceRecognizer {
	threshold := config.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.5 // default threshold
	}

	return &FaceRecognizer{
		endpoint:  config.ServiceEndpoint,
		enabled:   config.Enabled,
		threshold: threshold,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsEnabled returns whether face recognition is enabled
func (fr *FaceRecognizer) IsEnabled() bool {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.enabled
}

// SetEnabled enables or disables face recognition
func (fr *FaceRecognizer) SetEnabled(enabled bool) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.enabled = enabled
}

// IsHealthy returns whether the service is healthy
func (fr *FaceRecognizer) IsHealthy() bool {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return fr.healthy
}

// CheckHealth checks if the face recognition service is available
func (fr *FaceRecognizer) CheckHealth() error {
	if !fr.enabled {
		return fmt.Errorf("face recognition is disabled")
	}

	url := fmt.Sprintf("%s/health", fr.endpoint)
	resp, err := fr.client.Get(url)
	if err != nil {
		fr.mu.Lock()
		fr.healthy = false
		fr.mu.Unlock()
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fr.mu.Lock()
		fr.healthy = false
		fr.mu.Unlock()
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		fr.mu.Lock()
		fr.healthy = false
		fr.mu.Unlock()
		return fmt.Errorf("failed to decode health response: %w", err)
	}

	fr.mu.Lock()
	fr.healthy = health.Status == "healthy" && health.ModelLoaded
	fr.lastHealth = time.Now()
	fr.mu.Unlock()

	if !fr.healthy {
		return fmt.Errorf("service unhealthy: status=%s, model_loaded=%v", health.Status, health.ModelLoaded)
	}

	return nil
}

// DetectFaces detects faces in an image without recognition
func (fr *FaceRecognizer) DetectFaces(imageData []byte) (*FaceDetectResult, error) {
	if !fr.enabled {
		return nil, fmt.Errorf("face recognition is disabled")
	}

	url := fmt.Sprintf("%s/detect", fr.endpoint)
	result, err := fr.sendImageRequest(url, imageData)
	if err != nil {
		return nil, err
	}

	var detectResult FaceDetectResult
	if err := json.Unmarshal(result, &detectResult); err != nil {
		return nil, fmt.Errorf("failed to decode detect response: %w", err)
	}

	return &detectResult, nil
}

// RecognizeFaces detects and recognizes faces in an image
func (fr *FaceRecognizer) RecognizeFaces(imageData []byte) (*FaceRecognitionResult, error) {
	if !fr.enabled {
		return nil, fmt.Errorf("face recognition is disabled")
	}

	url := fmt.Sprintf("%s/recognize", fr.endpoint)
	result, err := fr.sendImageRequest(url, imageData)
	if err != nil {
		return nil, err
	}

	var recognizeResult FaceRecognitionResult
	if err := json.Unmarshal(result, &recognizeResult); err != nil {
		return nil, fmt.Errorf("failed to decode recognize response: %w", err)
	}

	return &recognizeResult, nil
}

// AnalyzeForensic performs NSA-style forensic face analysis with landmarks overlay
func (fr *FaceRecognizer) AnalyzeForensic(imageData []byte) (*ForensicAnalysisResult, error) {
	if !fr.enabled {
		return nil, fmt.Errorf("face recognition is disabled")
	}

	url := fmt.Sprintf("%s/analyze/forensic", fr.endpoint)
	result, err := fr.sendImageRequest(url, imageData)
	if err != nil {
		return nil, err
	}

	var forensicResult ForensicAnalysisResult
	if err := json.Unmarshal(result, &forensicResult); err != nil {
		return nil, fmt.Errorf("failed to decode forensic response: %w", err)
	}

	return &forensicResult, nil
}

// sendImageRequest sends an image to a recognition endpoint
func (fr *FaceRecognizer) sendImageRequest(url string, imageData []byte) ([]byte, error) {
	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add image file with proper Content-Type header
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="frame.jpg"`)
	h.Set("Content-Type", "image/jpeg")
	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(imageData); err != nil {
		return nil, fmt.Errorf("failed to write image data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Send request
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := fr.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetKnownIdentities returns a list of names from the recognition result
func (result *FaceRecognitionResult) GetKnownIdentities() []string {
	var identities []string
	for _, rec := range result.Recognitions {
		if rec.IsKnown && rec.Identity != nil {
			identities = append(identities, *rec.Identity)
		}
	}
	return identities
}

// HasUnknownFaces returns true if there are unknown faces in the result
func (result *FaceRecognitionResult) HasUnknownFaces() bool {
	return result.UnknownCount > 0
}

// HasKnownFaces returns true if there are known faces in the result
func (result *FaceRecognitionResult) HasKnownFaces() bool {
	return result.KnownCount > 0
}

// FormatForNotification returns a formatted string for Telegram notification
func (result *FaceRecognitionResult) FormatForNotification() string {
	if result.Count == 0 {
		return ""
	}

	var msg string
	if result.KnownCount > 0 {
		identities := result.GetKnownIdentities()
		if len(identities) == 1 {
			msg = fmt.Sprintf("👤 Identified: %s", identities[0])
		} else {
			msg = fmt.Sprintf("👤 Identified: %s", joinStrings(identities, ", "))
		}
	}

	if result.UnknownCount > 0 {
		unknownMsg := fmt.Sprintf("❓ Unknown faces: %d", result.UnknownCount)
		if msg != "" {
			msg += "\n" + unknownMsg
		} else {
			msg = unknownMsg
		}
	}

	return msg
}

// Helper function to join strings
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// LogRecognitionResult logs the face recognition result
func (fr *FaceRecognizer) LogRecognitionResult(result *FaceRecognitionResult, cameraID string) {
	if result.Count == 0 {
		return
	}

	fmt.Printf("Face recognition result for camera %s: total=%d, known=%d, unknown=%d (inference: %.1fms)\n",
		cameraID, result.Count, result.KnownCount, result.UnknownCount, result.InferenceTimeMs)

	for i, rec := range result.Recognitions {
		if rec.IsKnown && rec.Identity != nil {
			fmt.Printf("  Recognized face %d: %s (similarity: %.1f%%)\n",
				i, *rec.Identity, rec.Similarity*100)
		} else {
			fmt.Printf("  Unknown face %d (confidence: %.1f%%)\n",
				i, rec.Confidence*100)
		}
	}
}
