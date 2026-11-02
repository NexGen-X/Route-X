import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Providers } from './Providers';
import { ToastProvider } from '../context/ToastContext';
import type { Provider } from '../types';

// Spec regresi level halaman untuk:
// - B-075: kegagalan memuat kredensial/sesi OAuth pada queryFn
//   poolsSummary harus tampil sebagai penanda gagal-muat di kartu
//   provider, BUKAN daftar kosong yang meyakinkan.
// - Sisa V-FF-28: hasil listAll bertanda `terpotong` memunculkan
//   peringatan daftar tidak lengkap.

const PROVIDER: Provider = {
  id: 'prov-1',
  name: 'openai-main',
  display_name: 'OpenAI Production',
  kind: 'openai',
  base_url: 'https://api.openai.com/v1',
  enabled: true,
  priority: 100,
  weight: 100,
  timeout_ms: 30000,
  max_retries: 2,
  consecutive_failures: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const mockListAll = vi.fn(async () => ({ items: [PROVIDER] as Provider[], terpotong: undefined as boolean | undefined }));
const mockCredentialsList = vi.fn(async (_id: string) => ({ items: [] as unknown[] }));
const mockOAuthSessions = vi.fn(async (_id: string) => ({ items: [] as unknown[] }));

vi.mock('../api/client', () => ({
  api: {
    providers: {
      listAll: () => mockListAll(),
      listOAuthSessions: (id: string) => mockOAuthSessions(id),
    },
    egress: {
      list: async () => ({ items: [] }),
    },
    credentials: {
      list: (id: string) => mockCredentialsList(id),
    },
  },
}));

const renderHalaman = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <Providers />
      </ToastProvider>
    </QueryClientProvider>
  );
};

describe('Providers — ringkasan pool & penanda terpotong', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListAll.mockResolvedValue({ items: [PROVIDER], terpotong: undefined });
    mockCredentialsList.mockResolvedValue({ items: [] });
    mockOAuthSessions.mockResolvedValue({ items: [] });
  });

  it('B-075: kredensial gagal dimuat → kartu menampilkan penanda gagal-muat', async () => {
    mockCredentialsList.mockRejectedValue(new Error('jaringan putus'));
    renderHalaman();
    expect(await screen.findByText('Ringkasan kredensial gagal dimuat')).toBeInTheDocument();
    expect(screen.queryByText('Belum ada kredensial')).toBeNull();
  });

  it('B-075: semua sumber terbaca → tidak ada penanda gagal-muat', async () => {
    renderHalaman();
    expect(await screen.findByText('Belum ada kredensial')).toBeInTheDocument();
    expect(screen.queryByText('Ringkasan kredensial gagal dimuat')).toBeNull();
  });

  it('V-FF-28: listAll terpotong → peringatan daftar tidak lengkap tampil', async () => {
    mockListAll.mockResolvedValue({ items: [PROVIDER], terpotong: true });
    renderHalaman();
    expect(await screen.findByText(/Daftar provider mungkin tidak lengkap/)).toBeInTheDocument();
  });
});
