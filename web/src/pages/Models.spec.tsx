import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Models } from './Models';
import type { Model, ProviderModel, Price } from '../types';

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

const MOCK_MODELS: Model[] = [
  {
    id: 'm-1',
    model_id: 'gpt-4o',
    display_name: 'OpenAI GPT-4o Omni',
    family: 'gpt-4',
    context_window: 128000,
    max_output_tokens: 4096,
    capabilities: ['text', 'vision'],
    enabled: true,
    routing_priority: 1,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
    providers: [
      {
        provider_id: 'p-1',
        provider_name: 'openai-direct',
        display_name: 'OpenAI US East',
        upstream_model_name: 'gpt-4o-2024-08-06',
      },
    ],
  },
  {
    id: 'm-2',
    model_id: 'claude-3-5-sonnet',
    display_name: 'Anthropic Claude 3.5 Sonnet',
    family: 'claude-3.5',
    context_window: 200000,
    max_output_tokens: 8192,
    capabilities: ['text', 'vision'],
    enabled: true,
    routing_priority: 1,
    created_at: '2026-09-02T00:00:00Z',
    updated_at: '2026-09-02T00:00:00Z',
    providers: [
      {
        provider_id: 'p-2',
        provider_name: 'anthropic-direct',
        display_name: 'Anthropic Cloud',
        upstream_model_name: 'claude-3-5-sonnet-20241022',
      },
    ],
  },
  {
    id: 'm-3',
    model_id: 'deepseek-r1',
    display_name: 'DeepSeek R1 Reasoning',
    family: 'deepseek-reasoner',
    context_window: 64000,
    max_output_tokens: 8000,
    capabilities: ['text', 'reasoning'],
    enabled: true,
    routing_priority: 1,
    created_at: '2026-09-03T00:00:00Z',
    updated_at: '2026-09-03T00:00:00Z',
    providers: [],
  },
];

const MOCK_MAPPINGS: ProviderModel[] = [
  {
    id: 'mp-1',
    model_id: 'm-1',
    provider_id: 'p-1',
    upstream_model_name: 'gpt-4o-2024-08-06',
    enabled: true,
    created_at: '2026-09-01T00:00:00Z',
  },
];

const MOCK_PRICING_HISTORY: Price[] = [
  {
    id: 'pr-1',
    provider_model_id: 'mp-1',
    input_per_1m_usd: '2.50',
    output_per_1m_usd: '10.00',
    cached_input_per_1m_usd: '1.25',
    currency: 'USD',
    effective_from: '2026-09-01T00:00:00Z',
    created_at: '2026-09-01T00:00:00Z',
  },
];

const mockListModels = vi.fn();
const mockGetModel = vi.fn();
const mockCreateModel = vi.fn();
const mockDeleteModel = vi.fn();
const mockSetPrice = vi.fn();
const mockPricingHistory = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    models: {
      list: () => mockListModels(),
      get: (id: string) => mockGetModel(id),
      create: (data: unknown) => mockCreateModel(data),
      delete: (id: string) => mockDeleteModel(id),
      setPrice: (mappingId: string, data: unknown) => mockSetPrice(mappingId, data),
      pricingHistory: (mappingId: string) => mockPricingHistory(mappingId),
    },
  },
}));

