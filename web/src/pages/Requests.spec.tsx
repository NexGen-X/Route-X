import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Requests } from './Requests';
import type { RequestLog, RequestEvent, RequestPayload } from '../types';
import { copyTextToClipboard } from '../utils/clipboard';

// Mock focus-trap-react untuk jsdom
vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

// Mock clipboard
vi.mock('../utils/clipboard', () => ({
  copyTextToClipboard: vi.fn().mockResolvedValue(undefined),
}));

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

const MOCK_REQUEST_SUCCESS: RequestLog = {
  id: 'req-1',
  request_id: 'req-uuid-1',
  provider_id: 'openai-main',
  provider_name: 'OpenAI Production',
  model_id: 'gpt-4o',
  requested_model: 'gpt-4o',
  status_code: 200,
  duration_ms: 180,
  ttft_ms: 95,
  prompt_tokens: 150,
  completion_tokens: 200,
  total_tokens: 350,
  cost_usd: '0.000700',
  client_ip: '192.168.1.10',
  is_stream: true,
  created_at: '2026-09-25T10:00:00Z',
};

const MOCK_REQUEST_ERROR: RequestLog = {
  id: 'req-2',
  request_id: 'req-uuid-2',
  provider_id: 'anthropic-backup',
  provider_name: 'Anthropic Secondary',
  model_id: 'claude-3-5-sonnet',
  requested_model: 'claude-3-5-sonnet',
  status_code: 500,
  duration_ms: 850,
  prompt_tokens: 100,
  completion_tokens: 20,
  total_tokens: 120,
  cost_usd: '0.000300',
  client_ip: '192.168.1.20',
  is_stream: false,
  error_message: 'Provider upstream 500 internal server error',
  created_at: '2026-09-25T10:05:00Z',
};

const MOCK_EVENTS: RequestEvent[] = [
  {
    seq: 1,
    kind: 'upstream_attempt',
    provider_id: 'openai-main',
    status_code: 200,
    latency_ms: 120,
    created_at: '2026-09-25T10:00:01Z',
  },
  {
    seq: 2,
    kind: 'stream_chunk',
    provider_id: 'openai-main',
    latency_ms: 60,
    created_at: '2026-09-25T10:00:02Z',
  },
];

const MOCK_PAYLOAD: RequestPayload = {
  request_body: {
    messages: [{ role: 'user', content: 'What is Route-X architecture?' }],
  },
  response_body: {
    choices: [
      {
        message: {
          role: 'assistant',
          content: 'Route-X is a high-performance AI Gateway in Go.',
        },
      },
    ],
  },
  truncated: false,
  size_bytes: 512,
};

const mockListFn = vi.fn();
const mockEventsFn = vi.fn();
const mockPayloadFn = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    requests: {
      list: (params: Record<string, string | number>) => mockListFn(params),
      get: vi.fn(),
      events: (id: string, createdAt?: string) => mockEventsFn(id, createdAt),
      payload: (id: string, createdAt?: string) => mockPayloadFn(id, createdAt),
    },
  },
}));

