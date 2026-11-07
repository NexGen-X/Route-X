import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { CircuitBreakersPanel } from './CircuitBreakersPanel';
import type { CircuitBreakerStatus, Model, Provider, RoutingRule } from '../../types';

// Fase 21 PR-E — B-511: gerbang label definitif panel breaker
// mencakup dua kasus terprobe Putaran Bersih #8 yang lolos dari
// gerbang B-491 (yang hanya menutup cabang konf == null):
//   1. Cabang sehat, rules kosong, modelsGagal → chip ambang
//      menampilkan "Bawaan Server" definitif.
//   2. Kartu breaker dengan rule khusus-model yang tak dapat
//      dicocokkan karena daftar model gagal → konfigurasi rule
//      generik tampil sebagai "Konfigurasi Efektif" definitif.

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

const propsDasar = {
  providers: [PROVIDER],
  isBreakersLoading: false,
  breakersError: null,
  loadBreakers: vi.fn(async () => undefined),
  handleResetBreaker: vi.fn(async () => undefined),
};

describe('CircuitBreakersPanel — gerbang modelsGagal (B-511)', () => {
  it('kasus 1: cabang sehat + modelsGagal → chip ambang netral, bukan Bawaan Server', () => {
    render(
      <CircuitBreakersPanel
        {...propsDasar}
        models={[MODEL]}
        breakers={[]}
        rules={[]}
        modelsGagal={true}
      />,
    );
    expect(screen.queryByText('Bawaan Server')).not.toBeInTheDocument();
    expect(screen.getByText('Ambang Isolasi')).toBeInTheDocument();
  });

  it('kasus 1 kontrol: cabang sehat terukur penuh → Bawaan Server tetap tampil', () => {
    render(
      <CircuitBreakersPanel {...propsDasar} models={[MODEL]} breakers={[]} rules={[]} />,
    );
    expect(screen.getByText('Bawaan Server')).toBeInTheDocument();
  });

  it('kasus 2: rule khusus-model tak terperiksa (modelsGagal) → konfigurasi efektif Belum terukur', () => {
    const generik = rule({ id: 'r-gen', name: 'Generik', priority: 5, failure_threshold: 9 });
    const khusus = rule({
      id: 'r-khus',
      name: 'Khusus Model',
      priority: 99,
      failure_threshold: 99,
      match_model_id: 'model-uuid-1',
    });
    render(
      <CircuitBreakersPanel
        {...propsDasar}
        models={[]}
        breakers={[BREAKER_OPEN]}
        rules={[generik, khusus]}
        modelsGagal={true}
      />,
    );
    expect(screen.getByText(/Belum terukur/)).toBeInTheDocument();
    expect(screen.queryByText(/Ambang 9x/)).not.toBeInTheDocument();
    expect(screen.queryByText(/dari rule "Generik"/)).not.toBeInTheDocument();
  });

  it('kasus 2 kontrol: daftar model terukur → rule khusus-model menang secara definitif', () => {
    const generik = rule({ id: 'r-gen', name: 'Generik', priority: 5, failure_threshold: 9 });
    const khusus = rule({
      id: 'r-khus',
      name: 'Khusus Model',
      priority: 99,
      failure_threshold: 99,
      match_model_id: 'model-uuid-1',
    });
    render(
      <CircuitBreakersPanel
        {...propsDasar}
        models={[MODEL]}
        breakers={[BREAKER_OPEN]}
        rules={[generik, khusus]}
      />,
    );
    expect(screen.getByText(/Ambang 99x/)).toBeInTheDocument();
    expect(screen.getByText(/dari rule "Khusus Model"/)).toBeInTheDocument();
  });
});
