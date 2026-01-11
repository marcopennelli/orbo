package detectors

import (
	"context"
	"fmt"

	"orbo/internal/detection"
	"orbo/internal/pipeline"
)

// YOLOAdapter wraps GPUDetector to implement the unified Detector interface
type YOLOAdapter struct {
	detector      *detection.GPUDetector
	confThreshold float32
}

// NewYOLOAdapter creates a new YOLO detector adapter
func NewYOLOAdapter(detector *detection.GPUDetector, confThreshold float32) *YOLOAdapter {
	if confThreshold <= 0 {
		confThreshold = 0.5
	}
	return &YOLOAdapter{
		detector:      detector,
		confThreshold: confThreshold,
	}
}

func (a *YOLOAdapter) Name() string {
	return "yolo"
}

func (a *YOLOAdapter) Type() pipeline.DetectorType {
	return pipeline.DetectorTypeYOLO
}

func (a *YOLOAdapter) IsHealthy() bool {
	if a.detector == nil {
		return false
	}
	return a.detector.IsHealthy()
}

func (a *YOLOAdapter) Detect(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	if a.detector == nil {
		return nil, fmt.Errorf("YOLO detector not configured")
	}

	// Use security detection endpoint for security-focused results
	result, err := a.detector.DetectSecurityObjects(frame.Data, a.confThreshold)
	if err != nil {
		return nil, fmt.Errorf("YOLO detection failed: %w", err)
	}

	return a.convertResult(frame, result), nil
}

func (a *YOLOAdapter) DetectAnnotated(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	if a.detector == nil {
		return nil, fmt.Errorf("YOLO detector not configured")
	}

	// Use annotated security detection endpoint
	result, err := a.detector.DetectSecurityObjectsAnnotated(frame.Data, a.confThreshold)
	if err != nil {
		return nil, fmt.Errorf("YOLO annotated detection failed: %w", err)
	}

	return a.convertAnnotatedResult(frame, result), nil
}

func (a *YOLOAdapter) SupportsAnnotation() bool {
	return true
}

func (a *YOLOAdapter) Close() error {
	// GPUDetector doesn't have a close method (HTTP client based)
	return nil
}

// SetConfThreshold updates the confidence threshold
func (a *YOLOAdapter) SetConfThreshold(threshold float32) {
	a.confThreshold = threshold
}

// convertResult converts GPUDetector result to pipeline.DetectionResult
func (a *YOLOAdapter) convertResult(frame *pipeline.FrameData, result *detection.SecurityDetectionResult) *pipeline.DetectionResult {
	detections := make([]pipeline.Detection, 0, len(result.Detections))
	for _, d := range result.Detections {
		var bbox pipeline.BBox
		if len(d.BBox) >= 4 {
			bbox = pipeline.BBox{
				X1: d.BBox[0],
				Y1: d.BBox[1],
				X2: d.BBox[2],
				Y2: d.BBox[3],
			}
		}

		detections = append(detections, pipeline.Detection{
			Class:      d.Class,
			Confidence: d.Confidence,
			BBox:       bbox,
			Metadata:   map[string]interface{}{"class_id": d.ClassID},
		})
	}

	return &pipeline.DetectionResult{
		CameraID:     frame.CameraID,
		FrameSeq:     frame.Seq,
		Timestamp:    frame.Timestamp,
		DetectorType: pipeline.DetectorTypeYOLO,
		Detections:   detections,
		InferenceMs:  result.InferenceTimeMs,
	}
}

// convertAnnotatedResult converts annotated GPUDetector result to pipeline.DetectionResult
func (a *YOLOAdapter) convertAnnotatedResult(frame *pipeline.FrameData, result *detection.AnnotatedSecurityResult) *pipeline.DetectionResult {
	detections := make([]pipeline.Detection, 0, len(result.Detections))
	for _, d := range result.Detections {
		var bbox pipeline.BBox
		if len(d.BBox) >= 4 {
			bbox = pipeline.BBox{
				X1: d.BBox[0],
				Y1: d.BBox[1],
				X2: d.BBox[2],
				Y2: d.BBox[3],
			}
		}

		detections = append(detections, pipeline.Detection{
			Class:      d.Class,
			Confidence: d.Confidence,
			BBox:       bbox,
		})
	}

	return &pipeline.DetectionResult{
		CameraID:     frame.CameraID,
		FrameSeq:     frame.Seq,
		Timestamp:    frame.Timestamp,
		DetectorType: pipeline.DetectorTypeYOLO,
		Detections:   detections,
		ImageData:    result.ImageData,
		InferenceMs:  result.InferenceTimeMs,
	}
}

// Ensure YOLOAdapter implements Detector
var _ pipeline.Detector = (*YOLOAdapter)(nil)
