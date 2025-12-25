import type { DetectionConfig } from '../../types';
import { Select, Switch } from '../ui';

interface DetectionSettingsProps {
  config: DetectionConfig;
  onUpdate: (config: Partial<DetectionConfig>) => void;
  isLoading?: boolean;
}

const detectorOptions = [
  { value: 'basic', label: 'Basic Motion Detection' },
  { value: 'yolo', label: 'YOLO AI Detection' },
  { value: 'dinov3', label: 'DINOv3 Detection' },
];

export default function DetectionSettings({ config, onUpdate, isLoading }: DetectionSettingsProps) {
  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-medium text-text-primary mb-1">Detection Settings</h3>
        <p className="text-xs text-text-muted">Configure motion and object detection</p>
      </div>

      <Select
        label="Primary Detector"
        options={detectorOptions}
        value={config.primary_detector}
        onChange={(e) => onUpdate({ primary_detector: e.target.value as DetectionConfig['primary_detector'] })}
        disabled={isLoading}
      />

      <div className="flex items-center justify-between">
        <div>
          <span className="text-sm text-text-secondary">Enable Fallback</span>
          <p className="text-xs text-text-muted">Fall back to basic detection when AI unavailable</p>
        </div>
        <Switch
          checked={config.fallback_enabled ?? true}
          onChange={(fallback_enabled) => onUpdate({ fallback_enabled })}
          disabled={isLoading}
        />
      </div>

      <div className="mt-4 p-3 bg-bg-card rounded-lg">
        <p className="text-xs text-text-muted">
          Additional settings for YOLO and DINOv3 can be configured in their respective sections above.
        </p>
      </div>
    </div>
  );
}
