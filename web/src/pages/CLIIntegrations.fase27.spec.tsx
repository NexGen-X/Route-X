import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { CLIIntegrations } from './CLIIntegrations';
import { api } from '../api/client';
import type { CLIDetectedResponse, Model, APIKey } from '../types';

// Spec pengunci B-592: cabang `catch` handleTestConnection (dan cabang
// batal tanpa model) TIDAK BOLEH mengarang latensi `0ms` — tidak ada
// pengukuran yang terjadi di sana, jadi chip `ms` tidak boleh tampil.
// Merender halaman sungguhan + fetch global nyata yang di-stub, seperti
// spec B-227/B-525 yang sudah ada.

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
  total_available: 1,
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
      exportScript: vi.fn(async () => ({
        content: '#!/usr/bin/env bash\nexport ROUTEX_GATEWAY="http://localhost:8080/v1"',
      })),
    },
    models: {
      list: vi.fn(async () => ({ items: MOCK_MODELS })),
      listAll: vi.fn(async () => ({ items: MOCK_MODELS })),
    },
    routing: {
      list: vi.fn(async () => ({ items: [] })),
      listAll: vi.fn(async () => ({ items: [] })),
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

async function renderHalaman() {
  render(<CLIIntegrations />);
  await waitFor(() => {
    expect(screen.getByText('Claude Code')).toBeInTheDocument();
  });
}

function klikUji() {
  fireEvent.click(screen.getByTitle(/Uji sambungan inferensi nyata/i));
}

describe('CLIIntegrations — chip latensi uji koneksi gagal (B-592)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('fetch GAGAL (galat jaringan) → pesan gagal tampil TANPA chip `0ms`', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('jaringan putus');
    }));
    try {
      await renderHalaman();
      klikUji();
      expect(await screen.findByText(/jaringan putus/)).toBeInTheDocument();
      expect(screen.queryByText('0ms')).not.toBeInTheDocument();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('fetch ABORT (timeout 20 detik) → pesan timeout tampil TANPA chip `0ms`', async () => {
    const abortError = new Error('The operation was aborted.');
    abortError.name = 'AbortError';
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw abortError;
    }));
    try {
      await renderHalaman();
      klikUji();
      expect(
        await screen.findByText(/melewati batas waktu 20 detik/),
      ).toBeInTheDocument();
      expect(screen.queryByText('0ms')).not.toBeInTheDocument();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('uji DIBATALKAN (tanpa model) → pesan batal tampil TANPA chip `0ms`', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('fetch tidak boleh dipanggil pada jalur batal');
    }));
    try {
      vi.mocked(api.cli.detected).mockResolvedValueOnce({
        ...MOCK_CLI_DATA,
        tools: [{ ...MOCK_CLI_DATA.tools[0], active_target: '' }],
      });
      vi.mocked(api.models.listAll).mockResolvedValueOnce({ items: [] });
      await renderHalaman();
      klikUji();
      expect(
        await screen.findByText(/belum ada model terdaftar di katalog untuk diuji/i),
      ).toBeInTheDocument();
      expect(screen.queryByText('0ms')).not.toBeInTheDocument();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('kontrol: fetch SUKSES → chip latensi terukur tampil', async () => {
    vi.stubGlobal('fetch', vi.fn(
      async () =>
        new Response(JSON.stringify({ id: 'x' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
    ));
    try {
      await renderHalaman();
      klikUji();
      expect(await screen.findByText(/200 OK • Terhubung/)).toBeInTheDocument();
      expect(screen.getByText(/^\d+ms$/)).toBeInTheDocument();
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
