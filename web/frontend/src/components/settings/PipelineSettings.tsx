import { useState, useCallback, useEffect, useMemo } from 'react';
import { Info, Save, RotateCcw, Zap, CheckCircle, XCircle } from 'lucide-react';
import type { PipelineConfig, DetectionMode, DetectorType, YoloConfig } from '../../types';
import { Select, Input, Button } from '../ui';

interface PipelineSettingsProps {
  config: PipelineConfig;
  onSave: (config: PipelineConfig) => void;
  isLoading?: boolean;
  isSaving?: boolean;
  // YOLO settings (shown when YOLO detector is enabled)
  yoloConfig?: YoloConfig;
  onSaveYolo?: (config: YoloConfig) => void;
  onTestYolo?: () => Promise<{ success: boolean; message: string }>;
  isSavingYolo?: boolean;
}

const COMMON_CLASSES = ['person', 'car', 'truck', 'motorcycle', 'bicycle', 'dog', 'cat'];

// Parse comma-separated string to array
const parseClasses = (filter: string | undefined): string[] => {
  if (!filter) return [];
  return filter.split(',').map(c => c.trim().toLowerCase()).filter(c => c.length > 0);
};

// Convert array to comma-separated string
const formatClasses = (classes: string[]): string => {
  return classes.join(', ');
};

