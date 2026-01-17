import { useState, useEffect, useRef, useCallback } from 'react';
import { Maximize2, Wifi, WifiOff, Video, VideoOff, Zap, Image, Bell, BellOff } from 'lucide-react';
import type { Camera } from '../../types';
import { Button, Spinner } from '../ui';
import WebCodecsPlayer from './WebCodecsPlayer';

type StreamMode = 'mjpeg' | 'webcodecs';

interface CameraFeedProps {
  camera: Camera;
  onFullscreen?: () => void;
  className?: string;
  defaultMode?: StreamMode;
  rawMode?: boolean; // Use raw stream without annotations/bounding boxes
}

// Debug logging helper
const DEBUG = true;
const log = (cameraId: string, ...args: unknown[]) => {
  if (DEBUG) {
    console.log(`[CameraFeed:${cameraId}]`, ...args);
  }
};

const STREAM_MODE_KEY = 'orbo-stream-mode';

export default function CameraFeed({
  camera,
  onFullscreen,
  className = '',
  defaultMode = 'webcodecs',
  rawMode = false,
}: CameraFeedProps) {
  const [streamMode, setStreamMode] = useState<StreamMode>(() => {
    const saved = localStorage.getItem(STREAM_MODE_KEY);
    return (saved as StreamMode) || defaultMode;
  });
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  // Use timestamp for unique stream key on each mount to prevent browser caching
  const [streamKey, setStreamKey] = useState(() => Date.now());
  const imgRef = useRef<HTMLImageElement>(null);
  const mountIdRef = useRef(0); // Track mount instance to ignore stale callbacks

  const isActive = camera.status === 'active';

  const toggleStreamMode = useCallback(() => {
    const newMode = streamMode === 'mjpeg' ? 'webcodecs' : 'mjpeg';
    setStreamMode(newMode);
    localStorage.setItem(STREAM_MODE_KEY, newMode);
    log(camera.id, `Switched to ${newMode} mode`);
  }, [streamMode, camera.id]);

  // Build MJPEG stream URL with cache-busting param
  // Note: MJPEG doesn't support raw mode yet, only WebCodecs does
  const streamUrl = `/video/stream/${camera.id}?t=${streamKey}`;

  // Handle component mount/unmount and stream connection
  useEffect(() => {
    // Increment mount ID to invalidate any pending callbacks from previous mount
    mountIdRef.current += 1;
    const currentMountId = mountIdRef.current;

    log(camera.id, `Component mounted (mountId: ${currentMountId}), isActive:`, isActive);

    if (!isActive) {
      log(camera.id, 'Camera not active, resetting state');
      setIsLoading(false);
      setIsConnected(false);
      setHasError(false);
      return;
    }

    // Generate new stream key on mount to force fresh connection
    const newStreamKey = Date.now();
    log(camera.id, `Starting stream connection, streamKey: ${newStreamKey}`);
    setStreamKey(newStreamKey);
    setIsLoading(true);
    setHasError(false);
    setIsConnected(false);

    // Cleanup on unmount
    return () => {
      log(camera.id, `Component unmounting (mountId: ${currentMountId})`);
      // Clear the image source to stop the MJPEG stream
      if (imgRef.current) {
        log(camera.id, 'Clearing image source to stop stream');
        imgRef.current.src = '';
      }
    };
  }, [isActive, camera.id]);

  const handleImageLoad = useCallback(() => {
    const currentMountId = mountIdRef.current;
    log(camera.id, `Stream connected (onLoad event, mountId: ${currentMountId})`);
    // Only update state if this is still the current mount
    setIsLoading(false);
    setIsConnected(true);
    setHasError(false);
  }, [camera.id]);

  const handleImageError = useCallback(() => {
    const currentMountId = mountIdRef.current;
    log(camera.id, `Stream error (onError event, mountId: ${currentMountId})`);
    setIsLoading(false);
    setIsConnected(false);
    setHasError(true);
  }, [camera.id]);

  // Retry connection function
  const handleRetry = useCallback(() => {
    const newKey = Date.now();
    log(camera.id, `Retry requested, new streamKey: ${newKey}`);
    setStreamKey(newKey);
    setIsLoading(true);
    setHasError(false);
    setIsConnected(false);
  }, [camera.id]);

  // Use WebCodecs player for low-latency mode
  if (streamMode === 'webcodecs') {
    return (
      <div className={`relative ${className}`}>
        <WebCodecsPlayer
          cameraId={camera.id}
          cameraName={camera.name}
          enabled={isActive}
          alertsEnabled={camera.alerts_enabled}
          rawMode={rawMode}
          onFullscreen={onFullscreen}
          className="h-full"
        />
        {/* Mode toggle button */}
        <Button
          variant="ghost"
          size="sm"
          onClick={toggleStreamMode}
          className="absolute bottom-2 right-2 z-20 text-white/70 hover:text-white hover:bg-black/50"
          title="Switch to MJPEG (higher latency)"
        >
          <Image className="w-4 h-4" />
        </Button>
      </div>
    );
  }

  // MJPEG mode
  return (
    <div className={`relative bg-bg-card rounded-lg overflow-hidden ${className}`}>
      {/* Header */}
      <div className="absolute top-0 left-0 right-0 z-10 flex items-center justify-between p-2 bg-gradient-to-b from-black/60 to-transparent">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-white truncate">{camera.name}</span>
          {isActive && (
            <span className="flex items-center gap-1 text-xs">
              {isConnected ? (
                <>
                  <Wifi className="w-3 h-3 text-accent-green" />
                  <Video className="w-3 h-3 text-accent-green" />
                </>
              ) : (
                <>
                  <WifiOff className="w-3 h-3 text-accent-red" />
                  <VideoOff className="w-3 h-3 text-accent-red" />
                </>
              )}
              {/* Alerts status indicator */}
              <span title={camera.alerts_enabled ? 'Alerts enabled' : 'Alerts disabled (bounding boxes only)'}>
                {camera.alerts_enabled ? (
                  <Bell className="w-3 h-3 text-accent-green" />
                ) : (
                  <BellOff className="w-3 h-3 text-text-muted" />
                )}
              </span>
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          {onFullscreen && (
            <Button variant="ghost" size="sm" onClick={onFullscreen} className="text-white hover:bg-white/20">
              <Maximize2 className="w-4 h-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Video feed */}
      <div className="aspect-video bg-bg-dark flex items-center justify-center relative">
        {!isActive ? (
          <div className="text-center text-text-muted">
            <VideoOff className="w-8 h-8 mx-auto mb-2 opacity-50" />
            <p>Camera {camera.status === 'error' ? 'error' : 'inactive'}</p>
          </div>
        ) : hasError ? (
          <div className="text-center text-text-muted">
            <VideoOff className="w-8 h-8 mx-auto mb-2 text-accent-red opacity-75" />
            <p>Stream unavailable</p>
            <p className="text-xs mt-1">Check video pipeline service</p>
            <Button variant="ghost" size="sm" onClick={handleRetry} className="mt-2 text-accent-blue">
              Retry
            </Button>
          </div>
        ) : (
          <>
            {isLoading && (
              <div className="absolute inset-0 flex flex-col items-center justify-center bg-bg-dark z-10">
                <Spinner />
                <span className="text-xs text-text-muted mt-2">Connecting to stream...</span>
              </div>
            )}
            {/* MJPEG stream - bounding boxes are rendered in the stream itself */}
            <img
              key={`stream-${camera.id}-${streamKey}`}
              ref={imgRef}
              src={streamUrl}
              alt={camera.name}
              className="w-full h-full object-contain"
              onLoad={handleImageLoad}
              onError={handleImageError}
            />
          </>
        )}
      </div>

      {/* Mode toggle button */}
      <Button
        variant="ghost"
        size="sm"
        onClick={toggleStreamMode}
        className="absolute bottom-2 right-2 z-20 text-white/70 hover:text-white hover:bg-black/50"
        title="Switch to WebCodecs (lower latency)"
      >
        <Zap className="w-4 h-4" />
      </Button>
    </div>
  );
}
