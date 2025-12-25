import { get, post } from './client';
import type { SystemStatus } from '../types';

export async function getSystemStatus(): Promise<SystemStatus> {
  return get<SystemStatus>('/system/status');
}

export async function startDetection(): Promise<void> {
  return post<void>('/system/detection/start');
}

export async function stopDetection(): Promise<void> {
  return post<void>('/system/detection/stop');
}
