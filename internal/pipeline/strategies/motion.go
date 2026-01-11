package strategies

import (
	"sync"
	"time"

	"orbo/internal/pipeline"
)

// MotionTriggeredStrategy only triggers detection when motion is detected
// This is the current default behavior in orbo
type MotionTriggeredStrategy struct {
	motionDetector  pipeline.MotionDetector
	sensitivity     float32
	cooldownPeriod  time.Duration // Time to continue detecting after motion stops
	lastMotionTime  time.Time
	hasActiveMotion bool
	mu              sync.Mutex
}

// NewMotionTriggeredStrategy creates a motion-triggered detection strategy
func NewMotionTriggeredStrategy(motionDetector pipeline.MotionDetector, sensitivity float32, cooldownPeriod time.Duration) *MotionTriggeredStrategy {
	if cooldownPeriod == 0 {
		cooldownPeriod = 2 * time.Second // Default cooldown
	}
	if sensitivity <= 0 {
		sensitivity = 0.1 // Default sensitivity
	}

	if motionDetector != nil {
		motionDetector.SetSensitivity(sensitivity)
	}

	return &MotionTriggeredStrategy{
		motionDetector: motionDetector,
		sensitivity:    sensitivity,
		cooldownPeriod: cooldownPeriod,
	}
}

func (s *MotionTriggeredStrategy) Name() string {
	return string(pipeline.DetectionModeMotionTriggered)
}

func (s *MotionTriggeredStrategy) ShouldDetect(frame *pipeline.FrameData, lastDetection *pipeline.DetectionResult) bool {
	if s.motionDetector == nil {
		// No motion detector available, fall back to continuous
		return true
	}

	hasMotion, _, err := s.motionDetector.DetectMotion(frame)
	if err != nil {
		// On error, conservatively trigger detection
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if hasMotion {
		s.lastMotionTime = now
		s.hasActiveMotion = true
		return true
	}

	// Check if we're in cooldown period after motion stopped
	if s.hasActiveMotion && now.Sub(s.lastMotionTime) < s.cooldownPeriod {
		return true
	}

	// Motion has stopped and cooldown expired
	s.hasActiveMotion = false
	return false
}

func (s *MotionTriggeredStrategy) OnDetectionComplete(result *pipeline.DetectionResult) {
	// Motion strategy doesn't need to track detection completion
}

func (s *MotionTriggeredStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastMotionTime = time.Time{}
	s.hasActiveMotion = false
	if s.motionDetector != nil {
		s.motionDetector.Reset()
	}
}

// SetMotionDetector allows updating the motion detector
func (s *MotionTriggeredStrategy) SetMotionDetector(detector pipeline.MotionDetector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.motionDetector = detector
	if detector != nil {
		detector.SetSensitivity(s.sensitivity)
	}
}
