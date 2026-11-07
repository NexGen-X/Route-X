import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SystemHealthSection } from './SystemHealthSection';
import { ToastProvider } from '../../context/ToastContext';

// Spec pengunci B-594: worker yang BELUM PERNAH berjalan (last_status
// kosong, last_duration_ms 0, last_run_at null) TIDAK BOLEH berlencana
// IDLE merah (varian error) dan TIDAK BOLEH menampilkan durasi "0 ms"
// — keadaannya netral dan durasinya belum terukur.

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

describe('SystemHealthSection — worker belum pernah berjalan (B-594)', () => {
  it("job last_status:'' + last_duration_ms:0 + last_run_at null → IDLE netral (bukan rose) + '—' + 'Belum pernah'", async () => {
    jobsMock.mockResolvedValue({
      items: [
        {
          name: 'budget-resetter',
          interval: '1m',
          last_status: '',
          last_duration_ms: 0,
          last_run_at: null,
        },
      ],
    });
    renderSection();

    const lencana = await screen.findByText('IDLE');
    expect(lencana.className).not.toMatch(/rose/);
    expect(screen.getByText('Belum pernah')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
    expect(screen.queryByText('0 ms')).not.toBeInTheDocument();
  });

  it('kontrol: job ok → lencana OK emerald + durasi terukur', async () => {
    jobsMock.mockResolvedValue({
      items: [
        {
          name: 'budget-resetter',
          interval: '1m',
          last_status: 'ok',
          last_duration_ms: 250,
          last_run_at: '2026-11-07T15:00:00Z',
        },
      ],
    });
    renderSection();

    const lencana = await screen.findByText('OK');
    expect(lencana.className).toMatch(/emerald/);
    expect(screen.getByText('250 ms')).toBeInTheDocument();
  });

  it('job FAILED tetap berlencana error (rose) — netral hanya untuk status kosong/IDLE', async () => {
    jobsMock.mockResolvedValue({
      items: [
        {
          name: 'budget-resetter',
          interval: '1m',
          last_status: 'failed',
          last_duration_ms: 120,
          last_run_at: '2026-11-07T15:00:00Z',
        },
      ],
    });
    renderSection();

    const lencana = await screen.findByText('FAILED');
    expect(lencana.className).toMatch(/rose/);
    expect(screen.getByText('120 ms')).toBeInTheDocument();
  });
});
