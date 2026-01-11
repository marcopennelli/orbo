package pipeline

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// DetectionPipeline orchestrates frame capture and detection for a single camera
type DetectionPipeline struct {
	cameraID      string
	config        *EffectiveConfig
	strategy      DetectionStrategy
	detectors     []Detector
	frameProvider FrameProvider
	eventBus      *EventBus
	subscription  *FrameSubscription
	lastResult    *DetectionResult
	stopCh        chan struct{}
	running       bool
	mu            sync.RWMutex
	stats         *PipelineStats
	statsMu       sync.RWMutex
}

// DetectionPipelineManager manages detection pipelines for all cameras
type DetectionPipelineManager struct {
	pipelines     map[string]*DetectionPipeline
	frameProvider FrameProvider
	registry      DetectorRegistry
	eventBus      *EventBus
	strategyFactory func(*EffectiveConfig) (DetectionStrategy, error)
	mu            sync.RWMutex
	globalConfig  *GlobalDetectionConfig
}

// NewDetectionPipelineManager creates a new pipeline manager
func NewDetectionPipelineManager(
	frameProvider FrameProvider,
	registry DetectorRegistry,
	eventBus *EventBus,
	strategyFactory func(*EffectiveConfig) (DetectionStrategy, error),
) *DetectionPipelineManager {
	return &DetectionPipelineManager{
		pipelines:       make(map[string]*DetectionPipeline),
		frameProvider:   frameProvider,
		registry:        registry,
		eventBus:        eventBus,
		strategyFactory: strategyFactory,
		globalConfig:    DefaultGlobalConfig(),
	}
}

// SetGlobalConfig updates the global detection configuration
func (m *DetectionPipelineManager) SetGlobalConfig(config *GlobalDetectionConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalConfig = config
}

// GetGlobalConfig returns the current global configuration
func (m *DetectionPipelineManager) GetGlobalConfig() *GlobalDetectionConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.globalConfig != nil {
		cfg := *m.globalConfig
		return &cfg
	}
	return DefaultGlobalConfig()
}

// StartCamera starts detection pipeline for a camera
func (m *DetectionPipelineManager) StartCamera(cameraID string, device string, cameraConfig *CameraDetectionConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pipelines[cameraID]; exists {
		return fmt.Errorf("pipeline already exists for camera %s", cameraID)
	}

	// Merge camera config with global defaults
	effectiveConfig := cameraConfig.MergeWithGlobal(cameraID, m.globalConfig)

	// Create detection strategy
	strategy, err := m.strategyFactory(effectiveConfig)
	if err != nil {
		return fmt.Errorf("failed to create strategy: %w", err)
	}

	// Get configured detectors
	detectors := m.registry.GetHealthyByNames(effectiveConfig.Detectors)
	if len(detectors) == 0 && effectiveConfig.Mode != DetectionModeDisabled {
		log.Printf("[Pipeline] Warning: no healthy detectors available for camera %s", cameraID)
	}

	pipeline := &DetectionPipeline{
		cameraID:      cameraID,
		config:        effectiveConfig,
		strategy:      strategy,
		detectors:     detectors,
		frameProvider: m.frameProvider,
		eventBus:      m.eventBus,
		stopCh:        make(chan struct{}),
		stats: &PipelineStats{
			CameraID:        cameraID,
			CurrentMode:     effectiveConfig.Mode,
			ActiveDetectors: effectiveConfig.Detectors,
		},
	}

	m.pipelines[cameraID] = pipeline

	// Start pipeline processing
	go pipeline.run()

	log.Printf("[Pipeline] Started detection pipeline for camera %s (mode: %s, detectors: %v)",
		cameraID, effectiveConfig.Mode, effectiveConfig.Detectors)
	return nil
}

