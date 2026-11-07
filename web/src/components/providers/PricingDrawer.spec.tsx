import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { PricingDrawer, hargaEfektifSekarang } from './PricingDrawer';
import type { Price, Provider, ProviderModel, ReferencePrice } from '../../types';

// Drawer memakai focus-trap-react; di jsdom trap dimatikan seperti spec lain.
vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const mockToastSuccess = vi.fn();
vi.mock('../../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: mockToastSuccess, error: vi.fn(), warn: vi.fn(), info: vi.fn() },
    confirmModal: vi.fn(),
  }),
}));

const mockHistory = vi.fn();
const mockReference = vi.fn();
const mockSetPrice = vi.fn();
vi.mock('../../api/client', () => ({
  api: {
    models: {
      pricingHistory: (...args: unknown[]) => mockHistory(...args),
      pricingReference: (...args: unknown[]) => mockReference(...args),
      setPrice: (...args: unknown[]) => mockSetPrice(...args),
    },
  },
}));

const MAPPING: ProviderModel = {
  id: 'pm-1',
  model_id: 'model-uuid-1',
  provider_id: 'prov-1',
  upstream_model_name: 'claude-sonnet-5',
  enabled: true,
  created_at: '2026-09-01T00:00:00Z',
};

const PROVIDER: Provider = {
  id: 'prov-1',
  name: 'anthropic-utama',
  display_name: 'Anthropic Utama',
  kind: 'anthropic',
  base_url: 'https://api.anthropic.com',
  enabled: true,
  priority: 1,
  weight: 1,
  timeout_ms: 30000,
  max_retries: 2,
  consecutive_failures: 0,
  created_at: '2026-09-01T00:00:00Z',
  updated_at: '2026-09-01T00:00:00Z',
};

// Harga efektif contoh: sama persis dengan acuan katalog seed Anthropic
// untuk claude-sonnet-5 (input 2, output 10, cached 0.20).
const HARGA_BERLAKU: Price = {
  id: 'price-2',
  provider_model_id: 'pm-1',
  input_per_1m_usd: '2',
  output_per_1m_usd: '10',
  cached_input_per_1m_usd: '0.2',
  reasoning_per_1m_usd: null,
  currency: 'USD',
  effective_from: '2026-09-02T00:00:00Z',
  effective_to: null,
  source: 'manual',
  created_at: '2026-09-02T00:00:00Z',
};

const HARGA_LAMA: Price = {
  ...HARGA_BERLAKU,
  id: 'price-1',
  input_per_1m_usd: '3',
  output_per_1m_usd: '15',
  effective_from: '2026-09-01T00:00:00Z',
  effective_to: '2026-09-02T00:00:00Z',
};

const ACUAN_TERSEDIA: ReferencePrice = {
  available: true,
  provider_kind: 'anthropic',
  upstream_model: 'claude-sonnet-5',
  input_per_1m_usd: '2',
  output_per_1m_usd: '10',
  cached_input_per_1m_usd: '0.2',
  source: 'platform.claude.com/docs/en/about-claude/pricing',
  fetched_on: '2026-09-02',
};

const ACUAN_KOSONG: ReferencePrice = {
  available: false,
  provider_kind: 'anthropic',
  upstream_model: 'claude-sonnet-5',
};

function renderDrawer() {
  return render(<PricingDrawer mapping={MAPPING} provider={PROVIDER} onClose={vi.fn()} />);
}

beforeEach(() => {
  vi.clearAllMocks();
  mockHistory.mockResolvedValue({ items: [HARGA_BERLAKU, HARGA_LAMA] });
  mockReference.mockResolvedValue(ACUAN_TERSEDIA);
  mockSetPrice.mockResolvedValue(HARGA_BERLAKU);
});

describe('hargaEfektifSekarang', () => {
  const sekarang = new Date('2026-09-03T00:00:00Z');
  it('memilih baris yang rentang efektifnya memuat sekarang, bukan sekadar baris terbuka', () => {
    expect(hargaEfektifSekarang([HARGA_LAMA, HARGA_BERLAKU], sekarang)?.id).toBe('price-2');
  });
  it('mengabaikan baris terbuka yang effective_from-nya masih di masa depan', () => {
    const masaDepan: Price = { ...HARGA_BERLAKU, id: 'price-3', effective_from: '2027-01-01T00:00:00Z' };
    expect(hargaEfektifSekarang([masaDepan], sekarang)).toBeUndefined();
  });
  it('mengembalikan undefined untuk riwayat kosong', () => {
    expect(hargaEfektifSekarang([], sekarang)).toBeUndefined();
  });
});

