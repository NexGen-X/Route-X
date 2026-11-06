import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Observability } from './Observability';
import { ToastProvider } from '../context/ToastContext';

// Spec B-225: halaman Observability harus jujur soal muatan parsial —
// kegagalan statistik cache terlihat sebagai peringatan + KPI
// "tidak tersedia" (bukan 0,0% hijau karangan), dan kegagalan tombol
// Test Live tampil sebagai pesan galat eksplisit.

const seriesMock = vi.fn();
const breakdownMock = vi.fn();
const summaryMock = vi.fn();
const cacheMock = vi.fn();
const diagMock = vi.fn();
const jobsMock = vi.fn();
const healthMock = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    observability: {
      series: (...a: unknown[]) => seriesMock(...a),
      breakdown: (...a: unknown[]) => breakdownMock(...a),
      summary: (...a: unknown[]) => summaryMock(...a),
      health: () => healthMock(),
    },
    system: {
      cacheStats: () => cacheMock(),
      diagnostics: () => diagMock(),
      jobs: () => jobsMock(),
      triggerJob: vi.fn(),
    },
  },
}));

const diagOk = {
  version: 'v-test',
  num_goroutine: 42,
  db_pool: { max_conns: 10, acquired_conns: 2 },
};

beforeEach(() => {
  seriesMock
    .mockReset()
    .mockResolvedValue({ points: [{ timestamp: '2026-11-06T00:00:00Z', requests: 10, errors: 0, tokens: 100, cost_usd: 0.1 }] });
  breakdownMock.mockReset().mockResolvedValue({ items: [] });
  summaryMock
    .mockReset()
    .mockResolvedValue({ total_requests: 10, error_requests: 0, error_rate: 0, total_tokens: 100, total_cost_usd: 0.1 });
  cacheMock.mockReset().mockResolvedValue({ enabled: true, total_entries: 5, hits: 3, misses: 1 });
  diagMock.mockReset().mockResolvedValue(diagOk);
  jobsMock.mockReset().mockResolvedValue({ items: [] });
  healthMock.mockReset().mockResolvedValue({ providers: [] });
});

function renderPage() {
  return render(
    <ToastProvider>
      <Observability />
    </ToastProvider>,
  );
}

describe('Observability — kejujuran muatan parsial (B-225)', () => {
  it('statistik cache gagal → peringatan parsial + KPI "tidak tersedia", BUKAN 0.0% hijau', async () => {
    cacheMock.mockRejectedValue(new Error('cache tidak terjangkau'));
    renderPage();

    expect(await screen.findByText(/Sebagian data gagal dimuat: statistik cache/)).toBeInTheDocument();
    expect(await screen.findByText('statistik cache tidak tersedia')).toBeInTheDocument();
    expect(screen.queryByText('0.0%')).not.toBeInTheDocument();
  });

  it('semua sumber sukses → tidak ada peringatan parsial dan rasio cache terukur tampil', async () => {
    renderPage();

    expect(await screen.findByText('75.0%')).toBeInTheDocument();
    expect(screen.queryByText(/Sebagian data gagal dimuat/)).not.toBeInTheDocument();
  });

  it('Test Live gagal → pesan galat eksplisit tampil (bukan tombol diam kembali)', async () => {
    diagMock.mockRejectedValue(new Error('diagnostik tidak terjangkau'));
    renderPage();

    fireEvent.click(await screen.findByRole('button', { name: /uji latensi live gateway/i }));

    expect(await screen.findByText(/Uji live gagal: diagnostik tidak terjangkau/)).toBeInTheDocument();
  });
});
