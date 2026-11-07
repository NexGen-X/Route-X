import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { CLIIntegrations } from './CLIIntegrations';
import { api } from '../api/client';
import type { CLIDetectedResponse, Model, RoutingRule, APIKey } from '../types';

// Fase 24 PR-D — kesegaran PER SUMBER di CLIIntegrations:
//
// B-545: hasil sumber pendamping (model/aturan/kunci) wajib
// dicatat/dibersihkan dari muatannya sendiri, independen dari
// nasib sumber utama cli.detected. Versi lama memprosesnya hanya
// di cabang cli berhasil — kegagalan serentak membuang hasil
// pendamping (banner gagal tidak muncul), dan keberhasilan
// pendamping di tengah kegagalan cli tidak membersihkan penanda
// basi (klaim gerbang B-512/B-525 membaca keadaan basi).
//
// B-553: pindaian ulang verifikasi pada handleApply adalah
// muatan segar sumber utama yang BERHASIL — banner basi sumber
// itu dibersihkan di sana, bukan menunggu Pindai Ulang penuh.

const MOCK_MODELS: Model[] = [
  {
    id: 'm-gpt4o',
    model_id: 'gpt-4o',
    display_name: 'GPT-4o',
    capabilities: [],
    enabled: true,
    routing_priority: 0,
    created_at: '',
    updated_at: '',
  },
];

const MOCK_RULES: RoutingRule[] = [
  {
    id: 'r-failover',
    name: 'smart-failover',
    priority: 100,
    match_capabilities: [],
    strategy: 'priority',
    max_attempts: 3,
    backoff_ms: 200,
    failure_threshold: 5,
    open_duration_ms: 30000,
    half_open_probes: 2,
    enabled: true,
    created_at: '',
    updated_at: '',
    virtual_alias: 'smart-combo',
  },
];

const MOCK_KEYS: APIKey[] = [
  {
    id: 'key-1',
    name: 'Dev Key',
    masked_key: 'sk_live_...1234',
    raw_key: 'sk_live_abc123456789',
    scopes: ['read', 'write'],
    allowed_models: [],
    allowed_providers: [],
    rate_limit_rps: 10,
    enabled: true,
    created_at: '',
    updated_at: '',
  },
];

const MOCK_CLI_DATA: CLIDetectedResponse = {
  total_detected: 1,
  total_available: 2,
  gateway_url: 'http://localhost:8080',
  default_env_file: '~/.routex/cli-env.sh',
  tools: [
    {
      id: 'claude',
      name: 'Claude Code',
      category: 'Coding Agent',
      installed: true,
      path: '/usr/local/bin/claude',
      version: '1.0.0',
      description: 'Official Anthropic CLI agent.',
      config_path: '~/.claude.json',
      supported_modes: ['model_only', 'routing', 'combo'],
      active_mode: 'model_only',
      active_target: 'gpt-4o',
      env_vars: {},
      export_snippet: '',
      updated_at: '2026-11-07T00:00:00Z',
    },
  ],
};

vi.mock('../api/client', () => ({
  api: {
    cli: {
      detected: vi.fn(async () => MOCK_CLI_DATA),
      configure: vi.fn(async (payload: { mode: string; target: string }) => ({
        success: true,
        tool: {
          ...MOCK_CLI_DATA.tools[0],
          active_mode: payload.mode,
          active_target: payload.target,
        },
      })),
      exportScript: vi.fn(async () => ({ content: '# skrip' })),
    },
    models: {
      list: vi.fn(async () => ({ items: MOCK_MODELS })),
      listAll: vi.fn(async () => ({ items: MOCK_MODELS })),
    },
    routing: {
      list: vi.fn(async () => ({ items: MOCK_RULES })),
      listAll: vi.fn(async () => ({ items: MOCK_RULES })),
    },
    apiKeys: {
      list: vi.fn(async () => ({ items: MOCK_KEYS })),
      listAll: vi.fn(async () => ({ items: MOCK_KEYS })),
    },
  },
}));

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() },
  }),
}));

describe('CLIIntegrations — kesegaran per sumber (Fase 24 PR-D)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('B-545: cli + model gagal SERENTAK saat Pindai Ulang → kegagalan model tetap tercatat di banner sumber gagal', async () => {
    render(<CLIIntegrations />);
    await waitFor(() => {
      expect(screen.getByText('Claude Code')).toBeInTheDocument();
    });
    expect(screen.queryByText(/Sebagian data gagal dimuat/)).not.toBeInTheDocument();

    vi.mocked(api.cli.detected).mockRejectedValueOnce(new Error('server mati'));
    vi.mocked(api.models.listAll).mockRejectedValueOnce(new Error('daftar model tidak terjangkau'));
    fireEvent.click(screen.getAllByRole('button', { name: /Pindai Ulang/i })[0]);

    // Banner basi sumber utama (B-534) DAN banner kegagalan
    // pendamping (B-225) sama-sama wajib tampil — versi lama
    // membuang hasil pendamping karena cli gagal lebih dulu.
    expect(
      await screen.findByText(/Pindai ulang gagal: server mati/),
    ).toBeInTheDocument();
    expect(
      await screen.findByText(/Sebagian data gagal dimuat: model/),
    ).toBeInTheDocument();
  });

  it('B-545 arah sebaliknya: model pulih saat cli gagal → penanda gagal model dibersihkan walau cli gagal', async () => {
    // Muat pertama: model gagal (cli berhasil) → penanda tercatat.
    vi.mocked(api.models.listAll).mockRejectedValueOnce(new Error('daftar model tidak terjangkau'));
    render(<CLIIntegrations />);
    expect(
      await screen.findByText(/Sebagian data gagal dimuat: model/),
    ).toBeInTheDocument();

    // Pindai ulang: cli GAGAL tetapi model BERHASIL — kesegaran
    // model wajib diperbarui dari muatannya sendiri; penanda
    // basi tidak boleh bertahan hanya karena sumber utama gagal.
    vi.mocked(api.cli.detected).mockRejectedValueOnce(new Error('server mati'));
    fireEvent.click(screen.getAllByRole('button', { name: /Pindai Ulang/i })[0]);

    expect(
      await screen.findByText(/Pindai ulang gagal: server mati/),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText(/Sebagian data gagal dimuat/)).not.toBeInTheDocument();
    });
  });

  it('B-553: Apply terverifikasi membersihkan banner basi sumber utama tanpa Pindai Ulang penuh', async () => {
    render(<CLIIntegrations />);
    await waitFor(() => {
      expect(screen.getByText('Claude Code')).toBeInTheDocument();
    });

    // Pindai Ulang gagal → banner basi tampil di atas data lama.
    vi.mocked(api.cli.detected).mockRejectedValueOnce(new Error('server mati'));
    fireEvent.click(screen.getAllByRole('button', { name: /Pindai Ulang/i })[0]);
    expect(
      await screen.findByText(/Pindai ulang gagal: server mati/),
    ).toBeInTheDocument();

    // Terapkan: configure berhasil dan pindaian verifikasi
    // (cli.detected) berhasil — itu muatan segar sumber utama,
    // jadi banner basinya wajib hilang.
    fireEvent.click(screen.getAllByRole('button', { name: /Terapkan ke CLI/i })[0]);
    await waitFor(() => {
      expect(vi.mocked(api.cli.configure)).toHaveBeenCalledWith(
        expect.objectContaining({ tool_id: 'claude' }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByText(/Pindai ulang gagal/)).not.toBeInTheDocument();
    });
    expect(
      await screen.findByText(/terverifikasi dari pindaian ulang/),
    ).toBeInTheDocument();
  });
});
