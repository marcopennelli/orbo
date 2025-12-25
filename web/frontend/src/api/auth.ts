import { post, get, setToken, clearToken } from './client';

export interface LoginPayload {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  expires_at: number;
}

export interface AuthStatus {
  enabled: boolean;
  authenticated: boolean;
  username?: string;
}

export async function login(credentials: LoginPayload): Promise<LoginResponse> {
  const response = await post<LoginResponse>('/auth/login', credentials);
  setToken(response.token);
  return response;
}

export function logout(): void {
  clearToken();
}

export async function getAuthStatus(): Promise<AuthStatus> {
  return get<AuthStatus>('/auth/status');
}
