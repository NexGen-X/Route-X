import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { SystemHealthSection } from './SystemHealthSection';
import { ToastProvider } from '../../context/ToastContext';

// Spec B-226: toast hasil Uji Live harus diturunkan dari checklist
// FINAL yang dirender, dengan masukan SEGAR yang diambil di dalam
// uji — bukan dari state cache/jobs hasil muatan terakhir dan bukan
// hanya dari syarat "DB ok + tak ada provider unhealthy".

const diagMock = vi.fn();
const jobsMock = vi.fn();
const cacheMock = vi.fn();
const healthMock = vi.fn();

vi.mock('../../api/client', () => ({
  api: {
    system: {
      diagnostics: () => diagMock(),
      jobs: () => jobsMock(),
      cacheStats: () => cacheMock(),
      triggerJob: vi.fn(),
    },
    observability: {
      health: () => healthMock(),
    },
  },
}));

const diagOk = {
  version: 'v-test',
  num_goroutine: 42,
  db_pool: { max_conns: 10, acquired_conns: 2 },
};

beforeEach(() => {
  diagMock.mockReset().mockResolvedValue(diagOk);
  jobsMock.mockReset();
  cacheMock
    .mockReset()
    .mockResolvedValue({ enabled: true, total_entries: 5, hits: 3, misses: 1, hit_ratio: 0.75 });
  healthMock.mockReset().mockResolvedValue({ providers: [{ status: 'healthy' }] });
});

function renderSection() {
  return render(
    <ToastProvider>
      <SystemHealthSection />
    </ToastProvider>,
  );
}

describe('SystemHealthSection — gerbang galat muat (B-413)', () => {
  it('jobs GAGAL saja (diagnostik sehat) → QueryError tampil menamai sumber, klaim "tidak ada worker" diganti teks netral', async () => {
    jobsMock.mockRejectedValue(new Error('jobs putus'));
    renderSection();

    expect(await screen.findByText(/daftar worker gagal dimuat/)).toBeInTheDocument();
    expect(screen.getByText('Daftar worker latar belakang belum dapat dimuat.')).toBeInTheDocument();
    expect(
      screen.queryByText('Tidak ada worker latar belakang yang terdaftar atau sedang berjalan.'),
    ).not.toBeInTheDocument();
  });

  it('kedua sumber sehat dan jobs kosong → klaim tidak ada worker tetap sah', async () => {
    jobsMock.mockResolvedValue({ items: [] });
    renderSection();

    expect(
      await screen.findByText('Tidak ada worker latar belakang yang terdaftar atau sedang berjalan.'),
    ).toBeInTheDocument();
  });
});

describe('SystemHealthSection — Uji Live (B-226)', () => {
  it('worker yang BARU gagal (hanya terlihat dari pengambilan segar) → checklist DEGRADED + toast peringatan, BUKAN "Semua sistem nominal"', async () => {
    // Muatan awal: belum ada worker terdaftar. Pada pengambilan segar
    // di dalam uji, satu worker berstatus failed muncul.
    jobsMock
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValue({
        items: [
          { name: 'sync-model', interval: '5m', last_status: 'failed', last_run_at: null, last_duration_ms: 12 },
        ],
      });
    renderSection();

    fireEvent.click(await screen.findByRole('button', { name: /uji live gateway/i }));

    expect(await screen.findByText(/1 dari 1 worker terakhir gagal/)).toBeInTheDocument();
    expect(await screen.findByText(/Background Task Supervisor terdegradasi/)).toBeInTheDocument();
    expect(screen.queryByText(/Semua sistem nominal/)).not.toBeInTheDocument();
  });

  it('semua komponen benar-benar sehat → toast sukses tetap tampil', async () => {
    jobsMock.mockResolvedValue({
      items: [
        { name: 'sync-model', interval: '5m', last_status: 'ok', last_run_at: null, last_duration_ms: 12 },
      ],
    });
    renderSection();

    fireEvent.click(await screen.findByRole('button', { name: /uji live gateway/i }));

    expect(await screen.findByText(/Semua sistem nominal/)).toBeInTheDocument();
  });

  it('statistik cache gagal dimuat saat uji → cache "unknown" ikut menurunkan toast (bukan sukses)', async () => {
    jobsMock.mockResolvedValue({
      items: [
        { name: 'sync-model', interval: '5m', last_status: 'ok', last_run_at: null, last_duration_ms: 12 },
      ],
    });
    // Pengambilan segar statistik cache di dalam uji GAGAL.
    cacheMock.mockReset().mockRejectedValue(new Error('cache tidak terjangkau'));
    renderSection();

    fireEvent.click(await screen.findByRole('button', { name: /uji live gateway/i }));

    expect(await screen.findByText(/Statistik cache tidak tersedia/)).toBeInTheDocument();
    expect(await screen.findByText(/Redis Response Caching Engine terdegradasi atau tidak diketahui/)).toBeInTheDocument();
    expect(screen.queryByText(/Semua sistem nominal/)).not.toBeInTheDocument();
  });
});
