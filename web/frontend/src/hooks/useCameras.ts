import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as camerasApi from '../api/cameras';
import type { Camera, CameraCreatePayload, CameraUpdatePayload } from '../types';

export function useCameras() {
  return useQuery({
    queryKey: ['cameras'],
    queryFn: camerasApi.getCameras,
    refetchInterval: 5000,
  });
}

export function useCamera(id: string) {
  return useQuery({
    queryKey: ['cameras', id],
    queryFn: () => camerasApi.getCamera(id),
    enabled: !!id,
  });
}

export function useCreateCamera() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CameraCreatePayload) => camerasApi.createCamera(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}

export function useUpdateCamera() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: CameraUpdatePayload }) =>
      camerasApi.updateCamera(id, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
      queryClient.invalidateQueries({ queryKey: ['cameras', id] });
    },
  });
}

export function useDeleteCamera() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => camerasApi.deleteCamera(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}

export function useActivateCamera() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => camerasApi.activateCamera(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}

export function useDeactivateCamera() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => camerasApi.deactivateCamera(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}

export function useEnableAlerts() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => camerasApi.enableAlerts(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}

export function useDisableAlerts() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => camerasApi.disableAlerts(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}

export function useSetEventsEnabled() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      camerasApi.setEventsEnabled(id, enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}

export function useSetNotificationsEnabled() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      camerasApi.setNotificationsEnabled(id, enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['cameras'] });
    },
  });
}