describe('PricingDrawer', () => {
  it('menampilkan harga acuan dari endpoint reference beserta sumbernya', async () => {
    renderDrawer();
    expect(await screen.findByText(/platform\.claude\.com\/docs\/en\/about-claude\/pricing/)).toBeTruthy();
    expect(mockReference).toHaveBeenCalledWith('pm-1');
    // Nilai acuan tampil di seksi acuan DAN (karena sama) di seksi efektif.
    expect(screen.getAllByText('$2 / 1M token').length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText(/hanya tawaran awal/)).toBeTruthy();
  });

  it('menampilkan harga efektif dari riwayat dan riwayat lamanya', async () => {
    renderDrawer();
    await screen.findByText(/platform\.claude\.com/);
    expect(mockHistory).toHaveBeenCalledWith('pm-1');
    // Baris riwayat lama tampil sebagai riwayat, bukan sebagai harga efektif.
    expect(screen.getByText(/\$3 in · \$15 out/)).toBeTruthy();
    expect(screen.getByText(/masih terbuka/)).toBeTruthy();
  });

  it('menyatakan belum ada harga efektif dan acuan kosong secara jujur', async () => {
    mockHistory.mockResolvedValue({ items: [] });
    mockReference.mockResolvedValue(ACUAN_KOSONG);
    renderDrawer();
    expect(await screen.findByText(/tanpa biaya \(unpriced\)/)).toBeTruthy();
    expect(screen.getByText(/tidak punya harga yang bisa dipertanggungjawabkan/)).toBeTruthy();
    expect(screen.getByText(/Belum ada riwayat harga/)).toBeTruthy();
  });

  it('menampilkan galat yang jujur dan tombol coba lagi saat endpoint gagal', async () => {
    mockHistory.mockRejectedValue(new Error('jaringan putus'));
    renderDrawer();
    expect(await screen.findByText(/Data gagal dimuat/)).toBeTruthy();
    expect(screen.getByRole('button', { name: /Coba Lagi/ })).toBeTruthy();
  });

  it('mengisi formulir dari acuan tanpa langsung memasang, lalu menulis lewat endpoint tulis', async () => {
    renderDrawer();
    await screen.findByText(/platform\.claude\.com/);
    fireEvent.click(screen.getByRole('button', { name: /Pakai harga acuan sebagai isian/ }));
    expect((screen.getByLabelText(/Input \(USD/) as HTMLInputElement).value).toBe('2');
    expect((screen.getByLabelText(/Output \(USD/) as HTMLInputElement).value).toBe('10');
    // Mengisi dari acuan TIDAK boleh memasang harga dengan sendirinya.
    expect(mockSetPrice).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: /Pasang harga/ }));
    await waitFor(() => expect(mockSetPrice).toHaveBeenCalledTimes(1));
    expect(mockSetPrice).toHaveBeenCalledWith('pm-1', {
      input_per_1m_usd: '2',
      output_per_1m_usd: '10',
      cached_input_per_1m_usd: '0.2',
    });
    await waitFor(() => expect(mockToastSuccess).toHaveBeenCalled());
    // Sesudah menulis, riwayat dimuat ulang agar harga efektif terbaru tampil.
    expect(mockHistory).toHaveBeenCalledTimes(2);
  });

  it('menolak submit kosong tanpa memanggil endpoint tulis', async () => {
    renderDrawer();
    await screen.findByText(/platform\.claude\.com/);
    fireEvent.click(screen.getByRole('button', { name: /Pasang harga/ }));
    expect(await screen.findByText(/wajib diisi/)).toBeTruthy();
    expect(mockSetPrice).not.toHaveBeenCalled();
  });
});

describe('PricingDrawer — penjaga urutan muat (B-486)', () => {
  it('respons basi pemetaan sebelumnya tidak menimpa acuan pemetaan yang sedang dibuka', async () => {
    let resolveHistA!: (v: { items: Price[] }) => void;
    let resolveRefA!: (v: ReferencePrice) => void;
    const janjiHistA = new Promise<{ items: Price[] }>((r) => {
      resolveHistA = r;
    });
    const janjiRefA = new Promise<ReferencePrice>((r) => {
      resolveRefA = r;
    });
    mockHistory.mockImplementation((id: string) =>
      id === 'pm-1' ? janjiHistA : Promise.resolve({ items: [] as Price[] })
    );
    mockReference.mockImplementation((id: string) =>
      id === 'pm-1'
        ? janjiRefA
        : Promise.resolve({
            ...ACUAN_TERSEDIA,
            input_per_1m_usd: '7',
            output_per_1m_usd: '70',
            cached_input_per_1m_usd: '0.7',
            source: 'sumber-b.example',
          } as ReferencePrice)
    );

    const { rerender } = render(
      <PricingDrawer mapping={MAPPING} provider={PROVIDER} onClose={vi.fn()} />
    );
    // Berpindah ke pemetaan B selagi muatan A tertahan.
    rerender(
      <PricingDrawer mapping={{ ...MAPPING, id: 'pm-2' }} provider={PROVIDER} onClose={vi.fn()} />
    );
    expect(await screen.findByText(/sumber-b\.example/)).toBeTruthy();

    // Respons A tiba terakhir — wajib diabaikan.
    await act(async () => {
      resolveHistA({ items: [] });
      resolveRefA(ACUAN_TERSEDIA);
    });
    expect(screen.queryByText(/platform\.claude\.com/)).toBeNull();

    // "Pakai acuan" mengisi harga MILIK B (7), bukan milik A (2).
    fireEvent.click(screen.getByRole('button', { name: /Pakai harga acuan sebagai isian/ }));
    expect((screen.getByLabelText(/Input \(USD/) as HTMLInputElement).value).toBe('7');
  });
});
