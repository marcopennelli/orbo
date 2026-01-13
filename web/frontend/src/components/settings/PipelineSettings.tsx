import { useState, useCallback, useEffect, useMemo } from 'react';
import { Info, Save, RotateCcw } from 'lucide-react';
import type { PipelineConfig, DetectionMode, DetectorType } from '../../types';
import { Select, Input, Button } from '../ui';

interface PipelineSettingsProps {
  config: PipelineConfig;
  onSave: (config: PipelineConfig) => void;
  isLoading?: boolean;
  isSaving?: boolean;
}

const modeOptions = [
  { value: 'disabled', label: 'Disabled' },
  { value: 'visual_only', label: 'Visual Only (No Alerts)' },
  { value: 'continuous', label: 'Continuous (Every Frame)' },
  { value: 'motion_triggered', label: 'Motion Triggered' },
  { value: 'scheduled', label: 'Scheduled Interval' },
  { value: 'hybrid', label: 'Hybrid (Motion + Scheduled)' },
];

const availableDetectors: { id: DetectorType; label: string; description: string; available: boolean }[] = [
  { id: 'yolo', label: 'YOLO Object Detection', description: 'Detects persons, vehicles, animals', available: true },
  { id: 'face', label: 'Face Recognition', description: 'Identifies known faces (requires YOLO)', available: true },
  { id: 'plate', label: 'License Plate', description: 'Reads vehicle plates (coming soon)', available: false },
];

