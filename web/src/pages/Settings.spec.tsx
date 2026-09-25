import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Settings } from './Settings';
import { api } from '../api/client';
import type { DomainConfig, BackupStatusResponse } from '../types';

// Mock focus-trap-react untuk jsdom
vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const MOCK_DOMAIN_CONFIG: DomainConfig = {
  domain: 'ai.routex.io',
  mode: 'letsencrypt',
  status: 'active',
  public_url: 'https://ai.routex.io',
  base_url: 'https://ai.routex.io/v1',
  server_ip: '203.0.113.10',
  dns_matched: true,
};

const MOCK_BACKUP_STATUS: BackupStatusResponse = {
  database_name: 'routex_prod',
  database_size: '12.4 MB',
  database_bytes: 13002342,
  total_tables: 24,
  models_count: 8,
  providers_count: 4,
  routing_rules_count: 6,
  api_keys_count: 12,
  last_server_backup: {
    file_name: 'backup_2026-09-25.sql.gz',
    file_size: '1.8 MB',
    created_at: '2026-09-25T12:00:00Z',
  },
};

const MOCK_SETTINGS = [
  {
    key: 'custom:timeout_threshold',
    value: 5000,
    description: 'Custom global timeout',
  },
  {
    key: 'cli:config:version',
    value: '1.2.0',
    description: 'CLI Subsystem Config',
  },
  {
    key: 'system:instance_id',
    value: 'rtx-prod-01',
    description: 'Internal System ID',
  },
];

const mockToast = {
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
  info: vi.fn(),
};

const mockConfirmModal = vi.fn(async () => true);

vi.mock('../context/ToastContext', () => ({
  useToast: () => ({
    toast: mockToast,
    confirmModal: mockConfirmModal,
  }),
}));

vi.mock('../api/client', () => ({
  api: {
    system: {
      settings: vi.fn(async () => ({ items: MOCK_SETTINGS })),
      updateSetting: vi.fn(async () => {}),
      deleteSetting: vi.fn(async () => {}),
      domain: {
        get: vi.fn(async () => MOCK_DOMAIN_CONFIG),
        update: vi.fn(async (payload) => ({
          ...MOCK_DOMAIN_CONFIG,
          domain: payload.domain,
          mode: payload.mode,
          message: 'Domain berhasil diverifikasi dan HTTPS otomatis telah diaktifkan!',
        })),
        delete: vi.fn(async () => ({ status: 'ok' })),
      },
      backup: {
        status: vi.fn(async () => MOCK_BACKUP_STATUS),
        exportUrl: vi.fn((format: string = 'sql.gz') => `/api/admin/system/backup/export?format=${format}`),
        restore: vi.fn(async () => ({ status: 'ok' })),
      },
    },
  },
}));

