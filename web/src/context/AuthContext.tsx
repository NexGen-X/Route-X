import React, { createContext, useContext, useState, useEffect } from 'react';
import { api, ApiError } from '../api/client';
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

  const refresh = async () => {
    try {
      const p = await api.auth.me();
      setPrincipal(p);
    } catch (err) {
      setPrincipal(null);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  const login = async (email: string, pass: string, keep = false) => {
    const res = await api.auth.login({ email, password: pass, keep_signed_in: keep });
    setPrincipal(res.principal);
  };

  const logout = async () => {
    try {
      await api.auth.logout();
    } finally {
      setPrincipal(null);
      window.location.hash = '#/login';
    }
  };

  const can = (perm: string): boolean => {
    if (!principal) return false;
    if (principal.roles.includes('Super Admin')) return true;
    return principal.permissions.includes(perm);
  };

  const hasRole = (role: string): boolean => {
    if (!principal) return false;
    return principal.roles.includes(role);
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
