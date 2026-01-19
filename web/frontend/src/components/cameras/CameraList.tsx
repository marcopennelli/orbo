import { Plus } from 'lucide-react';
import type { Camera } from '../../types';
import { Panel } from '../layout';
import { Button, Spinner } from '../ui';
import CameraItem from './CameraItem';

interface CameraListProps {
  cameras: Camera[];
  selectedCamera?: Camera | null;
  onSelectCamera: (camera: Camera) => void;
  onAddCamera: () => void;
  onEditCamera: (camera: Camera) => void;
  onDeleteCamera: (camera: Camera) => void;
  onToggleCameraActive: (camera: Camera) => void;
  onToggleCameraEvents: (camera: Camera) => void;
  onToggleCameraNotifications: (camera: Camera) => void;
  isLoading?: boolean;
  loadingCameraId?: string;
}

export default function CameraList({
  cameras,
  selectedCamera,
  onSelectCamera,
  onAddCamera,
  onEditCamera,
  onDeleteCamera,
  onToggleCameraActive,
  onToggleCameraEvents,
  onToggleCameraNotifications,
  isLoading,
  loadingCameraId,
}: CameraListProps) {
  return (
    <Panel
      title="Cameras"
      actions={
        <Button size="sm" onClick={onAddCamera}>
          <Plus className="w-4 h-4 mr-1" />
          Add
        </Button>
      }
    >
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : cameras.length === 0 ? (
        <div className="text-center py-8 text-text-muted">
          <p>No cameras configured</p>
          <Button variant="secondary" size="sm" onClick={onAddCamera} className="mt-2">
            Add your first camera
          </Button>
        </div>
      ) : (
        <div className="space-y-2 max-h-[calc(100vh-300px)] overflow-y-auto">
          {cameras.map((camera) => (
            <CameraItem
              key={camera.id}
              camera={camera}
              isSelected={selectedCamera?.id === camera.id}
              onSelect={() => onSelectCamera(camera)}
              onEdit={() => onEditCamera(camera)}
              onDelete={() => onDeleteCamera(camera)}
              onToggleActive={() => onToggleCameraActive(camera)}
              onToggleEvents={() => onToggleCameraEvents(camera)}
              onToggleNotifications={() => onToggleCameraNotifications(camera)}
              isLoading={loadingCameraId === camera.id}
            />
          ))}
        </div>
      )}
    </Panel>
  );
}
