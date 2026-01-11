package pipeline

import (
	"log"
	"sync"
)

// StreamingBridge connects the detection pipeline to streaming infrastructure
// It forwards detection results and annotated frames to stream providers
type StreamingBridge struct {
	providers []StreamOverlayProvider
	mu        sync.RWMutex
}

// NewStreamingBridge creates a new streaming bridge
func NewStreamingBridge(providers ...StreamOverlayProvider) *StreamingBridge {
	return &StreamingBridge{
		providers: providers,
	}
}

// AddProvider adds a stream provider to the bridge
func (b *StreamingBridge) AddProvider(provider StreamOverlayProvider) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.providers = append(b.providers, provider)
}

// OnDetectionResult implements DetectionResultHandler
// Forwards annotated frames and detection data to all registered stream providers
func (b *StreamingBridge) OnDetectionResult(result *MergedDetectionResult) {
	if result == nil {
		return
	}

	b.mu.RLock()
	providers := b.providers
	b.mu.RUnlock()

	for _, provider := range providers {
		if provider == nil {
			continue
		}

		// Forward annotated image with sequence number for ordering validation
		if len(result.ImageData) > 0 {
			provider.SetAnnotatedFrame(result.CameraID, result.FrameSeq, result.ImageData)
		}

		// Forward detection metadata (for client-side rendering if needed)
		provider.UpdateDetections(result.CameraID, result.Detections, result.Faces)
	}
}

// Ensure StreamingBridge implements DetectionResultHandler
var _ DetectionResultHandler = (*StreamingBridge)(nil)

// FrameDistributor distributes frames from FrameProvider to multiple consumers
// This allows both streaming and detection pipelines to receive frames
type FrameDistributor struct {
	frameProvider FrameProvider
	subscriptions map[string][]*distributorSubscription
	mu            sync.RWMutex
}

type distributorSubscription struct {
	consumer FrameConsumer
	sub      *FrameSubscription
}

// NewFrameDistributor creates a new frame distributor
func NewFrameDistributor(frameProvider FrameProvider) *FrameDistributor {
	return &FrameDistributor{
		frameProvider: frameProvider,
		subscriptions: make(map[string][]*distributorSubscription),
	}
}

// Subscribe registers a consumer to receive frames for a camera
func (d *FrameDistributor) Subscribe(cameraID string, consumer FrameConsumer) error {
	sub, err := d.frameProvider.Subscribe(cameraID, 5)
	if err != nil {
		return err
	}

	ds := &distributorSubscription{
		consumer: consumer,
		sub:      sub,
	}

	d.mu.Lock()
	d.subscriptions[cameraID] = append(d.subscriptions[cameraID], ds)
	d.mu.Unlock()

	// Start forwarding frames to consumer
	go func() {
		for {
			select {
			case <-sub.Done:
				return
			case frame := <-sub.Channel:
				if frame != nil {
					consumer.OnFrame(frame)
				}
			}
		}
	}()

	log.Printf("[FrameDistributor] Consumer subscribed to camera %s", cameraID)
	return nil
}

// Unsubscribe removes a consumer from receiving frames
func (d *FrameDistributor) Unsubscribe(cameraID string, consumer FrameConsumer) {
	d.mu.Lock()
	defer d.mu.Unlock()

	subs := d.subscriptions[cameraID]
	for i, ds := range subs {
		if ds.consumer == consumer {
			d.frameProvider.Unsubscribe(ds.sub)
			d.subscriptions[cameraID] = append(subs[:i], subs[i+1:]...)
			log.Printf("[FrameDistributor] Consumer unsubscribed from camera %s", cameraID)
			return
		}
	}
}

// UnsubscribeAll removes all consumers for a camera
func (d *FrameDistributor) UnsubscribeAll(cameraID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, ds := range d.subscriptions[cameraID] {
		d.frameProvider.Unsubscribe(ds.sub)
	}
	delete(d.subscriptions, cameraID)
	log.Printf("[FrameDistributor] All consumers unsubscribed from camera %s", cameraID)
}

// StreamConsumer wraps a StreamOverlayProvider to receive raw frames
// Used when detection is disabled but streaming is still needed
type StreamConsumer struct {
	cameraID string
	provider StreamOverlayProvider
}

// NewStreamConsumer creates a consumer that forwards frames to a stream provider
func NewStreamConsumer(cameraID string, provider StreamOverlayProvider) *StreamConsumer {
	return &StreamConsumer{
		cameraID: cameraID,
		provider: provider,
	}
}

// OnFrame implements FrameConsumer
func (c *StreamConsumer) OnFrame(frame *FrameData) {
	if c.provider != nil && frame != nil {
		// Forward raw frame to stream when no detection is running
		c.provider.SetAnnotatedFrame(c.cameraID, frame.Seq, frame.Data)
	}
}

// Ensure StreamConsumer implements FrameConsumer
var _ FrameConsumer = (*StreamConsumer)(nil)