describe('Requests Page — Modular Architecture & Integration Spec', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListFn.mockResolvedValue({
      items: [MOCK_REQUEST_SUCCESS, MOCK_REQUEST_ERROR],
      next_cursor: undefined,
    });
    mockEventsFn.mockResolvedValue({ events: MOCK_EVENTS });
    mockPayloadFn.mockResolvedValue(MOCK_PAYLOAD);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('merender daftar request dengan mock data secara lengkap', async () => {
    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalled();
    });

    // Cek header dan filter tabs
    expect(screen.getByText('Requests Inspector')).toBeInTheDocument();
    expect(screen.getByText('All Requests')).toBeInTheDocument();
    expect(screen.getByText('Success (2xx)')).toBeInTheDocument();
    expect(screen.getByText('Errors (4xx/5xx)')).toBeInTheDocument();

    // Verifikasi baris request tampil di desktop & mobile
    expect(screen.getAllByText('req-uuid-1').length).toBeGreaterThan(0);
    expect(screen.getAllByText('req-uuid-2').length).toBeGreaterThan(0);
    expect(screen.getAllByText('gpt-4o').length).toBeGreaterThan(0);
    expect(screen.getAllByText('claude-3-5-sonnet').length).toBeGreaterThan(0);
  });

  it('menampilkan empty state ketika daftar request kosong', async () => {
    mockListFn.mockResolvedValueOnce({ items: [] });
    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalled();
    });

    expect(screen.getByText('Tidak ada catatan permintaan')).toBeInTheDocument();
    expect(
      screen.getByText(/Belum ada permintaan inferensi yang cocok/i)
    ).toBeInTheDocument();
  });

  it('menangani filter tab (Success, Errors, All) dan pencarian teks', async () => {
    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalledWith({ limit: 25 });
    });

    // 1. Klik tab "Success (2xx)"
    fireEvent.click(screen.getByText('Success (2xx)'));
    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalledWith({
        limit: 25,
        status_class: '2xx',
      });
    });

    // 2. Klik tab "Errors (4xx/5xx)"
    fireEvent.click(screen.getByText('Errors (4xx/5xx)'));
    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalledWith({
        limit: 25,
        status_class: 'error',
      });
    });

    // 3. Pencarian teks dengan tombol Enter
    const searchInput = screen.getByPlaceholderText('Cari ID request, model, IP...');
    fireEvent.change(searchInput, { target: { value: 'gpt-4o' } });
    fireEvent.keyDown(searchInput, { key: 'Enter', code: 'Enter' });

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalledWith(
        expect.objectContaining({
          search: 'gpt-4o',
        })
      );
    });
  });

  it('membuka RequestInspectorModal saat baris request diklik dan menampilkan metrics & event timeline', async () => {
    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalled();
    });

    // Klik baris request pertama
    const row = screen.getByTestId('request-row-req-uuid-1');
    fireEvent.click(row);

    // Modal terbuka
    await waitFor(() => {
      expect(screen.getByText('Detail Request & Timeline Event')).toBeInTheDocument();
    });

    // API events dan payload dipanggil
    expect(mockEventsFn).toHaveBeenCalledWith('req-1', MOCK_REQUEST_SUCCESS.created_at);
    expect(mockPayloadFn).toHaveBeenCalledWith('req-1', MOCK_REQUEST_SUCCESS.created_at);

    // Verifikasi Quick Metrics dalam modal
    expect(screen.getByText('Total Durasi')).toBeInTheDocument();
    expect(screen.getByText('Waterfall Latensi Inferensi')).toBeInTheDocument();
    expect(screen.getByText('upstream_attempt')).toBeInTheDocument();
    expect(screen.getByText('stream_chunk')).toBeInTheDocument();
  });

  it('menangani aksi salin perintah cURL dan menampilkan isi JSON payload', async () => {
    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalled();
    });

    // Buka modal untuk request 1
    fireEvent.click(screen.getByTestId('request-row-req-uuid-1'));

    await waitFor(() => {
      expect(screen.getByText('Detail Request & Timeline Event')).toBeInTheDocument();
    });

    // Verifikasi isi payload ditampilkan
    expect(
      screen.getByText(/What is Route-X architecture\?/)
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Route-X is a high-performance AI Gateway in Go\./)
    ).toBeInTheDocument();

    // Klik tombol Salin cURL
    const copyButton = screen.getByRole('button', { name: /Salin.*cURL/i });
    fireEvent.click(copyButton);

    await waitFor(() => {
      expect(copyTextToClipboard).toHaveBeenCalled();
    });

    const copiedArg = (copyTextToClipboard as unknown as { mock: { calls: [string][] } })
      .mock.calls[0][0];
    expect(copiedArg).toContain('curl -X POST');
    expect(copiedArg).toContain('What is Route-X architecture?');
    expect(copiedArg).toContain('gpt-4o');

    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Perintah cURL disalin ke clipboard.'
    );
  });

  it('mendukung toggle tombol Live Feed Polling', async () => {
    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalled();
    });

    const liveBtn = screen.getByRole('button', { name: /Live Stream/i });
    expect(liveBtn).toBeInTheDocument();

    // Toggle On
    fireEvent.click(liveBtn);
    expect(screen.getByText('Live Polling Aktif')).toBeInTheDocument();

    // Toggle Off
    fireEvent.click(screen.getByText('Live Polling Aktif'));
    expect(screen.getByText('Live Stream')).toBeInTheDocument();
  });

  it('menangani pagination Muat Lebih Banyak jika next_cursor tersedia', async () => {
    mockListFn.mockResolvedValueOnce({
      items: [MOCK_REQUEST_SUCCESS],
      next_cursor: 'cursor-page-2',
    });

    render(<Requests />);

    await waitFor(() => {
      expect(screen.getByText('Muat Lebih Banyak...')).toBeInTheDocument();
    });

    mockListFn.mockResolvedValueOnce({
      items: [MOCK_REQUEST_ERROR],
      next_cursor: undefined,
    });

    fireEvent.click(screen.getByText('Muat Lebih Banyak...'));

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalledWith({
        limit: 25,
        cursor: 'cursor-page-2',
      });
    });
  });

  it('dapat membuka inspector modal melalui kartu mobile dan mendukung keyboard navigation', async () => {
    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalled();
    });

    const mobileCard = screen.getByTestId('request-card-mobile-req-uuid-2');
    fireEvent.keyDown(mobileCard, { key: 'Enter', code: 'Enter' });

    await waitFor(() => {
      expect(screen.getByText('Detail Request & Timeline Event')).toBeInTheDocument();
    });

    // Request 2 memiliki pesan kesalahan
    expect(screen.getByText('Pesan Kesalahan:')).toBeInTheDocument();
    expect(
      screen.getByText('Provider upstream 500 internal server error')
    ).toBeInTheDocument();
  });

  it('menampilkan QueryError dan mendukung tombol Coba Lagi saat loadRequests gagal', async () => {
    mockListFn.mockRejectedValueOnce(new Error('Gagal menghubungi gateway'));

    render(<Requests />);

    await waitFor(() => {
      expect(screen.getByText('Gagal menghubungi gateway')).toBeInTheDocument();
    });

    mockListFn.mockResolvedValueOnce({
      items: [MOCK_REQUEST_SUCCESS],
    });

    const retryBtn = screen.getByRole('button', { name: /Coba Lagi/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByText('req-uuid-1')).toBeInTheDocument();
    });
  });

  it('menangani kegagalan fungsi copyTextToClipboard dengan notifikasi toast error', async () => {
    (copyTextToClipboard as unknown as { mockRejectedValueOnce: (err: Error) => void }).mockRejectedValueOnce(
      new Error('Clipboard ditolak oleh izin sistem')
    );

    render(<Requests />);

    await waitFor(() => {
      expect(mockListFn).toHaveBeenCalled();
    });

    fireEvent.click(screen.getByTestId('request-row-req-uuid-1'));

    await waitFor(() => {
      expect(screen.getByText('Detail Request & Timeline Event')).toBeInTheDocument();
    });

    const copyBtn = screen.getByRole('button', { name: /Salin.*cURL/i });
    fireEvent.click(copyBtn);

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        expect.stringContaining('Gagal menyalin perintah cURL: Clipboard ditolak oleh izin sistem')
      );
    });
  });
});
