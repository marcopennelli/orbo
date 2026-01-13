import { useState, useEffect, useCallback, useMemo } from 'react';
import { Zap, CheckCircle, XCircle, Save, RotateCcw, Users } from 'lucide-react';
import type { RecognitionConfig, TestRecognitionResult } from '../../types';
import { Button, Input, Switch } from '../ui';

interface RecognitionSettingsProps {
  config: RecognitionConfig;
  onSave: (config: RecognitionConfig) => void;
  onTest: () => Promise<TestRecognitionResult>;
  isLoading?: boolean;
  isSaving?: boolean;
}

export default function RecognitionSettings({ config, onSave, onTest, isLoading, isSaving }: RecognitionSettingsProps) {
  const [testResult, setTestResult] = useState<TestRecognitionResult | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  // Local state for editing
  const [localConfig, setLocalConfig] = useState<RecognitionConfig>(config);

  // Sync local state when config changes from server
  useEffect(() => {
    setLocalConfig(config);
  }, [config]);

  // Check if there are unsaved changes
  const hasChanges = useMemo(() => {
    return (
      localConfig.enabled !== config.enabled ||
      localConfig.service_endpoint !== config.service_endpoint ||
      localConfig.similarity_threshold !== config.similarity_threshold ||
      localConfig.known_face_color !== config.known_face_color ||
      localConfig.unknown_face_color !== config.unknown_face_color ||
      localConfig.box_thickness !== config.box_thickness
    );
  }, [localConfig, config]);

  // Update local config
  const updateLocal = useCallback((updates: Partial<RecognitionConfig>) => {
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
      setTestResult({ healthy: false, message: error instanceof Error ? error.message : 'Test failed' });
    } finally {
      setIsTesting(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto space-y-4 pb-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-text-primary">Face Recognition</h3>
            <p className="text-xs text-text-muted">InsightFace AI-powered face detection & recognition</p>
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
              placeholder="http://recognition-service:8082"
              disabled={isLoading || isSaving}
            />

            <div>
              <label className="block text-sm font-medium text-text-secondary mb-1">
                Similarity Threshold: {((localConfig.similarity_threshold || 0.5) * 100).toFixed(0)}%
              </label>
              <input
                type="range"
                min="0"
                max="100"
                value={(localConfig.similarity_threshold || 0.5) * 100}
                onChange={(e) => updateLocal({ similarity_threshold: parseInt(e.target.value) / 100 })}
                className="w-full h-2 bg-bg-hover rounded-lg appearance-none cursor-pointer accent-accent"
                disabled={isLoading || isSaving}
              />
              <p className="text-xs text-text-muted mt-1">
                Higher values require closer matches for face recognition
              </p>
            </div>

            {/* Box Appearance Settings */}
            <div className="border border-border rounded-lg p-3 space-y-3 bg-bg-card/50">
              <h4 className="text-sm font-medium text-text-secondary">Box Appearance</h4>

              <div className="flex items-center gap-3">
                <label className="text-sm text-text-muted whitespace-nowrap">Known Face:</label>
                <input
                  type="color"
                  value={localConfig.known_face_color || '#00FF00'}
                  onChange={(e) => updateLocal({ known_face_color: e.target.value })}
                  className="w-10 h-8 rounded border border-border cursor-pointer bg-transparent"
                  disabled={isLoading || isSaving}
                />
                <span className="text-xs text-text-muted font-mono">{localConfig.known_face_color || '#00FF00'}</span>
              </div>

              <div className="flex items-center gap-3">
                <label className="text-sm text-text-muted whitespace-nowrap">Unknown Face:</label>
                <input
                  type="color"
                  value={localConfig.unknown_face_color || '#FF0000'}
                  onChange={(e) => updateLocal({ unknown_face_color: e.target.value })}
                  className="w-10 h-8 rounded border border-border cursor-pointer bg-transparent"
                  disabled={isLoading || isSaving}
                />
                <span className="text-xs text-text-muted font-mono">{localConfig.unknown_face_color || '#FF0000'}</span>
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

            <div className="pt-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={handleTest}
                loading={isTesting}
                disabled={isLoading || isSaving || hasChanges}
              >
                <Zap className="w-4 h-4 mr-2" />
                Test Recognition Service
              </Button>
              {hasChanges && (
                <p className="text-xs text-text-muted mt-1">Save changes before testing</p>
              )}

              {testResult && (
                <div
                  className={`mt-2 flex items-start gap-2 text-sm ${
                    testResult.healthy ? 'text-accent-green' : 'text-accent-red'
                  }`}
                >
                  {testResult.healthy ? <CheckCircle className="w-4 h-4 flex-shrink-0 mt-0.5" /> : <XCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />}
                  <div>
                    <span>{testResult.message}</span>
                    {testResult.healthy && testResult.known_faces_count !== undefined && (
                      <div className="flex items-center gap-1 text-text-muted mt-1">
                        <Users className="w-3 h-3" />
                        <span className="text-xs">{testResult.known_faces_count} registered faces</span>
                      </div>
                    )}
                  </div>
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
