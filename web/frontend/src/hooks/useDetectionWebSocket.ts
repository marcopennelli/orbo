import { useState, useEffect, useRef, useCallback } from 'react';

// Types matching backend ws/message.go
export interface ObjectDetection {
  class: string;
  confidence: number;
  bbox: number[]; // [x, y, w, h] in pixels
  threat_level?: string;
}

export interface DetectionMessage {
  type: 'detection';
  camera_id: string;
  timestamp: string;
  frame_width: number;
  frame_height: number;
  objects: ObjectDetection[];
  frame?: string; // Base64 encoded JPEG frame
}

export interface FaceDetection {
  bbox: number[]; // [x, y, w, h] in pixels
  confidence: number;
  identity?: string;
  is_known: boolean;
  similarity?: number;
  age?: number;
  gender?: string;
}

export interface FaceMessage {
  type: 'face';
  camera_id: string;
  timestamp: string;
  faces: FaceDetection[];
}

export interface FrameMessage {
  type: 'frame';
  camera_id: string;
  timestamp: string;
  frame_width: number;
  frame_height: number;
  frame: string; // Base64 encoded JPEG frame
}

type WSMessage = DetectionMessage | FaceMessage | FrameMessage;

interface UseDetectionWebSocketOptions {
  cameraId: string;
  enabled: boolean;
  onDetection?: (msg: DetectionMessage) => void;
  onFace?: (msg: FaceMessage) => void;
}

interface UseDetectionWebSocketReturn {
  detections: ObjectDetection[];
  faces: FaceDetection[];
  isConnected: boolean;
  frameWidth: number;
  frameHeight: number;
  lastUpdate: Date | null;
  frameData: string | null; // Base64 encoded JPEG frame synced with detections
}

export function useDetectionWebSocket({
  cameraId,
  enabled,
  onDetection,
  onFace,
}: UseDetectionWebSocketOptions): UseDetectionWebSocketReturn {
  const [detections, setDetections] = useState<ObjectDetection[]>([]);
  const [faces, setFaces] = useState<FaceDetection[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [frameWidth, setFrameWidth] = useState(0);
  const [frameHeight, setFrameHeight] = useState(0);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);
  const [frameData, setFrameData] = useState<string | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);
  const reconnectAttempts = useRef(0);
  const maxReconnectAttempts = 10;

  const clearAllState = useCallback(() => {
    setDetections([]);
    setFaces([]);
    setFrameData(null);
  }, []);

  // Track last detection update separately from frame updates
  const [lastDetectionUpdate, setLastDetectionUpdate] = useState<Date | null>(null);

  // Auto-clear detections after 3 seconds of no detection updates
  // This allows detections to persist while frames keep streaming
  useEffect(() => {
    if (!lastDetectionUpdate) return;

    const timeout = setTimeout(() => {
      setDetections([]);
      setFaces([]);
    }, 3000);

    return () => clearTimeout(timeout);
  }, [lastDetectionUpdate]);

  useEffect(() => {
    if (!enabled || !cameraId) {
      // Clean up if disabled
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      setIsConnected(false);
      clearAllState();
      return;
    }

    const connect = () => {
      // Build WebSocket URL
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws/detections/${cameraId}`;

      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log(`[WS] Connected to camera ${cameraId}`);
        setIsConnected(true);
        reconnectAttempts.current = 0;
      };

      ws.onmessage = (event) => {
        try {
          const msg: WSMessage = JSON.parse(event.data);
          setLastUpdate(new Date());

          if (msg.type === 'detection') {
            const detMsg = msg as DetectionMessage;
            setDetections(detMsg.objects || []);
            setLastDetectionUpdate(new Date()); // Track detection updates for timeout
            if (detMsg.frame_width > 0) setFrameWidth(detMsg.frame_width);
            if (detMsg.frame_height > 0) setFrameHeight(detMsg.frame_height);
            // Set synced frame if provided
            if (detMsg.frame) {
              setFrameData(`data:image/jpeg;base64,${detMsg.frame}`);
            }
            onDetection?.(detMsg);
          } else if (msg.type === 'face') {
            const faceMsg = msg as FaceMessage;
            setFaces(faceMsg.faces || []);
            setLastDetectionUpdate(new Date()); // Track detection updates for timeout
            onFace?.(faceMsg);
          } else if (msg.type === 'frame') {
            // Live frame without detections - keep existing detections until they expire via timeout
            const frameMsg = msg as FrameMessage;
            // Don't clear detections here - they will be cleared by the 3-second timeout
            // or updated when new detection messages arrive
            if (frameMsg.frame_width > 0) setFrameWidth(frameMsg.frame_width);
            if (frameMsg.frame_height > 0) setFrameHeight(frameMsg.frame_height);
            setFrameData(`data:image/jpeg;base64,${frameMsg.frame}`);
          }
        } catch (err) {
          console.error('[WS] Failed to parse message:', err);
        }
      };

      ws.onclose = () => {
        console.log(`[WS] Disconnected from camera ${cameraId}`);
        setIsConnected(false);
        wsRef.current = null;

        // Attempt to reconnect with exponential backoff
        if (enabled && reconnectAttempts.current < maxReconnectAttempts) {
          const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 30000);
          reconnectAttempts.current++;
          console.log(`[WS] Reconnecting in ${delay}ms (attempt ${reconnectAttempts.current})`);
          reconnectTimeoutRef.current = window.setTimeout(connect, delay);
        }
      };

      ws.onerror = (error) => {
        console.error(`[WS] Error for camera ${cameraId}:`, error);
      };
    };

    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [cameraId, enabled, onDetection, onFace, clearAllState]);

  return {
    detections,
    faces,
    isConnected,
    frameWidth,
    frameHeight,
    lastUpdate,
    frameData,
  };
}
