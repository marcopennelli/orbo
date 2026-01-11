package detectors

import (
	"context"
	"fmt"

	"orbo/internal/pipeline"
)

// PlateAdapter is a placeholder for future license plate recognition
// It implements ConditionalDetector since it should only run when vehicles are detected
type PlateAdapter struct {
	endpoint string
	healthy  bool
}

// NewPlateAdapter creates a new plate detector adapter
// Currently returns a non-functional placeholder
func NewPlateAdapter(endpoint string) *PlateAdapter {
	return &PlateAdapter{
		endpoint: endpoint,
		healthy:  false, // Not implemented yet
	}
}

func (a *PlateAdapter) Name() string {
	return "plate"
}

func (a *PlateAdapter) Type() pipeline.DetectorType {
	return pipeline.DetectorTypePlate
}

func (a *PlateAdapter) IsHealthy() bool {
	// TODO: Implement health check when plate recognition service is available
	return a.healthy
}

func (a *PlateAdapter) Detect(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	return nil, fmt.Errorf("plate recognition not implemented")
}

func (a *PlateAdapter) DetectAnnotated(ctx context.Context, frame *pipeline.FrameData) (*pipeline.DetectionResult, error) {
	return nil, fmt.Errorf("plate recognition not implemented")
}

func (a *PlateAdapter) SupportsAnnotation() bool {
	return true // Will support annotation when implemented
}

func (a *PlateAdapter) Close() error {
	return nil
}

// ShouldRun implements ConditionalDetector - only run if vehicles were detected
func (a *PlateAdapter) ShouldRun(priorResults *pipeline.DetectionResult) bool {
	if priorResults == nil {
		return false
	}

	// Check if any vehicle detections exist
	vehicleClasses := map[string]bool{
		"car":        true,
		"truck":      true,
		"bus":        true,
		"motorcycle": true,
	}

	for _, d := range priorResults.Detections {
		if vehicleClasses[d.Class] {
			return true
		}
	}
	return false
}

// GetTriggerClasses returns the classes that trigger plate detection
func (a *PlateAdapter) GetTriggerClasses() []string {
	return []string{"car", "truck", "bus", "motorcycle"}
}

// Ensure PlateAdapter implements both Detector and ConditionalDetector
var _ pipeline.Detector = (*PlateAdapter)(nil)
var _ pipeline.ConditionalDetector = (*PlateAdapter)(nil)
