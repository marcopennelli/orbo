import { Camera, Power, PowerOff, Edit, Trash2, AlertCircle, Bell, BellOff, Database, DatabaseZap } from 'lucide-react';
import type { Camera as CameraType } from '../../types';
import { Badge, Button, Tooltip } from '../ui';

interface CameraItemProps {
  camera: CameraType;
  isSelected?: boolean;
  onSelect?: () => void;
  onEdit?: () => void;
  onDelete?: () => void;
  onToggleActive?: () => void;
  onToggleEvents?: () => void;
  onToggleNotifications?: () => void;
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
  onToggleEvents,
  onToggleNotifications,
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
          {isActive && (
            <>
              <Tooltip content={camera.events_enabled ? 'Events stored' : 'Events disabled'}>
                <Badge
                  variant={camera.events_enabled ? 'success' : 'default'}
                  className="text-xs cursor-help"
                >
                  {camera.events_enabled ? <DatabaseZap className="w-3 h-3" /> : <Database className="w-3 h-3" />}
                </Badge>
              </Tooltip>
              <Tooltip content={camera.notifications_enabled ? 'Notifications enabled' : 'Notifications disabled'}>
                <Badge
                  variant={camera.notifications_enabled ? 'success' : 'default'}
                  className="text-xs cursor-help"
                >
                  {camera.notifications_enabled ? <Bell className="w-3 h-3" /> : <BellOff className="w-3 h-3" />}
                </Badge>
              </Tooltip>
            </>
          )}
        </div>
      </div>

      {/* Actions */}
      <div className="flex items-center justify-end gap-1 mt-3 pt-3 border-t border-border">
        <Tooltip content={isActive ? 'Deactivate camera' : 'Activate camera'} position="bottom">
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation();
              onToggleActive?.();
            }}
            disabled={isLoading}
          >
            {isActive ? <PowerOff className="w-4 h-4" /> : <Power className="w-4 h-4" />}
          </Button>
        </Tooltip>
        {isActive && (
          <>
            <Tooltip content={camera.events_enabled ? 'Disable event storage' : 'Enable event storage'} position="bottom">
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  onToggleEvents?.();
                }}
                disabled={isLoading}
                className={camera.events_enabled ? 'text-accent-green' : 'text-text-muted'}
              >
                {camera.events_enabled ? <DatabaseZap className="w-4 h-4" /> : <Database className="w-4 h-4" />}
              </Button>
            </Tooltip>
            <Tooltip content={camera.notifications_enabled ? 'Disable Telegram notifications' : 'Enable Telegram notifications'} position="bottom">
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => {
                  e.stopPropagation();
                  onToggleNotifications?.();
                }}
                disabled={isLoading}
                className={camera.notifications_enabled ? 'text-accent-green' : 'text-text-muted'}
              >
                {camera.notifications_enabled ? <Bell className="w-4 h-4" /> : <BellOff className="w-4 h-4" />}
              </Button>
            </Tooltip>
          </>
        )}
        <Tooltip content="Edit camera" position="bottom">
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation();
              onEdit?.();
            }}
          >
            <Edit className="w-4 h-4" />
          </Button>
        </Tooltip>
        <Tooltip content="Delete camera" position="bottom">
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation();
              onDelete?.();
            }}
            className="text-accent-red hover:text-accent-red"
          >
            <Trash2 className="w-4 h-4" />
          </Button>
        </Tooltip>
      </div>
    </div>
  );
}
