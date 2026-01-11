import { useState } from 'react';
import { MessageSquare, Sliders, X } from 'lucide-react';
import type { TelegramConfig, YoloConfig, PipelineConfig } from '../../types';
import { Button } from '../ui';
import TelegramSettings from './TelegramSettings';
import PipelineSettings from './PipelineSettings';

type SettingsTab = 'pipeline' | 'telegram';

interface SettingsPanelProps {
  isOpen: boolean;
  onClose: () => void;
  telegramConfig: TelegramConfig;
  yoloConfig: YoloConfig;
  pipelineConfig: PipelineConfig;
  onSaveTelegram: (config: TelegramConfig) => void;
  onSaveYolo: (config: YoloConfig) => void;
  onSavePipeline: (config: PipelineConfig) => void;
  onTestTelegram: () => Promise<{ success: boolean; message: string }>;
  onTestYolo: () => Promise<{ success: boolean; message: string }>;
  isSavingTelegram?: boolean;
  isSavingYolo?: boolean;
  isSavingPipeline?: boolean;
}

const tabs: { id: SettingsTab; label: string; icon: typeof MessageSquare }[] = [
  { id: 'pipeline', label: 'Pipeline', icon: Sliders },
  { id: 'telegram', label: 'Telegram', icon: MessageSquare },
];

export default function SettingsPanel({
  isOpen,
  onClose,
  telegramConfig,
  yoloConfig,
  pipelineConfig,
  onSaveTelegram,
  onSaveYolo,
  onSavePipeline,
  onTestTelegram,
  onTestYolo,
  isSavingTelegram,
  isSavingYolo,
  isSavingPipeline,
}: SettingsPanelProps) {
  const [activeTab, setActiveTab] = useState<SettingsTab>('pipeline');

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div className="fixed inset-0 bg-black/60" onClick={onClose} />

      {/* Panel */}
      <div className="fixed right-0 top-0 bottom-0 w-full max-w-lg bg-bg-panel border-l border-border shadow-xl flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border">
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
        <div className="flex-1 overflow-hidden p-6 flex flex-col">
          {activeTab === 'pipeline' && (
            <PipelineSettings
              config={pipelineConfig}
              onSave={onSavePipeline}
              isSaving={isSavingPipeline}
              yoloConfig={yoloConfig}
              onSaveYolo={onSaveYolo}
              onTestYolo={onTestYolo}
              isSavingYolo={isSavingYolo}
            />
          )}
          {activeTab === 'telegram' && (
            <TelegramSettings
              config={telegramConfig}
              onSave={onSaveTelegram}
              onTest={onTestTelegram}
              isSaving={isSavingTelegram}
            />
          )}
        </div>
      </div>
    </div>
  );
}
