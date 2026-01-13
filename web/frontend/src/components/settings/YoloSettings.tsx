import { useState, useEffect, useCallback, useMemo } from 'react';
import { Zap, CheckCircle, XCircle, Save, RotateCcw } from 'lucide-react';
import type { YoloConfig } from '../../types';
import { Button, Input, Switch } from '../ui';

interface YoloSettingsProps {
  config: YoloConfig;
  onSave: (config: YoloConfig) => void;
  onTest: () => Promise<{ success: boolean; message: string }>;
  isLoading?: boolean;
  isSaving?: boolean;
}

const COMMON_CLASSES = ['person', 'car', 'truck', 'motorcycle', 'bicycle', 'dog', 'cat', 'bird'];

// Parse comma-separated string to array
const parseClasses = (filter: string | undefined): string[] => {
  if (!filter) return [];
  return filter.split(',').map(c => c.trim().toLowerCase()).filter(c => c.length > 0);
};

// Convert array to comma-separated string
const formatClasses = (classes: string[]): string => {
  return classes.join(', ');
};

export default function YoloSettings({ config, onSave, onTest, isLoading, isSaving }: YoloSettingsProps) {
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  // Local state for editing
  const [localConfig, setLocalConfig] = useState<YoloConfig>(config);

  // Sync local state when config changes from server
  useEffect(() => {
    setLocalConfig(config);
  }, [config]);

  // Check if there are unsaved changes
  const hasChanges = useMemo(() => {
    return (
      localConfig.enabled !== config.enabled ||
      localConfig.service_endpoint !== config.service_endpoint ||
      localConfig.confidence_threshold !== config.confidence_threshold ||
      localConfig.security_mode !== config.security_mode ||
      localConfig.draw_boxes !== config.draw_boxes ||
      localConfig.classes_filter !== config.classes_filter ||
      localConfig.box_color !== config.box_color ||
      localConfig.box_thickness !== config.box_thickness
    );
  }, [localConfig, config]);

  // Update local config
  const updateLocal = useCallback((updates: Partial<YoloConfig>) => {
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

  const handleTest = async () => {
    setIsTesting(true);
    setTestResult(null);
    try {
      const result = await onTest();
      setTestResult(result);
    } catch (error) {
      setTestResult({ success: false, message: error instanceof Error ? error.message : 'Test failed' });
    } finally {
      setIsTesting(false);
    }
  };

  const toggleClass = (className: string) => {
    const currentClasses = parseClasses(localConfig.classes_filter);
    const newClasses = currentClasses.includes(className)
      ? currentClasses.filter((c) => c !== className)
      : [...currentClasses, className];
    updateLocal({ classes_filter: formatClasses(newClasses) });
  };

  const parsedClasses = parseClasses(localConfig.classes_filter);

  return (
    <div className="flex flex-col h-full">
      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto space-y-4 pb-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-text-primary">YOLO Detection</h3>
            <p className="text-xs text-text-muted">AI-powered object detection</p>
          </div>
          <Switch
            checked={localConfig.enabled}
            onChange={(enabled) => updateLocal({ enabled })}
            disabled={isLoading || isSaving}
          />
        </div>

        {localConfig.enabled && (
          <>
            <Input
              label="Service Endpoint"
              value={localConfig.service_endpoint || ''}
              onChange={(e) => updateLocal({ service_endpoint: e.target.value })}
              placeholder="http://yolo-service:8081"
              disabled={isLoading || isSaving}
            />

            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">
                Confidence Threshold: {((localConfig.confidence_threshold || 0.5) * 100).toFixed(0)}%
              </label>
              <input
                type="range"
                min="0"
                max="100"
                value={(localConfig.confidence_threshold || 0.5) * 100}
                onChange={(e) => updateLocal({ confidence_threshold: parseInt(e.target.value) / 100 })}
                className="w-full h-2 bg-bg-hover rounded-lg appearance-none cursor-pointer accent-accent"
                disabled={isLoading || isSaving}
              />
            </div>

            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm text-text-secondary block">Security Mode</span>
                <span className="text-xs text-text-muted">Focus on person, vehicle detection</span>
              </div>
              <Switch
                checked={localConfig.security_mode || false}
                onChange={(security_mode) => updateLocal({ security_mode })}
                disabled={isLoading || isSaving}
              />
            </div>

            <div className="flex items-center justify-between">
              <div>
                <span className="text-sm text-text-secondary block">Draw Bounding Boxes</span>
                <span className="text-xs text-text-muted">Show detection boxes in alerts</span>
              </div>
              <Switch
                checked={localConfig.draw_boxes || false}
                onChange={(draw_boxes) => updateLocal({ draw_boxes })}
                disabled={isLoading || isSaving}
              />
            </div>

            {/* Box Appearance Settings */}
            <div className="border border-border rounded-lg p-3 space-y-3 bg-bg-card/50">
              <h4 className="text-sm font-medium text-text-secondary">Box Appearance</h4>

              <div className="flex items-center gap-3">
                <label className="text-sm text-text-muted whitespace-nowrap">Box Color:</label>
                <input
                  type="color"
                  value={localConfig.box_color || '#0066FF'}
                  onChange={(e) => updateLocal({ box_color: e.target.value })}
                  className="w-10 h-8 rounded border border-border cursor-pointer bg-transparent"
                  disabled={isLoading || isSaving}
                />
                <span className="text-xs text-text-muted font-mono">{localConfig.box_color || '#0066FF'}</span>
              </div>

              <div>
                <label className="block text-sm text-text-muted mb-1">
                  Line Thickness: {localConfig.box_thickness || 2}px
                </label>
                <input
                  type="range"
                  min="1"
                  max="5"
                  value={localConfig.box_thickness || 2}
                  onChange={(e) => updateLocal({ box_thickness: parseInt(e.target.value) })}
                  className="w-full h-2 bg-bg-hover rounded-lg appearance-none cursor-pointer accent-accent"
                  disabled={isLoading || isSaving}
                />
              </div>
            </div>

            <div>
              <label className="block text-sm font-medium text-text-secondary mb-2">
                Classes Filter
              </label>
              <Input
                value={localConfig.classes_filter || ''}
                onChange={(e) => updateLocal({ classes_filter: e.target.value })}
                placeholder="person, car, truck"
                disabled={isLoading || isSaving}
                hint="Comma-separated list of classes to detect"
              />
              <div className="flex flex-wrap gap-1 mt-2">
                {COMMON_CLASSES.map((cls) => (
                  <button
                    key={cls}
                    onClick={() => toggleClass(cls)}
                    className={`
                      px-2 py-1 text-xs rounded-full border transition-colors
                      ${
                        parsedClasses.includes(cls)
                          ? 'bg-accent text-bg-dark border-accent'
                          : 'bg-bg-card text-text-secondary border-border hover:border-text-muted'
                      }
                    `}
                    disabled={isLoading || isSaving}
                  >
                    {cls}
                  </button>
                ))}
              </div>
            </div>

            <div className="pt-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={handleTest}
                loading={isTesting}
                disabled={isLoading || isSaving || hasChanges}
              >
                <Zap className="w-4 h-4 mr-2" />
                Test YOLO Connection
              </Button>
              {hasChanges && (
                <p className="text-xs text-text-muted mt-1">Save changes before testing</p>
              )}

              {testResult && (
                <div
                  className={`mt-2 flex items-center gap-2 text-sm ${
                    testResult.success ? 'text-accent-green' : 'text-accent-red'
                  }`}
                >
                  {testResult.success ? <CheckCircle className="w-4 h-4" /> : <XCircle className="w-4 h-4" />}
                  <span>{testResult.message}</span>
                </div>
              )}
            </div>
          </>
        )}
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