// StopCamera stops detection pipeline for a camera
func (m *DetectionPipelineManager) StopCamera(cameraID string) error {
	m.mu.Lock()
	pipeline, exists := m.pipelines[cameraID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("pipeline not found for camera %s", cameraID)
	}
	delete(m.pipelines, cameraID)
	m.mu.Unlock()

	pipeline.stop()
	log.Printf("[Pipeline] Stopped detection pipeline for camera %s", cameraID)
	return nil
}

// UpdateConfig updates detection configuration for a camera
func (m *DetectionPipelineManager) UpdateConfig(cameraID string, cameraConfig *CameraDetectionConfig) error {
	m.mu.Lock()
	pipeline, exists := m.pipelines[cameraID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("pipeline not found for camera %s", cameraID)
	}
	m.mu.Unlock()

	// Merge with global config
	effectiveConfig := cameraConfig.MergeWithGlobal(cameraID, m.globalConfig)

	// Update strategy if mode changed
	if effectiveConfig.Mode != pipeline.config.Mode {
		strategy, err := m.strategyFactory(effectiveConfig)
		if err != nil {
			return fmt.Errorf("failed to create strategy: %w", err)
		}
		pipeline.mu.Lock()
		pipeline.strategy = strategy
		pipeline.mu.Unlock()
	}

	// Update detectors if list changed
	pipeline.mu.Lock()
	pipeline.config = effectiveConfig
	pipeline.detectors = m.registry.GetHealthyByNames(effectiveConfig.Detectors)
	pipeline.mu.Unlock()

	// Update stats
	pipeline.statsMu.Lock()
	pipeline.stats.CurrentMode = effectiveConfig.Mode
	pipeline.stats.ActiveDetectors = effectiveConfig.Detectors
	pipeline.statsMu.Unlock()

	log.Printf("[Pipeline] Updated config for camera %s (mode: %s)", cameraID, effectiveConfig.Mode)
	return nil
}

// GetStats returns pipeline statistics for a camera
func (m *DetectionPipelineManager) GetStats(cameraID string) *PipelineStats {
	m.mu.RLock()
	pipeline, exists := m.pipelines[cameraID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	pipeline.statsMu.RLock()
	defer pipeline.statsMu.RUnlock()

	// Return a copy
	stats := *pipeline.stats
	return &stats
}

// SubscribeResults registers a handler for detection results
func (m *DetectionPipelineManager) SubscribeResults(handler DetectionResultHandler) func() {
	return m.eventBus.Subscribe(handler)
}

// Close shuts down all pipelines
func (m *DetectionPipelineManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for cameraID, pipeline := range m.pipelines {
		pipeline.stop()
		delete(m.pipelines, cameraID)
	}

	log.Printf("[Pipeline] Closed all detection pipelines")
	return nil
}

// GetEffectiveConfig returns the effective detection configuration for a camera
// Returns nil if the camera doesn't have an active pipeline
func (m *DetectionPipelineManager) GetEffectiveConfig(cameraID string) *EffectiveConfig {
	m.mu.RLock()
	pipeline, exists := m.pipelines[cameraID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	pipeline.mu.RLock()
	defer pipeline.mu.RUnlock()

	// Return a copy to avoid race conditions
	config := *pipeline.config
	return &config
}

// ConfigProviderFunc returns a function that can be used by legacy components
// (like StreamDetector) to check detection mode before running YOLO
func (m *DetectionPipelineManager) ConfigProviderFunc() func(cameraID string) *EffectiveConfig {
	return func(cameraID string) *EffectiveConfig {
		return m.GetEffectiveConfig(cameraID)
	}
}

// run is the main processing loop for a single camera pipeline
func (p *DetectionPipeline) run() {
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
	}()

	// Subscribe to frame provider
	sub, err := p.frameProvider.Subscribe(p.cameraID, 5)
	if err != nil {
		log.Printf("[Pipeline] Failed to subscribe to frames for camera %s: %v", p.cameraID, err)
		return
	}
	p.subscription = sub
	defer p.frameProvider.Unsubscribe(sub)

	log.Printf("[Pipeline] Processing loop started for camera %s", p.cameraID)

	for {
		select {
		case <-p.stopCh:
			return
		case <-sub.Done:
			return
		case frame := <-sub.Channel:
			if frame == nil {
				continue
			}
			p.processFrame(frame)
		}
	}
}

func (p *DetectionPipeline) stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	close(p.stopCh)

	// Wait briefly for graceful shutdown
	time.Sleep(100 * time.Millisecond)
}

