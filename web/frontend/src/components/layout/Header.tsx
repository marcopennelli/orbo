import { Settings, Play, Square, LogOut } from 'lucide-react';
import { Badge, Button } from '../ui';

interface HeaderProps {
  detectionRunning: boolean;
  activeCameras: number;
  totalCameras: number;
  yoloEnabled: boolean;
  telegramEnabled: boolean;
  onToggleDetection: () => void;
  onOpenSettings: () => void;
  isLoading?: boolean;
  isAuthEnabled?: boolean;
  onLogout?: () => void;
}

export default function Header({
  detectionRunning,
  activeCameras,
  totalCameras,
  yoloEnabled,
  telegramEnabled,
  onToggleDetection,
  onOpenSettings,
  isLoading,
  isAuthEnabled,
  onLogout,
}: HeaderProps) {
  return (
    <header className="bg-bg-panel border-b border-border px-6 py-4">
      <div className="flex items-center justify-between">
        {/* Logo and title */}
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg bg-accent flex items-center justify-center">
              <span className="text-bg-dark font-bold text-lg">O</span>
            </div>
            <h1 className="text-xl font-semibold text-text-primary">Orbo</h1>
          </div>

          {/* Status badges */}
          <div className="flex items-center gap-2">
            <Badge variant={detectionRunning ? 'success' : 'default'}>
              {detectionRunning ? 'Detection Active' : 'Detection Stopped'}
            </Badge>
            <Badge variant="info">
              {activeCameras}/{totalCameras} Cameras
            </Badge>
            {yoloEnabled && <Badge variant="info">YOLO</Badge>}
            {telegramEnabled && <Badge variant="success">Telegram</Badge>}
          </div>
        </div>

        {/* Controls */}
        <div className="flex items-center gap-3">
          <Button
            variant={detectionRunning ? 'danger' : 'primary'}
            onClick={onToggleDetection}
            loading={isLoading}
          >
            {detectionRunning ? (
              <>
                <Square className="w-4 h-4 mr-2" />
                Stop Detection
              </>
            ) : (
              <>
                <Play className="w-4 h-4 mr-2" />
                Start Detection
              </>
            )}
          </Button>
          <Button variant="secondary" onClick={onOpenSettings}>
            <Settings className="w-4 h-4 mr-2" />
            Settings
          </Button>
          {isAuthEnabled && onLogout && (
            <Button variant="secondary" onClick={onLogout}>
              <LogOut className="w-4 h-4 mr-2" />
              Logout
            </Button>
          )}
        </div>
      </div>
    </header>
  );
}
