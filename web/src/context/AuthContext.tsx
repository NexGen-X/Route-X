import React, { createContext, useContext, useState, useEffect, useCallback, useRef, useMemo } from 'react';
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

interface AuthResponse extends Partial<Principal> {
  principal?: Principal;
  csrf_token?: string;
}

function extractHttpStatus(err: unknown): number | undefined {
  if (err instanceof ApiError) return err.status;
  if (typeof err === 'object' && err !== null && 'status' in err) {
    const s = (err as Record<string, unknown>).status;
    if (typeof s === 'number') return s;
  }
  return undefined;
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
  // Penanda generasi sesi (audit3 V-FF-25): refresh() yang dimulai SEBELUM
  // login/logout tidak boleh menimpa hasil login/logout ketika ia selesai
  // belakangan — versi lama hanya memeriksa mounted, sehingga refresh basi
  // bisa menghidupkan lagi sesi yang baru saja logout (atau menghapus sesi
  // yang baru saja login). login()/logout() menaikkan generasi; refresh
  // membuang hasilnya bila generasi sudah bergerak.
  const generasiRef = useRef(0);
  // Verifikasi sesi akibat event 401 global sedang berjalan (V-FF-26).
  const verifikasiRef = useRef(false);

  const refresh = useCallback(async () => {
    const generasiAwal = generasiRef.current;
    try {
      const res = (await api.auth.me()) as AuthResponse;
      if (!mountedRef.current || generasiRef.current !== generasiAwal) return;
      if (res) {
        const p: Principal = (res.principal || res) as Principal;
        if (!p.session_id && p.session?.id) {
          p.session_id = p.session.id;
        }
        if (res.csrf_token) {
          setCsrfToken(res.csrf_token);
        }
        setPrincipal(p);
        refreshRetriedRef.current = false;
      } else {
        setPrincipal(null);
      }
    } catch (err: unknown) {
      if (!mountedRef.current || generasiRef.current !== generasiAwal) return;
      // Hanya 401 yang berarti sesi benar-benar mati -> logout.
      const status = extractHttpStatus(err);
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
    // Dengarkan 401 global dari api client. Audit3 V-FF-26: 401 dari request
    // LATAR (polling, widget) TIDAK lagi berarti logout seketika — sesi
    // diverifikasi dulu lewat /api/auth/me; hanya bila /me sendiri menjawab
    // 401 barulah refresh() di atas membersihkan principal. Dulu satu 401
    // dari polling yang basi langsung membuang sesi yang sebenarnya sehat.
    const handleUnauthorized = () => {
      if (verifikasiRef.current) return;
      verifikasiRef.current = true;
      void refresh().finally(() => {
        verifikasiRef.current = false;
      });
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

  const login = useCallback(async (email: string, pass: string, keep = false) => {
    // Generasi baru: refresh lama yang masih terbang tidak boleh menimpa
    // principal hasil login ini (V-FF-25).
    generasiRef.current += 1;
    const res = (await api.auth.login({ email, password: pass, keep_signed_in: keep })) as AuthResponse;
    const p: Principal = (res.principal || res) as Principal;
    if (!p.session_id && p.session?.id) {
      p.session_id = p.session.id;
    }
    if (res.csrf_token) {
      setCsrfToken(res.csrf_token);
    }
    setPrincipal(p);
  }, []);

  const logout = useCallback(async () => {
    // Generasi baru SEJAK AWAL logout: refresh yang selesai di tengah proses
    // logout tidak boleh menghidupkan kembali principal (V-FF-25).
    generasiRef.current += 1;
    try {
      await api.auth.logout();
    } finally {
      setPrincipal(null);
      setCsrfToken('');
      window.location.hash = '#/login';
    }
  }, []);

  const can = useCallback((_perm: string): boolean => {
    // Single-admin architecture: all authenticated principals have full access.
    return !!principal;
  }, [principal]);

  const hasRole = useCallback((_role: string): boolean => {
    // Single-admin architecture: all authenticated principals have full admin rights.
    return !!principal;
  }, [principal]);

  const user = useMemo(() => principal?.user || null, [principal]);

  const value = useMemo<AuthContextType>(
    () => ({
      principal,
      user,
      isLoading,
      login,
      logout,
      refresh,
      can,
      hasRole,
    }),
    [principal, user, isLoading, login, logout, refresh, can, hasRole]
  );

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
};
