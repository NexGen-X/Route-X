import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { AuditLogs } from './AuditLogs';
import { ToastProvider } from '../context/ToastContext';
import type { AuditLogEntry } from '../types';

// Spec halaman Log Audit (B-108): halaman mengonsumsi klien
// `api.system.auditLogs` yang sudah ada, dengan filter yang persis
// didukung handler backend (window/from, action, resource_type, cursor).

const ENTRI: AuditLogEntry = {
  id: 42,
  occurred_at: '2026-11-04T01:15:00Z',
  actor_user_id: '0d6f2f3a-1111-4222-8333-444455556666',
  actor_email: 'admin@routex.local',
  actor_role: 'Admin',
  action: 'provider.create',
  resource_type: 'provider',
  resource_id: 'prov-1',
  ip: '127.0.0.1',
  user_agent: 'Mozilla/5.0',
  request_id: 'req-abc',
  metadata: { name: 'openai-main' },
};

const mockAuditLogs = vi.fn(
  async (_params?: Record<string, string | number | boolean | undefined | null>) => ({
    items: [ENTRI] as AuditLogEntry[],
    next_cursor: undefined as string | undefined,
  })
);

vi.mock('../api/client', () => ({
  api: {
    system: {
      auditLogs: (params?: Record<string, string | number | boolean | undefined | null>) =>
        mockAuditLogs(params),
    },
  },
}));

const renderHalaman = () =>
  render(
    <ToastProvider>
      <AuditLogs />
    </ToastProvider>
  );

describe('AuditLogs (B-108)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuditLogs.mockResolvedValue({ items: [ENTRI], next_cursor: undefined });
  });

  it('memuat dan menampilkan catatan dengan jendela bawaan 24h', async () => {
    renderHalaman();
    expect(await screen.findByText('provider.create')).toBeInTheDocument();
    expect(screen.getByText('admin@routex.local')).toBeInTheDocument();
    expect(mockAuditLogs).toHaveBeenCalledWith({ window: '24h' });
  });

  it('keadaan kosong tampil jujur sebagai belum ada catatan', async () => {
    mockAuditLogs.mockResolvedValue({ items: [], next_cursor: undefined });
    renderHalaman();
    expect(await screen.findByText('Belum ada catatan audit')).toBeInTheDocument();
  });

  it('kegagalan muat tampil sebagai error dengan tombol coba lagi, bukan daftar kosong', async () => {
    mockAuditLogs.mockRejectedValue(new Error('server audit tidak terjangkau'));
    renderHalaman();
    expect(await screen.findByText('Data gagal dimuat')).toBeInTheDocument();
    expect(screen.getByText(/server audit tidak terjangkau/)).toBeInTheDocument();
    expect(screen.queryByText('Belum ada catatan audit')).toBeNull();
  });

  it('filter aksi & jenis sumber daya dikirim sebagai parameter API yang didukung', async () => {
    renderHalaman();
    expect(await screen.findByText('provider.create')).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText('Aksi persis, mis. provider.create'), {
      target: { value: 'api_key.rotate' },
    });
    fireEvent.change(screen.getByPlaceholderText('Jenis sumber daya, mis. provider'), {
      target: { value: 'api_key' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Terapkan Filter/ }));

    await waitFor(() => {
      expect(mockAuditLogs).toHaveBeenLastCalledWith({
        window: '24h',
        action: 'api_key.rotate',
        resource_type: 'api_key',
      });
    });
  });

  it('pilihan Semua Waktu mengirim from eksplisit, bukan membiarkan bawaan backend', async () => {
    renderHalaman();
    expect(await screen.findByText('provider.create')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Semua Waktu' }));
    await waitFor(() => {
      expect(mockAuditLogs).toHaveBeenLastCalledWith({ from: '1970-01-01T00:00:00Z' });
    });
  });

  it('klik baris membuka drawer detail berisi metadata catatan', async () => {
    renderHalaman();
    const sel = await screen.findByText('provider.create');
    fireEvent.click(sel);
    expect(await screen.findByText('Detail Catatan Audit')).toBeInTheDocument();
    expect(screen.getByText(/"name": "openai-main"/)).toBeInTheDocument();
    expect(screen.getByText('req-abc')).toBeInTheDocument();
  });

  it('kursor berikutnya memuat halaman lanjutan dan menggabungkannya', async () => {
    const entri2: AuditLogEntry = { ...ENTRI, id: 41, action: 'auth.login' };
    mockAuditLogs
      .mockResolvedValueOnce({ items: [ENTRI], next_cursor: 'kursor-2' })
      .mockResolvedValueOnce({ items: [entri2], next_cursor: undefined });
    renderHalaman();
    expect(await screen.findByText('provider.create')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Muat Lebih Banyak' }));
    expect(await screen.findByText('auth.login')).toBeInTheDocument();
    expect(screen.getByText('provider.create')).toBeInTheDocument();
    expect(mockAuditLogs).toHaveBeenLastCalledWith({ window: '24h', cursor: 'kursor-2' });
  });
});
