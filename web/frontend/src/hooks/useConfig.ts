import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as configApi from '../api/config';
import type { TelegramConfig, YoloConfig, DetectionConfig } from '../types';

// Telegram config
export function useTelegramConfig() {
  return useQuery({
    queryKey: ['config', 'telegram'],
    queryFn: configApi.getTelegramConfig,
  });
}

export function useUpdateTelegramConfig() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (config: Partial<TelegramConfig>) => configApi.updateTelegramConfig(config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['config', 'telegram'] });
      queryClient.invalidateQueries({ queryKey: ['system', 'status'] });
    },
  });
}

export function useTestTelegram() {
  return useMutation({
    mutationFn: configApi.testTelegramNotification,
  });
}

// YOLO config
export function useYoloConfig() {
  return useQuery({
    queryKey: ['config', 'yolo'],
    queryFn: configApi.getYoloConfig,
  });
}

export function useUpdateYoloConfig() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (config: Partial<YoloConfig>) => configApi.updateYoloConfig(config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['config', 'yolo'] });
      queryClient.invalidateQueries({ queryKey: ['system', 'status'] });
    },
  });
}

export function useTestYolo() {
  return useMutation({
    mutationFn: configApi.testYoloDetection,
  });
}

// Detection config
export function useDetectionConfig() {
  return useQuery({
    queryKey: ['config', 'detection'],
    queryFn: configApi.getDetectionConfig,
  });
}

export function useUpdateDetectionConfig() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (config: Partial<DetectionConfig>) => configApi.updateDetectionConfig(config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['config', 'detection'] });
    },
  });
}
