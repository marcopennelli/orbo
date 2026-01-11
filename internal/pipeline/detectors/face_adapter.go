package detectors

import (
	"context"
	"fmt"
	"log"

	"orbo/internal/detection"
	"orbo/internal/pipeline"
)

// FaceAdapter wraps FaceRecognizer to implement the unified Detector interface
// It also implements ConditionalDetector since it should only run when persons are detected
// Supports both HTTP (legacy) and gRPC (low-latency) backends
type FaceAdapter struct {
	httpRecognizer *detection.FaceRecognizer
	grpcRecognizer *detection.GRPCFaceRecognizer
	useGRPC        bool
}

// FaceAdapterConfig holds configuration for the face adapter
type FaceAdapterConfig struct {
	// HTTP configuration (legacy)
	HTTPEnabled   bool
	HTTPRecognizer *detection.FaceRecognizer

	// gRPC configuration (low-latency)
	GRPCEnabled   bool
	GRPCRecognizer *detection.GRPCFaceRecognizer
}

// NewFaceAdapter creates a new face detector adapter (legacy HTTP-only constructor)
func NewFaceAdapter(recognizer *detection.FaceRecognizer) *FaceAdapter {
	return &FaceAdapter{
		httpRecognizer: recognizer,
		useGRPC:        false,
	}
}

// NewFaceAdapterWithConfig creates a face adapter with configurable backend
func NewFaceAdapterWithConfig(config FaceAdapterConfig) *FaceAdapter {
	adapter := &FaceAdapter{}

	if config.GRPCEnabled && config.GRPCRecognizer != nil {
		adapter.grpcRecognizer = config.GRPCRecognizer
		adapter.useGRPC = true
		log.Printf("[FaceAdapter] Using gRPC backend for low-latency face recognition")
	}

	if config.HTTPEnabled && config.HTTPRecognizer != nil {
		adapter.httpRecognizer = config.HTTPRecognizer
		if !adapter.useGRPC {
			log.Printf("[FaceAdapter] Using HTTP backend for face recognition")
		} else {
			log.Printf("[FaceAdapter] HTTP backend available as fallback")
		}
	}

	return adapter
}

func (a *FaceAdapter) Name() string {
	return "face"
}

func (a *FaceAdapter) Type() pipeline.DetectorType {
	return pipeline.DetectorTypeFace
}

func (a *FaceAdapter) IsHealthy() bool {
	if a.useGRPC && a.grpcRecognizer != nil {
		return a.grpcRecognizer.IsEnabled() && a.grpcRecognizer.IsHealthy()
	}
	if a.httpRecognizer != nil {
		return a.httpRecognizer.IsEnabled() && a.httpRecognizer.IsHealthy()
	}
	return false
}

func (a *FaceAdapter) Detect(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	if a.useGRPC && a.grpcRecognizer != nil {
		return a.detectWithGRPC(ctx, frame, nil, nil)
	}
	return a.detectWithHTTP(ctx, frame)
}

// DetectWithTracking performs face recognition with YOLO track hints for better performance
// personBBoxes are the bounding boxes of detected persons from YOLO
// trackIDs are the corresponding YOLO track IDs for face-track association
func (a *FaceAdapter) DetectWithTracking(ctx context.Context, frame *pipeline.FrameData, personBBoxes [][]float32, trackIDs []int32) (*pipeline.DetectionResult, error) {
	if a.useGRPC && a.grpcRecognizer != nil {
		return a.detectWithGRPC(ctx, frame, personBBoxes, trackIDs)
	}
	// HTTP fallback doesn't support region-based detection, use full frame
	return a.detectWithHTTP(ctx, frame)
}

func (a *FaceAdapter) detectWithHTTP(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	if a.httpRecognizer == nil {
		return nil, fmt.Errorf("face recognizer not configured")
	}

	if !a.httpRecognizer.IsEnabled() {
		return nil, fmt.Errorf("face recognition is disabled")
	}

	result, err := a.httpRecognizer.RecognizeFaces(frame.Data)
	if err != nil {
		return nil, fmt.Errorf("face recognition failed: %w", err)
	}

	return a.convertResult(frame, result), nil
}

