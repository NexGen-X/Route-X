import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { CLIIntegrations } from './CLIIntegrations';
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
      id: 'agy',
      name: 'Antigravity CLI',
      category: 'Coding Agent',
      installed: true,
      path: '/root/.gemini/antigravity-cli/bin/agy',
      version: '2.0.0',
      description: 'Protected agent pair programming CLI.',
      supported_modes: ['model_only'],
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
    },
    routing: {
      list: vi.fn(async () => ({ items: MOCK_RULES })),
    },
    apiKeys: {
      list: vi.fn(async () => ({ items: MOCK_KEYS })),
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

    // Menampilkan tab CLI Terpasang dengan count 2 (claude + agy)
    expect(screen.getByRole('button', { name: /CLI Terpasang/i })).toBeInTheDocument();
    expect(screen.getByText('Claude Code')).toBeInTheDocument();
    expect(screen.getByText('Antigravity CLI')).toBeInTheDocument();
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

  it('memastikan Antigravity CLI berstatus terkunci sistem (read-only)', async () => {
    render(<CLIIntegrations />);

    await waitFor(() => {
      expect(screen.getByText('Antigravity CLI')).toBeInTheDocument();
    });

    expect(screen.getByText('Dilindungi Sistem')).toBeInTheDocument();
    expect(screen.getByText(/Antigravity CLI \(Read-Only\)/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Terkunci \(Sistem\)/i })).toBeDisabled();
  });
});
