import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Dashboard } from './Dashboard';
import type {
  ObservabilitySummary,
  TimeSeriesPoint,
  Provider,
  SystemOverview,
  RequestLog,
} from '../types';

// Mock focus-trap-react untuk jsdom
vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

// Mock clipboard
vi.mock('../utils/clipboard', () => ({
  copyTextToClipboard: vi.fn(),
}));

// Mock recharts ResponsiveContainer untuk jsdom
vi.mock('recharts', async () => {
  const original = await vi.importActual<Record<string, unknown>>('recharts');
  return {
    ...original,
    ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
      <div data-testid="responsive-container" style={{ width: 800, height: 400 }}>
        {children}
      </div>
    ),
  };
});

const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
vi.mock('../context/ToastContext', () => ({
  useToast: () => ({
    toast: {
      success: mockToastSuccess,
      error: mockToastError,
      warn: vi.fn(),
      info: vi.fn(),
    },
  }),
}));

const MOCK_SUMMARY: ObservabilitySummary = {
  window: '24h',
  total_requests: 1250,
  success_requests: 1230,
  error_requests: 20,
  error_rate: 1.6,
  total_tokens: 450000,
  prompt_tokens: 300000,
  completion_tokens: 150000,
  total_cost_usd: '0.04500000',
  avg_latency_ms: 180,
  p95_latency_ms: 320,
  active_providers: 2,
  circuit_breakers_open: 0,
};

const MOCK_SERIES: TimeSeriesPoint[] = [
  {
    timestamp: '12:00',
    requests: 50,
    errors: 1,
    tokens: 10000,
    cost_usd: '0.001',
    p50_latency_ms: 120,
    p95_latency_ms: 250,
  },
  {
    timestamp: '13:00',
    requests: 80,
    errors: 0,
    tokens: 18000,
    cost_usd: '0.002',
    p50_latency_ms: 140,
    p95_latency_ms: 280,
  },
];

