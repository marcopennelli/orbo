import { get, put, post } from './client';
import type { TelegramConfig, YoloConfig, DetectionConfig, Dinov3Config, PipelineConfig, RecognitionConfig, TestRecognitionResult } from '../types';

// Telegram configuration
export async function getTelegramConfig(): Promise<TelegramConfig> {
  return get<TelegramConfig>('/config/notifications');
}

export async function updateTelegramConfig(config: Partial<TelegramConfig>): Promise<TelegramConfig> {
  return put<TelegramConfig>('/config/notifications', config);
}

export async function testTelegramNotification(): Promise<{ success: boolean; message: string }> {
  return post<{ success: boolean; message: string }>('/config/notifications/test');
}

// YOLO configuration
export async function getYoloConfig(): Promise<YoloConfig> {
  return get<YoloConfig>('/config/yolo');
}

export async function updateYoloConfig(config: Partial<YoloConfig>): Promise<YoloConfig> {
  return put<YoloConfig>('/config/yolo', config);
}

export async function testYoloDetection(): Promise<{ success: boolean; message: string }> {
  return post<{ success: boolean; message: string }>('/config/yolo/test');
}

// DINOv3 configuration
export async function getDinov3Config(): Promise<Dinov3Config> {
  return get<Dinov3Config>('/config/dinov3');
}

export async function updateDinov3Config(config: Partial<Dinov3Config>): Promise<Dinov3Config> {
  return put<Dinov3Config>('/config/dinov3', config);
}

// Detection configuration
export async function getDetectionConfig(): Promise<DetectionConfig> {
  return get<DetectionConfig>('/config/detection');
}

export async function updateDetectionConfig(config: Partial<DetectionConfig>): Promise<DetectionConfig> {
  return put<DetectionConfig>('/config/detection', config);
}

// Pipeline configuration
export async function getPipelineConfig(): Promise<PipelineConfig> {
  return get<PipelineConfig>('/config/pipeline');
}

export async function updatePipelineConfig(config: Partial<PipelineConfig>): Promise<PipelineConfig> {
  return put<PipelineConfig>('/config/pipeline', config);
}

// Face Recognition configuration
export async function getRecognitionConfig(): Promise<RecognitionConfig> {
  return get<RecognitionConfig>('/config/recognition');
}

export async function updateRecognitionConfig(config: Partial<RecognitionConfig>): Promise<RecognitionConfig> {
  return put<RecognitionConfig>('/config/recognition', config);
}

export async function testRecognitionService(): Promise<TestRecognitionResult> {
  return post<TestRecognitionResult>('/config/recognition/test');
}
