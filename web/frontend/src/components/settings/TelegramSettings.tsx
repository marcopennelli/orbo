import { useState } from 'react';
import { Send, CheckCircle, XCircle } from 'lucide-react';
import type { TelegramConfig } from '../../types';
import { Button, Input, Switch } from '../ui';

interface TelegramSettingsProps {
  config: TelegramConfig;
  onUpdate: (config: Partial<TelegramConfig>) => void;
  onTest: () => Promise<{ success: boolean; message: string }>;
  isLoading?: boolean;
}

export default function TelegramSettings({ config, onUpdate, onTest, isLoading }: TelegramSettingsProps) {
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [isTesting, setIsTesting] = useState(false);

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
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-text-primary">Telegram Notifications</h3>
          <p className="text-xs text-text-muted">Receive alerts via Telegram bot</p>
        </div>
        <Switch
          checked={config.telegram_enabled}
          onChange={(telegram_enabled) => onUpdate({ telegram_enabled })}
          disabled={isLoading}
        />
      </div>

      {config.telegram_enabled && (
        <>
          <Input
            label="Bot Token"
            type="password"
            value={config.telegram_bot_token || ''}
            onChange={(e) => onUpdate({ telegram_bot_token: e.target.value })}
            placeholder="123456:ABC-DEF1234..."
            disabled={isLoading}
          />

          <Input
            label="Chat ID"
            value={config.telegram_chat_id || ''}
            onChange={(e) => onUpdate({ telegram_chat_id: e.target.value })}
            placeholder="-1001234567890"
            disabled={isLoading}
          />

          <Input
            label="Cooldown (seconds)"
            type="number"
            value={config.cooldown_seconds || 30}
            onChange={(e) => onUpdate({ cooldown_seconds: parseInt(e.target.value) || 30 })}
            min={1}
            max={300}
            disabled={isLoading}
          />

          <div className="pt-2">
            <Button variant="secondary" onClick={handleTest} loading={isTesting} disabled={isLoading}>
              <Send className="w-4 h-4 mr-2" />
              Send Test Message
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
