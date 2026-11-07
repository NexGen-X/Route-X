import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RequestInspectorModal } from './RequestInspectorModal';
import type { RequestLog } from '../../types';

// Spec pengunci B-593: bila events GAGAL dimuat, timeline TIDAK BOLEH
// mengklaim "Tidak ada rekaman event sekunder." — klaim kosong itu hanya
// sah bila events memang kosong secara sah. Penanda gagal khusus events
// (`eventsGagal`) membedakan kegagalan events dari kegagalan payload:
// bila HANYA payload gagal sementara events kosong secara sah, klaim
// kosong tetap benar dan harus tampil.

vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock('../../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() },
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

const KLAIM_KOSONG = 'Tidak ada rekaman event sekunder.';
const PENANDA_GAGAL = 'Riwayat event belum dapat dimuat.';

function renderModal(props: {
  eventsGagal?: boolean;
  error?: string | null;
}) {
  return render(
    <RequestInspectorModal
      isOpen
      onClose={() => {}}
      request={MOCK_REQUEST}
      events={[]}
      payload={null}
      eventsGagal={props.eventsGagal}
      error={props.error ?? null}
    />,
  );
}

describe('RequestInspectorModal — klaim kosong timeline saat events gagal (B-593)', () => {
  it('events=[] + events GAGAL → penanda belum-terukur tampil, klaim kosong TIDAK tampil', () => {
    renderModal({
      eventsGagal: true,
      error: 'Sebagian data inspector gagal dimuat — events: boom',
    });
    expect(screen.getByText(PENANDA_GAGAL)).toBeInTheDocument();
    expect(screen.queryByText(KLAIM_KOSONG)).not.toBeInTheDocument();
  });

  it('kontrol: events=[] + tanpa galat → klaim kosong tampil (benar)', () => {
    renderModal({ eventsGagal: false });
    expect(screen.getByText(KLAIM_KOSONG)).toBeInTheDocument();
    expect(screen.queryByText(PENANDA_GAGAL)).not.toBeInTheDocument();
  });

  it('kontrol: HANYA payload gagal (events kosong secara sah) → klaim kosong tetap tampil', () => {
    renderModal({
      eventsGagal: false,
      error: 'Sebagian data inspector gagal dimuat — payload: boom',
    });
    expect(screen.getByText(KLAIM_KOSONG)).toBeInTheDocument();
    expect(screen.queryByText(PENANDA_GAGAL)).not.toBeInTheDocument();
  });
});
