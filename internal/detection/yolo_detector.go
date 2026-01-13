package detection

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"
)

// YoloTask represents YOLO11 task types
type YoloTask string

const (
	YoloTaskDetect   YoloTask = "detect"
	YoloTaskPose     YoloTask = "pose"
	YoloTaskSegment  YoloTask = "segment"
	YoloTaskOBB      YoloTask = "obb"
	YoloTaskClassify YoloTask = "classify"
)

// ValidYoloTasks contains all valid YOLO tasks
var ValidYoloTasks = []YoloTask{
	YoloTaskDetect,
	YoloTaskPose,
	YoloTaskSegment,
	YoloTaskOBB,
	YoloTaskClassify,
}

// YOLODetector handles YOLO-powered object detection
type YOLODetector struct {
	endpoint      string
	client        *http.Client
	enabled       bool
	securityMode  bool
	confThreshold float32
	classesFilter string
	drawBoxes     bool
	tasks         []YoloTask // YOLO tasks to run (default: detect)
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

// YOLOKeypoint represents a single body keypoint
type YOLOKeypoint struct {
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
	Confidence float32 `json:"confidence"`
	Name       string  `json:"name"`
}

// YOLOPoseDetection represents a pose estimation result
type YOLOPoseDetection struct {
	BBox       []float32      `json:"bbox"` // [x1, y1, x2, y2]
	Confidence float32        `json:"confidence"`
	Keypoints  []YOLOKeypoint `json:"keypoints"`
	PoseClass  string         `json:"pose_class"` // standing, lying, crouching, etc.
}

// YOLOSegmentDetection represents a segmentation result
type YOLOSegmentDetection struct {
	Class       string    `json:"class"`
	ClassID     int       `json:"class_id"`
	Confidence  float32   `json:"confidence"`
	BBox        []float32 `json:"bbox"`
	MaskPolygon []float32 `json:"mask_polygon,omitempty"`
}

// YOLOOBBDetection represents an oriented bounding box detection
type YOLOOBBDetection struct {
	Class      string    `json:"class"`
	ClassID    int       `json:"class_id"`
	Confidence float32   `json:"confidence"`
	Cx         float32   `json:"cx"`
	Cy         float32   `json:"cy"`
	Width      float32   `json:"width"`
	Height     float32   `json:"height"`
	Angle      float32   `json:"angle"`
	Corners    []float32 `json:"corners"`
}

// YOLOClassification represents a classification result
type YOLOClassification struct {
	Class      string  `json:"class"`
	ClassID    int     `json:"class_id"`
	Confidence float32 `json:"confidence"`
}

// YOLOTaskResult represents results for a single task
type YOLOTaskResult struct {
	Detections      []YOLODetection        `json:"detections,omitempty"`
	Poses           []YOLOPoseDetection    `json:"poses,omitempty"`
	Segments        []YOLOSegmentDetection `json:"segments,omitempty"`
	OBBs            []YOLOOBBDetection     `json:"obbs,omitempty"`
	Classifications []YOLOClassification   `json:"classifications,omitempty"`
	Count           int                    `json:"count"`
	InferenceTimeMs float32                `json:"inference_time_ms"`
}

// YOLOAnalyzeResult represents the multi-task analysis response
type YOLOAnalyzeResult struct {
	Tasks            map[string]*YOLOTaskResult `json:"tasks"`
	TotalInferenceMs float32                    `json:"total_inference_ms"`
	Device           string                     `json:"device"`
	ImageData        []byte                     `json:"-"` // Annotated image
	Alerts           []string                   `json:"alerts,omitempty"`
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
	Detections      []YOLODetection    `json:"detections"`
	Count           int                `json:"count"`
	ThreatAnalysis  YOLOThreatAnalysis `json:"threat_analysis"`
	InferenceTimeMs float32            `json:"inference_time_ms"`
	Device          string             `json:"device"`
	SecurityFilter  []string           `json:"security_filter"`
}

// YOLOThreatAnalysis contains threat categorization
type YOLOThreatAnalysis struct {
	HighPriority   []YOLODetection `json:"high_priority"`   // persons
	MediumPriority []YOLODetection `json:"medium_priority"` // vehicles
	LowPriority    []YOLODetection `json:"low_priority"`    // bikes
}

// YOLOHealthResponse represents health check response
type YOLOHealthResponse struct {
	Status       string   `json:"status"`
	Device       string   `json:"device"`
	GPUAvailable bool     `json:"gpu_available"`
	ModelLoaded  bool     `json:"model_loaded"`
	LoadedModels []string `json:"loaded_models,omitempty"`
	EnabledTasks []string `json:"enabled_tasks,omitempty"`
}

// YOLOConfig holds configuration for the detector
type YOLOConfig struct {
	Enabled             bool
	ServiceEndpoint     string
	ConfidenceThreshold float32
	SecurityMode        bool
	ClassesFilter       string
	DrawBoxes           bool
	Tasks               []YoloTask // YOLO tasks to run (default: detect only)
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
	if len(cfg.Tasks) > 0 {
		d.tasks = cfg.Tasks
	} else {
		d.tasks = []YoloTask{YoloTaskDetect} // Default to detection only
	}
	return d
}

// SetTasks updates the YOLO tasks to run
func (yd *YOLODetector) SetTasks(tasks []YoloTask) {
	yd.mu.Lock()
	defer yd.mu.Unlock()
	if len(tasks) > 0 {
		yd.tasks = tasks
	} else {
		yd.tasks = []YoloTask{YoloTaskDetect}
	}
}

// GetTasks returns the current YOLO tasks
func (yd *YOLODetector) GetTasks() []YoloTask {
	yd.mu.RLock()
	defer yd.mu.RUnlock()
	return yd.tasks
}

// ParseYoloTasks parses a comma-separated string into YoloTask slice
func ParseYoloTasks(tasksStr string) []YoloTask {
	if tasksStr == "" {
		return []YoloTask{YoloTaskDetect}
	}

	parts := strings.Split(tasksStr, ",")
	tasks := make([]YoloTask, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		switch p {
		case "detect":
			tasks = append(tasks, YoloTaskDetect)
		case "pose":
			tasks = append(tasks, YoloTaskPose)
		case "segment":
			tasks = append(tasks, YoloTaskSegment)
		case "obb":
			tasks = append(tasks, YoloTaskOBB)
		case "classify":
			tasks = append(tasks, YoloTaskClassify)
		}
	}

	if len(tasks) == 0 {
		return []YoloTask{YoloTaskDetect}
	}
	return tasks
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

	// Make request to security annotated endpoint with base64 format to get detection details
	req, err := http.NewRequest("POST", yd.endpoint+"/detect/security/annotated?format=base64", &b)
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

	// Parse JSON response with base64 image and detection details
	var jsonResponse struct {
		Image struct {
			Data        string `json:"data"`
			ContentType string `json:"content_type"`
		} `json:"image"`
		Detections      []YOLODetection    `json:"detections"`
		Count           int                `json:"count"`
		InferenceTimeMs float32            `json:"inference_time_ms"`
		Device          string             `json:"device"`
		ThreatAnalysis  YOLOThreatAnalysis `json:"threat_analysis"`
		SecurityFilter  []string           `json:"security_filter"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to decode annotated response: %w", err)
	}

	// Decode base64 image
	annotatedImage, err := base64.StdEncoding.DecodeString(jsonResponse.Image.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 image: %w", err)
	}

	result := &YOLOAnnotatedResult{
		ImageData:       annotatedImage,
		Detections:      jsonResponse.Detections,
		Count:           jsonResponse.Count,
		InferenceTimeMs: jsonResponse.InferenceTimeMs,
		Device:          jsonResponse.Device,
	}

	// Add threat analysis
	if len(jsonResponse.ThreatAnalysis.HighPriority) > 0 ||
		len(jsonResponse.ThreatAnalysis.MediumPriority) > 0 ||
		len(jsonResponse.ThreatAnalysis.LowPriority) > 0 {
		result.ThreatAnalysis = &jsonResponse.ThreatAnalysis
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

// Analyze performs multi-task YOLO analysis on an image
// This calls the /analyze endpoint which supports all YOLO11 tasks
func (yd *YOLODetector) Analyze(imageData []byte, tasks []YoloTask, returnAnnotated bool) (*YOLOAnalyzeResult, error) {
	if !yd.IsHealthy() {
		return nil, fmt.Errorf("YOLO detection service unavailable")
	}

	yd.mu.RLock()
	confThreshold := yd.confThreshold
	classesFilter := yd.classesFilter
	if len(tasks) == 0 {
		tasks = yd.tasks
	}
	yd.mu.RUnlock()

	// Create multipart form data
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add image file
	fw, err := w.CreateFormFile("file", "frame.jpg")
	if err != nil {
		return nil, err
	}
	fw.Write(imageData)

	// Add tasks as comma-separated string
	taskStrs := make([]string, len(tasks))
	for i, t := range tasks {
		taskStrs[i] = string(t)
	}
	w.WriteField("tasks", strings.Join(taskStrs, ","))

	// Add confidence threshold
	w.WriteField("conf_threshold", fmt.Sprintf("%.3f", confThreshold))

	// Add return_annotated flag
	if returnAnnotated {
		w.WriteField("return_annotated", "true")
	}

	// Add classes filter if set
	if classesFilter != "" {
		w.WriteField("classes_filter", classesFilter)
	}

	w.Close()

	// Make request to analyze endpoint
	req, err := http.NewRequest("POST", yd.endpoint+"/analyze", &b)
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
		return nil, fmt.Errorf("YOLO analyze failed: %s", string(body))
	}

	// Parse JSON response
	var jsonResponse struct {
		Tasks map[string]struct {
			Detections []struct {
				Class      string    `json:"class"`
				ClassID    int       `json:"class_id"`
				Confidence float32   `json:"confidence"`
				BBox       []float32 `json:"bbox"`
				Center     []float32 `json:"center"`
				Area       float32   `json:"area"`
			} `json:"detections,omitempty"`
			Poses []struct {
				BBox       []float32 `json:"bbox"`
				Confidence float32   `json:"confidence"`
				Keypoints  []struct {
					X          float32 `json:"x"`
					Y          float32 `json:"y"`
					Confidence float32 `json:"confidence"`
					Name       string  `json:"name"`
				} `json:"keypoints"`
				PoseClass string `json:"pose_class"`
			} `json:"poses,omitempty"`
			Segments []struct {
				Class       string    `json:"class"`
				ClassID     int       `json:"class_id"`
				Confidence  float32   `json:"confidence"`
				BBox        []float32 `json:"bbox"`
				MaskPolygon []float32 `json:"mask_polygon,omitempty"`
			} `json:"segments,omitempty"`
			OBBs []struct {
				Class      string    `json:"class"`
				ClassID    int       `json:"class_id"`
				Confidence float32   `json:"confidence"`
				Cx         float32   `json:"cx"`
				Cy         float32   `json:"cy"`
				Width      float32   `json:"width"`
				Height     float32   `json:"height"`
				Angle      float32   `json:"angle"`
				Corners    []float32 `json:"corners"`
			} `json:"obbs,omitempty"`
			Classifications []struct {
				Class      string  `json:"class"`
				ClassID    int     `json:"class_id"`
				Confidence float32 `json:"confidence"`
			} `json:"classifications,omitempty"`
			Count           int     `json:"count"`
			InferenceTimeMs float32 `json:"inference_time_ms"`
		} `json:"tasks"`
		TotalInferenceMs float32  `json:"total_inference_ms"`
		Device           string   `json:"device"`
		Alerts           []string `json:"alerts,omitempty"`
		Image            *struct {
			Data        string `json:"data"`
			ContentType string `json:"content_type"`
		} `json:"image,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to decode analyze response: %w", err)
	}

	// Convert to YOLOAnalyzeResult
	result := &YOLOAnalyzeResult{
		Tasks:            make(map[string]*YOLOTaskResult),
		TotalInferenceMs: jsonResponse.TotalInferenceMs,
		Device:           jsonResponse.Device,
		Alerts:           jsonResponse.Alerts,
	}

	for taskName, taskData := range jsonResponse.Tasks {
		taskResult := &YOLOTaskResult{
			Count:           taskData.Count,
			InferenceTimeMs: taskData.InferenceTimeMs,
		}

		// Convert detections
		if len(taskData.Detections) > 0 {
			taskResult.Detections = make([]YOLODetection, len(taskData.Detections))
			for i, d := range taskData.Detections {
				taskResult.Detections[i] = YOLODetection{
					Class:      d.Class,
					ClassID:    d.ClassID,
					Confidence: d.Confidence,
					BBox:       d.BBox,
					Center:     d.Center,
					Area:       d.Area,
				}
			}
		}

		// Convert poses
		if len(taskData.Poses) > 0 {
			taskResult.Poses = make([]YOLOPoseDetection, len(taskData.Poses))
			for i, p := range taskData.Poses {
				pose := YOLOPoseDetection{
					BBox:       p.BBox,
					Confidence: p.Confidence,
					PoseClass:  p.PoseClass,
					Keypoints:  make([]YOLOKeypoint, len(p.Keypoints)),
				}
				for j, kp := range p.Keypoints {
					pose.Keypoints[j] = YOLOKeypoint{
						X:          kp.X,
						Y:          kp.Y,
						Confidence: kp.Confidence,
						Name:       kp.Name,
					}
				}
				taskResult.Poses[i] = pose
			}
		}

		// Convert segments
		if len(taskData.Segments) > 0 {
			taskResult.Segments = make([]YOLOSegmentDetection, len(taskData.Segments))
			for i, s := range taskData.Segments {
				taskResult.Segments[i] = YOLOSegmentDetection{
					Class:       s.Class,
					ClassID:     s.ClassID,
					Confidence:  s.Confidence,
					BBox:        s.BBox,
					MaskPolygon: s.MaskPolygon,
				}
			}
		}

		// Convert OBBs
		if len(taskData.OBBs) > 0 {
			taskResult.OBBs = make([]YOLOOBBDetection, len(taskData.OBBs))
			for i, o := range taskData.OBBs {
				taskResult.OBBs[i] = YOLOOBBDetection{
					Class:      o.Class,
					ClassID:    o.ClassID,
					Confidence: o.Confidence,
					Cx:         o.Cx,
					Cy:         o.Cy,
					Width:      o.Width,
					Height:     o.Height,
					Angle:      o.Angle,
					Corners:    o.Corners,
				}
			}
		}

		// Convert classifications
		if len(taskData.Classifications) > 0 {
			taskResult.Classifications = make([]YOLOClassification, len(taskData.Classifications))
			for i, c := range taskData.Classifications {
				taskResult.Classifications[i] = YOLOClassification{
					Class:      c.Class,
					ClassID:    c.ClassID,
					Confidence: c.Confidence,
				}
			}
		}

		result.Tasks[taskName] = taskResult
	}

	// Decode annotated image if present
	if jsonResponse.Image != nil && jsonResponse.Image.Data != "" {
		annotatedImage, err := base64.StdEncoding.DecodeString(jsonResponse.Image.Data)
		if err == nil {
			result.ImageData = annotatedImage
		}
	}

	return result, nil
}

