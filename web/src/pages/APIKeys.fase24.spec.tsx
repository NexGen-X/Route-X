import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { APIKeys } from './APIKeys';
import { ToastProvider } from '../context/ToastContext';
import type { APIKey } from '../types';

// B-555 (toolbar APIKeys): hitungan "Menampilkan X dari Y kunci"
// berada DI LUAR cabang galat daftar — saat refetch gagal daftar
// bertukar ke QueryError tetapi toolbar tetap mengutip hitungan
// data basi secara definitif. Kini pada keadaan galat hitungan
// tampil sebagai penanda netral '—' (pola chip B-518).

const KEY: APIKey = {
  id: 'key-1',
  name: 'Kunci Uji',
  masked_key: 'sk-****1234',
  scopes: ['inference'],
  allowed_models: [],
  allowed_providers: [],
  enabled: true,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const mockKeysList = vi.fn(async () => ({ items: [KEY] as APIKey[] }));
const mockModelsList = vi.fn(async () => ({ items: [] as unknown[] }));
const mockBudgetsList = vi.fn(async () => ({ items: [] as unknown[] }));

vi.mock('../api/client', () => ({
  api: {
    apiKeys: {
      listAll: () => mockKeysList(),
      getAllowed: vi.fn(async () => ({ model_ids: [], provider_ids: [] })),
      setAllowed: vi.fn(async () => ({})),
    },
    models: {
      listAll: () => mockModelsList(),
    },
    budgets: {
      list: () => mockBudgetsList(),
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
        <APIKeys />
      </ToastProvider>
    </QueryClientProvider>,
  );
};

describe('APIKeys — toolbar saat refetch gagal (B-555)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockKeysList.mockResolvedValue({ items: [KEY] });
    mockModelsList.mockResolvedValue({ items: [] });
    mockBudgetsList.mockResolvedValue({ items: [] });
  });

  it('refetch gagal → hitungan toolbar menjadi "—", bukan angka basi definitif', async () => {
    renderHalaman();

    const kolomCari = await screen.findByPlaceholderText(/Cari berdasarkan nama atau prefix kunci/);
    const toolbar = kolomCari.closest('div')?.parentElement as HTMLElement;
    expect(toolbar.textContent).toContain('Menampilkan');
    expect(toolbar.textContent).toContain('dari');
    expect(toolbar.textContent).not.toContain('—');

    mockKeysList.mockRejectedValue(new Error('daftar kunci meledak'));
    fireEvent.click(screen.getByRole('button', { name: /Segarkan daftar kunci API/i }));

    expect(await screen.findByText(/daftar kunci meledak/)).toBeInTheDocument();
    await waitFor(() => {
      expect(within(toolbar).getAllByText('—')).toHaveLength(2);
    });
  });
});