describe('Models Page — Modular Architecture & Unit Tests', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockListModels.mockResolvedValue({ items: MOCK_MODELS });
    mockGetModel.mockResolvedValue({
      model: MOCK_MODELS[0],
      aliases: [],
      mappings: MOCK_MAPPINGS,
    });
    mockPricingHistory.mockResolvedValue({ items: MOCK_PRICING_HISTORY });
    mockCreateModel.mockResolvedValue({ id: 'm-new' });
    mockDeleteModel.mockResolvedValue(undefined);
    mockSetPrice.mockResolvedValue(MOCK_PRICING_HISTORY[0]);
    mockConfirmModal.mockResolvedValue(true);
  });

  it('merender default tabel katalog model dengan mock data dan informasi model', async () => {
    render(<Models />);

    // Memastikan PageHeader muncul
    expect(screen.getByText('Model Registry & Pricing')).toBeInTheDocument();

    // Tunggu data model dimuat ke tabel
    await waitFor(() => {
      expect(screen.getByTestId('models-table')).toBeInTheDocument();
    });

    expect(screen.getByText('OpenAI GPT-4o Omni')).toBeInTheDocument();
    expect(screen.getByText('Anthropic Claude 3.5 Sonnet')).toBeInTheDocument();
    expect(screen.getByText('DeepSeek R1 Reasoning')).toBeInTheDocument();

    // Verifikasi counter ringkasan
    expect(screen.getByTestId('models-counter')).toHaveTextContent('3 dari 3 model');
  });

  it('berganti mode tampilan dari Table ke Grid dan sebaliknya serta menyimpan ke localStorage', async () => {
    render(<Models />);

    await waitFor(() => {
      expect(screen.getByTestId('models-table')).toBeInTheDocument();
    });

    // Klik tombol switch ke tampilan Kartu Grid
    const gridBtn = screen.getByLabelText('Tampilan Grid');
    fireEvent.click(gridBtn);

    // Verifikasi tampilan grid muncul
    expect(screen.getByTestId('models-grid')).toBeInTheDocument();
    expect(screen.queryByTestId('models-table')).not.toBeInTheDocument();
    expect(localStorage.getItem('routex_models_view_mode')).toBe('grid');

    // Beralih kembali ke tabel
    const tableBtn = screen.getByLabelText('Tampilan Tabel');
    fireEvent.click(tableBtn);

    expect(screen.getByTestId('models-table')).toBeInTheDocument();
    expect(screen.queryByTestId('models-grid')).not.toBeInTheDocument();
    expect(localStorage.getItem('routex_models_view_mode')).toBe('table');
  });

  it('memfilter model berdasarkan pencarian teks dan filter chip keluarga model (family pill)', async () => {
    render(<Models />);

    await waitFor(() => {
      expect(screen.getByTestId('models-table')).toBeInTheDocument();
    });

    // Lakukan pencarian teks "deepseek"
    const searchInput = screen.getByLabelText('Cari model');
    fireEvent.change(searchInput, { target: { value: 'deepseek' } });

    await waitFor(() => {
      expect(screen.getByText('DeepSeek R1 Reasoning')).toBeInTheDocument();
      expect(screen.queryByText('OpenAI GPT-4o Omni')).not.toBeInTheDocument();
      expect(screen.queryByText('Anthropic Claude 3.5 Sonnet')).not.toBeInTheDocument();
    });

    // Reset pencarian dengan tombol X
    const clearBtn = screen.getByLabelText('Hapus pencarian');
    fireEvent.click(clearBtn);

    await waitFor(() => {
      expect(screen.getByText('OpenAI GPT-4o Omni')).toBeInTheDocument();
      expect(screen.getByText('Anthropic Claude 3.5 Sonnet')).toBeInTheDocument();
      expect(screen.getByText('DeepSeek R1 Reasoning')).toBeInTheDocument();
    });

    // Filter menggunakan chip keluarga model "Claude"
    const claudeChip = screen.getByTestId('family-chip-claude');
    fireEvent.click(claudeChip);

    await waitFor(() => {
      expect(screen.getByText('Anthropic Claude 3.5 Sonnet')).toBeInTheDocument();
      expect(screen.queryByText('OpenAI GPT-4o Omni')).not.toBeInTheDocument();
      expect(screen.queryByText('DeepSeek R1 Reasoning')).not.toBeInTheDocument();
    });

    // Reset kembali dengan chip "Semua"
    const allChip = screen.getByTestId('family-chip-all');
    fireEvent.click(allChip);

    await waitFor(() => {
      expect(screen.getByText('OpenAI GPT-4o Omni')).toBeInTheDocument();
      expect(screen.getByText('Anthropic Claude 3.5 Sonnet')).toBeInTheDocument();
      expect(screen.getByText('DeepSeek R1 Reasoning')).toBeInTheDocument();
    });
  });

  it('membuka drawer Tambah Model dan submit model baru dengan validasi dan pemanggilan API', async () => {
    render(<Models />);

    await waitFor(() => {
      expect(screen.getByTestId('models-table')).toBeInTheDocument();
    });

    // Buka Drawer Tambah Model
    const createBtn = screen.getByRole('button', { name: /Daftarkan Model/i });
    fireEvent.click(createBtn);

    expect(screen.getByText('Registrasi Model Kanonik')).toBeInTheDocument();

    // Isi formulir
    const modelIdInput = screen.getByLabelText(/ID Model Kanonik \*/i);
    const displayNameInput = screen.getByLabelText(/Nama Tampilan \*/i);
    const contextInput = screen.getByLabelText(/Context Window/i);
    const maxOutputInput = screen.getByLabelText(/Max Output Tokens/i);

    fireEvent.change(modelIdInput, { target: { value: 'gemini-1.5-pro' } });
    fireEvent.change(displayNameInput, { target: { value: 'Google Gemini 1.5 Pro' } });
    fireEvent.change(contextInput, { target: { value: '1000000' } });
    fireEvent.change(maxOutputInput, { target: { value: '8192' } });

    // Klik preset keluarga model Gemini
    const geminiPresetBtn = screen.getByRole('button', { name: 'Gemini' });
    fireEvent.click(geminiPresetBtn);

    // Submit formulir
    const submitBtn = screen.getByRole('button', { name: /Simpan Model/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(mockCreateModel).toHaveBeenCalledWith({
        model_id: 'gemini-1.5-pro',
        display_name: 'Google Gemini 1.5 Pro',
        family: 'gemini',
        context_window: 1000000,
        max_output_tokens: 8192,
        capabilities: ['text'],
      });
    });

    expect(mockToastSuccess).toHaveBeenCalledWith('Model kanonik baru berhasil didaftarkan');
  });

  it('membuka drawer Pricing dan menyimpan konfigurasi tarif harga', async () => {
    render(<Models />);

    await waitFor(() => {
      expect(screen.getByTestId('models-table')).toBeInTheDocument();
    });

    // Klik tombol Harga pada model OpenAI GPT-4o Omni
    const pricingBtn = screen.getByLabelText('Konfigurasi harga untuk OpenAI GPT-4o Omni');
    fireEvent.click(pricingBtn);

    await waitFor(() => {
      expect(mockGetModel).toHaveBeenCalledWith('m-1');
      expect(mockPricingHistory).toHaveBeenCalledWith('mp-1');
    });

    expect(screen.getByText(/Konfigurasi Harga: OpenAI GPT-4o Omni/i)).toBeInTheDocument();

    // Verifikasi nilai awal form pricing
    const inputPrice = screen.getByLabelText(/Input \/ 1M Token \(USD\) \*/i) as HTMLInputElement;
    const outputPrice = screen.getByLabelText(/Output \/ 1M Token \(USD\) \*/i) as HTMLInputElement;
    const cachedInputPrice = screen.getByLabelText(/Cached Input \/ 1M Token \(USD\) \(Opsional\)/i) as HTMLInputElement;

    expect(inputPrice.value).toBe('2.50');
    expect(outputPrice.value).toBe('10.00');

    // Ubah nilai tarif harga
    fireEvent.change(inputPrice, { target: { value: '2.75' } });
    fireEvent.change(outputPrice, { target: { value: '11.00' } });
    fireEvent.change(cachedInputPrice, { target: { value: '1.35' } });

    // Simpan harga
    const saveBtn = screen.getByRole('button', { name: /Simpan Harga/i });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(mockSetPrice).toHaveBeenCalledWith('mp-1', {
        input_per_1m_usd: '2.75',
        output_per_1m_usd: '11.00',
        cached_input_per_1m_usd: '1.35',
      });
    });

    expect(mockToastSuccess).toHaveBeenCalledWith('Harga model berhasil disimpan');
  });

  it('menampilkan dialog konfirmasi hapus model dan memanggil API delete', async () => {
    render(<Models />);

    await waitFor(() => {
      expect(screen.getByTestId('models-table')).toBeInTheDocument();
    });

    const deleteBtn = screen.getByLabelText('Hapus model DeepSeek R1 Reasoning');
    fireEvent.click(deleteBtn);

    await waitFor(() => {
      expect(mockConfirmModal).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Hapus Model Kanonik',
          message: expect.stringContaining('DeepSeek R1 Reasoning'),
          danger: true,
        })
      );
    });

    await waitFor(() => {
      expect(mockDeleteModel).toHaveBeenCalledWith('m-3');
    });

    expect(mockToastSuccess).toHaveBeenCalledWith('Model "DeepSeek R1 Reasoning" berhasil dihapus');
  });

  it('menangani penanganan error pemuatan API dengan komponen QueryError dan opsi coba lagi', async () => {
    mockListModels.mockRejectedValueOnce(new Error('Koneksi database gagal'));

    render(<Models />);

    await waitFor(() => {
      expect(screen.getByText('Koneksi database gagal')).toBeInTheDocument();
    });

    // Set mock berikutnya sukses
    mockListModels.mockResolvedValueOnce({ items: MOCK_MODELS });

    // Klik tombol Coba Lagi
    const retryBtn = screen.getByRole('button', { name: /Coba Lagi/i });
    fireEvent.click(retryBtn);

    await waitFor(() => {
      expect(screen.getByTestId('models-table')).toBeInTheDocument();
    });
  });
});
