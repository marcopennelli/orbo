import { useState, useEffect, useCallback, useMemo } from 'react';
import { Send, CheckCircle, XCircle, Save, RotateCcw } from 'lucide-react';
import type { TelegramConfig } from '../../types';
import { Button, Input, Switch } from '../ui';

interface TelegramSettingsProps {
  config: TelegramConfig;
  onSave: (config: TelegramConfig) => void;
  onTest: () => Promise<{ success: boolean; message: string }>;
  isLoading?: boolean;
  isSaving?: boolean;
}

export default function TelegramSettings({ config, onSave, onTest, isLoading, isSaving }: TelegramSettingsProps) {
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [isTesting, setIsTesting] = useState(false);

  // Local state for editing
  const [localConfig, setLocalConfig] = useState<TelegramConfig>(config);

  // Sync local state when config changes from server
  useEffect(() => {
    setLocalConfig(config);
  }, [config]);

  // Check if there are unsaved changes
  const hasChanges = useMemo(() => {
    return (
      localConfig.telegram_enabled !== config.telegram_enabled ||
      localConfig.telegram_bot_token !== config.telegram_bot_token ||
      localConfig.telegram_chat_id !== config.telegram_chat_id ||
      localConfig.cooldown_seconds !== config.cooldown_seconds
    );
  }, [localConfig, config]);

  // Update local config
  const updateLocal = useCallback((updates: Partial<TelegramConfig>) => {
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

  return (
    <div className="flex flex-col h-full">
      {/* Scrollable content */}
      <div className="flex-1 overflow-y-auto space-y-4 pb-4">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-text-primary">Telegram Notifications</h3>
            <p className="text-xs text-text-muted">Receive alerts via Telegram bot</p>
          </div>
          <Switch
            checked={localConfig.telegram_enabled}
            onChange={(telegram_enabled) => updateLocal({ telegram_enabled })}
            disabled={isLoading || isSaving}
          />
        </div>

        {localConfig.telegram_enabled && (
          <>
            <Input
              label="Bot Token"
              type="password"
              value={localConfig.telegram_bot_token || ''}
              onChange={(e) => updateLocal({ telegram_bot_token: e.target.value })}
              placeholder="123456:ABC-DEF1234..."
              disabled={isLoading || isSaving}
            />

            <Input
              label="Chat ID"
              value={localConfig.telegram_chat_id || ''}
              onChange={(e) => updateLocal({ telegram_chat_id: e.target.value })}
              placeholder="-1001234567890"
              disabled={isLoading || isSaving}
            />

            <Input
              label="Cooldown (seconds)"
              type="number"
              value={localConfig.cooldown_seconds || 30}
              onChange={(e) => updateLocal({ cooldown_seconds: parseInt(e.target.value) || 30 })}
              min={1}
              max={300}
              disabled={isLoading || isSaving}
              hint="Minimum time between notifications"
            />

            <div className="pt-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={handleTest}
                loading={isTesting}
                disabled={isLoading || isSaving || hasChanges}
              >
                <Send className="w-4 h-4 mr-2" />
                Send Test Message
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
