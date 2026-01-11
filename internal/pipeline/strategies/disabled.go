package strategies

import (
	"orbo/internal/pipeline"
)

// DisabledStrategy never triggers detection
// Used when streaming only is desired
type DisabledStrategy struct{}

// NewDisabledStrategy creates a disabled detection strategy
func NewDisabledStrategy() *DisabledStrategy {
	return &DisabledStrategy{}
}

func (s *DisabledStrategy) Name() string {
	return string(pipeline.DetectionModeDisabled)
}

func (s *DisabledStrategy) ShouldDetect(frame *pipeline.FrameData, lastDetection *pipeline.DetectionResult) bool {
	return false
}

func (s *DisabledStrategy) OnDetectionComplete(result *pipeline.DetectionResult) {
	// No-op
}

func (s *DisabledStrategy) Reset() {
	// No-op
}