func (a *FaceAdapter) detectWithGRPC(ctx context.Context, frame *pipeline.FrameData, personBBoxes [][]float32, trackIDs []int32) (*pipeline.DetectionResult, error) {
	if a.grpcRecognizer == nil {
		return nil, fmt.Errorf("gRPC face recognizer not configured")
	}

	if !a.grpcRecognizer.IsEnabled() {
		return nil, fmt.Errorf("face recognition is disabled")
	}

	// Use track-aware recognition if we have person regions
	result, err := a.grpcRecognizer.RecognizeFacesWithTracking(frame.Data, personBBoxes, trackIDs)
	if err != nil {
		// Try HTTP fallback if available
		if a.httpRecognizer != nil && a.httpRecognizer.IsEnabled() {
			log.Printf("[FaceAdapter] gRPC failed, falling back to HTTP: %v", err)
			return a.detectWithHTTP(ctx, frame)
		}
		return nil, fmt.Errorf("face recognition failed: %w", err)
	}

	return a.convertResultWithTracks(frame, result), nil
}

func (a *FaceAdapter) DetectAnnotated(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	if a.useGRPC && a.grpcRecognizer != nil {
		return a.detectAnnotatedWithGRPC(ctx, frame)
	}
	return a.detectAnnotatedWithHTTP(ctx, frame)
}

func (a *FaceAdapter) detectAnnotatedWithHTTP(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	if a.httpRecognizer == nil {
		return nil, fmt.Errorf("face recognizer not configured")
	}

	if !a.httpRecognizer.IsEnabled() {
		return nil, fmt.Errorf("face recognition is disabled")
	}

	result, err := a.httpRecognizer.RecognizeFacesAnnotated(frame.Data)
	if err != nil {
		return nil, fmt.Errorf("face annotated recognition failed: %w", err)
	}

	return a.convertAnnotatedResult(frame, result), nil
}

func (a *FaceAdapter) detectAnnotatedWithGRPC(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	if a.grpcRecognizer == nil {
		return nil, fmt.Errorf("gRPC face recognizer not configured")
	}

	if !a.grpcRecognizer.IsEnabled() {
		return nil, fmt.Errorf("face recognition is disabled")
	}

	result, err := a.grpcRecognizer.RecognizeFacesAnnotatedGRPC(frame.Data)
	if err != nil {
		// Try HTTP fallback if available
		if a.httpRecognizer != nil && a.httpRecognizer.IsEnabled() {
			log.Printf("[FaceAdapter] gRPC annotated failed, falling back to HTTP: %v", err)
			return a.detectAnnotatedWithHTTP(ctx, frame)
		}
		return nil, fmt.Errorf("face annotated recognition failed: %w", err)
	}

	return a.convertAnnotatedResult(frame, result), nil
}

func (a *FaceAdapter) SupportsAnnotation() bool {
	return true
}

func (a *FaceAdapter) Close() error {
	if a.grpcRecognizer != nil {
		return a.grpcRecognizer.Close()
	}
	// HTTP client doesn't need explicit cleanup
	return nil
}

// ShouldRun implements ConditionalDetector - only run if persons were detected
func (a *FaceAdapter) ShouldRun(priorResults *pipeline.DetectionResult) bool {
	if priorResults == nil {
		return false
	}

	// Check if any person detections exist
	for _, d := range priorResults.Detections {
		if d.Class == "person" {
			return true
		}
	}
	return false
}

// GetTriggerClasses returns the classes that trigger face detection
func (a *FaceAdapter) GetTriggerClasses() []string {
	return []string{"person"}
}

// GetPersonRegions extracts person bounding boxes and track IDs from prior detection results
// This can be used to optimize face detection by only looking within person regions
func (a *FaceAdapter) GetPersonRegions(priorResults *pipeline.DetectionResult) (bboxes [][]float32, trackIDs []int32) {
	if priorResults == nil {
		return nil, nil
	}

	for _, d := range priorResults.Detections {
		if d.Class == "person" {
			bboxes = append(bboxes, []float32{d.BBox.X1, d.BBox.Y1, d.BBox.X2, d.BBox.Y2})
			trackID := int32(0)
			if d.TrackID != nil {
				trackID = int32(*d.TrackID)
			}
			trackIDs = append(trackIDs, trackID)
		}
	}
	return bboxes, trackIDs
}

