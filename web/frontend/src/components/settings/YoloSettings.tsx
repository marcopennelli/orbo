import { useState, useEffect } from 'react';
import { Zap, CheckCircle, XCircle } from 'lucide-react';
import type { YoloConfig } from '../../types';
import { Button, Input, Switch } from '../ui';

interface YoloSettingsProps {
  config: YoloConfig;
  onUpdate: (config: Partial<YoloConfig>) => void;
  onTest: () => Promise<{ success: boolean; message: string }>;
  isLoading?: boolean;
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

export default function YoloSettings({ config, onUpdate, onTest, isLoading }: YoloSettingsProps) {
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  // Local state for text inputs to avoid API calls on every keystroke
  const [serviceEndpoint, setServiceEndpoint] = useState(config.service_endpoint || '');
  const [classInput, setClassInput] = useState(config.classes_filter || '');

  // Sync local state when config changes from server
  useEffect(() => {
    setServiceEndpoint(config.service_endpoint || '');
    setClassInput(config.classes_filter || '');
  }, [config.service_endpoint, config.classes_filter]);

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

  const handleClassesChange = (value: string) => {
    setClassInput(value);
  };

  const handleClassesBlur = () => {
    if (classInput !== config.classes_filter) {
      onUpdate({ classes_filter: classInput });
    }
  };

  const toggleClass = (className: string) => {
    const currentClasses = parseClasses(config.classes_filter);
    const newClasses = currentClasses.includes(className)
      ? currentClasses.filter((c) => c !== className)
      : [...currentClasses, className];
    const newValue = formatClasses(newClasses);
    setClassInput(newValue);
    onUpdate({ classes_filter: newValue });
  };

  const parsedClasses = parseClasses(config.classes_filter);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-text-primary">YOLO Detection</h3>
          <p className="text-xs text-text-muted">AI-powered object detection</p>
        </div>
        <Switch
          checked={config.enabled}
          onChange={(enabled) => onUpdate({ enabled })}
          disabled={isLoading}
        />
      </div>

      {config.enabled && (
        <>
          <Input
            label="Service Endpoint"
            value={serviceEndpoint}
            onChange={(e) => setServiceEndpoint(e.target.value)}
            onBlur={() => {
              if (serviceEndpoint !== config.service_endpoint) {
                onUpdate({ service_endpoint: serviceEndpoint });
              }
            }}
            placeholder="http://yolo-service:8081"
            disabled={isLoading}
          />

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-1">
              Confidence Threshold: {((config.confidence_threshold || 0.5) * 100).toFixed(0)}%
            </label>
            <input
              type="range"
              min="0"
              max="100"
              value={(config.confidence_threshold || 0.5) * 100}
              onChange={(e) => onUpdate({ confidence_threshold: parseInt(e.target.value) / 100 })}
              className="w-full h-2 bg-bg-hover rounded-lg appearance-none cursor-pointer accent-accent"
              disabled={isLoading}
            />
          </div>

          <div className="flex items-center justify-between">
            <span className="text-sm text-text-secondary">Security Mode</span>
            <Switch
              checked={config.security_mode || false}
              onChange={(security_mode) => onUpdate({ security_mode })}
              disabled={isLoading}
            />
          </div>

          <div className="flex items-center justify-between">
            <span className="text-sm text-text-secondary">Draw Bounding Boxes</span>
            <Switch
              checked={config.draw_boxes || false}
              onChange={(draw_boxes) => onUpdate({ draw_boxes })}
              disabled={isLoading}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">
              Classes Filter (comma-separated)
            </label>
            <Input
              value={classInput}
              onChange={(e) => handleClassesChange(e.target.value)}
              onBlur={handleClassesBlur}
              placeholder="person, car, truck"
              disabled={isLoading}
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
                  disabled={isLoading}
                >
                  {cls}
                </button>
              ))}
            </div>
          </div>

          <div className="pt-2">
            <Button variant="secondary" onClick={handleTest} loading={isTesting} disabled={isLoading}>
              <Zap className="w-4 h-4 mr-2" />
              Test YOLO Connection
            </Button>

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
  );
}
