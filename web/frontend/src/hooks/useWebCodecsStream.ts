import { useState, useEffect, useRef, useCallback } from 'react';

interface WebCodecsStreamState {
  isConnected: boolean;
  isLoading: boolean;
  hasError: boolean;
  errorMessage: string | null;
  fps: number;
  latencyMs: number;
}

interface UseWebCodecsStreamOptions {
  cameraId: string;
  enabled: boolean;
  onFrame?: (imageData: ImageData, isAnnotated: boolean) => void;
}

// Message types from server
const FRAME_TYPE_RAW = 0;
const FRAME_TYPE_ANNOTATED = 1;

export function useWebCodecsStream({
  cameraId,
  enabled,
  onFrame,
}: UseWebCodecsStreamOptions) {
  const [state, setState] = useState<WebCodecsStreamState>({
    isConnected: false,
    isLoading: true,
    hasError: false,
    errorMessage: null,
    fps: 0,
    latencyMs: 0,
  });

  const wsRef = useRef<WebSocket | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const ctxRef = useRef<CanvasRenderingContext2D | null>(null);
  const frameCountRef = useRef(0);
  const lastFpsUpdateRef = useRef(Date.now());
  const reconnectTimeoutRef = useRef<number | null>(null);
  const mountedRef = useRef(true);

  // Create offscreen canvas for decoding
  useEffect(() => {
    canvasRef.current = document.createElement('canvas');
    ctxRef.current = canvasRef.current.getContext('2d');
    return () => {
      canvasRef.current = null;
      ctxRef.current = null;
    };
  }, []);

  // Decode JPEG frame and return ImageData
  const decodeFrame = useCallback(async (frameData: ArrayBuffer): Promise<ImageData | null> => {
    if (!ctxRef.current || !canvasRef.current) return null;

    try {
      const blob = new Blob([frameData], { type: 'image/jpeg' });
      const bitmap = await createImageBitmap(blob);

      // Resize canvas if needed
      if (canvasRef.current.width !== bitmap.width || canvasRef.current.height !== bitmap.height) {
        canvasRef.current.width = bitmap.width;
        canvasRef.current.height = bitmap.height;
      }

      ctxRef.current.drawImage(bitmap, 0, 0);
      bitmap.close();

      return ctxRef.current.getImageData(0, 0, canvasRef.current.width, canvasRef.current.height);
    } catch (err) {
      console.error('[WebCodecs] Frame decode error:', err);
      return null;
    }
  }, []);

  // Handle incoming WebSocket message
  const handleMessage = useCallback(async (event: MessageEvent) => {
    if (!(event.data instanceof ArrayBuffer)) return;

    const data = new Uint8Array(event.data);
    if (data.length < 5) return;

    // Parse message: 1 byte type + 4 bytes length + frame data
    const frameType = data[0];
    const frameLength = (data[1] << 24) | (data[2] << 16) | (data[3] << 8) | data[4];
    const frameData = data.slice(5, 5 + frameLength);

    const isAnnotated = frameType === FRAME_TYPE_ANNOTATED;

    // Decode and callback
    const imageData = await decodeFrame(frameData.buffer);
    if (imageData && onFrame) {
      onFrame(imageData, isAnnotated);
    }

    // Update FPS counter
    frameCountRef.current++;
    const now = Date.now();
    if (now - lastFpsUpdateRef.current >= 1000) {
      if (mountedRef.current) {
        setState(prev => ({
          ...prev,
          fps: frameCountRef.current,
        }));
      }
      frameCountRef.current = 0;
      lastFpsUpdateRef.current = now;
    }
  }, [decodeFrame, onFrame]);

  // Connect to WebSocket
  const connect = useCallback(() => {
    if (!enabled || !cameraId) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/video/${cameraId}`;

    console.log(`[WebCodecs] Connecting to ${wsUrl}`);
    setState(prev => ({ ...prev, isLoading: true, hasError: false, errorMessage: null }));

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      console.log(`[WebCodecs] Connected to camera ${cameraId}`);
      if (mountedRef.current) {
        setState(prev => ({
          ...prev,
          isConnected: true,
          isLoading: false,
          hasError: false,
        }));
      }
    };

    ws.onmessage = handleMessage;

    ws.onerror = (error) => {
      console.error(`[WebCodecs] WebSocket error:`, error);
      if (mountedRef.current) {
        setState(prev => ({
          ...prev,
          hasError: true,
          errorMessage: 'WebSocket connection error',
        }));
      }
    };

    ws.onclose = (event) => {
      console.log(`[WebCodecs] Disconnected from camera ${cameraId}, code: ${event.code}`);
      if (mountedRef.current) {
        setState(prev => ({
          ...prev,
          isConnected: false,
          isLoading: false,
        }));

        // Attempt reconnect after 2 seconds
        if (enabled) {
          reconnectTimeoutRef.current = window.setTimeout(() => {
            console.log(`[WebCodecs] Attempting reconnect to camera ${cameraId}`);
            connect();
          }, 2000);
        }
      }
    };

    wsRef.current = ws;
  }, [cameraId, enabled, handleMessage]);

  // Disconnect
  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
  }, []);

  // Connect/disconnect on mount/unmount or when enabled changes
  useEffect(() => {
    mountedRef.current = true;

    if (enabled) {
      connect();
    } else {
      disconnect();
    }

    return () => {
      mountedRef.current = false;
      disconnect();
    };
  }, [enabled, cameraId, connect, disconnect]);

  return {
    ...state,
    reconnect: connect,
  };
}

export default useWebCodecsStream;
