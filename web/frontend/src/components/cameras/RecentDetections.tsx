import { useState, useEffect } from 'react';
import { Activity, Clock, AlertTriangle, Eye } from 'lucide-react';
import type { MotionEvent } from '../../types';
import { getEventFrame, frameResponseToDataUrl } from '../../api/events';
import { Badge, Button } from '../ui';

interface DetectionThumbnailProps {
  event: MotionEvent;
  onClick: () => void;
}

function DetectionThumbnail({ event, onClick }: DetectionThumbnailProps) {
  const [imageSrc, setImageSrc] = useState<string | null>(null);
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
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  const getDetectionType = () => {
    if (event.detection_device === 'dinov3') return 'DINOv3';
    if (event.detection_device === 'cuda') return 'YOLO';
    if (event.detection_device === 'cpu') return 'YOLO';
    return 'MOTION';
  };

  const getThreatColor = (level?: string) => {
    switch (level) {
      case 'high':
        return 'text-accent-red';
      case 'medium':
        return 'text-accent-orange';
      case 'low':
        return 'text-accent';
      default:
        return 'text-text-muted';
    }
  };

  return (
    <div
      className="bg-bg-card border border-border rounded-lg overflow-hidden hover:border-accent cursor-pointer transition-colors"
      onClick={onClick}
    >
      {/* Thumbnail */}
      <div className="aspect-video bg-bg-dark relative">
        {imageSrc && !imageError ? (
          <img
            src={imageSrc}
            alt={`Detection ${event.id}`}
            className="w-full h-full object-cover"
          />
        ) : imageError ? (
          <div className="absolute inset-0 flex items-center justify-center text-text-muted">
            <Activity className="w-6 h-6" />
          </div>
        ) : (
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="w-4 h-4 border-2 border-accent border-t-transparent rounded-full animate-spin" />
          </div>
        )}

        {/* Detection type badge */}
        <div className="absolute top-1 left-1">
          <Badge variant="info" className="text-[10px] px-1 py-0">
            {getDetectionType()}
          </Badge>
        </div>

        {/* Threat indicator */}
        {event.threat_level && event.threat_level !== 'none' && (
          <div className={`absolute top-1 right-1 ${getThreatColor(event.threat_level)}`}>
            <AlertTriangle className="w-4 h-4" />
          </div>
        )}
      </div>

      {/* Info */}
      <div className="p-2">
        <div className="flex items-center justify-between text-[10px]">
          <span className="text-text-secondary truncate">
            {event.object_class || 'Motion'}
          </span>
          <span className="text-text-muted flex items-center gap-1">
            <Clock className="w-3 h-3" />
            {formatTime(event.timestamp)}
          </span>
        </div>
      </div>
    </div>
  );
}

interface RecentDetectionsProps {
  events: MotionEvent[];
  isLoading?: boolean;
  onViewEvent?: (event: MotionEvent) => void;
  maxItems?: number;
}

export default function RecentDetections({
  events,
  isLoading,
  onViewEvent,
  maxItems = 10,
}: RecentDetectionsProps) {
  const recentEvents = events.slice(0, maxItems);

  return (
    <div className="h-full flex flex-col bg-bg-panel border-l border-border">
      {/* Header */}
      <div className="flex items-center justify-between p-3 border-b border-border">
        <h3 className="text-sm font-semibold text-text-primary flex items-center gap-2">
          <Activity className="w-4 h-4 text-accent-red" />
          Recent Detections
        </h3>
        <span className="text-xs text-text-muted">{events.length} total</span>
      </div>

      {/* Events list */}
      <div className="flex-1 overflow-y-auto p-2 space-y-2">
        {isLoading ? (
          <div className="flex items-center justify-center h-32">
            <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin" />
          </div>
        ) : recentEvents.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-32 text-text-muted">
            <Activity className="w-8 h-8 mb-2" />
            <p className="text-sm">No detections yet</p>
          </div>
        ) : (
          recentEvents.map((event) => (
            <DetectionThumbnail
              key={event.id}
              event={event}
              onClick={() => onViewEvent?.(event)}
            />
          ))
        )}
      </div>
    </div>
  );
}
