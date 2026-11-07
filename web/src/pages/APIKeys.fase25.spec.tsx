import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { APIKeys } from './APIKeys';
import { ToastProvider } from '../context/ToastContext';
import type { APIKey } from '../types';

// B-558 (meter anggaran APIKeys): query ['budgets'] hanya
// mendestruktur `data` tanpa `isError`; saat refetch gagal
// sesudah sukses React Query menahan data terakhir dan kartu
// tetap menampilkan `$45.00 / $50.00 (90%)` + batang secara
// definitif tanpa penanda. Pada muat-awal gagal kartu kunci
// beranggaran tak terbedakan dari kartu tanpa anggaran (meter
// satu-satunya indikator anggaran di kartu). Kini gerbang
// isError budgets → meter '—' berpenanda basi per kartu +
// spanduk gagal-muat persisten di halaman (pola B-554/B-555).

const KEY_BERANGGARAN: APIKey = {
  id: 'key-1',
  name: 'Kunci Beranggaran',
  masked_key: 'sk-****1234',
  scopes: ['inference'],
  allowed_models: [],
  allowed_providers: [],
  enabled: true,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const KEY_TANPA_ANGGARAN: APIKey = {
  id: 'key-2',
  name: 'Kunci Tanpa Anggaran',
  masked_key: 'sk-****5678',
  scopes: ['inference'],
  allowed_models: [],
  allowed_providers: [],
  enabled: true,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const ANGGARAN_KEY_1 = {
  id: 'b-1',
  name: 'Anggaran Kunci Beranggaran',
  scope: 'api_key',
  scope_id: 'key-1',
  period: 'monthly',
  max_spend_usd: '50.00',
  spent_usd: '45.00',
  enabled: true,
  alert_threshold_pct: 80,
};

const mockKeysList = vi.fn(async () => ({ items: [KEY_BERANGGARAN, KEY_TANPA_ANGGARAN] as APIKey[] }));
const mockModelsList = vi.fn(async () => ({ items: [] as unknown[] }));
const mockBudgetsList = vi.fn(async () => ({ items: [ANGGARAN_KEY_1] as unknown[] }));

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
  const hasil = render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <APIKeys />
      </ToastProvider>
    </QueryClientProvider>,
  );
  return { ...hasil, queryClient };
};

describe('APIKeys — meter anggaran saat budgets gagal (B-558)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockKeysList.mockResolvedValue({ items: [KEY_BERANGGARAN, KEY_TANPA_ANGGARAN] });
    mockModelsList.mockResolvedValue({ items: [] });
    mockBudgetsList.mockResolvedValue({ items: [ANGGARAN_KEY_1] });
  });

  it('budgets sukses → meter definitif tampil, tanpa spanduk gagal-muat', async () => {
    renderHalaman();

    expect(await screen.findByText(/\$45\.00 \/ \$50\.00 \(90%\)/)).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    // Hanya kartu beranggaran yang punya meter; kartu tanpa anggaran tetap tanpa meter.
    expect(screen.getAllByText(/Anggaran Bulanan/)).toHaveLength(1);
  });

  it('refetch budgets gagal sesudah sukses → meter menjadi "—" berpenanda basi, bukan angka basi definitif', async () => {
    const { queryClient } = renderHalaman();

    expect(await screen.findByText(/\$45\.00 \/ \$50\.00 \(90%\)/)).toBeInTheDocument();

    // Jalur nyata: refetch budgets lewat invalidasi query — jalur
    // yang sama dipakai handleCreateSubmit sesudah kunci dibuat.
    mockBudgetsList.mockRejectedValue(new Error('anggaran meledak'));
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['budgets'] });
    });

    expect(await screen.findByText(/Data anggaran basi/)).toBeInTheDocument();
    expect(screen.queryByText(/\$45\.00 \/ \$50\.00 \(90%\)/)).not.toBeInTheDocument();
    // Spanduk gagal-muat persisten ikut tampil di halaman.
    expect(await screen.findByRole('alert')).toHaveTextContent(/Anggaran gagal dimuat/);
    // Label meter tetap ada (kartu beranggaran tetap terbedakan),
    // tetapi angkanya penanda netral '—', bukan angka basi.
    expect(screen.getAllByText(/Anggaran Bulanan/)).toHaveLength(1);
    expect(screen.getByRole('progressbar', { name: /belum terukur/ })).toBeInTheDocument();
  });

  it('muat-awal budgets gagal → spanduk gagal-muat persisten tampil, meter tidak tampil sama sekali', async () => {
    mockBudgetsList.mockRejectedValue(new Error('anggaran meledak'));
    renderHalaman();

    expect(await screen.findByText('Kunci Beranggaran')).toBeInTheDocument();
    expect(await screen.findByRole('alert')).toHaveTextContent(/Anggaran gagal dimuat/);
    expect(screen.queryByText(/\$45\.00/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Anggaran Bulanan/)).not.toBeInTheDocument();
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });
});
