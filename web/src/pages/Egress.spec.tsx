import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Egress } from './Egress';
import type { EgressPool, EgressProbeResult } from '../types';

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

const MOCK_POOLS: EgressPool[] = [
  {
    id: 'pool-sg-1',
    name: 'Singapore SOCKS5',
    kind: 'socks5',
    masked_hint: 'socks5://user:***@sg.proxy.com:1080',
    enabled: true,
    weight: 100,
    region: 'ap-southeast-1',
    last_health_status: 'healthy',
    last_latency_ms: 45,
    created_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'pool-us-2',
    name: 'US HTTP Gateway',
    kind: 'http',
    masked_hint: 'http://us-east.proxy.corp:8080',
    enabled: false,
    weight: 50,
    region: 'us-east-1',
    last_health_status: 'unhealthy',
    last_latency_ms: 0,
    created_at: '2026-01-02T00:00:00Z',
  },
];

const mockListFn = vi.fn();
const mockCreateFn = vi.fn();
const mockUpdateFn = vi.fn();
const mockDeleteFn = vi.fn();
const mockTestFn = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    egress: {
      list: () => mockListFn(),
      create: (data: unknown) => mockCreateFn(data),
      update: (id: string, data: unknown) => mockUpdateFn(id, data),
      delete: (id: string) => mockDeleteFn(id),
      test: (id: string) => mockTestFn(id),
      provisionCloudflare: vi.fn(),
      provisionDeno: vi.fn(),
    },
  },
}));

