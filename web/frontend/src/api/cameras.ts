import { get, post, put, del } from './client';
import type { Camera, CameraCreatePayload, CameraUpdatePayload, FrameResponse } from '../types';

export async function getCameras(): Promise<Camera[]> {
  return get<Camera[]>('/cameras');
}

export async function getCamera(id: string): Promise<Camera> {
  return get<Camera>(`/cameras/${id}`);
}

export async function createCamera(data: CameraCreatePayload): Promise<Camera> {
  return post<Camera>('/cameras', data);
}

export async function updateCamera(id: string, data: CameraUpdatePayload): Promise<Camera> {
  return put<Camera>(`/cameras/${id}`, data);
}

export async function deleteCamera(id: string): Promise<void> {
  return del<void>(`/cameras/${id}`);
}

export async function activateCamera(id: string): Promise<Camera> {
  return post<Camera>(`/cameras/${id}/activate`);
}

export async function deactivateCamera(id: string): Promise<Camera> {
  return post<Camera>(`/cameras/${id}/deactivate`);
}

export async function enableDetection(id: string): Promise<Camera> {
  return post<Camera>(`/cameras/${id}/detection/enable`);
}

export async function disableDetection(id: string): Promise<Camera> {
  return post<Camera>(`/cameras/${id}/detection/disable`);
}

export async function getCameraFrame(id: string): Promise<FrameResponse> {
  return get<FrameResponse>(`/cameras/${id}/frame`);
}

// Helper to convert base64 frame response to data URL
export function frameResponseToDataUrl(frame: FrameResponse): string {
  return `data:${frame.content_type};base64,${frame.data}`;
}
