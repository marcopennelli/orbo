import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as systemApi from '../api/system';

export function useSystemStatus() {
  return useQuery({
    queryKey: ['system', 'status'],
    queryFn: systemApi.getSystemStatus,
    refetchInterval: 3000,
  });
}

export function useStartDetection() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: systemApi.startDetection,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['system', 'status'] });
    },
  });
}

export function useStopDetection() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: systemApi.stopDetection,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['system', 'status'] });
    },
  });
}