// AnalyzeAnnotated performs multi-task analysis and returns annotated image
func (yd *YOLODetector) AnalyzeAnnotated(imageData []byte, tasks []YoloTask) (*YOLOAnalyzeResult, error) {
	return yd.Analyze(imageData, tasks, true)
}

// HasPoseTask returns true if pose task is configured
func (yd *YOLODetector) HasPoseTask() bool {
	yd.mu.RLock()
	defer yd.mu.RUnlock()
	for _, t := range yd.tasks {
		if t == YoloTaskPose {
			return true
		}
	}
	return false
}

// HasMultipleTasks returns true if more than one task is configured
func (yd *YOLODetector) HasMultipleTasks() bool {
	yd.mu.RLock()
	defer yd.mu.RUnlock()
	return len(yd.tasks) > 1
}

// ShouldUseAnalyzeEndpoint returns true if we should use /analyze instead of /detect
// This is true when multiple tasks are configured or when non-detect tasks are needed
func (yd *YOLODetector) ShouldUseAnalyzeEndpoint() bool {
	yd.mu.RLock()
	defer yd.mu.RUnlock()
	if len(yd.tasks) > 1 {
		return true
	}
	for _, t := range yd.tasks {
		if t != YoloTaskDetect {
			return true
		}
	}
	return false
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
	if len(cfg.Tasks) > 0 {
		yd.tasks = cfg.Tasks
	}

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
		Tasks:               yd.tasks,
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
