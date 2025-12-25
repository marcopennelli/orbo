import { useState, useEffect, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as authApi from '../api/auth';
import { isAuthenticated, clearToken } from '../api/client';

export function useAuthStatus() {
  return useQuery({
    queryKey: ['auth', 'status'],
    queryFn: authApi.getAuthStatus,
    staleTime: 60000,
    retry: false,
  });
}

export function useLogin() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: authApi.login,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['auth'] });
    },
  });
}

export function useLogout() {
  const queryClient = useQueryClient();

  return useCallback(() => {
    authApi.logout();
    queryClient.invalidateQueries();
  }, [queryClient]);
}

export function useAuth() {
  const [authenticated, setAuthenticated] = useState(isAuthenticated());
  const { data: authStatus, isLoading } = useAuthStatus();
  const logout = useLogout();

  // Listen for auth:unauthorized events
  useEffect(() => {
    const handleUnauthorized = () => {
      setAuthenticated(false);
    };

    window.addEventListener('auth:unauthorized', handleUnauthorized);
    return () => {
      window.removeEventListener('auth:unauthorized', handleUnauthorized);
    };
  }, []);

  // Update authenticated state when token changes
  useEffect(() => {
    setAuthenticated(isAuthenticated());
  }, [authStatus]);

  const handleLogout = useCallback(() => {
    logout();
    setAuthenticated(false);
  }, [logout]);

  return {
    isAuthenticated: authenticated,
    authEnabled: authStatus?.enabled ?? false,
    username: authStatus?.username,
    isLoading,
    logout: handleLogout,
  };
}
