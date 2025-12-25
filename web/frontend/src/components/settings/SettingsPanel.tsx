import { useState } from 'react';
import { MessageSquare, Cpu, Activity, X } from 'lucide-react';
import type { TelegramConfig, YoloConfig, DetectionConfig } from '../../types';
import { Button } from '../ui';
import TelegramSettings from './TelegramSettings';
import YoloSettings from './YoloSettings';
import DetectionSettings from './DetectionSettings';

type SettingsTab = 'telegram' | 'yolo' | 'detection';

interface SettingsPanelProps {
  isOpen: boolean;
  onClose: () => void;
  telegramConfig: TelegramConfig;
  yoloConfig: YoloConfig;
  detectionConfig: DetectionConfig;
  onUpdateTelegram: (config: Partial<TelegramConfig>) => void;
  onUpdateYolo: (config: Partial<YoloConfig>) => void;
  onUpdateDetection: (config: Partial<DetectionConfig>) => void;
  onTestTelegram: () => Promise<{ success: boolean; message: string }>;
  onTestYolo: () => Promise<{ success: boolean; message: string }>;
  isLoading?: boolean;
}

const tabs: { id: SettingsTab; label: string; icon: typeof MessageSquare }[] = [
  { id: 'telegram', label: 'Telegram', icon: MessageSquare },
  { id: 'yolo', label: 'YOLO', icon: Cpu },
  { id: 'detection', label: 'Detection', icon: Activity },
];

export default function SettingsPanel({
  isOpen,
  onClose,
  telegramConfig,
  yoloConfig,
  detectionConfig,
  onUpdateTelegram,
  onUpdateYolo,
  onUpdateDetection,
  onTestTelegram,
  onTestYolo,
  isLoading,
}: SettingsPanelProps) {
  const [activeTab, setActiveTab] = useState<SettingsTab>('telegram');

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div className="fixed inset-0 bg-black/60" onClick={onClose} />

      {/* Panel */}
      <div className="fixed right-0 top-0 bottom-0 w-full max-w-md bg-bg-panel border-l border-border shadow-xl flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-border">
          <h2 className="text-lg font-semibold text-text-primary">Settings</h2>
          <Button variant="ghost" size="sm" onClick={onClose}>
            <X className="w-5 h-5" />
          </Button>
        </div>

        {/* Tabs */}
        <div className="flex border-b border-border">
          {tabs.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              onClick={() => setActiveTab(id)}
              className={`
                flex-1 flex items-center justify-center gap-2 px-4 py-3
                text-sm font-medium transition-colors
                ${
                  activeTab === id
                    ? 'text-accent border-b-2 border-accent -mb-px'
                    : 'text-text-secondary hover:text-text-primary'
                }
              `}
            >
              <Icon className="w-4 h-4" />
              {label}
            </button>
          ))}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">
          {activeTab === 'telegram' && (
            <TelegramSettings
              config={telegramConfig}
              onUpdate={onUpdateTelegram}
              onTest={onTestTelegram}
              isLoading={isLoading}
            />
          )}
          {activeTab === 'yolo' && (
            <YoloSettings
              config={yoloConfig}
              onUpdate={onUpdateYolo}
              onTest={onTestYolo}
              isLoading={isLoading}
            />
          )}
          {activeTab === 'detection' && (
            <DetectionSettings
              config={detectionConfig}
              onUpdate={onUpdateDetection}
              isLoading={isLoading}
            />
          )}
        </div>
      </div>
    </div>
  );
}
