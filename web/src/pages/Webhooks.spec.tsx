import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Webhooks } from './Webhooks';
import { ToastProvider } from '../context/ToastContext';
import type { Webhook, WebhookDelivery } from '../types';

// B-452: muatan ULANG riwayat pengiriman yang gagal tidak boleh
// meninggalkan daftar basi tanpa penanda — banner kesegaran wajib
// tampil di atas daftar lama.

const WH: Webhook = {
  id: 'wh-1',
  name: 'Webhook Uji',
  url: 'https://example.com/hook',
  events: ['*'],
  masked_hint: '****abcd',
  enabled: true,
  max_retries: 3,
  timeout_ms: 5000,
  consecutive_failures: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const DELIVERY: WebhookDelivery = {
  id: 1,
  webhook_id: 'wh-1',
  event: 'request.completed',
  status: 'success',
  attempt_count: 1,
  next_attempt_at: new Date().toISOString(),
  response_status_code: 200,
  created_at: new Date().toISOString(),
};

const mockList = vi.fn(async () => ({ items: [WH] as Webhook[] }));
const mockDeliveries = vi.fn();
const mockTest = vi.fn(async (_id: string) => ({ status: 'success', duration_ms: 12 }));

vi.mock('../api/client', () => ({
  api: {
    webhooks: {
      list: () => mockList(),
      getDeliveries: (id: string) => mockDeliveries(id),
      test: (id: string) => mockTest(id),
      create: async () => ({}),
      toggle: async () => ({}),
      delete: async () => ({}),
      getDelivery: async () => DELIVERY,
    },
  },
}));

describe('Webhooks — muatan ulang riwayat gagal (B-452)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockList.mockResolvedValue({ items: [WH] });
    mockTest.mockResolvedValue({ status: 'success', duration_ms: 12 });
  });

  it('daftar lama tetap tampil DENGAN penanda basi sesudah muatan ulang gagal', async () => {
    mockDeliveries
      .mockResolvedValueOnce({ items: [DELIVERY] })
      .mockRejectedValueOnce(new Error('jaringan putus'));
    render(
      <ToastProvider>
        <Webhooks />
      </ToastProvider>
    );

    // Buka drawer riwayat lewat kartu webhook.
    fireEvent.click(await screen.findByText('Webhook Uji'));
    expect(await screen.findByText('request.completed')).toBeInTheDocument();
    expect(screen.queryByText(/mungkin basi/)).toBeNull();

    // Picu muatan ulang lewat tombol Test pada kartu yang sama.
    fireEvent.click(screen.getByRole('button', { name: /^test$/i }));

    expect(await screen.findByText(/Riwayat di bawah mungkin basi/)).toBeInTheDocument();
    // Daftar lama masih ada — kini berpenanda, bukan seolah terkini.
    expect(screen.getByText('request.completed')).toBeInTheDocument();
  });
});
