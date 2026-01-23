import { useQuery, useInfiniteQuery } from '@tanstack/react-query';
import * as eventsApi from '../api/events';

const PAGE_SIZE = 50;

export function useEvents(cameraId?: string) {
  return useQuery({
    queryKey: ['events', cameraId],
    queryFn: () => eventsApi.getEvents({ camera_id: cameraId, limit: PAGE_SIZE }),
    refetchInterval: 10000,
  });
}

export function useInfiniteEvents(cameraId?: string) {
  return useInfiniteQuery({
    queryKey: ['events', 'infinite', cameraId],
    queryFn: async ({ pageParam }) => {
      const events = await eventsApi.getEvents({
        camera_id: cameraId,
        limit: PAGE_SIZE,
        before: pageParam, // Get events older than this timestamp
      });
      return events;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => {
      // If we got fewer events than PAGE_SIZE, there are no more pages
      if (lastPage.length < PAGE_SIZE) {
        return undefined;
      }
      // Use the oldest event's timestamp as the cursor for the next page
      const oldestEvent = lastPage[lastPage.length - 1];
      return oldestEvent?.timestamp;
    },
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
