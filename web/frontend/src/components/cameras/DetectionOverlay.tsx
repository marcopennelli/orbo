import { useRef, useEffect } from 'react';
import type { ObjectDetection, FaceDetection } from '../../hooks/useDetectionWebSocket';

interface DetectionOverlayProps {
  detections: ObjectDetection[];
  faces: FaceDetection[];
  frameWidth: number;
  frameHeight: number;
  displayWidth: number;
  displayHeight: number;
  offsetX?: number;
  offsetY?: number;
}

// Color mapping for threat levels
const THREAT_COLORS: Record<string, string> = {
  low: '#3B82F6',    // blue
  medium: '#F59E0B', // yellow/amber
  high: '#EF4444',   // red
  default: '#3B82F6', // blue
};

// Face colors
const FACE_KNOWN_COLOR = '#22C55E';   // green
const FACE_UNKNOWN_COLOR = '#F97316'; // orange

export default function DetectionOverlay({
  detections,
  faces,
  frameWidth,
  frameHeight,
  displayWidth,
  displayHeight,
  offsetX = 0,
  offsetY = 0,
}: DetectionOverlayProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Clear canvas
    ctx.clearRect(0, 0, displayWidth, displayHeight);

    // Calculate scale factors
    const scaleX = frameWidth > 0 ? displayWidth / frameWidth : 1;
    const scaleY = frameHeight > 0 ? displayHeight / frameHeight : 1;

    // Draw object detections
    for (const det of detections || []) {
      if (!det.bbox || det.bbox.length < 4) continue;
      const [x, y, w, h] = det.bbox;
      const scaledX = x * scaleX;
      const scaledY = y * scaleY;
      const scaledW = w * scaleX;
      const scaledH = h * scaleY;

      const color = THREAT_COLORS[det.threat_level || 'default'] || THREAT_COLORS.default;

      // Draw bounding box
      ctx.strokeStyle = color;
      ctx.lineWidth = 2;
      ctx.strokeRect(scaledX, scaledY, scaledW, scaledH);

      // Draw label background
      const label = `${det.class} ${Math.round(det.confidence * 100)}%`;
      ctx.font = '12px Inter, system-ui, sans-serif';
      const textMetrics = ctx.measureText(label);
      const textHeight = 16;
      const padding = 4;

      ctx.fillStyle = color;
      ctx.fillRect(
        scaledX,
        scaledY - textHeight - padding,
        textMetrics.width + padding * 2,
        textHeight + padding
      );

      // Draw label text
      ctx.fillStyle = '#FFFFFF';
      ctx.fillText(label, scaledX + padding, scaledY - padding - 2);
    }

    // Draw face detections
    for (const face of faces || []) {
      if (!face.bbox || face.bbox.length < 4) continue;
      const [x, y, w, h] = face.bbox;
      const scaledX = x * scaleX;
      const scaledY = y * scaleY;
      const scaledW = w * scaleX;
      const scaledH = h * scaleY;

      const color = face.is_known ? FACE_KNOWN_COLOR : FACE_UNKNOWN_COLOR;

      // Draw face bounding box
      ctx.strokeStyle = color;
      ctx.lineWidth = 2;
      ctx.strokeRect(scaledX, scaledY, scaledW, scaledH);

      // Build label
      let label = face.is_known && face.identity ? face.identity : 'Unknown';
      if (face.similarity !== undefined) {
        label += ` ${Math.round(face.similarity * 100)}%`;
      }

      // Draw label background
      ctx.font = '12px Inter, system-ui, sans-serif';
      const textMetrics = ctx.measureText(label);
      const textHeight = 16;
      const padding = 4;

      ctx.fillStyle = color;
      ctx.fillRect(
        scaledX,
        scaledY - textHeight - padding,
        textMetrics.width + padding * 2,
        textHeight + padding
      );

      // Draw label text
      ctx.fillStyle = '#FFFFFF';
      ctx.fillText(label, scaledX + padding, scaledY - padding - 2);

      // Draw age/gender info below the box if available
      if (face.age !== undefined || face.gender !== undefined) {
        const info = [face.gender, face.age ? `${face.age}y` : ''].filter(Boolean).join(', ');
        if (info) {
          const infoMetrics = ctx.measureText(info);
          ctx.fillStyle = 'rgba(0, 0, 0, 0.7)';
          ctx.fillRect(
            scaledX,
            scaledY + scaledH + 2,
            infoMetrics.width + padding * 2,
            textHeight
          );
          ctx.fillStyle = '#FFFFFF';
          ctx.fillText(info, scaledX + padding, scaledY + scaledH + textHeight - 2);
        }
      }
    }
  }, [detections, faces, frameWidth, frameHeight, displayWidth, displayHeight, offsetX, offsetY]);

  return (
    <canvas
      ref={canvasRef}
      width={displayWidth}
      height={displayHeight}
      className="absolute pointer-events-none"
      style={{
        width: displayWidth,
        height: displayHeight,
        left: offsetX,
        top: offsetY,
      }}
    />
  );
}
