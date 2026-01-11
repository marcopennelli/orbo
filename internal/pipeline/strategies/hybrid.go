package strategies

import (
	"sync"
	"time"

	"orbo/internal/pipeline"
)

// HybridStrategy triggers detection on motion OR at scheduled intervals
// Combines the benefits of motion-triggered (responsive) and scheduled (guaranteed coverage)
type HybridStrategy struct {
	motionDetector  pipeline.MotionDetector
	sensitivity     float32
	scheduleInterval time.Duration
	cooldownPeriod   time.Duration
	lastMotionTime   time.Time
	lastScheduledTime time.Time
	hasActiveMotion  bool
	mu               sync.Mutex
}

// NewHybridStrategy creates a hybrid detection strategy
func NewHybridStrategy(
	motionDetector pipeline.MotionDetector,
	sensitivity float32,
	scheduleInterval time.Duration,
	cooldownPeriod time.Duration,
) *HybridStrategy {
	if cooldownPeriod == 0 {
		cooldownPeriod = 2 * time.Second
	}
	if sensitivity <= 0 {
		sensitivity = 0.1
	}
	if scheduleInterval <= 0 {
		scheduleInterval = 5 * time.Second
	}

	if motionDetector != nil {
		motionDetector.SetSensitivity(sensitivity)
	}

	return &HybridStrategy{
		motionDetector:   motionDetector,
		sensitivity:      sensitivity,
		scheduleInterval: scheduleInterval,
		cooldownPeriod:   cooldownPeriod,
	}
}

func (s *HybridStrategy) Name() string {
	return string(pipeline.DetectionModeHybrid)
}

func (s *HybridStrategy) ShouldDetect(frame *pipeline.FrameData, lastDetection *pipeline.DetectionResult) bool {
	now := time.Now()

	// Check scheduled trigger first (guaranteed coverage)
	s.mu.Lock()
	scheduledTrigger := now.Sub(s.lastScheduledTime) >= s.scheduleInterval
	s.mu.Unlock()

	if scheduledTrigger {
		return true
	}

	// Check motion trigger
	if s.motionDetector != nil {
		hasMotion, _, err := s.motionDetector.DetectMotion(frame)
		if err == nil {
			s.mu.Lock()
			if hasMotion {
				s.lastMotionTime = now
				s.hasActiveMotion = true
				s.mu.Unlock()
				return true
			}

			// Check cooldown period
			if s.hasActiveMotion && now.Sub(s.lastMotionTime) < s.cooldownPeriod {
				s.mu.Unlock()
				return true
			}

			s.hasActiveMotion = false
			s.mu.Unlock()
		}
	}

	return false
}

func (s *HybridStrategy) OnDetectionComplete(result *pipeline.DetectionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.lastScheduledTime = now
}

func (s *HybridStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastMotionTime = time.Time{}
	s.lastScheduledTime = time.Time{}
	s.hasActiveMotion = false
	if s.motionDetector != nil {
		s.motionDetector.Reset()
	}
}

// SetMotionDetector allows updating the motion detector
func (s *HybridStrategy) SetMotionDetector(detector pipeline.MotionDetector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.motionDetector = detector
	if detector != nil {
		detector.SetSensitivity(s.sensitivity)
	}
}

// SetScheduleInterval updates the scheduled detection interval
func (s *HybridStrategy) SetScheduleInterval(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduleInterval = interval
}
