import { useQuery } from '@tanstack/react-query';
import * as eventsApi from '../api/events';

export function useEvents(cameraId?: string) {
  return useQuery({
    queryKey: ['events', cameraId],
    queryFn: () => eventsApi.getEvents({ camera_id: cameraId, limit: 50 }),
    refetchInterval: 10000,
  });
}

export function useEvent(id: string) {
  return useQuery({
    queryKey: ['events', 'detail', id],
    queryFn: () => eventsApi.getEvent(id),
    enabled: !!id,
  });
}
