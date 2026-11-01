import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import {
  CircuitBreakersPanel,
  konfigurasiEfektif,
  labelDurasiBuka,
} from './CircuitBreakersPanel';
import type { CircuitBreakerStatus, Model, Provider, RoutingRule } from '../../types';

// Panel breaker diperkaya (Fase 2 R2): gema konfigurasi efektif dari rule
// yang sudah dimuat halaman Routing, hitung mundur probe, dan banner
// sehat yang tidak lagi mengarang angka ambang.

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

function rule(partial: Partial<RoutingRule>): RoutingRule {
  return {
    id: 'rule-x',
    name: 'Rule',
    priority: 10,
    match_capabilities: [],
    strategy: 'priority',
    max_attempts: 3,
    backoff_ms: 200,
    failure_threshold: 5,
    open_duration_ms: 60000,
    half_open_probes: 2,
    enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...partial,
  } as unknown as RoutingRule;
}

const BREAKER_OPEN: CircuitBreakerStatus = {
  provider_id: 'prov-1',
  model: 'gpt-5',
  state: 'open',
  failure_count: 7,
  next_probe_at: new Date(Date.now() + 45000).toISOString(),
};

describe('konfigurasiEfektif', () => {
  it('memilih rule aktif prioritas tertinggi yang mencakup provider', () => {
    const rendah = rule({ id: 'r1', name: 'Rendah', priority: 5, failure_threshold: 3 });
    const tinggi = rule({
      id: 'r2',
      name: 'Tinggi',
      priority: 50,
      failure_threshold: 9,
      providers: [{ provider_id: 'prov-1' } as never],
    });
    const hasil = konfigurasiEfektif(BREAKER_OPEN, [rendah, tinggi], [MODEL]);
    expect(hasil?.rule.name).toBe('Tinggi');
    expect(hasil?.jumlahRuleLain).toBe(1);
  });

  it('mengabaikan rule nonaktif dan rule khusus model lain', () => {
    const nonaktif = rule({ id: 'r1', name: 'Nonaktif', enabled: false, priority: 99 });
    const modelLain = rule({
      id: 'r2',
      name: 'Model Lain',
      priority: 90,
      match_model_id: 'model-uuid-1',
    });
    const breakerSemuaModel: CircuitBreakerStatus = { ...BREAKER_OPEN, model: '' };
    expect(konfigurasiEfektif(breakerSemuaModel, [nonaktif, modelLain], [MODEL])).toBeNull();
    // Untuk breaker gpt-5, rule khusus model itu berlaku.
    expect(konfigurasiEfektif(BREAKER_OPEN, [modelLain], [MODEL])?.rule.name).toBe('Model Lain');
  });

  it('labelDurasiBuka memformat milidetik menjadi label ringkas', () => {
    expect(labelDurasiBuka(500)).toBe('500 ms');
    expect(labelDurasiBuka(30000)).toBe('30 dtk');
    expect(labelDurasiBuka(60000)).toBe('1 mnt');
    expect(labelDurasiBuka(90000)).toBe('1.5 mnt');
  });
});

describe('CircuitBreakersPanel — tampilan diperkaya', () => {
  const propsDasar = {
    providers: [PROVIDER],
    models: [MODEL],
    isBreakersLoading: false,
    breakersError: null,
    loadBreakers: vi.fn(async () => undefined),
    handleResetBreaker: vi.fn(async () => undefined),
  };

  it('menampilkan gema konfigurasi efektif dan hitung mundur probe pada kartu open', () => {
    const r = rule({ name: 'Utama', failure_threshold: 9, open_duration_ms: 120000, half_open_probes: 3 });
    render(
      <CircuitBreakersPanel {...propsDasar} breakers={[BREAKER_OPEN]} rules={[r]} />,
    );
    expect(screen.getByText(/Ambang 9x · jeda 2 mnt · probe 3x/)).toBeInTheDocument();
    expect(screen.getByText(/dari rule "Utama"/)).toBeInTheDocument();
    expect(screen.getByText(/dtk lagi\)/)).toBeInTheDocument();
  });

  it('banner sehat menampilkan ambang dari rule global, bukan angka statis', () => {
    const r = rule({ name: 'Global', failure_threshold: 8 });
    render(<CircuitBreakersPanel {...propsDasar} breakers={[]} rules={[r]} />);
    expect(screen.getByText('8x Gagal')).toBeInTheDocument();
  });

  it('banner sehat tanpa rule global menampilkan Bawaan Server', () => {
    render(<CircuitBreakersPanel {...propsDasar} breakers={[]} rules={[]} />);
    expect(screen.getByText('Bawaan Server')).toBeInTheDocument();
  });
});
