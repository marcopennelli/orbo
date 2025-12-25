import { useState, useEffect, useCallback, useRef } from 'react';
import { RefreshCw, Maximize2 } from 'lucide-react';
import { getCameraFrame, frameResponseToDataUrl } from '../../api/cameras';
import type { Camera } from '../../types';
import { Button, Spinner } from '../ui';

interface CameraFeedProps {
  camera: Camera;
  refreshInterval?: number;
  onFullscreen?: () => void;
  className?: string;
}

export default function CameraFeed({
  camera,
  refreshInterval = 1000,
  onFullscreen,
  className = '',
}: CameraFeedProps) {
  const [imageSrc, setImageSrc] = useState<string>('');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isPaused, setIsPaused] = useState(false);
  const mountedRef = useRef(true);

  const isActive = camera.status === 'active';

  const loadFrame = useCallback(async () => {
    if (!isActive || isPaused) return;

    try {
      const frame = await getCameraFrame(camera.id);
      if (mountedRef.current) {
        const dataUrl = frameResponseToDataUrl(frame);
        setImageSrc(dataUrl);
        setIsLoading(false);
        setError(null);
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(err instanceof Error ? err.message : 'Failed to load frame');
        setIsLoading(false);
      }
    }
  }, [camera.id, isActive, isPaused]);

  useEffect(() => {
    mountedRef.current = true;
    loadFrame();

    let interval: ReturnType<typeof setInterval> | null = null;
    if (isActive && !isPaused) {
      interval = setInterval(loadFrame, refreshInterval);
    }

    return () => {
      mountedRef.current = false;
      if (interval) clearInterval(interval);
    };
  }, [loadFrame, refreshInterval, isActive, isPaused]);

  const handleRefresh = () => {
    setIsLoading(true);
    setError(null);
    loadFrame();
  };

  return (
    <div className={`relative bg-bg-card rounded-lg overflow-hidden ${className}`}>
      {/* Header */}
      <div className="absolute top-0 left-0 right-0 z-10 flex items-center justify-between p-2 bg-gradient-to-b from-black/60 to-transparent">
        <span className="text-sm font-medium text-white truncate">{camera.name}</span>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={handleRefresh} className="text-white hover:bg-white/20">
            <RefreshCw className="w-4 h-4" />
          </Button>
          {onFullscreen && (
            <Button variant="ghost" size="sm" onClick={onFullscreen} className="text-white hover:bg-white/20">
              <Maximize2 className="w-4 h-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Video feed */}
      <div className="aspect-video bg-bg-dark flex items-center justify-center">
        {!isActive ? (
          <div className="text-center text-text-muted">
            <p>Camera {camera.status === 'error' ? 'error' : 'inactive'}</p>
          </div>
        ) : isLoading && !imageSrc ? (
          <Spinner />
        ) : error ? (
          <div className="text-center text-accent-red">
            <p>{error}</p>
            <Button variant="secondary" size="sm" onClick={handleRefresh} className="mt-2">
              Retry
            </Button>
          </div>
        ) : (
          <img
            src={imageSrc}
            alt={camera.name}
            className="w-full h-full object-contain"
            onClick={() => setIsPaused(!isPaused)}
          />
        )}
      </div>

      {/* Paused indicator */}
      {isPaused && (
        <div className="absolute bottom-2 left-2 px-2 py-1 bg-accent-orange/80 text-white text-xs rounded">
          Paused - Click to resume
        </div>
      )}
    </div>
  );
}
