import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Providers } from './Providers';
import { ToastProvider } from '../context/ToastContext';
import type { Provider } from '../types';

// Spec B-345: strip ringkasan Providers tidak boleh menampilkan
// angka dari daftar yang belum tiba. Query yang masih berjalan
// atau gagal membuat `providers`/`egressPools` jatuh ke `?? []` —
// menghitungnya menghasilkan "0" karangan ("Total Provider 0",
// "Status Sehat 0 / 0") di halaman yang datanya belum ada atau
// gagal dimuat. Sampai query-nya ber-data, strip menampilkan "—"
// (pola B-284 di Observability).

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
  last_health_status: 'healthy',
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const mockListAll = vi.fn();
const mockEgressList = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    providers: {
      listAll: () => mockListAll(),
      listOAuthSessions: async () => ({ items: [] }),
      toggle: async () => ({}),
    },
    egress: {
      list: () => mockEgressList(),
    },
    credentials: {
      list: async () => ({ items: [] }),
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
    </QueryClientProvider>,
  );
};

describe('Providers — strip ringkasan hanya dari data yang tiba (B-345)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('daftar provider gagal dimuat → strip "—", BUKAN 0 karangan', async () => {
    mockListAll.mockRejectedValue(new Error('daftar tidak terjangkau'));
    mockEgressList.mockResolvedValue({ items: [] });
    renderHalaman();

    // Penanda galat halaman tampil (query selesai, gagal) — pada
    // titik ini strip versi lama sudah mengklaim 0 / 0.
    expect(await screen.findByText(/Daftar provider tidak tersedia|daftar tidak terjangkau/i)).toBeInTheDocument();
    expect(screen.queryByText('0 / 0')).not.toBeInTheDocument();
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(2);
  });

  it('semua query ber-data → angka nyata tampil, termasuk nol yang terukur', async () => {
    mockListAll.mockResolvedValue({ items: [PROVIDER], terpotong: undefined });
    mockEgressList.mockResolvedValue({
      items: [
        { id: 'pool-1', name: 'Pool Satu', description: '', kind: 'http', enabled: true, weight: 1 },
        { id: 'pool-2', name: 'Pool Dua', description: '', kind: 'http', enabled: true, weight: 1 },
      ],
    });
    renderHalaman();

    expect(await screen.findByText('1 / 1')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
  });
});
