import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { AuthProvider, useAuth } from './AuthContext';
import type { Principal } from '../types';

const mockMe = vi.fn();
const mockLogin = vi.fn();
const mockLogout = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    auth: {
      me: () => mockMe(),
      login: (creds: { email: string; password: string; keep_signed_in?: boolean }) => mockLogin(creds),
      logout: () => mockLogout(),
    },
  },
  setCsrfToken: vi.fn(),
  ApiError: class ApiError extends Error {
    status?: number;
    constructor(msg: string, status?: number) {
      super(msg);
      this.status = status;
    }
  },
}));

const MOCK_PRINCIPAL: Principal = {
  user: {
    id: 'u-1',
    email: 'admin@routex.internal',
    display_name: 'Administrator',
    status: 'active',
    must_change_password: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    roles: ['admin'],
  },
  roles: ['admin'],
  permissions: ['*'],
  session_id: 'sess-123',
  session: {
    id: 'sess-123',
    ip: '127.0.0.1',
    user_agent: 'vitest',
    expires_at: '2026-12-31T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z',
  },
};

const TestConsumer: React.FC = () => {
  const { principal, isLoading, login, logout, can, hasRole } = useAuth();
  return (
    <div>
      <div data-testid="auth-loading">{isLoading ? 'loading' : 'ready'}</div>
      <div data-testid="auth-user">{principal ? principal.user.email : 'anonymous'}</div>
      <div data-testid="auth-can">{can('admin') ? 'allowed' : 'denied'}</div>
      <div data-testid="auth-role">{hasRole('admin') ? 'has-admin' : 'no-admin'}</div>
      <button onClick={() => void login('test@routex.internal', 'secret')}>Login</button>
      <button onClick={() => void logout()}>Logout</button>
    </div>
  );
};

describe('AuthContext — Provider & Value Memoization Spec', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('menginisialisasi sesi dari api.auth.me saat mount', async () => {
    mockMe.mockResolvedValueOnce({
      principal: MOCK_PRINCIPAL,
      csrf_token: 'csrf-token-123',
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('auth-loading')).toHaveTextContent('ready');
      expect(screen.getByTestId('auth-user')).toHaveTextContent('admin@routex.internal');
      expect(screen.getByTestId('auth-can')).toHaveTextContent('allowed');
      expect(screen.getByTestId('auth-role')).toHaveTextContent('has-admin');
    });
  });

  it('memastikan provider value tetap ter-memoize saat rerender tanpa perubahan state', async () => {
    mockMe.mockResolvedValueOnce({
      principal: MOCK_PRINCIPAL,
      csrf_token: 'csrf-token-123',
    });

    let contextRef1: ReturnType<typeof useAuth> | null = null;
    let contextRef2: ReturnType<typeof useAuth> | null = null;

    const MemoProbe: React.FC<{ pass: number }> = ({ pass }) => {
      const ctx = useAuth();
      if (pass === 1) contextRef1 = ctx;
      if (pass === 2) contextRef2 = ctx;
      return <div>Pass: {pass}</div>;
    };

    const { rerender } = render(
      <AuthProvider>
        <MemoProbe pass={1} />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(contextRef1).not.toBeNull();
      expect(contextRef1?.isLoading).toBe(false);
    });

    rerender(
      <AuthProvider>
        <MemoProbe pass={2} />
      </AuthProvider>
    );

    expect(contextRef2).not.toBeNull();
    // Referensi value context harus tetap stabil (identik)
    expect(contextRef1).toBe(contextRef2);
    expect(contextRef1!.login).toBe(contextRef2!.login);
    expect(contextRef1!.logout).toBe(contextRef2!.logout);
    expect(contextRef1!.refresh).toBe(contextRef2!.refresh);
  });

  it('menangani login dan pembaruan state principal dengan benar', async () => {
    mockMe.mockResolvedValueOnce(null);
    mockLogin.mockResolvedValueOnce({
      principal: MOCK_PRINCIPAL,
      csrf_token: 'new-token',
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('auth-loading')).toHaveTextContent('ready');
      expect(screen.getByTestId('auth-user')).toHaveTextContent('anonymous');
    });

    fireEvent.click(screen.getByText('Login'));

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith({
        email: 'test@routex.internal',
        password: 'secret',
        keep_signed_in: false,
      });
      expect(screen.getByTestId('auth-user')).toHaveTextContent('admin@routex.internal');
    });
  });

  // Audit3 V-FF-26: event 401 global TIDAK lagi membuang sesi seketika —
  // sesi diverifikasi lewat /me; hanya 401 dari /me sendiri yang logout.
  it('event unauthorized memverifikasi sesi dulu: /me sehat berarti tetap masuk', async () => {
    mockMe.mockResolvedValue({
      principal: MOCK_PRINCIPAL,
      csrf_token: 'token-abc',
    });

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('auth-user')).toHaveTextContent('admin@routex.internal');
    });
    expect(mockMe).toHaveBeenCalledTimes(1);

    act(() => {
      window.dispatchEvent(new CustomEvent('routex:unauthorized'));
    });

    await waitFor(() => {
      expect(mockMe).toHaveBeenCalledTimes(2);
    });
    // Sesi terverifikasi sehat: principal TIDAK dibuang.
    expect(screen.getByTestId('auth-user')).toHaveTextContent('admin@routex.internal');
  });

  it('event unauthorized dengan /me menjawab 401 membersihkan sesi', async () => {
    mockMe
      .mockResolvedValueOnce({
        principal: MOCK_PRINCIPAL,
        csrf_token: 'token-abc',
      })
      // Objek berbentuk ApiError berstatus numerik — extractHttpStatus
      // membaca status dari instance ApiError maupun objek sejenis.
      .mockRejectedValueOnce(Object.assign(new Error('Sesi berakhir'), { status: 401 }));

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('auth-user')).toHaveTextContent('admin@routex.internal');
    });

    act(() => {
      window.dispatchEvent(new CustomEvent('routex:unauthorized'));
    });

    await waitFor(() => {
      expect(screen.getByTestId('auth-user')).toHaveTextContent('anonymous');
    });
  });

  // Audit3 V-FF-25: refresh yang dimulai sebelum logout tidak boleh
  // menghidupkan kembali principal ketika ia selesai belakangan.
  it('refresh basi yang selesai sesudah logout tidak menghidupkan sesi lagi', async () => {
    let selesaikanMe: (v: unknown) => void = () => {};
    mockMe
      .mockResolvedValueOnce({
        principal: MOCK_PRINCIPAL,
        csrf_token: 'token-abc',
      })
      .mockImplementationOnce(
        () => new Promise((resolve) => { selesaikanMe = resolve; })
      );
    mockLogout.mockResolvedValueOnce(undefined);

    render(
      <AuthProvider>
        <TestConsumer />
      </AuthProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('auth-user')).toHaveTextContent('admin@routex.internal');
    });

    // Picu refresh verifikasi yang menggantung.
    act(() => {
      window.dispatchEvent(new CustomEvent('routex:unauthorized'));
    });

    // Logout di tengah refresh itu.
    fireEvent.click(screen.getByText('Logout'));
    await waitFor(() => {
      expect(screen.getByTestId('auth-user')).toHaveTextContent('anonymous');
    });

    // Refresh basi selesai dengan principal yang sah — harus dibuang.
    await act(async () => {
      selesaikanMe({ principal: MOCK_PRINCIPAL, csrf_token: 'token-abc' });
    });
    expect(screen.getByTestId('auth-user')).toHaveTextContent('anonymous');
  });
});