describe('Egress Page — Unit & Modular Decomposition Tests', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListFn.mockResolvedValue({ items: MOCK_POOLS });
    mockConfirmModal.mockResolvedValue(true);
  });

  it('merender daftar kartu pool dengan status, protokol, masked hint, dan bobot', async () => {
    render(<Egress />);

    // Tunggu data dimuat dan kartu ditampilkan
    await waitFor(() => {
      expect(screen.getByText('Singapore SOCKS5')).toBeInTheDocument();
    });

    expect(screen.getByText('US HTTP Gateway')).toBeInTheDocument();

    // Verifikasi badge status enabled
    expect(screen.getByText('active')).toBeInTheDocument();
    expect(screen.getByText('disabled')).toBeInTheDocument();

    // Verifikasi protokol dan wilayah
    expect(screen.getByText(/SOCKS5 • ap-southeast-1/i)).toBeInTheDocument();
    expect(screen.getByText(/HTTP • us-east-1/i)).toBeInTheDocument();

    // Verifikasi masked hint
    expect(screen.getByText('socks5://user:***@sg.proxy.com:1080')).toBeInTheDocument();
    expect(screen.getByText('http://us-east.proxy.corp:8080')).toBeInTheDocument();

    // Verifikasi status koneksi
    expect(screen.getByText(/Terhubung \(45 ms\)/i)).toBeInTheDocument();
    expect(screen.getByText('Tidak Terhubung')).toBeInTheDocument();
  });

  it('merender empty state dengan benar saat tidak ada proxy pool terdaftar', async () => {
    mockListFn.mockResolvedValueOnce({ items: [] });
    render(<Egress />);

    await waitFor(() => {
      expect(screen.getByText('Belum Ada Egress Proxy Pool')).toBeInTheDocument();
    });

    expect(
      screen.getByText(/Tambahkan proxy keluar \(seperti BrightData, Smartproxy, atau VPS\)/i)
    ).toBeInTheDocument();
    expect(screen.getByText('Tambah Egress Pool Sekarang')).toBeInTheDocument();
  });

  it('dapat membuka drawer Tambah Pool dan menambahkan pool baru', async () => {
    mockCreateFn.mockResolvedValueOnce({
      id: 'pool-new',
      name: 'Tokyo SOCKS5',
      kind: 'socks5',
      weight: 150,
      region: 'ap-northeast-1',
      enabled: true,
      created_at: new Date().toISOString(),
    });

    render(<Egress />);

    await waitFor(() => {
      expect(screen.getByText('Singapore SOCKS5')).toBeInTheDocument();
    });

    // Buka drawer buat pool
    const tambahButtons = screen.getAllByRole('button', { name: /Tambah Pool/i });
    fireEvent.click(tambahButtons[0]);

    // Drawer muncul
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('Tambah Egress Proxy Pool')).toBeInTheDocument();

    // Isi formulir
    const nameInput = screen.getByPlaceholderText('residential-sg-1 atau my-proxy');
    const urlInput = screen.getByPlaceholderText('socks5://user:pass@host:1080');
    const regionInput = screen.getByPlaceholderText('auto / ap-southeast-1');

    fireEvent.change(nameInput, { target: { value: 'Tokyo SOCKS5' } });
    fireEvent.change(urlInput, { target: { value: 'socks5://tokyo.proxy:1080' } });
    fireEvent.change(regionInput, { target: { value: 'ap-northeast-1' } });

    // Submit formulir
    const simpanBtn = screen.getByRole('button', { name: /Simpan Egress Pool/i });
    fireEvent.click(simpanBtn);

    await waitFor(() => {
      expect(mockCreateFn).toHaveBeenCalledWith({
        name: 'Tokyo SOCKS5',
        kind: 'socks5',
        proxy_url: 'socks5://tokyo.proxy:1080',
        weight: 100,
        region: 'ap-northeast-1',
      });
    });

    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Egress proxy pool baru berhasil ditambahkan.',
      'Egress Dibuat'
    );
  });

  it('dapat membuka drawer Edit Pool dan memperbarui data pool', async () => {
    mockUpdateFn.mockResolvedValueOnce({
      ...MOCK_POOLS[0],
      weight: 200,
    });

    render(<Egress />);

    await waitFor(() => {
      expect(screen.getByText('Singapore SOCKS5')).toBeInTheDocument();
    });

    // Klik tombol Edit pada kartu pertama
    const editButtons = screen.getAllByRole('button', { name: /Edit/i });
    fireEvent.click(editButtons[0]);

    // Drawer edit terbuka
    expect(screen.getByText('Edit Egress Proxy Pool')).toBeInTheDocument();

    // Ganti bobot
    const weightInput = screen.getByDisplayValue('100');
    fireEvent.change(weightInput, { target: { value: '200' } });

    // Submit edit
    const simpanPerubahanBtn = screen.getByRole('button', { name: /Simpan Perubahan/i });
    fireEvent.click(simpanPerubahanBtn);

    await waitFor(() => {
      expect(mockUpdateFn).toHaveBeenCalledWith('pool-sg-1', {
        name: 'Singapore SOCKS5',
        kind: 'socks5',
        weight: 200,
        region: 'ap-southeast-1',
        enabled: true,
      });
    });

    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Egress proxy pool berhasil diperbarui.',
      'Perubahan Disimpan'
    );
  });

  it('menjalankan live probe ping test dan menampilkan banner EgressProbeFeedback', async () => {
    const probeResult: EgressProbeResult = {
      status: 'healthy',
      latency_ms: 32,
      exit_ip: '103.21.244.2',
      country: 'Singapore',
      datacenter: 'SIN-1',
      checked_at: new Date().toISOString(),
      message: 'Proxy SOCKS5 merespons dengan latensi normal',
    };
    mockTestFn.mockResolvedValueOnce(probeResult);

    render(<Egress />);

    await waitFor(() => {
      expect(screen.getByText('Singapore SOCKS5')).toBeInTheDocument();
    });

    // Klik tombol Uji Ping kartu pertama
    const ujiPingButtons = screen.getAllByRole('button', { name: /Uji Ping/i });
    fireEvent.click(ujiPingButtons[0]);

    // Verifikasi pemanggilan api
    await waitFor(() => {
      expect(mockTestFn).toHaveBeenCalledWith('pool-sg-1');
    });

    // Banner feedback live probe muncul
    await waitFor(() => {
      expect(screen.getByRole('status')).toBeInTheDocument();
      expect(screen.getByText('Uji Koneksi Berhasil')).toBeInTheDocument();
      expect(screen.getByText('32 ms')).toBeInTheDocument();
      expect(screen.getByText('103.21.244.2')).toBeInTheDocument();
      expect(screen.getByText('Singapore')).toBeInTheDocument();
      expect(screen.getByText('SIN-1')).toBeInTheDocument();
    });

    // Tombol tutup feedback dapat diklik
    const closeFeedbackBtn = screen.getByRole('button', { name: /Tutup notifikasi/i });
    fireEvent.click(closeFeedbackBtn);

    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('menampilkan notifikasi gagal saat live probe test mengalami error', async () => {
    mockTestFn.mockRejectedValueOnce(new Error('Koneksi timeout ke exit node'));

    render(<Egress />);

    await waitFor(() => {
      expect(screen.getByText('Singapore SOCKS5')).toBeInTheDocument();
    });

    const ujiPingButtons = screen.getAllByRole('button', { name: /Uji Ping/i });
    fireEvent.click(ujiPingButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
      expect(screen.getByText('Uji Koneksi Gagal')).toBeInTheDocument();
      expect(screen.getByText('Koneksi timeout ke exit node')).toBeInTheDocument();
    });

    expect(mockToastError).toHaveBeenCalledWith(
      'Uji koneksi egress gagal: Koneksi timeout ke exit node'
    );
  });

  it('menghapus pool saat konfirmasi disetujui', async () => {
    mockDeleteFn.mockResolvedValueOnce(undefined);

    render(<Egress />);

    await waitFor(() => {
      expect(screen.getByText('Singapore SOCKS5')).toBeInTheDocument();
    });

    const hapusButtons = screen.getAllByRole('button', { name: /Hapus/i });
    fireEvent.click(hapusButtons[0]);

    await waitFor(() => {
      expect(mockConfirmModal).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Hapus Egress Pool?',
          danger: true,
        })
      );
      expect(mockDeleteFn).toHaveBeenCalledWith('pool-sg-1');
    });

    expect(mockToastSuccess).toHaveBeenCalledWith(
      'Egress pool "Singapore SOCKS5" berhasil dihapus.',
      'Egress Dihapus'
    );
  });
});
