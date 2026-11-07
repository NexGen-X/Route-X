import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { CircuitBreakersPanel } from './CircuitBreakersPanel';
import type { CircuitBreakerStatus, Model, Provider } from '../../types';

// B-555 (lencana kepala panel breaker): klaim "{n} Sirkuit
// Terganggu" dulu hanya menggantung pada hitungan daftar — saat
// refetch GAGAL (daftar basi dipertahankan + breakersError
// terisi + QueryError tampil di panel yang sama) lencana merah
// definitif tetap tampil. Kini cabang terganggu ikut gerbang
// statusDiketahui yang sama dengan klaim sehat B-312.

const PROVIDER: Provider = {
  id: 'prov-1',
  name: 'openai-main',
  display_name: 'OpenAI Main',
  kind: 'openai',
  base_url: 'https://api.openai.com/v1',
  enabled: true,
  priority: 1,
  weight: 100,
  timeout_ms: 30000,
  max_retries: 3,
  consecutive_failures: 0,
  last_health_status: 'healthy',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
} as unknown as Provider;

const MODEL: Model = {
  id: 'model-uuid-1',
  model_id: 'gpt-5',
  display_name: 'GPT-5',
  capabilities: [],
  enabled: true,
} as unknown as Model;

const BREAKER_OPEN: CircuitBreakerStatus = {
  provider_id: 'prov-1',
  model: 'gpt-5',
  state: 'open',
  failures: 5,
  opened_at: '2026-01-01T00:00:00Z',
  next_probe_at: '2026-01-01T00:01:00Z',
} as unknown as CircuitBreakerStatus;

const propsDasar = {
  providers: [PROVIDER],
  models: [MODEL],
  rules: [],
  isBreakersLoading: false,
  breakersError: null,
  loadBreakers: vi.fn(async () => undefined),
  handleResetBreaker: vi.fn(async () => undefined),
};

describe('CircuitBreakersPanel — lencana terganggu saat refetch gagal (B-555)', () => {
  it('daftar basi + breakersError → lencana netral, BUKAN "{n} Sirkuit Terganggu" definitif', () => {
    render(
      <CircuitBreakersPanel
        {...propsDasar}
        breakers={[BREAKER_OPEN]}
        breakersError="status breaker tidak terjangkau"
      />,
    );
    expect(screen.queryByText(/Sirkuit Terganggu/)).not.toBeInTheDocument();
    expect(screen.getByText(/Status sirkuit tidak tersedia/i)).toBeInTheDocument();
    expect(screen.getByText(/status breaker tidak terjangkau/)).toBeInTheDocument();
  });

  it('data terukur → lencana "1 Sirkuit Terganggu" tampil seperti biasa', () => {
    render(<CircuitBreakersPanel {...propsDasar} breakers={[BREAKER_OPEN]} />);
    expect(screen.getByText(/1 Sirkuit Terganggu/)).toBeInTheDocument();
  });
});
