import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { CircuitBreakersPanel } from './CircuitBreakersPanel';
import type { CircuitBreakerStatus, Model, Provider, RoutingRule } from '../../types';

// Fase 22 PR-E — B-518: cabang daftar BASI panel breaker masuk
// gerbang keraguan. B-511 menutup cabang daftar model/rule yang
// KOSONG saat sumbernya gagal; Putaran Bersih #9 membuktikan
// cabang BASI: muat-ulang gagal TIDAK mengosongkan daftar sukses
// sebelumnya (ia bertahan di state) sementara rulesGagal terisi —
// panel tetap menampilkan angka definitif dari data basi di dua
// permukaan: chip "Ambang Isolasi" dan kartu "Konfigurasi Efektif".

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

describe('CircuitBreakersPanel — gerbang rulesGagal cabang basi (B-518)', () => {
  it('kasus chip: rule global basi + rulesGagal → chip netral, bukan angka definitif', () => {
    const globalBasi = rule({ id: 'r-glob', name: 'Global Basi', priority: 99, failure_threshold: 9 });
    render(
      <CircuitBreakersPanel
        {...propsDasar}
        models={[MODEL]}
        breakers={[]}
        rules={[globalBasi]}
        rulesGagal={true}
      />,
    );
    expect(screen.queryByText(/9x Gagal/)).not.toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('kasus chip kontrol: rule global segar → angka definitif tetap tampil', () => {
    const globalSegar = rule({ id: 'r-glob', name: 'Global Segar', priority: 99, failure_threshold: 9 });
    render(
      <CircuitBreakersPanel {...propsDasar} models={[MODEL]} breakers={[]} rules={[globalSegar]} />,
    );
    expect(screen.getByText(/9x Gagal/)).toBeInTheDocument();
  });

  it('kasus kartu: konf dari rule basi + rulesGagal → Belum terukur, bukan definitif', () => {
    const generikBasi = rule({ id: 'r-gen', name: 'Generik', priority: 5, failure_threshold: 9 });
    render(
      <CircuitBreakersPanel
        {...propsDasar}
        models={[MODEL]}
        breakers={[BREAKER_OPEN]}
        rules={[generikBasi]}
        rulesGagal={true}
      />,
    );
    expect(screen.getByText(/Belum terukur/)).toBeInTheDocument();
    expect(screen.queryByText(/Ambang 9x/)).not.toBeInTheDocument();
    expect(screen.queryByText(/dari rule "Generik"/)).not.toBeInTheDocument();
  });

  it('kasus kartu kontrol: rule segar → konfigurasi efektif definitif tetap tampil', () => {
    const generikSegar = rule({ id: 'r-gen', name: 'Generik', priority: 5, failure_threshold: 9 });
    render(
      <CircuitBreakersPanel
        {...propsDasar}
        models={[MODEL]}
        breakers={[BREAKER_OPEN]}
        rules={[generikSegar]}
      />,
    );
    expect(screen.getByText(/Ambang 9x/)).toBeInTheDocument();
    expect(screen.getByText(/dari rule "Generik"/)).toBeInTheDocument();
  });
});
