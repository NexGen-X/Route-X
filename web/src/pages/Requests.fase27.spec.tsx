import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Requests } from './Requests';
import type { RequestLog, RequestPayload } from '../types';

// Spec pengunci B-593 (level halaman): wiring `eventsGagal` di
// `handleInspect` — events gagal → timeline modal menampilkan penanda
// belum-terukur, bukan klaim kosong; bila HANYA payload gagal, klaim
// kosong tetap tampil karena events kosong secara sah.

vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('../utils/clipboard', () => ({
  copyTextToClipboard: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({
    toast: {
      success: vi.fn(),
      error: vi.fn(),
      warn: vi.fn(),
      info: vi.fn(),
    },
  }),
}));

const MOCK_REQUEST: RequestLog = {
  id: 'req-1',
  request_id: 'req-uuid-1',
  provider_id: 'openai-main',
  provider_name: 'OpenAI Production',
  model_id: 'gpt-4o',
  requested_model: 'gpt-4o',
  status_code: 200,
  duration_ms: 180,
  prompt_tokens: 150,
  completion_tokens: 200,
  total_tokens: 350,
  cost_usd: '0.000700',
  client_ip: '192.168.1.10',
  is_stream: false,
  created_at: '2026-09-25T10:00:00Z',
};

const MOCK_PAYLOAD: RequestPayload = {
  request_body: { messages: [{ role: 'user', content: 'ping' }] },
  response_body: { choices: [{ message: { role: 'assistant', content: 'pong' } }] },
  truncated: false,
  size_bytes: 128,
};

const mockListFn = vi.fn();
const mockEventsFn = vi.fn();
const mockPayloadFn = vi.fn();
const mockProvidersFn = vi.fn();

vi.mock('../api/client', () => ({
  ApiError: class ApiError extends Error {
    status: number;
    code: string;
    param?: string;
    constructor(status: number, code: string, message: string, param?: string) {
      super(message);
      this.name = 'ApiError';
      this.status = status;
      this.code = code;
      this.param = param;
    }
  },
  api: {
    requests: {
      list: (params: Record<string, string | number>) => mockListFn(params),
      get: vi.fn(),
      events: (id: string, createdAt?: string) => mockEventsFn(id, createdAt),
      payload: (id: string, createdAt?: string) => mockPayloadFn(id, createdAt),
    },
    providers: {
      list: (params?: Record<string, unknown>) => mockProvidersFn(params),
    },
  },
}));

async function bukaInspector() {
  render(<Requests />);
  const row = await screen.findByTestId('request-row-req-uuid-1');
  fireEvent.click(row);
  await waitFor(() => {
    expect(screen.getByText('Detail Request & Timeline Event')).toBeInTheDocument();
  });
}

describe('Requests — penanda gagal events di timeline inspector (B-593)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListFn.mockResolvedValue({ items: [MOCK_REQUEST], next_cursor: undefined });
    mockPayloadFn.mockResolvedValue(MOCK_PAYLOAD);
    mockProvidersFn.mockResolvedValue({
      items: [{ id: 'openai-main', name: 'OpenAI Production' }],
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('events GAGAL + payload OK → penanda belum-terukur tampil, klaim kosong TIDAK tampil', async () => {
    mockEventsFn.mockRejectedValue(new Error('boom'));
    await bukaInspector();

    expect(
      await screen.findByText('Riwayat event belum dapat dimuat.'),
    ).toBeInTheDocument();
    expect(screen.queryByText('Tidak ada rekaman event sekunder.')).not.toBeInTheDocument();
  });

  it('HANYA payload gagal (500) + events kosong sah → klaim kosong tetap tampil', async () => {
    mockEventsFn.mockResolvedValue({ events: [] });
    mockPayloadFn.mockRejectedValue(new Error('payload putus'));
    await bukaInspector();

    expect(
      await screen.findByText('Tidak ada rekaman event sekunder.'),
    ).toBeInTheDocument();
    expect(screen.queryByText('Riwayat event belum dapat dimuat.')).not.toBeInTheDocument();
  });
});