describe('Settings — Pengujian Halaman Modular & Terisolasi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.system.settings).mockResolvedValue({ items: [...MOCK_SETTINGS] });
    vi.mocked(api.system.domain.get).mockResolvedValue({ ...MOCK_DOMAIN_CONFIG });
    vi.mocked(api.system.backup.status).mockResolvedValue({ ...MOCK_BACKUP_STATUS });
  });

  it('merender default tab Domain & HTTPS beserta data domain aktif', async () => {
    render(<Settings />);

    await waitFor(() => {
      expect(screen.getByText('Pengaturan Sistem')).toBeInTheDocument();
      expect(screen.getByText('Domain Publik & Otomatis HTTPS (SSL)')).toBeInTheDocument();
    });

    // Verifikasi badge status aktif
    expect(screen.getByText(/Aktif & Terlindungi/i)).toBeInTheDocument();

    // Verifikasi nilai domain yang terisi pada input
    const domainInput = screen.getByLabelText(/Nama Domain FQDN/i) as HTMLInputElement;
    expect(domainInput.value).toBe('ai.routex.io');

    // Verifikasi IP server
    expect(screen.getByText('203.0.113.10')).toBeInTheDocument();

    // Verifikasi info panel dan endpoint live
    expect(screen.getByText('https://ai.routex.io')).toBeInTheDocument();
    expect(screen.getByText('https://ai.routex.io/v1')).toBeInTheDocument();
    expect(screen.getByText('Terverifikasi')).toBeInTheDocument();

    // Verifikasi tombol submit & lepas domain
    expect(screen.getByRole('button', { name: /Perbarui & Verifikasi Domain/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Lepas Domain/i })).toBeInTheDocument();
  });

  it('berganti ke tab Backup dan memverifikasi status backup serta tombol download/upload', async () => {
    render(<Settings />);

    await waitFor(() => {
      expect(screen.getByText('Pengaturan Sistem')).toBeInTheDocument();
    });

    // Pindah ke tab Backup & Restore
    fireEvent.click(screen.getByRole('button', { name: /Backup & Restore/i }));

    await waitFor(() => {
      expect(screen.getByText('Pencadangan & Pemulihan Database (SQL)')).toBeInTheDocument();
    });

    // Cek ringkasan database
    expect(screen.getByText('routex_prod')).toBeInTheDocument();
    expect(screen.getByText('Ukuran: 12.4 MB')).toBeInTheDocument();
    expect(screen.getByText('24 Tabel')).toBeInTheDocument();
    expect(screen.getByText(/8 Model · 4 Provider/i)).toBeInTheDocument();

    // Cek opsi format dan tombol download
    expect(screen.getAllByText('.sql.gz').length).toBeGreaterThan(0);
    expect(screen.getAllByText('.sql').length).toBeGreaterThan(0);
    const downloadLink = screen.getByRole('link', { name: /Unduh Cadangan SQL/i });
    expect(downloadLink).toHaveAttribute('href', '/api/admin/system/backup/export?format=sql.gz');

    // Tombol restore dalam keadaan belum ada berkas terpilih
    const restoreBtn = screen.getByRole('button', { name: /Pilih Berkas Dahulu/i });
    expect(restoreBtn).toBeDisabled();
  });

  it('berganti ke tab Advanced Settings dan memverifikasi daftar runtime settings', async () => {
    render(<Settings />);

    await waitFor(() => {
      expect(screen.getByText('Pengaturan Sistem')).toBeInTheDocument();
    });

    // Pindah ke tab Pengaturan Lanjutan
    fireEvent.click(screen.getByRole('button', { name: /Pengaturan Lanjutan/i }));

    await waitFor(() => {
      expect(screen.getByText('Runtime Parameters')).toBeInTheDocument();
    });

    // Setting kustom muncul dengan key dan textarea nilainya
    expect(screen.getByText('custom:timeout_threshold')).toBeInTheDocument();
    expect(screen.getByText('Custom global timeout')).toBeInTheDocument();
    expect(screen.getByDisplayValue('5000')).toBeInTheDocument();
    expect(screen.getByText('Kustom')).toBeInTheDocument();

    // Section Subsistem Terkelola (Read-Only)
    expect(screen.getByText('Setelan Terkelola Subsistem (Read-Only)')).toBeInTheDocument();
    expect(screen.getByText('cli:config:version')).toBeInTheDocument();
    expect(screen.getByText('system:instance_id')).toBeInTheDocument();
    expect(screen.getByText('CLI Integrations')).toBeInTheDocument();
  });

  it('membuka modal Tambah Setting dan submit setting baru', async () => {
    render(<Settings />);

    await waitFor(() => {
      expect(screen.getByText('Pengaturan Sistem')).toBeInTheDocument();
    });

    // Pindah ke tab Pengaturan Lanjutan
    fireEvent.click(screen.getByRole('button', { name: /Pengaturan Lanjutan/i }));

    await waitFor(() => {
      expect(screen.getByText('Runtime Parameters')).toBeInTheDocument();
    });

    // Klik tombol Tambah Parameter
    const addBtns = screen.getAllByRole('button', { name: /Tambah Parameter/i });
    fireEvent.click(addBtns[0]);

    // Modal terbuka
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument();
      expect(screen.getByText('Kunci Parameter (Key) *')).toBeInTheDocument();
    });

    // Isi formulir
    const keyInput = screen.getByPlaceholderText(/contoh: gateway:maintenance_mode atau timeout_ms/i);
    const descInput = screen.getByPlaceholderText(/Keterangan fungsi atau tujuan parameter ini.../i);
    const valInput = screen.getByPlaceholderText(/Masukkan teks biasa, angka, boolean, atau format JSON.../i);

    fireEvent.change(keyInput, { target: { value: 'feature:smart_routing' } });
    fireEvent.change(descInput, { target: { value: 'Enable smart routing engine' } });
    fireEvent.change(valInput, { target: { value: 'true' } });

    // Submit formulir
    fireEvent.click(screen.getByRole('button', { name: /Simpan Parameter/i }));

    await waitFor(() => {
      expect(api.system.updateSetting).toHaveBeenCalledWith(
        'feature:smart_routing',
        'true',
        'Enable smart routing engine'
      );
    });

    expect(mockToast.success).toHaveBeenCalledWith('Parameter "feature:smart_routing" berhasil dibuat.');
  });

  it('membuka modal Restore Backup saat file dipilih dan melakukan pemulihan', async () => {
    render(<Settings />);

    await waitFor(() => {
      expect(screen.getByText('Pengaturan Sistem')).toBeInTheDocument();
    });

    // Pindah ke tab Backup
    fireEvent.click(screen.getByRole('button', { name: /Backup & Restore/i }));

    await waitFor(() => {
      expect(screen.getByText('Pencadangan & Pemulihan Database (SQL)')).toBeInTheDocument();
    });

    // Simulasikan pemilihan berkas .sql
    const dummyFile = new File(['-- Route-X PostgreSQL dump'], 'routex_restore_test.sql', {
      type: 'application/sql',
    });

    const fileInput = document.getElementById('restore-file-input') as HTMLInputElement;
    expect(fileInput).toBeInTheDocument();

    fireEvent.change(fileInput, { target: { files: [dummyFile] } });

    // Tombol restore aktif
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Mulai Pemulihan Database.../i })).toBeEnabled();
    });

    // Klik tombol pemicu modal
    const restoreTriggerBtn = screen.getByRole('button', { name: /Mulai Pemulihan Database.../i });
    fireEvent.click(restoreTriggerBtn);

    // Verifikasi modal konfirmasi muncul
    await waitFor(() => {
      expect(screen.getByText('Konfirmasi Pemulihan Database')).toBeInTheDocument();
      expect(screen.getByText('Tindakan Ini Berdampak Besar')).toBeInTheDocument();
      expect(screen.getAllByText('routex_restore_test.sql').length).toBeGreaterThan(0);
    });

    // Klik konfirmasi pemulihan
    fireEvent.click(screen.getByRole('button', { name: /Ya, Pulihkan Sekarang/i }));

    await waitFor(() => {
      expect(api.system.backup.restore).toHaveBeenCalledWith(dummyFile);
    });

    expect(mockToast.success).toHaveBeenCalledWith('Basis data berhasil dipulihkan!');
  });

  it('menangani penanganan error API dengan notifikasi query error / feedback', async () => {
    // Simulasikan kegagalan load settings
    vi.mocked(api.system.settings).mockRejectedValueOnce(new Error('Koneksi basis data terputus'));

    render(<Settings />);

    await waitFor(() => {
      expect(screen.getByText('Koneksi basis data terputus')).toBeInTheDocument();
    });

    // Cek tombol coba lagi
    expect(screen.getByRole('button', { name: /Coba Lagi/i })).toBeInTheDocument();
  });
});
