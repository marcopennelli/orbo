import { useState, useEffect } from 'react';
import { Activity, Clock, Camera, Target, AlertTriangle } from 'lucide-react';
import type { MotionEvent } from '../../types';
import { getEventFrame, frameResponseToDataUrl } from '../../api/events';
import { Modal, Badge, Spinner } from '../ui';

interface EventModalProps {
  event: MotionEvent | null;
  isOpen: boolean;
  onClose: () => void;
}

export default function EventModal({ event, isOpen, onClose }: EventModalProps) {
  const [imageSrc, setImageSrc] = useState<string | null>(null);
  const [imageLoading, setImageLoading] = useState(false);
  const [imageError, setImageError] = useState(false);

  useEffect(() => {
    if (!event || !isOpen) {
      setImageSrc(null);
      setImageError(false);
      return;
    }

    let cancelled = false;
    setImageLoading(true);

    async function loadFrame() {
      try {
        const frame = await getEventFrame(event!.id);
        if (!cancelled) {
          setImageSrc(frameResponseToDataUrl(frame));
          setImageLoading(false);
        }
      } catch {
        if (!cancelled) {
          setImageError(true);
          setImageLoading(false);
        }
      }
    }

    loadFrame();

    return () => {
      cancelled = true;
    };
  }, [event, isOpen]);

  if (!event) return null;

  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    return date.toLocaleString();
  };

  const formatConfidence = (confidence: number) => {
    return `${(confidence * 100).toFixed(1)}%`;
  };

  const getDetectionType = () => {
    if (event.detection_device === 'dinov3') return 'DINOv3';
    if (event.detection_device === 'cuda') return 'YOLO GPU';
    if (event.detection_device === 'cpu') return 'YOLO CPU';
    return 'MOTION';
  };

  const getThreatVariant = (level?: string) => {
    switch (level) {
      case 'high':
        return 'danger';
      case 'medium':
        return 'warning';
      case 'low':
        return 'info';
      default:
        return 'default';
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Event Details" size="lg">
      <div className="space-y-4">
        {/* Image */}
        <div className="aspect-video bg-bg-dark rounded-lg overflow-hidden flex items-center justify-center">
          {imageLoading ? (
            <Spinner />
          ) : imageError ? (
            <div className="text-text-muted">
              <Activity className="w-8 h-8" />
            </div>
          ) : imageSrc ? (
            <img
              src={imageSrc}
              alt={`Event ${event.id}`}
              className="w-full h-full object-contain"
            />
          ) : null}
        </div>

        {/* Details grid */}
        <div className="grid grid-cols-2 gap-4">
          <div className="flex items-center gap-2 p-3 bg-bg-card rounded-lg">
            <Camera className="w-5 h-5 text-accent" />
            <div>
              <p className="text-xs text-text-muted">Camera</p>
              <p className="text-sm text-text-primary">{event.camera_id.slice(0, 8)}...</p>
            </div>
          </div>

          <div className="flex items-center gap-2 p-3 bg-bg-card rounded-lg">
            <Clock className="w-5 h-5 text-accent" />
            <div>
              <p className="text-xs text-text-muted">Timestamp</p>
              <p className="text-sm text-text-primary">{formatTime(event.timestamp)}</p>
            </div>
          </div>

          <div className="flex items-center gap-2 p-3 bg-bg-card rounded-lg">
            <Activity className="w-5 h-5 text-accent" />
            <div>
              <p className="text-xs text-text-muted">Detection Type</p>
              <p className="text-sm text-text-primary">{getDetectionType()}</p>
            </div>
          </div>

          <div className="flex items-center gap-2 p-3 bg-bg-card rounded-lg">
            <Target className="w-5 h-5 text-accent" />
            <div>
              <p className="text-xs text-text-muted">Confidence</p>
              <p className="text-sm text-text-primary">{formatConfidence(event.confidence)}</p>
            </div>
          </div>
        </div>

        {/* Threat level */}
        {event.threat_level && event.threat_level !== 'none' && (
          <div className="flex items-center gap-2 p-3 bg-bg-card rounded-lg">
            <AlertTriangle className="w-5 h-5 text-accent-orange" />
            <div>
              <p className="text-xs text-text-muted">Threat Level</p>
              <Badge variant={getThreatVariant(event.threat_level)}>
                {event.threat_level.toUpperCase()}
              </Badge>
            </div>
          </div>
        )}

        {/* Detected object class */}
        {event.object_class && (
          <div className="p-3 bg-bg-card rounded-lg">
            <p className="text-xs text-text-muted mb-2">Detected Object</p>
            <Badge variant="info">
              {event.object_class}
              {event.object_confidence && ` (${(event.object_confidence * 100).toFixed(1)}%)`}
            </Badge>
          </div>
        )}

        {/* Inference time */}
        {event.inference_time_ms !== undefined && (
          <div className="text-xs text-text-muted">
            Inference time: {event.inference_time_ms.toFixed(1)}ms
          </div>
        )}

        {/* Event ID */}
        <div className="text-xs text-text-muted">
          Event ID: {event.id}
        </div>
      </div>
    </Modal>
  );
}
