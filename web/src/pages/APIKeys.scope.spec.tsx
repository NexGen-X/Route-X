import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { APIKeys } from './APIKeys';
import { ToastProvider } from '../context/ToastContext';
import type { APIKey } from '../types';

// B-445: drawer scope kunci API — daftar model yang GAGAL dimuat
// tidak boleh tampil sebagai klaim kosong + footer "Akses Bebas",
// dan Simpan harus nonaktif (menyimpan dari daftar kosong akan
// menulis scope kosong = akses bebas).

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
const mockModelsList = vi.fn();
const mockBudgetsList = vi.fn(async () => ({ items: [] as unknown[] }));
const mockGetAllowed = vi.fn(async (_id: string) => ({ model_ids: [] as string[], provider_ids: [] as string[] }));
const mockSetAllowed = vi.fn(async (_id: string, _body: unknown) => ({}));

vi.mock('../api/client', () => ({
  api: {
    apiKeys: {
      listAll: () => mockKeysList(),
      getAllowed: (id: string) => mockGetAllowed(id),
      setAllowed: (id: string, body: unknown) => mockSetAllowed(id, body),
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
    </QueryClientProvider>
  );
};

describe('APIKeys — drawer scope saat model gagal dimuat (B-445)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockKeysList.mockResolvedValue({ items: [KEY] });
    mockBudgetsList.mockResolvedValue({ items: [] });
    mockGetAllowed.mockResolvedValue({ model_ids: [], provider_ids: [] });
  });

  it('model gagal → klaim kosong & footer akses bebas TIDAK tampil, Simpan nonaktif', async () => {
    mockModelsList.mockRejectedValue(new Error('jaringan putus'));
    renderHalaman();

    fireEvent.click(await screen.findByRole("button", { name: /konfigurasi scope model/i }));

    expect((await screen.findAllByText(/Daftar model belum dapat dimuat/)).length).toBeGreaterThan(0);
    expect(
      screen.queryByText('Tidak ada model atau provider yang cocok dengan kriteria pencarian.')
    ).toBeNull();
    expect(screen.queryByText('Akses Bebas (Seluruh model diizinkan)')).toBeNull();
    const simpan = screen.getByRole('button', { name: /simpan hak akses/i });
    expect(simpan).toBeDisabled();
    fireEvent.click(simpan);
    expect(mockSetAllowed).not.toHaveBeenCalled();
  });

  it('getAllowed gagal → keadaan galat tampil dan Simpan nonaktif walau model termuat', async () => {
    mockModelsList.mockResolvedValue({
      items: [
        {
          id: 'model-1',
          model_id: 'gpt-uji',
          display_name: 'GPT Uji',
          providers: [],
        },
      ],
    });
    mockGetAllowed.mockRejectedValue(new Error('server menolak'));
    renderHalaman();

    fireEvent.click(await screen.findByRole("button", { name: /konfigurasi scope model/i }));

    expect((await screen.findAllByText(/Daftar model belum dapat dimuat/)).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: /simpan hak akses/i })).toBeDisabled();
    expect(mockSetAllowed).not.toHaveBeenCalled();
  });
});