func (p *DetectionPipeline) processFrame(frame *FrameData) {
	p.mu.RLock()
	strategy := p.strategy
	detectors := p.detectors
	lastResult := p.lastResult
	p.mu.RUnlock()

	// Check if strategy says we should detect
	if !strategy.ShouldDetect(frame, lastResult) {
		return
	}

	// Run detection in sequential mode (YOLO → Face → Plate)
	// Parallel mode was removed because it causes time-travel effects
	result := p.runSequential(frame, detectors)

	if result == nil {
		return
	}

	// Update last result for strategy
	if len(result.Results) > 0 {
		p.mu.Lock()
		p.lastResult = result.Results[0]
		p.mu.Unlock()
	}

	// Notify strategy
	strategy.OnDetectionComplete(result.Results[0])

	// Update stats
	p.statsMu.Lock()
	p.stats.DetectionsTotal++
	p.stats.LastDetectionTime = time.Now().Unix()
	p.stats.AvgInferenceMs = (p.stats.AvgInferenceMs + result.TotalInferenceMs) / 2
	p.statsMu.Unlock()

	// Publish result
	p.eventBus.Publish(result)
}

// runSequential chains detectors: YOLO → Face (if person) → Plate (if vehicle)
func (p *DetectionPipeline) runSequential(frame *FrameData, detectors []Detector) *MergedDetectionResult {
	if len(detectors) == 0 {
		return nil
	}

	ctx := context.Background()
	merged := &MergedDetectionResult{
		CameraID:  frame.CameraID,
		FrameSeq:  frame.Seq,
		Timestamp: frame.Timestamp,
		Results:   make([]*DetectionResult, 0),
	}

	var primaryResult *DetectionResult

	// Run primary detector (usually YOLO)
	for _, det := range detectors {
		if det.Name() == "yolo" || det.Name() == "dino" {
			result, err := det.DetectAnnotated(ctx, frame)
			if err != nil {
				log.Printf("[Pipeline] Primary detection error for camera %s: %v", p.cameraID, err)
				continue
			}

			if result != nil {
				primaryResult = result
				merged.Results = append(merged.Results, result)
				merged.Detections = append(merged.Detections, result.Detections...)
				merged.TotalInferenceMs += result.InferenceMs
				merged.ImageData = result.ImageData
				break
			}
		}
	}

	if primaryResult == nil {
		return nil
	}

	// Run conditional detectors based on primary results
	for _, det := range detectors {
		condDet, isConditional := det.(ConditionalDetector)
		if !isConditional {
			continue
		}

		// Check if this detector should run based on primary results
		if !condDet.ShouldRun(primaryResult) {
			continue
		}

		result, err := det.DetectAnnotated(ctx, frame)
		if err != nil {
			log.Printf("[Pipeline] Conditional detection error (%s) for camera %s: %v",
				det.Name(), p.cameraID, err)
			continue
		}

		if result != nil {
			merged.Results = append(merged.Results, result)
			merged.Faces = append(merged.Faces, result.Faces...)
			merged.Plates = append(merged.Plates, result.Plates...)
			merged.TotalInferenceMs += result.InferenceMs

			// Use annotated image from face detector if available (has both YOLO boxes and face boxes)
			if len(result.ImageData) > 0 && det.Name() == "face" {
				merged.ImageData = result.ImageData
			}
		}
	}

	// Set motion flag based on detection results
	merged.HasMotion = len(merged.Detections) > 0 || len(merged.Faces) > 0

	return merged
}

// Ensure DetectionPipelineManager implements PipelineManager
var _ PipelineManager = (*DetectionPipelineManager)(nil)
