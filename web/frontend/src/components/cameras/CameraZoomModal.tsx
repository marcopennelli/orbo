import { useEffect, useCallback } from 'react';
import { X } from 'lucide-react';
import type { Camera } from '../../types';
import { Button } from '../ui';
import CameraFeed from './CameraFeed';

interface CameraZoomModalProps {
  camera: Camera | null;
  onClose: () => void;
}

export default function CameraZoomModal({ camera, onClose }: CameraZoomModalProps) {
  const handleEscape = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose]
  );

  useEffect(() => {
    if (camera) {
      document.addEventListener('keydown', handleEscape);
      document.body.style.overflow = 'hidden';
    }
    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.body.style.overflow = 'unset';
    };
  }, [camera, handleEscape]);

  if (!camera) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-hidden">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/80" onClick={onClose} />

      {/* Modal content */}
      <div className="relative h-full flex flex-col p-4">
        {/* Header */}
        <div className="flex items-center justify-between mb-4 z-10">
          <h2 className="text-lg font-semibold text-white">{camera.name}</h2>
          <Button variant="ghost" size="sm" onClick={onClose} className="text-white hover:bg-white/20">
            <X className="w-5 h-5" />
          </Button>
        </div>

        {/* Camera feed - takes most of the space */}
        <div className="flex-1 flex items-center justify-center min-h-0">
          <div className="w-full h-full max-w-6xl">
            <CameraFeed
              camera={camera}
              className="h-full w-full !max-w-none"
            />
          </div>
        </div>
      </div>
    </div>
  );
}
