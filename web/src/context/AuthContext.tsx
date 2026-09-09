import React, { createContext, useContext, useState, useEffect, useCallback, useRef } from 'react';
import { api, setCsrfToken, ApiError } from '../api/client';
import type { Principal, User } from '../types';

interface AuthContextType {
  principal: Principal | null;
  user: User | null;
  isLoading: boolean;
  login: (email: string, pass: string, keep?: boolean) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  can: (perm: string) => boolean;
  hasRole: (role: string) => boolean;
}

const AuthContext = createContext<AuthContextType | null>(null);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [principal, setPrincipal] = useState<Principal | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  // Penanda retry refresh non-401 agar tidak infinite loop (maksimal 1x).
  const refreshRetriedRef = useRef(false);
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Penanda unmount: refresh async yang selesai setelah unmount tidak boleh setState.
  const mountedRef = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const res: any = await api.auth.me();
      if (!mountedRef.current) return;
      if (res) {
        const p: Principal = res.principal || res;
        if (!p.session_id && p.session?.id) {
          p.session_id = p.session.id;
        }
        if ((res as any).csrf_token) {
          setCsrfToken((res as any).csrf_token);
        }
        setPrincipal(p);
        refreshRetriedRef.current = false;
      } else {
        setPrincipal(null);
      }
    } catch (err) {
      if (!mountedRef.current) return;
      // Hanya 401 yang berarti sesi benar-benar mati -> logout.
      const status = err instanceof ApiError ? err.status : (err as any)?.status;
      if (status === 401) {
        setPrincipal(null);
      } else if (!refreshRetriedRef.current) {
        // Error non-401 (misal jaringan): pertahankan state, coba sekali lagi setelah 2 detik.
        refreshRetriedRef.current = true;
        if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
        refreshTimerRef.current = setTimeout(() => {
          refreshTimerRef.current = null;
          refresh().catch(() => {
            // Biarkan state lama bila retry pun gagal; jangan logout paksa.
          });
        }, 2000);
      }
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    refresh();
    // Dengarkan 401 global dari api client agar sesi kedaluwarsa langsung logout.
    const handleUnauthorized = () => {
      setPrincipal(null);
      setCsrfToken('');
    };
    window.addEventListener('routex:unauthorized', handleUnauthorized);
    return () => {
      mountedRef.current = false;
      window.removeEventListener('routex:unauthorized', handleUnauthorized);
      if (refreshTimerRef.current) {
        clearTimeout(refreshTimerRef.current);
        refreshTimerRef.current = null;
      }
    };
  }, [refresh]);

  const login = async (email: string, pass: string, keep = false) => {
    const res: any = await api.auth.login({ email, password: pass, keep_signed_in: keep });
    const p: Principal = res.principal || res;
    if (!p.session_id && p.session?.id) {
      p.session_id = p.session.id;
    }
    if ((res as any).csrf_token) {
      setCsrfToken((res as any).csrf_token);
    }
    setPrincipal(p);
  };

  const logout = async () => {
    try {
      await api.auth.logout();
    } finally {
      setPrincipal(null);
      setCsrfToken('');
      window.location.hash = '#/login';
    }
  };

  const can = (perm: string): boolean => {
    if (!principal) return false;
    const roles = principal.roles ?? [];
    const perms = principal.permissions ?? [];
    if (roles.includes('Super Admin')) return true;
    return perms.includes(perm);
  };

  const hasRole = (role: string): boolean => {
    if (!principal) return false;
    return (principal.roles ?? []).includes(role);
  };

  return (
    <AuthContext.Provider
      value={{
        principal,
        user: principal?.user || null,
        isLoading,
        login,
        logout,
        refresh,
        can,
        hasRole,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
};
