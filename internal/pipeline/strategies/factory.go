package strategies

import (
	"fmt"
	"time"

	"orbo/internal/pipeline"
)

// StrategyFactory creates detection strategies based on configuration
type StrategyFactory struct {
	motionDetector pipeline.MotionDetector
}

// NewStrategyFactory creates a new strategy factory
func NewStrategyFactory(motionDetector pipeline.MotionDetector) *StrategyFactory {
	return &StrategyFactory{
		motionDetector: motionDetector,
	}
}

// Create creates a detection strategy based on the effective configuration
func (f *StrategyFactory) Create(config *pipeline.EffectiveConfig) (pipeline.DetectionStrategy, error) {
	if config == nil {
		return NewDisabledStrategy(), nil
	}

	switch config.Mode {
	case pipeline.DetectionModeDisabled:
		return NewDisabledStrategy(), nil

	case pipeline.DetectionModeVisualOnly:
		// Visual only runs like continuous but results don't trigger alerts
		// (alert suppression is handled in the motion detector/event handler)
		return NewContinuousStrategy(0), nil

	case pipeline.DetectionModeContinuous:
		// No rate limiting for continuous mode
		return NewContinuousStrategy(0), nil

	case pipeline.DetectionModeMotionTriggered:
		// Use configured cooldown (default 2000ms if not set)
		cooldownMs := config.MotionCooldownMs
		if cooldownMs <= 0 {
			cooldownMs = 2000
		}
		return NewMotionTriggeredStrategy(
			f.motionDetector,
			config.MotionSensitivity,
			time.Duration(cooldownMs)*time.Millisecond,
		), nil

	case pipeline.DetectionModeScheduled:
		return NewScheduledStrategy(config.ScheduleInterval), nil

	case pipeline.DetectionModeHybrid:
		// Use configured cooldown (default 2000ms if not set)
		cooldownMs := config.MotionCooldownMs
		if cooldownMs <= 0 {
			cooldownMs = 2000
		}
		return NewHybridStrategy(
			f.motionDetector,
			config.MotionSensitivity,
			config.ScheduleInterval,
			time.Duration(cooldownMs)*time.Millisecond,
		), nil

	default:
		return nil, fmt.Errorf("unknown detection mode: %s", config.Mode)
	}
}

// CreateFromMode creates a strategy from just a mode and default settings
func (f *StrategyFactory) CreateFromMode(mode pipeline.DetectionMode) (pipeline.DetectionStrategy, error) {
	defaultConfig := pipeline.DefaultGlobalConfig()
	effectiveConfig := &pipeline.EffectiveConfig{
		Mode:              mode,
		ScheduleInterval:  defaultConfig.ScheduleInterval,
		MotionSensitivity: defaultConfig.MotionSensitivity,
	}
	return f.Create(effectiveConfig)
}

// SetMotionDetector updates the motion detector used by motion-based strategies
func (f *StrategyFactory) SetMotionDetector(detector pipeline.MotionDetector) {
	f.motionDetector = detector
}
