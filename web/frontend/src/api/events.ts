import { get } from './client';
import type { MotionEvent, FrameResponse } from '../types';

export interface EventsQuery {
  camera_id?: string;
  since?: string;
  limit?: number;
}

export async function getEvents(query?: EventsQuery): Promise<MotionEvent[]> {
  const params = new URLSearchParams();
  if (query?.camera_id) params.set('camera_id', query.camera_id);
  if (query?.since) params.set('since', query.since);
  if (query?.limit) params.set('limit', query.limit.toString());

  const queryString = params.toString();
  const endpoint = queryString ? `/motion/events?${queryString}` : '/motion/events';
  return get<MotionEvent[]>(endpoint);
}

export async function getEvent(id: string): Promise<MotionEvent> {
  return get<MotionEvent>(`/motion/events/${id}`);
}

export async function getEventFrame(id: string): Promise<FrameResponse> {
  return get<FrameResponse>(`/motion/events/${id}/frame`);
}

// Helper to convert base64 frame response to data URL
export function frameResponseToDataUrl(frame: FrameResponse): string {
  return `data:${frame.content_type};base64,${frame.data}`;
}
