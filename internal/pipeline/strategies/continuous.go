package strategies

import (
	"sync"
	"time"

	"orbo/internal/pipeline"
)

// ContinuousStrategy triggers detection on every frame
// Optionally rate-limits to avoid overwhelming the detector
type ContinuousStrategy struct {
	minInterval     time.Duration // Minimum time between detections
	lastDetection   time.Time
	mu              sync.Mutex
}

// NewContinuousStrategy creates a continuous detection strategy
// minInterval can be 0 to process every frame, or a duration to rate-limit
func NewContinuousStrategy(minInterval time.Duration) *ContinuousStrategy {
	return &ContinuousStrategy{
		minInterval: minInterval,
	}
}

func (s *ContinuousStrategy) Name() string {
	return string(pipeline.DetectionModeContinuous)
}

func (s *ContinuousStrategy) ShouldDetect(frame *pipeline.FrameData, lastDetection *pipeline.DetectionResult) bool {
	if s.minInterval == 0 {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if now.Sub(s.lastDetection) >= s.minInterval {
		return true
	}
	return false
}

func (s *ContinuousStrategy) OnDetectionComplete(result *pipeline.DetectionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastDetection = time.Now()
}

func (s *ContinuousStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastDetection = time.Time{}
}
