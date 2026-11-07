import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { CLIIntegrations } from './CLIIntegrations';
import { api } from '../api/client';
import type { CLIDetectedResponse, Model, RoutingRule, APIKey } from '../types';

// Fase 21 PR-E:
//   B-512 — CLIToolCard tidak mengklaim katalog kosong secara
//           definitif saat sumber pendamping GAGAL dimuat.
//   B-513 — pilihan palet CLI dikonsumsi juga bila halaman sudah
//           terpasang (event 'routex:cli-tujuan' / fokus jendela),
//           bukan hanya saat mount.

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
    },
    {
      id: 'opencode',
      name: 'OpenCode Interpreter',
      category: 'Terminal Assistant',
      installed: false,
      description: 'Open source terminal coder.',
      supported_modes: ['model_only', 'routing', 'combo'],
      active_mode: 'model_only',
      active_target: 'gpt-4o',
      env_vars: {},
      export_snippet: '',
    },
  ],
};

vi.mock('../api/client', () => ({
  api: {
    cli: {
      detected: vi.fn(async () => MOCK_CLI_DATA),
      configure: vi.fn(async () => ({ success: true, tool: MOCK_CLI_DATA.tools[0] })),
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

describe('CLIIntegrations — gerbang galat kartu & konsumsi palet (Fase 21 PR-E)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();
    // clearAllMocks membuang implementasi bawaan mock: pasang ulang.
    vi.mocked(api.models.listAll).mockImplementation(async () => ({ items: MOCK_MODELS }) as never);
    vi.mocked(api.routing.listAll).mockImplementation(async () => ({ items: MOCK_RULES }) as never);
  });

  it('B-512: model+routing gagal → banner tampil dan kartu TIDAK mengklaim katalog kosong', async () => {
    vi.mocked(api.models.listAll).mockImplementation(async () => {
      throw new Error('jaringan putus');
    });
    vi.mocked(api.routing.listAll).mockImplementation(async () => {
      throw new Error('jaringan putus');
    });
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText(/Sebagian data gagal dimuat/)).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: 'Buka konfigurasi target' }));

    await waitFor(() => {
      expect(screen.getByText(/Daftar model gagal dimuat/)).toBeInTheDocument();
    });
    expect(screen.queryByText('Belum ada model terdaftar di katalog.')).not.toBeInTheDocument();
  });

  it('B-513: pilihan palet diterapkan pada halaman yang SUDAH terpasang (event)', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Claude Code')).toBeInTheDocument();
    });
    // Tab bawaan 'installed': OpenCode belum terpasang → tidak tampil.
    expect(screen.queryByText('OpenCode Interpreter')).not.toBeInTheDocument();

    // Persis aksi App saat palet memilih tool: tulis storage lalu
    // tembakkan event — TANPA me-mount ulang halaman.
    act(() => {
      sessionStorage.setItem('routex_cli_tujuan', 'opencode');
      window.dispatchEvent(new Event('routex:cli-tujuan'));
    });

    await waitFor(() => {
      expect(screen.getByText('OpenCode Interpreter')).toBeInTheDocument();
    });
    const input = screen.getByPlaceholderText(/Cari \(misal: claude, opencode\)/) as HTMLInputElement;
    expect(input.value).toBe('opencode');
    expect(sessionStorage.getItem('routex_cli_tujuan')).toBeNull();
  });

  it('B-513: pilihan palet diterapkan juga lewat fokus jendela', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Claude Code')).toBeInTheDocument();
    });

    act(() => {
      sessionStorage.setItem('routex_cli_tujuan', 'claude');
      window.dispatchEvent(new Event('focus'));
    });

    const input = screen.getByPlaceholderText(/Cari \(misal: claude, opencode\)/) as HTMLInputElement;
    await waitFor(() => {
      expect(input.value).toBe('claude');
    });
    expect(sessionStorage.getItem('routex_cli_tujuan')).toBeNull();
  });
});