const modeOptions = [
  { value: 'disabled', label: 'Disabled (Streaming Only)' },
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
  yoloConfig,
  onSaveYolo,
  onTestYolo,
  isSavingYolo,
}: PipelineSettingsProps) {
  // Local state for editing
  const [localConfig, setLocalConfig] = useState<PipelineConfig>(config);
  const [localYoloConfig, setLocalYoloConfig] = useState<YoloConfig | undefined>(yoloConfig);
  const [yoloTestResult, setYoloTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [isTestingYolo, setIsTestingYolo] = useState(false);

  // Sync local state when config changes from server
  useEffect(() => {
    setLocalConfig(config);
  }, [config]);

  // Sync YOLO config
  useEffect(() => {
    setLocalYoloConfig(yoloConfig);
  }, [yoloConfig]);

  // Check if there are unsaved changes (pipeline)
  const hasPipelineChanges = useMemo(() => {
    return (
      localConfig.mode !== config.mode ||
      localConfig.execution_mode !== config.execution_mode ||
      JSON.stringify(localConfig.detectors) !== JSON.stringify(config.detectors) ||
      localConfig.schedule_interval !== config.schedule_interval ||
      localConfig.motion_sensitivity !== config.motion_sensitivity ||
      localConfig.motion_cooldown_seconds !== config.motion_cooldown_seconds
    );
  }, [localConfig, config]);

  // Check if YOLO config has changes
  const hasYoloChanges = useMemo(() => {
    if (!localYoloConfig || !yoloConfig) return false;
    return (
      localYoloConfig.confidence_threshold !== yoloConfig.confidence_threshold ||
      localYoloConfig.classes_filter !== yoloConfig.classes_filter ||
      localYoloConfig.draw_boxes !== yoloConfig.draw_boxes ||
      localYoloConfig.service_endpoint !== yoloConfig.service_endpoint
    );
  }, [localYoloConfig, yoloConfig]);

  const hasChanges = hasPipelineChanges || hasYoloChanges;

  const showMotionSettings = localConfig.mode === 'motion_triggered' || localConfig.mode === 'hybrid';
  const showScheduleSettings = localConfig.mode === 'scheduled' || localConfig.mode === 'hybrid';

  // Update local config
  const updateLocal = useCallback((updates: Partial<PipelineConfig>) => {
    setLocalConfig(prev => ({ ...prev, ...updates }));
  }, []);

  // Handle save (both pipeline and YOLO if changed)
  const handleSave = useCallback(() => {
    if (hasPipelineChanges) {
      onSave(localConfig);
    }
    if (hasYoloChanges && onSaveYolo && localYoloConfig) {
      onSaveYolo(localYoloConfig);
    }
  }, [localConfig, onSave, hasPipelineChanges, hasYoloChanges, onSaveYolo, localYoloConfig]);

  // Handle reset
  const handleReset = useCallback(() => {
    setLocalConfig(config);
    setLocalYoloConfig(yoloConfig);
  }, [config, yoloConfig]);

  // Update local YOLO config
  const updateLocalYolo = useCallback((updates: Partial<YoloConfig>) => {
    setLocalYoloConfig(prev => prev ? { ...prev, ...updates } : undefined);
  }, []);

  // Handle YOLO class toggle
  const toggleYoloClass = useCallback((className: string) => {
    if (!localYoloConfig) return;
    const currentClasses = parseClasses(localYoloConfig.classes_filter);
    const newClasses = currentClasses.includes(className)
      ? currentClasses.filter((c) => c !== className)
      : [...currentClasses, className];
    updateLocalYolo({ classes_filter: formatClasses(newClasses) });
  }, [localYoloConfig, updateLocalYolo]);

  // Handle YOLO test
  const handleTestYolo = useCallback(async () => {
    if (!onTestYolo) return;
    setIsTestingYolo(true);
    setYoloTestResult(null);
    try {
      const result = await onTestYolo();
      setYoloTestResult(result);
    } catch (error) {
      setYoloTestResult({ success: false, message: error instanceof Error ? error.message : 'Test failed' });
    } finally {
      setIsTestingYolo(false);
    }
  }, [onTestYolo]);

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
        <div className="p-3 bg-bg-card/50 rounded-lg border border-border">
          <div className="flex items-center gap-2 mb-1">
            <span className="text-sm font-medium text-text-secondary">Execution Mode:</span>
            <span className="text-sm text-accent">Sequential</span>
          </div>
          <p className="text-xs text-text-muted">
            Detectors run in order: YOLO first, then Face if person detected, then Plate if vehicle detected.
          </p>
        </div>

        {/* Detectors */}
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

        {/* YOLO Settings - shown when YOLO detector is enabled */}
        {(localConfig.detectors || []).includes('yolo') && localYoloConfig && (
          <div className="space-y-4 p-3 bg-bg-card rounded-lg border border-accent/30">
            <h4 className="text-sm font-medium text-accent">YOLO Settings</h4>

            <Input
              label="Service Endpoint"
              value={localYoloConfig.service_endpoint || ''}
              onChange={(e) => updateLocalYolo({ service_endpoint: e.target.value })}
              placeholder="http://yolo-service:8081"
              disabled={isLoading || isSaving || isSavingYolo}
            />

            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">
                Confidence Threshold: {((localYoloConfig.confidence_threshold || 0.5) * 100).toFixed(0)}%
              </label>
              <input
                type="range"
                min="0"
                max="100"
                value={(localYoloConfig.confidence_threshold || 0.5) * 100}
                onChange={(e) => updateLocalYolo({ confidence_threshold: parseInt(e.target.value) / 100 })}
                className="w-full h-2 bg-border rounded-lg appearance-none cursor-pointer accent-accent"
                disabled={isLoading || isSaving || isSavingYolo}
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-text-secondary mb-2">
                Classes Filter
              </label>
              <Input
                value={localYoloConfig.classes_filter || ''}
                onChange={(e) => updateLocalYolo({ classes_filter: e.target.value })}
                placeholder="person, car, truck (empty = all)"
                disabled={isLoading || isSaving || isSavingYolo}
                hint="Comma-separated list of classes to detect"
              />
              <div className="flex flex-wrap gap-1 mt-2">
                {COMMON_CLASSES.map((cls) => {
                  const parsedClasses = parseClasses(localYoloConfig.classes_filter);
                  return (
                    <button
                      key={cls}
                      onClick={() => toggleYoloClass(cls)}
                      className={`
                        px-2 py-1 text-xs rounded-full border transition-colors
                        ${
                          parsedClasses.includes(cls)
                            ? 'bg-accent text-bg-dark border-accent'
                            : 'bg-bg-card text-text-secondary border-border hover:border-text-muted'
                        }
                      `}
                      disabled={isLoading || isSaving || isSavingYolo}
                    >
                      {cls}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm text-text-secondary block">Draw Bounding Boxes</span>
                <span className="text-xs text-text-muted">Show detection boxes on video</span>
              </div>
              <input
                type="checkbox"
                checked={localYoloConfig.draw_boxes || false}
                onChange={(e) => updateLocalYolo({ draw_boxes: e.target.checked })}
                disabled={isLoading || isSaving || isSavingYolo}
                className="w-4 h-4 text-accent border-border rounded focus:ring-accent"
              />
            </div>

            {/* Test YOLO connection */}
            <div className="pt-2 border-t border-border">
              <Button
                variant="secondary"
                size="sm"
                onClick={handleTestYolo}
                loading={isTestingYolo}
                disabled={isLoading || isSaving || hasYoloChanges}
              >
                <Zap className="w-4 h-4 mr-2" />
                Test Connection
              </Button>
              {hasYoloChanges && (
                <p className="text-xs text-text-muted mt-1">Save changes before testing</p>
              )}

              {yoloTestResult && (
                <div
                  className={`mt-2 flex items-center gap-2 text-sm ${
                    yoloTestResult.success ? 'text-accent-green' : 'text-accent-red'
                  }`}
                >
                  {yoloTestResult.success ? <CheckCircle className="w-4 h-4" /> : <XCircle className="w-4 h-4" />}
                  <span>{yoloTestResult.message}</span>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Info box */}
        <div className="flex items-start gap-2 p-3 bg-bg-card/50 rounded-lg border border-border">
          <Info size={16} className="text-accent flex-shrink-0 mt-0.5" />
          <p className="text-xs text-text-muted">
            {localConfig.mode === 'disabled' && 'Detection is disabled. Cameras will only stream video.'}
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
