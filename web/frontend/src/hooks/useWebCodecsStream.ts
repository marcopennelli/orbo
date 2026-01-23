import { useState, useEffect, useRef, useCallback } from 'react';

interface WebCodecsStreamState {
  isConnected: boolean;
  isLoading: boolean;
  hasError: boolean;
  errorMessage: string | null;
  fps: number;
  latencyMs: number;
  currentSeq: number; // Current frame sequence number
}

interface UseWebCodecsStreamOptions {
  cameraId: string;
  enabled: boolean;
  rawMode?: boolean; // Use raw stream without annotations/bounding boxes
  onFrame?: (imageData: ImageData, isAnnotated: boolean) => void;
}

// Message types from server
const FRAME_TYPE_ANNOTATED = 1;

export function useWebCodecsStream({
  cameraId,
  enabled,
  rawMode = false,
  onFrame,
}: UseWebCodecsStreamOptions) {
  const [state, setState] = useState<WebCodecsStreamState>({
    isConnected: false,
    isLoading: true,
    hasError: false,
    errorMessage: null,
    fps: 0,
    latencyMs: 0,
    currentSeq: 0,
  });

  const wsRef = useRef<WebSocket | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const ctxRef = useRef<CanvasRenderingContext2D | null>(null);
  const frameCountRef = useRef(0);
  const lastFpsUpdateRef = useRef(Date.now());
  const lastFrameTimeRef = useRef(Date.now()); // Track when we last received a frame
  const reconnectTimeoutRef = useRef<number | null>(null);
  const watchdogIntervalRef = useRef<number | null>(null);
  const mountedRef = useRef(true);
  const currentCameraIdRef = useRef(cameraId); // Track current camera to filter stale frames
  const lastSeqRef = useRef<bigint>(BigInt(0)); // Track last sequence for ordering validation

  // Watchdog timeout - if no frames received for this duration, reconnect
  const FRAME_TIMEOUT_MS = 5000; // 5 seconds without frames triggers reconnect

  // Keep cameraId ref updated and reset sequence on camera change
  useEffect(() => {
    currentCameraIdRef.current = cameraId;
    lastSeqRef.current = BigInt(0); // Reset sequence tracking for new camera
  }, [cameraId]);

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
  // Takes sourceCameraId to verify frames come from the expected connection
  const createMessageHandler = useCallback((sourceCameraId: string) => {
    return async (event: MessageEvent) => {
      // Ignore frames if camera changed (stale connection still sending)
      if (sourceCameraId !== currentCameraIdRef.current) {
        return;
      }

      if (!(event.data instanceof ArrayBuffer)) return;

      const data = new Uint8Array(event.data);
      // New format: 1 byte type + 8 bytes sequence + 4 bytes length + frame data
      if (data.length < 13) return;

      // Parse message header
      const frameType = data[0];

      // Parse 8-byte sequence number (BigInt for uint64)
      const seqView = new DataView(data.buffer, 1, 8);
      const frameSeq = seqView.getBigUint64(0, false); // big-endian

      // Parse 4-byte frame length
      const frameLength = (data[9] << 24) | (data[10] << 16) | (data[11] << 8) | data[12];
      const frameData = data.slice(13, 13 + frameLength);

      const isAnnotated = frameType === FRAME_TYPE_ANNOTATED;

      // For annotated frames, validate sequence ordering
      // Drop frames that arrive out of order (older than last displayed)
      if (isAnnotated && frameSeq > BigInt(0)) {
        if (frameSeq <= lastSeqRef.current) {
          // Out-of-order frame, drop it
          return;
        }
        lastSeqRef.current = frameSeq;
      }

      // Decode and callback
      const imageData = await decodeFrame(frameData.buffer);
      if (imageData && onFrame) {
        onFrame(imageData, isAnnotated);
      }

      // Update frame timing for watchdog
      lastFrameTimeRef.current = Date.now();

      // Update FPS counter and sequence
      frameCountRef.current++;
      const now = Date.now();
      if (now - lastFpsUpdateRef.current >= 1000) {
        if (mountedRef.current) {
          setState(prev => ({
            ...prev,
            fps: frameCountRef.current,
            currentSeq: Number(frameSeq),
          }));
        }
        frameCountRef.current = 0;
        lastFpsUpdateRef.current = now;
      }
    };
  }, [decodeFrame, onFrame]);

  // Connect to WebSocket
  const connect = useCallback(() => {
    if (!enabled || !cameraId) return;

    // Clear any existing watchdog before reconnecting
    if (watchdogIntervalRef.current) {
      clearInterval(watchdogIntervalRef.current);
      watchdogIntervalRef.current = null;
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    // Use /ws/video/raw/{id} for raw stream, /ws/video/{id} for processed stream
    const wsUrl = rawMode
      ? `${protocol}//${window.location.host}/ws/video/raw/${cameraId}`
      : `${protocol}//${window.location.host}/ws/video/${cameraId}`;

    console.log(`[WebCodecs] Connecting to ${wsUrl}`);
    setState(prev => ({ ...prev, isLoading: true, hasError: false, errorMessage: null }));

    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      console.log(`[WebCodecs] Connected to camera ${cameraId}`);
      // Reset frame timing on successful connection
      lastFrameTimeRef.current = Date.now();

      if (mountedRef.current) {
        setState(prev => ({
          ...prev,
          isConnected: true,
          isLoading: false,
          hasError: false,
        }));

        // Start watchdog timer to detect stalled streams
        watchdogIntervalRef.current = window.setInterval(() => {
          const timeSinceLastFrame = Date.now() - lastFrameTimeRef.current;
          if (timeSinceLastFrame > FRAME_TIMEOUT_MS && wsRef.current?.readyState === WebSocket.OPEN) {
            console.warn(`[WebCodecs] No frames received for ${timeSinceLastFrame}ms, reconnecting...`);
            // Close the connection - the onclose handler will trigger reconnect
            wsRef.current?.close();
          }
        }, 1000); // Check every second
      }
    };

    // Create message handler bound to this specific camera connection
    // This ensures frames from old connections are filtered out
    ws.onmessage = createMessageHandler(cameraId);

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

      // Stop watchdog on disconnect
      if (watchdogIntervalRef.current) {
        clearInterval(watchdogIntervalRef.current);
        watchdogIntervalRef.current = null;
      }

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
  }, [cameraId, enabled, rawMode, createMessageHandler]);

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
