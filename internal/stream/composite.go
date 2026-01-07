package stream

// CompositeStreamOverlayProvider broadcasts frames to multiple stream providers
// This allows the detection pipeline to send annotated frames to both MJPEG and WebCodecs
type CompositeStreamOverlayProvider struct {
	providers []StreamOverlayProvider
}

// NewCompositeStreamOverlayProvider creates a provider that broadcasts to multiple backends
func NewCompositeStreamOverlayProvider(providers ...StreamOverlayProvider) *CompositeStreamOverlayProvider {
	return &CompositeStreamOverlayProvider{
		providers: providers,
	}
}

// UpdateDetections forwards detection data to all providers
func (c *CompositeStreamOverlayProvider) UpdateDetections(cameraID string, detections []Detection, faces []FaceDetection) {
	for _, p := range c.providers {
		if p != nil {
			p.UpdateDetections(cameraID, detections, faces)
		}
	}
}

// SetAnnotatedFrame forwards annotated frame to all providers
func (c *CompositeStreamOverlayProvider) SetAnnotatedFrame(cameraID string, frameData []byte) {
	for _, p := range c.providers {
		if p != nil {
			p.SetAnnotatedFrame(cameraID, frameData)
		}
	}
}

// GetCurrentFrameSeq returns frame sequence from the first provider (used for freshness checks)
func (c *CompositeStreamOverlayProvider) GetCurrentFrameSeq(cameraID string) uint64 {
	for _, p := range c.providers {
		if p != nil {
			return p.GetCurrentFrameSeq(cameraID)
		}
	}
	return 0
}
