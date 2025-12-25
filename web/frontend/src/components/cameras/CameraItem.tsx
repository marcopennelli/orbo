import { Camera, Power, PowerOff, Edit, Trash2, AlertCircle } from 'lucide-react';
import type { Camera as CameraType } from '../../types';
import { Badge, Button } from '../ui';

interface CameraItemProps {
  camera: CameraType;
  isSelected?: boolean;
  onSelect?: () => void;
  onEdit?: () => void;
  onDelete?: () => void;
  onToggleActive?: () => void;
  isLoading?: boolean;
}

const statusVariant = (status: CameraType['status']) => {
  switch (status) {
    case 'active':
      return 'success';
    case 'error':
      return 'error';
    default:
      return 'default';
  }
};

const statusLabel = (status: CameraType['status']) => {
  switch (status) {
    case 'active':
      return 'Active';
    case 'error':
      return 'Error';
    default:
      return 'Inactive';
  }
};

export default function CameraItem({
  camera,
  isSelected,
  onSelect,
  onEdit,
  onDelete,
  onToggleActive,
  isLoading,
}: CameraItemProps) {
  const isActive = camera.status === 'active';
  const isError = camera.status === 'error';

  return (
    <div
      onClick={onSelect}
      className={`
        p-3 rounded-lg border cursor-pointer transition-colors
        ${
          isSelected
            ? 'bg-accent/10 border-accent'
            : 'bg-bg-card border-border hover:border-text-muted'
        }
      `}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-3 min-w-0 flex-1">
          <div
            className={`
              w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0
              ${isActive ? 'bg-accent-green/20 text-accent-green' : isError ? 'bg-accent-red/20 text-accent-red' : 'bg-bg-hover text-text-muted'}
            `}
          >
            {isError ? <AlertCircle className="w-5 h-5" /> : <Camera className="w-5 h-5" />}
          </div>
          <div className="min-w-0">
            <h3 className="font-medium text-text-primary truncate">{camera.name}</h3>
            <p className="text-xs text-text-muted truncate">{camera.device}</p>
          </div>
        </div>

        <div className="flex items-center gap-1 flex-shrink-0">
          <Badge variant={statusVariant(camera.status)} className="text-xs">
            {statusLabel(camera.status)}
          </Badge>
        </div>
      </div>

      {/* Actions */}
      <div className="flex items-center justify-end gap-1 mt-3 pt-3 border-t border-border">
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation();
            onToggleActive?.();
          }}
          disabled={isLoading}
          title={isActive ? 'Deactivate' : 'Activate'}
        >
          {isActive ? <PowerOff className="w-4 h-4" /> : <Power className="w-4 h-4" />}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation();
            onEdit?.();
          }}
          title="Edit"
        >
          <Edit className="w-4 h-4" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation();
            onDelete?.();
          }}
          title="Delete"
          className="text-accent-red hover:text-accent-red"
        >
          <Trash2 className="w-4 h-4" />
        </Button>
      </div>
    </div>
  );
}