// convertResult converts FaceRecognitionResult to pipeline.DetectionResult
func (a *FaceAdapter) convertResult(frame *pipeline.FrameData, result *detection.FaceRecognitionResult) *pipeline.DetectionResult {
	faces := make([]pipeline.FaceDetection, 0, len(result.Recognitions))
	for _, r := range result.Recognitions {
		var bbox pipeline.BBox
		if len(r.BBox) >= 4 {
			bbox = pipeline.BBox{
				X1: r.BBox[0],
				Y1: r.BBox[1],
				X2: r.BBox[2],
				Y2: r.BBox[3],
			}
		}

		face := pipeline.FaceDetection{
			BBox:       bbox,
			Confidence: r.Confidence,
			Similarity: r.Similarity,
		}

		if r.Identity != nil {
			face.PersonID = r.Identity
			face.PersonName = r.Identity // Identity is the name in current implementation
		}

		if r.Age > 0 {
			age := r.Age
			face.Age = &age
		}

		if r.Gender != "" {
			gender := r.Gender
			face.Gender = &gender
		}

		faces = append(faces, face)
	}

	return &pipeline.DetectionResult{
		CameraID:     frame.CameraID,
		FrameSeq:     frame.Seq,
		Timestamp:    frame.Timestamp,
		DetectorType: pipeline.DetectorTypeFace,
		Faces:        faces,
		InferenceMs:  result.InferenceTimeMs,
	}
}

// convertResultWithTracks converts FaceRecognitionResultWithTracks to pipeline.DetectionResult
func (a *FaceAdapter) convertResultWithTracks(frame *pipeline.FrameData, result *detection.FaceRecognitionResultWithTracks) *pipeline.DetectionResult {
	faces := make([]pipeline.FaceDetection, 0, len(result.Recognitions))
	for _, r := range result.Recognitions {
		var bbox pipeline.BBox
		if len(r.BBox) >= 4 {
			bbox = pipeline.BBox{
				X1: r.BBox[0],
				Y1: r.BBox[1],
				X2: r.BBox[2],
				Y2: r.BBox[3],
			}
		}

		face := pipeline.FaceDetection{
			BBox:            bbox,
			Confidence:      r.Confidence,
			Similarity:      r.Similarity,
			AssociatedTrackID: int(r.AssociatedTrackID),
		}

		if r.Identity != nil {
			face.PersonID = r.Identity
			face.PersonName = r.Identity
		}

		if r.Age > 0 {
			age := r.Age
			face.Age = &age
		}

		if r.Gender != "" {
			gender := r.Gender
			face.Gender = &gender
		}

		faces = append(faces, face)
	}

	return &pipeline.DetectionResult{
		CameraID:     frame.CameraID,
		FrameSeq:     frame.Seq,
		Timestamp:    frame.Timestamp,
		DetectorType: pipeline.DetectorTypeFace,
		Faces:        faces,
		InferenceMs:  result.InferenceTimeMs,
	}
}

// convertAnnotatedResult converts AnnotatedRecognitionResult to pipeline.DetectionResult
func (a *FaceAdapter) convertAnnotatedResult(frame *pipeline.FrameData, result *detection.AnnotatedRecognitionResult) *pipeline.DetectionResult {
	faces := make([]pipeline.FaceDetection, 0, len(result.Recognitions))
	for _, r := range result.Recognitions {
		var bbox pipeline.BBox
		if len(r.BBox) >= 4 {
			bbox = pipeline.BBox{
				X1: r.BBox[0],
				Y1: r.BBox[1],
				X2: r.BBox[2],
				Y2: r.BBox[3],
			}
		}

		face := pipeline.FaceDetection{
			BBox:       bbox,
			Confidence: r.Confidence,
			Similarity: r.Similarity,
		}

		if r.Identity != nil {
			face.PersonID = r.Identity
			face.PersonName = r.Identity
		}

		if r.Age > 0 {
			age := r.Age
			face.Age = &age
		}

		if r.Gender != "" {
			gender := r.Gender
			face.Gender = &gender
		}

		faces = append(faces, face)
	}

	return &pipeline.DetectionResult{
		CameraID:     frame.CameraID,
		FrameSeq:     frame.Seq,
		Timestamp:    frame.Timestamp,
		DetectorType: pipeline.DetectorTypeFace,
		Faces:        faces,
		ImageData:    result.ImageData,
		InferenceMs:  result.InferenceTimeMs,
	}
}

// Ensure FaceAdapter implements both Detector and ConditionalDetector
var _ pipeline.Detector = (*FaceAdapter)(nil)
var _ pipeline.ConditionalDetector = (*FaceAdapter)(nil)
