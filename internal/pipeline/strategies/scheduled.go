package strategies

import (
	"sync"
	"time"

	"orbo/internal/pipeline"
)

// ScheduledStrategy triggers detection at fixed time intervals
// Useful for periodic sampling without motion detection
type ScheduledStrategy struct {
	interval        time.Duration
	lastDetection   time.Time
	mu              sync.Mutex
}

// NewScheduledStrategy creates a scheduled detection strategy
func NewScheduledStrategy(interval time.Duration) *ScheduledStrategy {
	if interval <= 0 {
		interval = 5 * time.Second // Default to 5 second interval
	}
	return &ScheduledStrategy{
		interval: interval,
	}
}

func (s *ScheduledStrategy) Name() string {
	return string(pipeline.DetectionModeScheduled)
}

func (s *ScheduledStrategy) ShouldDetect(frame *pipeline.FrameData, lastDetection *pipeline.DetectionResult) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if now.Sub(s.lastDetection) >= s.interval {
		return true
	}
	return false
}

func (s *ScheduledStrategy) OnDetectionComplete(result *pipeline.DetectionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastDetection = time.Now()
}

func (s *ScheduledStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastDetection = time.Time{}
}

// SetInterval updates the detection interval
func (s *ScheduledStrategy) SetInterval(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interval = interval
}