const MOCK_PROVIDERS: Provider[] = [
  {
    id: 'prov-openai',
    name: 'openai-main',
    display_name: 'OpenAI Production',
    kind: 'openai',
    base_url: 'https://api.openai.com/v1',
    enabled: true,
    priority: 1,
    weight: 100,
    timeout_ms: 30000,
    max_retries: 3,
    consecutive_failures: 0,
    last_health_status: 'healthy',
    last_latency_ms: 145,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'prov-anthropic',
    name: 'anthropic-backup',
    display_name: 'Claude Anthropic',
    kind: 'anthropic',
    base_url: 'https://api.anthropic.com',
    enabled: true,
    priority: 2,
    weight: 80,
    timeout_ms: 30000,
    max_retries: 3,
    consecutive_failures: 1,
    last_health_status: 'degraded',
    last_latency_ms: 420,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
];

const MOCK_OVERVIEW: SystemOverview = {
  pid: 4321,
  os_arch: 'linux/amd64',
  uptime_seconds: 86400 + 3600 * 2, // 1d 2h 0m
  go_version: 'go1.22.4',
  in_flight_requests: 3,
  process_rss_bytes: 45 * 1024 * 1024,
  host_ram_total_bytes: 16 * 1024 * 1024 * 1024,
  host_ram_used_bytes: 4 * 1024 * 1024 * 1024,
  container_ram_bytes: 60 * 1024 * 1024,
  go_heap_bytes: 28 * 1024 * 1024,
  go_sys_bytes: 50 * 1024 * 1024,
  stack_inuse_bytes: 2 * 1024 * 1024,
  num_gc: 18,
  net_total_bytes: 250 * 1024 * 1024,
  net_recv_bytes: 100 * 1024 * 1024,
  net_sent_bytes: 150 * 1024 * 1024,
  net_rate_mb_s: 2.5,
  net_recv_rate_mb_s: 1.0,
  net_sent_rate_mb_s: 1.5,
  egress_total_routes: 2,
  egress_http_count: 2,
  egress_active_mode: 'DIRECT',
  container_cpu_cap: 4,
  proxy_cpu_pct: 3.2,
  host_cpu_pct: 12.5,
  num_goroutine: 42,
};

const MOCK_REQUESTS: RequestLog[] = [
  {
    id: 'req-1',
    request_id: 'req-uuid-1',
    status_code: 200,
    requested_model: 'gpt-4o',
    model_id: 'gpt-4o',
    provider_name: 'openai-main',
    duration_ms: 180,
    total_tokens: 350,
    prompt_tokens: 150,
    completion_tokens: 200,
    cost_usd: '0.0007',
    client_ip: '127.0.0.1',
    is_stream: true,
    created_at: new Date(Date.now() - 10000).toISOString(),
  },
  {
    id: 'req-2',
    request_id: 'req-uuid-2',
    status_code: 500,
    requested_model: 'claude-3-5-sonnet',
    model_id: 'claude-3-5-sonnet',
    provider_name: 'anthropic-backup',
    duration_ms: 850,
    total_tokens: 120,
    prompt_tokens: 100,
    completion_tokens: 20,
    cost_usd: '0.0003',
    client_ip: '127.0.0.1',
    is_stream: false,
    created_at: new Date(Date.now() - 60000).toISOString(),
  },
];

const mockSummaryFn = vi.fn(async (_window?: string) => MOCK_SUMMARY);
const mockSeriesFn = vi.fn(async (_metric?: string, _window?: string) => ({ points: MOCK_SERIES }));
const mockProvidersFn = vi.fn(async () => ({ items: MOCK_PROVIDERS }));
const mockOverviewFn = vi.fn(async () => MOCK_OVERVIEW);
const mockRequestsFn = vi.fn(async (_opts?: unknown) => ({ items: MOCK_REQUESTS }));

vi.mock('../api/client', () => ({
  api: {
    observability: {
      summary: (window: string) => mockSummaryFn(window),
      series: (metric: string, window: string) => mockSeriesFn(metric, window),
    },
    providers: {
      list: () => mockProvidersFn(),
    },
    system: {
      overview: () => mockOverviewFn(),
    },
    requests: {
      list: (opts: unknown) => mockRequestsFn(opts),
    },
  },
}));

describe('Dashboard Page — Modularity & Integration Spec', () => {
  const mockNavigate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('merender seluruh segmen halaman Dashboard secara lengkap', async () => {
    render(<Dashboard onNavigate={mockNavigate} />);

    // 1. DashboardHero
    await waitFor(() => {
      expect(screen.getByText('Route-X Gateway Control')).toBeInTheDocument();
    });
    expect(screen.getByText(/Gateway Siap & Beroperasi/i)).toBeInTheDocument();
    expect(screen.getByText(/1\/2 Upstream Sehat/i)).toBeInTheDocument();
    expect(screen.getAllByText('go1.22.4').length).toBeGreaterThan(0);
    expect(screen.getByText('linux/amd64')).toBeInTheDocument();

    // 2. Onboarding QuickStartGuide
    expect(screen.getByText('Panduan Cepat Memulai Route-X')).toBeInTheDocument();
    expect(screen.getByText('Tambah Penyedia AI Upstream')).toBeInTheDocument();

    // 3. System Telemetry Section
    expect(screen.getByText('Telemetri Runtime Gateway')).toBeInTheDocument();
    expect(screen.getByText('Memori Gateway')).toBeInTheDocument();
    expect(screen.getByText('Throughput I/O')).toBeInTheDocument();
    expect(screen.getByText('Egress & Tunnel Pool')).toBeInTheDocument();
    expect(screen.getByText('Uptime & Komputasi')).toBeInTheDocument();

    // 4. Performance Metrics Ribbon
    expect(screen.getByText('Total Permintaan')).toBeInTheDocument();
    expect(screen.getByText('1,250')).toBeInTheDocument();
    expect(screen.getByText('Total Token')).toBeInTheDocument();
    expect(screen.getByText('450.0k')).toBeInTheDocument();
    expect(screen.getByText('Estimasi Biaya')).toBeInTheDocument();
    expect(screen.getByText('Latensi P95')).toBeInTheDocument();
    expect(screen.getByText('320 ms')).toBeInTheDocument();

    // 5. Traffic Chart & Live Requests Feed
    expect(screen.getByText('Volume & Dinamika Lalu Lintas')).toBeInTheDocument();
    expect(screen.getByText('Live Requests Feed')).toBeInTheDocument();
    expect(screen.getByText('gpt-4o')).toBeInTheDocument();
    expect(screen.getByText('claude-3-5-sonnet')).toBeInTheDocument();

    // 6. Upstream Provider Health Matrix
    expect(screen.getByText('Status Kesehatan Upstream Providers')).toBeInTheDocument();
    expect(screen.getByText('OpenAI Production')).toBeInTheDocument();
    expect(screen.getByText('Claude Anthropic')).toBeInTheDocument();
  });

  it('menangani tombol Segarkan (Refresh) dan memicu pembaruan data telemetri', async () => {
    render(<Dashboard onNavigate={mockNavigate} />);

    await waitFor(() => {
      expect(screen.getByText('Route-X Gateway Control')).toBeInTheDocument();
    });

    const refreshBtn = screen.getByRole('button', { name: /segarkan/i });
    fireEvent.click(refreshBtn);

    await waitFor(() => {
      expect(mockSummaryFn).toHaveBeenCalled();
      expect(mockToastSuccess).toHaveBeenCalledWith(
        'Metrik telemetri dashboard berhasil disegarkan'
      );
    });
  });

  it('mendukung pergantian metrik dan rentang waktu grafik lalu lintas', async () => {
    render(<Dashboard onNavigate={mockNavigate} />);

    await waitFor(() => {
      expect(screen.getByText('Volume & Dinamika Lalu Lintas')).toBeInTheDocument();
    });

    // Switch metric ke Token
    const tokenMetricBtn = screen.getByRole('button', { name: 'Jumlah token' });
    fireEvent.click(tokenMetricBtn);

    await waitFor(() => {
      expect(mockSeriesFn).toHaveBeenCalledWith('tokens', '24h');
    });

    // Switch time window ke 7d
    const window7dBtn = screen.getByRole('button', { name: 'Rentang waktu 7d' });
    fireEvent.click(window7dBtn);

    await waitFor(() => {
      expect(mockSeriesFn).toHaveBeenCalledWith('tokens', '7d');
    });
  });

  it('menampilkan modal resep 1-klik dan merespons aksi navigasi resep', async () => {
    render(<Dashboard onNavigate={mockNavigate} />);

    await waitFor(() => {
      expect(screen.getByText('Panduan Cepat Memulai Route-X')).toBeInTheDocument();
    });

    // Klik resep Coding Asisten
    const codingRecipeBtn = screen.getByRole('button', { name: /Coding Asisten/i });
    fireEvent.click(codingRecipeBtn);

    // Modal terbuka
    expect(
      screen.getByText(/Resep: Hubungkan Coding Assistant/i)
    ).toBeInTheDocument();
    expect(screen.getByText(/Dual-Protocol/i)).toBeInTheDocument();

    // Klik tombol aksi di modal
    const actionBtn = screen.getByRole('button', {
      name: /Buka Pengaturan CLI Lengkap/i,
    });
    fireEvent.click(actionBtn);

    expect(mockNavigate).toHaveBeenCalledWith('/cli-integrations');
  });

  it('dapat menutup panduan onboarding cepat secara permanen', async () => {
    render(<Dashboard onNavigate={mockNavigate} />);

    await waitFor(() => {
      expect(screen.getByText('Panduan Cepat Memulai Route-X')).toBeInTheDocument();
    });

    const closeBtn = screen.getByRole('button', { name: /tutup panduan/i });
    fireEvent.click(closeBtn);

    expect(screen.queryByText('Panduan Cepat Memulai Route-X')).not.toBeInTheDocument();
    expect(localStorage.getItem('routex_dismiss_onboarding')).toBe('true');
  });

  it('menangani kegagalan pemanggilan API dengan notifikasi error', async () => {
    mockSummaryFn.mockRejectedValueOnce(new Error('Network timeout gateway'));

    render(<Dashboard onNavigate={mockNavigate} />);

    await waitFor(() => {
      expect(screen.getByText(/Gateway Tidak Tersedia/i)).toBeInTheDocument();
    });
  });
});
