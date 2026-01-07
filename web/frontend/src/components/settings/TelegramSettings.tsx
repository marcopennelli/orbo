import { useState, useEffect } from 'react';
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

  // Local state for text inputs to avoid API calls on every keystroke
  const [botToken, setBotToken] = useState(config.telegram_bot_token || '');
  const [chatId, setChatId] = useState(config.telegram_chat_id || '');
  const [cooldown, setCooldown] = useState(String(config.cooldown_seconds || 30));

  // Sync local state when config changes from server
  useEffect(() => {
    setBotToken(config.telegram_bot_token || '');
    setChatId(config.telegram_chat_id || '');
    setCooldown(String(config.cooldown_seconds || 30));
  }, [config.telegram_bot_token, config.telegram_chat_id, config.cooldown_seconds]);

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
            value={botToken}
            onChange={(e) => setBotToken(e.target.value)}
            onBlur={() => {
              if (botToken !== config.telegram_bot_token) {
                onUpdate({ telegram_bot_token: botToken });
              }
            }}
            placeholder="123456:ABC-DEF1234..."
            disabled={isLoading}
          />

          <Input
            label="Chat ID"
            value={chatId}
            onChange={(e) => setChatId(e.target.value)}
            onBlur={() => {
              if (chatId !== config.telegram_chat_id) {
                onUpdate({ telegram_chat_id: chatId });
              }
            }}
            placeholder="-1001234567890"
            disabled={isLoading}
          />

          <Input
            label="Cooldown (seconds)"
            type="number"
            value={cooldown}
            onChange={(e) => setCooldown(e.target.value)}
            onBlur={() => {
              const value = parseInt(cooldown) || 30;
              if (value !== config.cooldown_seconds) {
                onUpdate({ cooldown_seconds: value });
              }
            }}
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
