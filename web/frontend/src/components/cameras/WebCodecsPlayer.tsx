import { useRef, useEffect, useCallback, useState } from 'react';
import { Wifi, WifiOff, Video, VideoOff, Zap, Eye, EyeOff } from 'lucide-react';
import { Button, Spinner } from '../ui';
import useWebCodecsStream from '../../hooks/useWebCodecsStream';

interface WebCodecsPlayerProps {
  cameraId: string;
  cameraName: string;
  enabled: boolean;
  detectionEnabled?: boolean;
  className?: string;
  onFullscreen?: () => void;
}

export default function WebCodecsPlayer({
  cameraId,
  cameraName,
  enabled,
  detectionEnabled = true,
  className = '',
  onFullscreen,
}: WebCodecsPlayerProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const ctxRef = useRef<CanvasRenderingContext2D | null>(null);
  const [dimensions, setDimensions] = useState({ width: 640, height: 480 });

  // Initialize canvas context
  useEffect(() => {
    if (canvasRef.current) {
      ctxRef.current = canvasRef.current.getContext('2d', {
        alpha: false,
        desynchronized: true, // Reduces latency
      });
    }
  }, []);

  // Handle incoming frames
  const handleFrame = useCallback((imageData: ImageData, isAnnotated: boolean) => {
    if (!ctxRef.current || !canvasRef.current) return;

    // Update canvas size if needed
    if (canvasRef.current.width !== imageData.width || canvasRef.current.height !== imageData.height) {
      canvasRef.current.width = imageData.width;
      canvasRef.current.height = imageData.height;
      setDimensions({ width: imageData.width, height: imageData.height });
    }

    // Draw frame directly to canvas
    ctxRef.current.putImageData(imageData, 0, 0);
  }, []);

  const {
    isConnected,
    isLoading,
    hasError,
    fps,
    reconnect,
  } = useWebCodecsStream({
    cameraId,
    enabled,
    onFrame: handleFrame,
  });

  return (
    <div className={`relative bg-bg-card rounded-lg overflow-hidden ${className}`}>
      {/* Header */}
      <div className="absolute top-0 left-0 right-0 z-10 flex items-center justify-between p-2 bg-gradient-to-b from-black/60 to-transparent">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-white truncate">{cameraName}</span>
          {enabled && (
            <span className="flex items-center gap-1 text-xs">
              {isConnected ? (
                <>
                  <Wifi className="w-3 h-3 text-accent-green" />
                  <Video className="w-3 h-3 text-accent-green" />
                  <span title="WebCodecs low-latency mode">
                    <Zap className="w-3 h-3 text-accent-yellow" />
                  </span>
                </>
              ) : (
                <>
                  <WifiOff className="w-3 h-3 text-accent-red" />
                  <VideoOff className="w-3 h-3 text-accent-red" />
                </>
              )}
              {/* Detection status indicator */}
              <span title={detectionEnabled ? 'AI detection enabled' : 'AI detection disabled (streaming only)'}>
                {detectionEnabled ? (
                  <Eye className="w-3 h-3 text-accent-green" />
                ) : (
                  <EyeOff className="w-3 h-3 text-text-muted" />
                )}
              </span>
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {isConnected && fps > 0 && (
            <span className="text-xs text-white/70">{fps} fps</span>
          )}
          {onFullscreen && (
            <Button
              variant="ghost"
              size="sm"
              onClick={onFullscreen}
              className="text-white hover:bg-white/20"
            >
              <Video className="w-4 h-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Video canvas */}
      <div className="aspect-video bg-bg-dark flex items-center justify-center relative">
        {!enabled ? (
          <div className="text-center text-text-muted">
            <VideoOff className="w-8 h-8 mx-auto mb-2 opacity-50" />
            <p>Camera inactive</p>
          </div>
        ) : hasError ? (
          <div className="text-center text-text-muted">
            <VideoOff className="w-8 h-8 mx-auto mb-2 text-accent-red opacity-75" />
            <p>Stream unavailable</p>
            <p className="text-xs mt-1">WebSocket connection failed</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={reconnect}
              className="mt-2 text-accent-blue"
            >
              Retry
            </Button>
          </div>
        ) : (
          <>
            {isLoading && (
              <div className="absolute inset-0 flex flex-col items-center justify-center bg-bg-dark z-10">
                <Spinner />
                <span className="text-xs text-text-muted mt-2">Connecting via WebSocket...</span>
              </div>
            )}
            <canvas
              ref={canvasRef}
              width={dimensions.width}
              height={dimensions.height}
              className="w-full h-full object-contain"
              style={{ imageRendering: 'auto' }}
            />
          </>
        )}
      </div>
    </div>
  );
}
