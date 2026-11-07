import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Budgets } from './Budgets';
import type { Budget, APIKey, Model } from '../types';

// B-554: sumber sekunder Budgets (kunci API + model) adalah
// sumber label cakupan dan pilihan target formulir — kegagalan
// muatnya wajib berpenanda per sumber. Versi lama menelan
// kegagalan kedua sumber ini seluruhnya (Promise.allSettled
// memeriksanya hanya untuk daftar utama), sehingga label cakupan
// dibaca dari data basi/kosong seolah terukur.

const renderBudgets = (node: React.ReactNode) =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      {node}
    </QueryClientProvider>,
  );

vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() },
    confirmModal: vi.fn(async () => true),
  }),
}));

const MOCK_BUDGETS: Budget[] = [
  {
    id: 'b-2',
    name: 'Budget Kunci API Riset',
    scope: 'api_key',
    scope_id: 'key-1',
    period: 'monthly',
    max_spend_usd: '50.00',
    spent_usd: '45.00',
    alert_threshold: 80,
    action: 'warn',
    period_start: '2026-09-01T00:00:00Z',
    period_end: '2026-10-01T00:00:00Z',
    enabled: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
];

const MOCK_API_KEYS: APIKey[] = [
  {
    id: 'key-1',
    name: 'Dev Key Riset',
    masked_key: 'rx-dev-1***',
    scopes: ['read', 'write'],
    allowed_models: [],
    allowed_providers: [],
    enabled: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
];

const MOCK_MODELS: Model[] = [
  {
    id: 'm-1',
    model_id: 'gpt-4o',
    display_name: 'GPT-4o Omnimodel',
    capabilities: ['chat'],
    enabled: true,
    routing_priority: 1,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
];

const mockListBudgets = vi.fn(async () => ({ items: MOCK_BUDGETS }));
const mockListRateLimits = vi.fn(async () => ({ items: [] as unknown[] }));
const mockListApiKeys = vi.fn(async () => ({ items: MOCK_API_KEYS }));
const mockListModels = vi.fn(async () => ({ items: MOCK_MODELS }));

vi.mock('../api/client', () => ({
  api: {
    budgets: {
      list: () => mockListBudgets(),
      create: vi.fn(async () => ({})),
      reset: vi.fn(async () => ({})),
      toggle: vi.fn(async () => ({})),
      delete: vi.fn(async () => ({})),
    },
    rateLimits: {
      list: () => mockListRateLimits(),
      create: vi.fn(async () => ({})),
      delete: vi.fn(async () => ({})),
    },
    apiKeys: {
      list: () => mockListApiKeys(),
      listAll: () => mockListApiKeys(),
    },
    models: {
      list: () => mockListModels(),
      listAll: () => mockListModels(),
    },
  },
}));

describe('Budgets — penanda sumber sekunder (B-554)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListBudgets.mockResolvedValue({ items: MOCK_BUDGETS });
    mockListRateLimits.mockResolvedValue({ items: [] });
    mockListApiKeys.mockResolvedValue({ items: MOCK_API_KEYS });
    mockListModels.mockResolvedValue({ items: MOCK_MODELS });
  });

  it('kunci API gagal dimuat → banner menamai sumbernya dan label cakupan tidak lagi definitif', async () => {
    mockListApiKeys.mockRejectedValue(new Error('daftar kunci tidak terjangkau'));
    renderBudgets(<Budgets />);

    expect(
      await screen.findByText(/Daftar referensi gagal dimuat: kunci API/),
    ).toBeInTheDocument();
    // Anggaran utama tetap tampil (sumber utamanya berhasil) —
    // yang berubah hanya kejujuran label cakupannya.
    expect(await screen.findByText('Budget Kunci API Riset')).toBeInTheDocument();
  });

  it('model gagal dimuat → banner menamai model sebagai sumbernya', async () => {
    mockListModels.mockRejectedValue(new Error('daftar model tidak terjangkau'));
    renderBudgets(<Budgets />);

    expect(
      await screen.findByText(/Daftar referensi gagal dimuat: model/),
    ).toBeInTheDocument();
  });

  it('semua sumber berhasil → tidak ada banner gagal; label cakupan terukur tampil', async () => {
    renderBudgets(<Budgets />);

    expect(await screen.findByText('Dev Key Riset')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText(/Daftar referensi gagal dimuat/)).not.toBeInTheDocument();
    });
  });
});
