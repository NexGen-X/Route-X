import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { CLIIntegrations } from './CLIIntegrations';
import { api } from '../api/client';
import type { CLIDetectedResponse, Model, RoutingRule, APIKey } from '../types';

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
  total_detected: 2,
  total_available: 3,
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
      env_var_api_key: 'ANTHROPIC_API_KEY',
      env_vars: {
        ANTHROPIC_BASE_URL: 'http://localhost:8080',
      },
      export_snippet: "export ANTHROPIC_BASE_URL='http://localhost:8080'",
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
    {
      // Fixture fiktif (bukan entri registri): mewakili tool apa pun
      // yang suatu saat backend tandai tidak dapat diarahkan ke
      // gateway, supaya jalur tampilan jujur UI tetap teruji walau
      // registri kini hanya memuat tool yang didukung.
      id: 'contoh-non-gateway',
      name: 'Contoh Non-Gateway',
      category: 'Coding Agent',
      installed: true,
      path: '/usr/local/bin/contoh-non-gateway',
      version: '2.0.0',
      description: 'Fixture tool yang tidak dapat diarahkan ke gateway.',
      supported_modes: ['model_only', 'routing', 'combo'],
      active_mode: 'model_only',
      active_target: 'gpt-4o',
      env_vars: {},
      export_snippet: '',
      gateway_supported: false,
      unsupported_reason:
        'Tool contoh ini terikat akun vendor dan tidak membaca endpoint atau kunci OpenAI apa pun, sehingga tidak dapat diarahkan ke gateway Route-X.',
    },
  ],
};

vi.mock('../api/client', () => ({
  api: {
    cli: {
      detected: vi.fn(async () => MOCK_CLI_DATA),
      configure: vi.fn(async (payload) => ({
        success: true,
        tool: {
          ...MOCK_CLI_DATA.tools[0],
          active_mode: payload.mode,
          active_target: payload.target,
        },
      })),
      exportScript: vi.fn(async () => ({
        content: '#!/usr/bin/env bash\nexport ROUTEX_GATEWAY="http://localhost:8080/v1"',
      })),
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

describe('CLIIntegrations — Pengujian Unit Dekomposisi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('merender banner status dan perkakas terpasang secara default', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Integrasi CLI & Editor AI')).toBeInTheDocument();
    });

    // Menampilkan tab CLI Terpasang dengan count 2 (claude + fixture non-gateway)
    expect(screen.getByRole('button', { name: /CLI Terpasang/i })).toBeInTheDocument();
    expect(screen.getByText('Claude Code')).toBeInTheDocument();
    expect(screen.getByText('Contoh Non-Gateway')).toBeInTheDocument();
    // OpenCode (belum ada) tidak muncul di tab default 'installed'
    expect(screen.queryByText('OpenCode Interpreter')).not.toBeInTheDocument();
  });

  it('beralih ke katalog lengkap untuk melihat perkakas yang belum terpasang', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Claude Code')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: /Katalog Lengkap/i }));

    await waitFor(() => {
      expect(screen.getByText('OpenCode Interpreter')).toBeInTheDocument();
    });
    expect(screen.getByText('Belum Ada')).toBeInTheDocument();
  });

  it('menangani tombol kunci API terdaftar untuk autofill session', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Dev Key')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: /Dev Key/i }));

    // Input kunci API terisi dengan raw key
    const inputKey = screen.getByPlaceholderText('sk_live_...');
    expect(inputKey).toHaveValue('sk_live_abc123456789');
  });

  it('menampilkan label gateway yang jujur untuk tool bergateway_supported=false dari backend', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Contoh Non-Gateway')).toBeInTheDocument();
    });

    // Tidak ada penanda kunci sistem khusus untuk tool mana pun.
    expect(screen.queryByText('Dilindungi Sistem')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Terkunci \(Sistem\)/i })).not.toBeInTheDocument();

    // Kartu tool non-gateway menampilkan status + alasan jujur dari
    // backend dan TIDAK menawarkan Configure.
    expect(screen.getByText('Tidak Dapat Diarahkan ke Gateway')).toBeInTheDocument();
    expect(screen.getByText(/terikat akun vendor/i)).toBeInTheDocument();

    // Hanya tool yang didukung gateway (claude) yang memiliki tombol
    // Terapkan; mengkliknya tetap memanggil API configure seperti biasa.
    const tombolTerapkan = screen.getAllByRole('button', { name: /Terapkan ke CLI/i });
    expect(tombolTerapkan).toHaveLength(1);
    expect(tombolTerapkan[0]).not.toBeDisabled();
    fireEvent.click(tombolTerapkan[0]);
    await waitFor(() => {
      expect(vi.mocked(api.cli.configure)).toHaveBeenCalledWith(
        expect.objectContaining({ tool_id: 'claude' }),
      );
    });
    expect(vi.mocked(api.cli.configure)).not.toHaveBeenCalledWith(
      expect.objectContaining({ tool_id: 'contoh-non-gateway' }),
    );
  });
});

describe('CLIIntegrations — kejujuran muatan parsial (B-225)', () => {
  it('daftar model gagal dimuat → peringatan parsial menyebut sumbernya (bukan daftar kosong diam-diam)', async () => {
    vi.mocked(api.models.listAll).mockRejectedValueOnce(new Error('daftar model tidak terjangkau'));
    render(<CLIIntegrations />);

    expect(await screen.findByText(/Sebagian data gagal dimuat: model/)).toBeInTheDocument();
  });

  it('semua daftar sukses → tidak ada peringatan parsial', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Claude Code')).toBeInTheDocument();
    });
    expect(screen.queryByText(/Sebagian data gagal dimuat/)).not.toBeInTheDocument();
  });
});
