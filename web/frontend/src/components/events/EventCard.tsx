import { useState, useEffect } from 'react';
import { Activity, Clock, Camera, Eye, AlertTriangle } from 'lucide-react';
import type { MotionEvent } from '../../types';
import { getEventFrame, frameResponseToDataUrl } from '../../api/events';
import { Badge, Button } from '../ui';

interface EventCardProps {
  event: MotionEvent;
  onView: () => void;
}

export default function EventCard({ event, onView }: EventCardProps) {
  const [imageSrc, setImageSrc] = useState<string | null>(null);
  const [imageLoaded, setImageLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function loadFrame() {
      try {
        const frame = await getEventFrame(event.id);
        if (!cancelled) {
          setImageSrc(frameResponseToDataUrl(frame));
        }
      } catch {
        if (!cancelled) {
          setImageError(true);
        }
      }
    }

    loadFrame();

    return () => {
      cancelled = true;
    };
  }, [event.id]);

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
        return 'error';
      case 'medium':
        return 'warning';
      case 'low':
        return 'info';
      default:
        return 'default';
    }
  };

  return (
    <div className="bg-bg-card border border-border rounded-lg overflow-hidden hover:border-text-muted transition-colors">
      {/* Thumbnail */}
      <div className="aspect-video bg-bg-dark relative">
        {imageSrc && !imageError ? (
          <img
            src={imageSrc}
            alt={`Event ${event.id}`}
            className={`w-full h-full object-cover transition-opacity ${imageLoaded ? 'opacity-100' : 'opacity-0'}`}
            onLoad={() => setImageLoaded(true)}
            onError={() => setImageError(true)}
          />
        ) : imageError ? (
          <div className="absolute inset-0 flex items-center justify-center text-text-muted">
            <Activity className="w-8 h-8" />
          </div>
        ) : (
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin" />
          </div>
        )}

        {/* Detection type badge */}
        <div className="absolute top-2 left-2">
          <Badge variant="info">
            {getDetectionType()}
          </Badge>
        </div>

        {/* Confidence badge */}
        <div className="absolute top-2 right-2">
          <Badge variant="success">{formatConfidence(event.confidence)}</Badge>
        </div>

        {/* Threat level indicator */}
        {event.threat_level && event.threat_level !== 'none' && (
          <div className="absolute bottom-2 left-2">
            <Badge variant={getThreatVariant(event.threat_level)}>
              <AlertTriangle className="w-3 h-3 mr-1" />
              {event.threat_level.toUpperCase()}
            </Badge>
          </div>
        )}
      </div>

      {/* Info */}
      <div className="p-3">
        <div className="flex items-center gap-2 text-xs text-text-secondary mb-2">
          <Camera className="w-3 h-3" />
          <span className="truncate">Camera {event.camera_id.slice(0, 8)}...</span>
        </div>

        <div className="flex items-center gap-2 text-xs text-text-muted mb-3">
          <Clock className="w-3 h-3" />
          <span>{formatTime(event.timestamp)}</span>
        </div>

        {/* Detected object class */}
        {event.object_class && (
          <div className="mb-3">
            <Badge variant="info" className="text-xs">
              {event.object_class}
              {event.object_confidence && ` (${(event.object_confidence * 100).toFixed(0)}%)`}
            </Badge>
          </div>
        )}

        <Button variant="secondary" size="sm" onClick={onView} className="w-full">
          <Eye className="w-4 h-4 mr-1" />
          View Details
        </Button>
      </div>
    </div>
  );
}