export default function PipelineSettings({
  config,
  onSave,
  isLoading,
  isSaving,
}: PipelineSettingsProps) {
  // Local state for editing
  const [localConfig, setLocalConfig] = useState<PipelineConfig>(config);

  // Sync local state when config changes from server
  useEffect(() => {
    setLocalConfig(config);
  }, [config]);

  // Check if there are unsaved changes
  const hasChanges = useMemo(() => {
    return (
      localConfig.mode !== config.mode ||
      localConfig.execution_mode !== config.execution_mode ||
      JSON.stringify(localConfig.detectors) !== JSON.stringify(config.detectors) ||
      localConfig.schedule_interval !== config.schedule_interval ||
      localConfig.motion_sensitivity !== config.motion_sensitivity ||
      localConfig.motion_cooldown_seconds !== config.motion_cooldown_seconds
    );
  }, [localConfig, config]);

  const showMotionSettings = localConfig.mode === 'motion_triggered' || localConfig.mode === 'hybrid';
  const showScheduleSettings = localConfig.mode === 'scheduled' || localConfig.mode === 'hybrid';
  const showDetectorSettings = localConfig.mode !== 'disabled';

  // Update local config
  const updateLocal = useCallback((updates: Partial<PipelineConfig>) => {
    setLocalConfig(prev => ({ ...prev, ...updates }));
  }, []);

  // Handle save
  const handleSave = useCallback(() => {
    onSave(localConfig);
  }, [localConfig, onSave]);

  // Handle reset
  const handleReset = useCallback(() => {
    setLocalConfig(config);
  }, [config]);

  // Handle checkbox toggle for detectors
  const handleDetectorToggle = useCallback((detectorId: DetectorType, enabled: boolean) => {
    const currentDetectors = localConfig.detectors || [];
    let newDetectors: DetectorType[];

    if (enabled) {
      newDetectors = [...currentDetectors, detectorId];
    } else {
      newDetectors = currentDetectors.filter(d => d !== detectorId);
    }

    updateLocal({ detectors: newDetectors });
  }, [localConfig.detectors, updateLocal]);

  return (
    <div className="flex flex-col h-full">
      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto space-y-6 pb-4">
        {/* Header */}
        <div>
          <h3 className="text-sm font-medium text-text-primary mb-1">Detection Pipeline</h3>
          <p className="text-xs text-text-muted">Configure when and how detection runs</p>
        </div>

        {/* Detection Mode */}
        <Select
          label="Detection Mode"
          options={modeOptions}
          value={localConfig.mode}
          onChange={(e) => updateLocal({ mode: e.target.value as DetectionMode })}
          disabled={isLoading || isSaving}
        />

        {/* Execution Mode - fixed to sequential */}
        {showDetectorSettings && (
          <div className="p-3 bg-bg-card/50 rounded-lg border border-border">
            <div className="flex items-center gap-2 mb-1">
              <span className="text-sm font-medium text-text-secondary">Execution Mode:</span>
              <span className="text-sm text-accent">Sequential</span>
            </div>
            <p className="text-xs text-text-muted">
              Detectors run in order: YOLO first, then Face if person detected, then Plate if vehicle detected.
            </p>
          </div>
        )}

        {/* Detectors */}
        {showDetectorSettings && (
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">
              Detectors
            </label>
            <p className="text-xs text-text-muted mb-3">
              Select which detectors to enable. They run sequentially in this order: YOLO → Face → Plate.
            </p>

            {/* Checkboxes for all detectors */}
            <div className="space-y-2">
              {availableDetectors.map((detector) => {
                const isEnabled = (localConfig.detectors || []).includes(detector.id);
                return (
                  <label
                    key={detector.id}
                    className={`flex items-start gap-3 p-2 rounded-md border border-border hover:bg-bg-card/50 cursor-pointer
                      ${!detector.available ? 'opacity-50 cursor-not-allowed' : ''}
                    `}
                  >
                    <input
                      type="checkbox"
                      checked={isEnabled}
                      onChange={(e) => handleDetectorToggle(detector.id, e.target.checked)}
                      disabled={isLoading || isSaving || !detector.available}
                      className="mt-0.5 w-4 h-4 text-accent border-border rounded focus:ring-accent"
                    />
                    <div className="flex-1">
                      <span className="text-sm text-text-secondary block">{detector.label}</span>
                      <span className="text-xs text-text-muted">{detector.description}</span>
                    </div>
                  </label>
                );
              })}
            </div>
          </div>
        )}

        {/* Motion Settings */}
        {showMotionSettings && (
          <div className="space-y-4 p-3 bg-bg-card rounded-lg border border-border">
            <h4 className="text-sm font-medium text-text-secondary">Motion Settings</h4>

            <div>
              <label className="block text-sm text-text-secondary mb-2">
                Motion Sensitivity: {localConfig.motion_sensitivity.toFixed(2)}
              </label>
              <input
                type="range"
                min="0"
                max="1"
                step="0.01"
                value={localConfig.motion_sensitivity}
                onChange={(e) => updateLocal({ motion_sensitivity: parseFloat(e.target.value) })}
                disabled={isLoading || isSaving}
                className="w-full h-2 bg-border rounded-lg appearance-none cursor-pointer accent-accent"
              />
              <div className="flex justify-between text-xs text-text-muted mt-1">
                <span>More sensitive</span>
                <span>Less sensitive</span>
              </div>
            </div>

            <Input
              type="number"
              label="Cooldown (seconds)"
              value={localConfig.motion_cooldown_seconds}
              onChange={(e) => updateLocal({ motion_cooldown_seconds: parseInt(e.target.value) || 0 })}
              disabled={isLoading || isSaving}
              min={0}
              hint="Wait time after motion stops before detection pauses"
            />
          </div>
        )}

        {/* Schedule Settings */}
        {showScheduleSettings && (
          <div className="space-y-4 p-3 bg-bg-card rounded-lg border border-border">
            <h4 className="text-sm font-medium text-text-secondary">Schedule Settings</h4>

            <Input
              label="Detection Interval"
              value={localConfig.schedule_interval}
              onChange={(e) => updateLocal({ schedule_interval: e.target.value })}
              disabled={isLoading || isSaving}
              placeholder="5s"
              hint="Go duration format: '5s', '10s', '1m'"
            />
          </div>
        )}

        {/* Info box */}
        <div className="flex items-start gap-2 p-3 bg-bg-card/50 rounded-lg border border-border">
          <Info size={16} className="text-accent flex-shrink-0 mt-0.5" />
          <p className="text-xs text-text-muted">
            {localConfig.mode === 'disabled' && 'Detection is disabled. Cameras will only stream video without any processing.'}
            {localConfig.mode === 'visual_only' && 'Detection runs and draws bounding boxes on video, but no alerts are sent. Useful for testing.'}
            {localConfig.mode === 'continuous' && 'Every frame will be analyzed. This uses more resources but provides maximum detection coverage.'}
            {localConfig.mode === 'motion_triggered' && 'Detection runs only when motion is detected. Best for static scenes to save resources.'}
            {localConfig.mode === 'scheduled' && 'Detection runs at regular intervals regardless of motion.'}
            {localConfig.mode === 'hybrid' && 'Detection runs on motion OR at scheduled intervals, whichever comes first.'}
          </p>
        </div>
      </div>

      {/* Action buttons - fixed at bottom */}
      <div className="flex items-center justify-between pt-4 border-t border-border mt-auto">
        <div className="text-xs text-text-muted">
          {hasChanges && (
            <span className="text-accent-yellow">You have unsaved changes</span>
          )}
        </div>
        <div className="flex gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleReset}
            disabled={!hasChanges || isSaving}
          >
            <RotateCcw size={14} className="mr-1" />
            Reset
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={handleSave}
            disabled={!hasChanges || isSaving}
            loading={isSaving}
          >
            <Save size={14} className="mr-1" />
            Save Changes
          </Button>
        </div>
      </div>
    </div>
  );
}
