import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Providers } from './Providers';
import { ToastProvider } from '../context/ToastContext';
import type { Provider } from '../types';

// B-555 (strip Providers): saat refetch daftar provider GAGAL,
// React Query mempertahankan data lama dan daftar bertukar ke
// QueryError — angka strip tidak boleh tetap definitif (apalagi
// "Status Sehat" berwarna emerald) dari data basi. Strip kini
// bergerbang status galat query sumbernya, per sumber: strip
// provider pada query provider, Pool Egress pada query pool.

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
const mockEgressList = vi.fn(async () => ({
  items: [{ id: 'pool-1', name: 'Pool Utama', kind: 'proxy' }] as unknown[],
}));

vi.mock('../api/client', () => ({
  api: {
    providers: {
      listAll: () => mockListAll(),
      listOAuthSessions: vi.fn(async () => ({ items: [] })),
      toggle: vi.fn(async () => ({})),
    },
    egress: {
      list: () => mockEgressList(),
    },
    credentials: {
      list: vi.fn(async () => ({ items: [] })),
    },
  },
}));

const buatRender = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui = render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <Providers />
      </ToastProvider>
    </QueryClientProvider>,
  );
  return { queryClient, ...ui };
};

describe('Providers — strip ringkasan saat refetch gagal (B-555)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListAll.mockResolvedValue({ items: [PROVIDER], terpotong: undefined });
    mockEgressList.mockResolvedValue({ items: [{ id: 'pool-1', name: 'Pool Utama', kind: 'proxy' }] });
  });

  it('refetch provider gagal → strip provider netral ("—"), strip pool tetap terukur dari sumbernya sendiri', async () => {
    const { queryClient } = buatRender();

    // Muatan awal berhasil: strip definitif dari data terukur.
    expect(await screen.findByText('OpenAI Production')).toBeInTheDocument();
    expect(screen.getByText('0 / 1')).toBeInTheDocument();

    // Refetch berikutnya gagal.
    mockListAll.mockRejectedValue(new Error('daftar provider meledak'));
    await queryClient.invalidateQueries({ queryKey: ['providers'] });

    // Daftar bertukar ke QueryError (perilaku lama) DAN strip
    // provider berhenti definitif (B-555).
    expect(await screen.findByText(/daftar provider meledak/)).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText('0 / 1')).not.toBeInTheDocument();
    });
    // Pool Egress berasal dari query LAIN yang tetap berhasil —
    // angkanya tetap terukur (kesegaran per sumber).
    expect(screen.getByText('Pool Egress')).toBeInTheDocument();
  });
});
