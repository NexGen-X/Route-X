import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Budgets } from './Budgets';
import type { Budget, RateLimit, APIKey, Model } from '../types';

// Mock focus-trap-react untuk lingkungan jsdom
vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
const mockConfirmModal = vi.fn();

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({
    toast: {
      success: mockToastSuccess,
      error: mockToastError,
      warn: vi.fn(),
      info: vi.fn(),
    },
    confirmModal: mockConfirmModal,
  }),
}));

const MOCK_BUDGETS: Budget[] = [
  {
    id: 'b-1',
    name: 'Budget Tim Internal',
    scope: 'global',
    scope_id: undefined,
    period: 'monthly',
    max_spend_usd: '100.00',
    spent_usd: '25.00',
    alert_threshold: 80,
    action: 'block',
    period_start: '2026-09-01T00:00:00Z',
    period_end: '2026-10-01T00:00:00Z',
    enabled: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
  {
    id: 'b-2',
    name: 'Budget Kunci API Riset',
    scope: 'api_key',
    scope_id: 'key-1',
    period: 'daily',
    max_spend_usd: '50.00',
    spent_usd: '45.00',
    alert_threshold: 80,
    action: 'warn',
    period_start: '2026-09-26T00:00:00Z',
    period_end: '2026-09-27T00:00:00Z',
    enabled: false,
    created_at: '2026-09-26T00:00:00Z',
    updated_at: '2026-09-26T00:00:00Z',
  },
];

const MOCK_LIMITS: RateLimit[] = [
  {
    id: 'rl-1',
    scope: 'api_key',
    scope_id: 'key-1',
    requests_per_minute: 60,
    tokens_per_minute: 100000,
    requests_per_second: 10,
    enabled: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
  {
    id: 'rl-2',
    scope: 'ip',
    scope_id: '192.168.1.1',
    requests_per_minute: 30,
    tokens_per_minute: 50000,
    requests_per_second: 5,
    enabled: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
];

const MOCK_API_KEYS: APIKey[] = [
  {
    id: 'key-1',
    name: 'Dev Key Riset',
    masked_key: 'rx-dev-1***',
    scopes: ['read', 'write'],
    allowed_models: [],
    allowed_providers: [],
    enabled: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
];

const MOCK_MODELS: Model[] = [
  {
    id: 'm-1',
    model_id: 'gpt-4o',
    display_name: 'GPT-4o Omnimodel',
    capabilities: ['chat'],
    enabled: true,
    routing_priority: 1,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
];

const mockListBudgets = vi.fn();
const mockCreateBudget = vi.fn();
const mockResetBudget = vi.fn();
const mockToggleBudget = vi.fn();
const mockDeleteBudget = vi.fn();

const mockListRateLimits = vi.fn();
const mockCreateRateLimit = vi.fn();
const mockDeleteRateLimit = vi.fn();

const mockListApiKeys = vi.fn();
const mockListModels = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    budgets: {
      list: () => mockListBudgets(),
      create: (data: unknown) => mockCreateBudget(data),
      reset: (id: string) => mockResetBudget(id),
      toggle: (id: string, enabled: boolean) => mockToggleBudget(id, enabled),
      delete: (id: string) => mockDeleteBudget(id),
    },
    rateLimits: {
      list: () => mockListRateLimits(),
      create: (data: unknown) => mockCreateRateLimit(data),
      delete: (id: string) => mockDeleteRateLimit(id),
    },
    apiKeys: {
      list: () => mockListApiKeys(),
    },
    models: {
      list: () => mockListModels(),
    },
  },
}));

describe('Budgets Page — Modular Architecture & Unit Tests', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListBudgets.mockResolvedValue({ items: MOCK_BUDGETS });
    mockListRateLimits.mockResolvedValue({ items: MOCK_LIMITS });
    mockListApiKeys.mockResolvedValue({ items: MOCK_API_KEYS });
    mockListModels.mockResolvedValue({ items: MOCK_MODELS });
    mockConfirmModal.mockResolvedValue(true);
    mockCreateBudget.mockResolvedValue({ id: 'b-new' });
    mockCreateRateLimit.mockResolvedValue({ id: 'rl-new' });
    mockResetBudget.mockResolvedValue({ success: true });
    mockToggleBudget.mockResolvedValue({ success: true });
    mockDeleteBudget.mockResolvedValue({ success: true });
    mockDeleteRateLimit.mockResolvedValue({ success: true });
  });

  it('merender daftar alokasi anggaran beserta progress bar dan metrik spend USD', async () => {
    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });

    expect(screen.getByText('Budget Kunci API Riset')).toBeInTheDocument();

    // Verifikasi label scope
    expect(screen.getByText('Global Gateway')).toBeInTheDocument();
    expect(screen.getByText('Dev Key Riset')).toBeInTheDocument();

    // Verifikasi metrik pemakaian spend USD
    expect(screen.getByText('25 / 100')).toBeInTheDocument();
    expect(screen.getByText('45 / 50')).toBeInTheDocument();

    // Verifikasi badge persentase
    expect(screen.getByText('25%')).toBeInTheDocument();
    expect(screen.getByText('90%')).toBeInTheDocument();

    // Verifikasi status tombol toggle
    expect(screen.getByText('Aktif')).toBeInTheDocument();
    expect(screen.getByText('Nonaktif')).toBeInTheDocument();
  });

  it('beralih ke tab Rate Limits dan memverifikasi daftar aturan trafik', async () => {
    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });

    // Klik tab switcher Rate Limits
    const limitTab = screen.getByRole('tab', { name: /Batas Laju Trafik/i });
    fireEvent.click(limitTab);

    await waitFor(() => {
      expect(screen.getByText('IP: 192.168.1.1')).toBeInTheDocument();
    });

    // Verifikasi kartu limit kunci API & IP
    expect(screen.getByText('60')).toBeInTheDocument();
    expect(screen.getByText('100,000')).toBeInTheDocument();
    expect(screen.getByText('10/s')).toBeInTheDocument();

    expect(screen.getByText('30')).toBeInTheDocument();
    expect(screen.getByText('50,000')).toBeInTheDocument();
    expect(screen.getByText('5/s')).toBeInTheDocument();
  });

  it('melakukan toggle status aktif anggaran (handleToggleBudget)', async () => {
    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });

    // Klik tombol toggle aktif pada b-1 (enabled: true -> false)
    const toggleButton = screen.getByLabelText('Nonaktifkan anggaran');
    fireEvent.click(toggleButton);

    await waitFor(() => {
      expect(mockToggleBudget).toHaveBeenCalledWith('b-1', false);
      expect(mockToastSuccess).toHaveBeenCalledWith(
        expect.stringContaining('dinonaktifkan')
      );
    });
  });

  it('melakukan reset pemakaian anggaran dengan konfirmasi modal', async () => {
    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });

    const resetButtons = screen.getAllByRole('button', { name: /Reset Periode/i });
    fireEvent.click(resetButtons[0]);

    await waitFor(() => {
      expect(mockConfirmModal).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Reset Pemakaian Anggaran?',
          danger: true,
        })
      );
      expect(mockResetBudget).toHaveBeenCalledWith('b-1');
      expect(mockToastSuccess).toHaveBeenCalledWith(
        expect.stringContaining('berhasil direset'),
        'Anggaran Direset'
      );
    });
  });

  it('membuka drawer Buat Anggaran dan mengirim formulir dengan valid', async () => {
    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });

    // Buka drawer via tombol header
    const addBudgetBtn = screen.getByRole('button', { name: /Alokasikan Anggaran/i });
    fireEvent.click(addBudgetBtn);

    expect(screen.getByText('Alokasi Anggaran Moneter Baru')).toBeInTheDocument();

    // Isi formulir
    const nameInput = screen.getByPlaceholderText('Budget Bulanan Tim Internal');
    fireEvent.change(nameInput, { target: { value: 'Budget QA Automation' } });

    // Submit form
    const submitBtn = screen.getByRole('button', { name: /Simpan Anggaran/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockCreateBudget).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'Budget QA Automation',
          scope: 'global',
          period: 'monthly',
          limit_usd: '100.00',
          alert_threshold_pct: 80,
          action_on_exceed: 'block',
        })
      );
      expect(mockToastSuccess).toHaveBeenCalledWith(
        'Alokasi anggaran baru berhasil disimpan.',
        'Anggaran Dibuat'
      );
    });
  });

  it('membuka drawer Buat Rate Limit dan mengirim formulir dengan valid', async () => {
    render(<Budgets initialTab="limits" />);

    await waitFor(() => {
      expect(screen.getByText('Dev Key Riset')).toBeInTheDocument();
    });

    // Buka drawer via tombol header
    const addLimitBtn = screen.getByRole('button', { name: /Tambah Rate Limit/i });
    fireEvent.click(addLimitBtn);

    expect(screen.getByText('Tambah Aturan Rate Limit Baru')).toBeInTheDocument();

    // Pilih target Kunci API dari dropdown Select
    const selectKeyBtn = screen.getByText('-- Pilih Kunci API --');
    fireEvent.click(selectKeyBtn);

    const option = await screen.findByRole('option', { name: /Dev Key Riset/i });
    fireEvent.click(option);

    // Submit form
    const submitBtn = screen.getByRole('button', { name: /Simpan Limit/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockCreateRateLimit).toHaveBeenCalledWith(
        expect.objectContaining({
          scope: 'api_key',
          scope_id: 'key-1',
          requests_per_minute: 60,
          tokens_per_minute: 100000,
          requests_per_second: 10,
        })
      );
      expect(mockToastSuccess).toHaveBeenCalledWith('Aturan batas laju berhasil dibuat');
    });
  });

  it('menangani dialog konfirmasi dan penghapusan anggaran serta rate limit', async () => {
    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });

    // 1. Hapus anggaran
    const deleteBudgetBtns = screen.getAllByLabelText('Hapus anggaran');
    fireEvent.click(deleteBudgetBtns[0]);

    await waitFor(() => {
      expect(mockConfirmModal).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Hapus Alokasi Anggaran?',
          danger: true,
        })
      );
      expect(mockDeleteBudget).toHaveBeenCalledWith('b-1');
      expect(mockToastSuccess).toHaveBeenCalledWith(
        'Alokasi anggaran berhasil dihapus.',
        'Anggaran Dihapus'
      );
    });

    // 2. Beralih ke limits dan hapus rate limit
    const limitTab = screen.getByRole('tab', { name: /Batas Laju Trafik/i });
    fireEvent.click(limitTab);

    await waitFor(() => {
      expect(screen.getByText('IP: 192.168.1.1')).toBeInTheDocument();
    });

    const deleteLimitBtns = screen.getAllByLabelText('Hapus limit');
    fireEvent.click(deleteLimitBtns[0]);

    await waitFor(() => {
      expect(mockConfirmModal).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Hapus Aturan Rate Limit',
          danger: true,
        })
      );
      expect(mockDeleteRateLimit).toHaveBeenCalledWith('rl-1');
      expect(mockToastSuccess).toHaveBeenCalledWith('Aturan batas laju berhasil dihapus');
    });
  });

  it('menangani error pemuatan API dengan menampilkan QueryError dan tombol retry', async () => {
    mockListBudgets.mockRejectedValueOnce(new Error('Koneksi database PostgreSQL terputus'));

    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText(/Koneksi database PostgreSQL terputus/i)).toBeInTheDocument();
    });

    // Klik tombol Coba Lagi
    const retryBtn = screen.getByRole('button', { name: /Coba Lagi/i });
    mockListBudgets.mockResolvedValueOnce({ items: MOCK_BUDGETS });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });
  });

  it('menangani error notifikasi toast saat aksi toggle atau hapus gagal', async () => {
    render(<Budgets />);

    await waitFor(() => {
      expect(screen.getByText('Budget Tim Internal')).toBeInTheDocument();
    });

    // Simulasi kegagalan toggle
    mockToggleBudget.mockRejectedValueOnce(new Error('Sinkronisasi Redis gagal'));
    const toggleButton = screen.getByLabelText('Nonaktifkan anggaran');
    fireEvent.click(toggleButton);

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        expect.stringContaining('Sinkronisasi Redis gagal')
      );
    });
  });
});
