import { useState, useEffect } from 'react';
import { Activity, Clock, Camera, Eye, AlertTriangle, User, UserCheck, UserX } from 'lucide-react';
import type { MotionEvent, Camera as CameraType } from '../../types';
import { getEventFrame, frameResponseToDataUrl } from '../../api/events';
import { Badge, Button } from '../ui';

interface EventCardProps {
  event: MotionEvent;
  cameras: CameraType[];
  onView: () => void;
}

export default function EventCard({ event, cameras, onView }: EventCardProps) {
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

  const formatRelativeTime = (timestamp: string) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffSec = Math.floor(diffMs / 1000);
    const diffMin = Math.floor(diffSec / 60);
    const diffHour = Math.floor(diffMin / 60);
    const diffDay = Math.floor(diffHour / 24);

    if (diffSec < 60) return 'Just now';
    if (diffMin < 60) return `${diffMin} min ago`;
    if (diffHour < 24) return `${diffHour}h ago`;
    if (diffDay < 7) return `${diffDay}d ago`;
    return date.toLocaleDateString();
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

  // Get camera name from cameras list, fallback to truncated ID
  const getCameraName = () => {
    const camera = cameras.find(c => c.id === event.camera_id);
    return camera?.name || `Camera ${event.camera_id.slice(0, 8)}...`;
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

        {/* Face recognition badge */}
        {event.faces_detected !== undefined && event.faces_detected > 0 && (
          <div className="absolute top-2 right-2">
            {event.known_identities && event.known_identities.length > 0 ? (
              <Badge variant="success">
                <UserCheck className="w-3 h-3 mr-1" />
                {event.known_identities.length}
              </Badge>
            ) : event.unknown_faces_count && event.unknown_faces_count > 0 ? (
              <Badge variant="warning">
                <UserX className="w-3 h-3 mr-1" />
                {event.unknown_faces_count}
              </Badge>
            ) : (
              <Badge variant="default">
                <User className="w-3 h-3 mr-1" />
                {event.faces_detected}
              </Badge>
            )}
          </div>
        )}

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
          <span className="truncate">{getCameraName()}</span>
        </div>

        <div className="flex items-center gap-2 text-xs text-text-muted mb-3">
          <Clock className="w-3 h-3" />
          <span>{formatRelativeTime(event.timestamp)}</span>
        </div>

        {/* Detected object class */}
        {event.object_class && (
          <div className="mb-2">
            <Badge variant="info" className="text-xs">
              {event.object_class}
              {event.object_confidence && ` (${(event.object_confidence * 100).toFixed(0)}%)`}
            </Badge>
          </div>
        )}

        {/* Face recognition identities */}
        {event.known_identities && event.known_identities.length > 0 && (
          <div className="flex items-center gap-1 text-xs text-green-400 mb-2">
            <UserCheck className="w-3 h-3" />
            <span className="truncate">{event.known_identities.join(', ')}</span>
          </div>
        )}
        {event.unknown_faces_count !== undefined && event.unknown_faces_count > 0 && (
          <div className="flex items-center gap-1 text-xs text-yellow-400 mb-2">
            <UserX className="w-3 h-3" />
            <span>{event.unknown_faces_count} unknown</span>
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
