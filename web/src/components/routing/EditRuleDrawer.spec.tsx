import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { EditRuleDrawer } from './EditRuleDrawer';
import type { Model, Provider, RoutingRule } from '../../types';

// Regresi audit3 V-FF-01 + V-FF-10:
// - V-FF-01: menyimpan edit aturan model_only yang terikat ke >= 2 provider
//   menghapus seluruh ikatannya (payload lama selalu provider_ids: [] dan
//   weights: {}), karena pilihan tunggal drawer tidak bisa menampilkan
//   ikatan multi-provider dan inisialisasinya menjadi ''.
// - V-FF-10: max_attempts selalu ditimpa 1 walau field-nya diisi lain.

const payloadStore = {
  update: null as Record<string, unknown> | null,
};

vi.mock('../../api/client', () => ({
  api: {
    routing: {
      update: vi.fn(async (_id: string, data: Record<string, unknown>) => {
        payloadStore.update = data;
        return { id: _id };
      }),
    },
  },
}));

vi.mock('../../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() },
    showToast: vi.fn(),
    confirmModal: vi.fn(async () => true),
  }),
}));

vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const MODEL: Model = {
  id: 'm-1',
  model_id: 'gpt-uji',
  display_name: 'GPT Uji',
  capabilities: [],
  enabled: true,
  routing_priority: 0,
  created_at: '',
  updated_at: '',
  providers: [
    {
      provider_id: 'prov-a',
      provider_name: 'Provider A',
      display_name: 'Provider A',
      upstream_model_name: 'gpt-uji-a',
    },
  ],
};

const PROVIDERS: Provider[] = [
  {
    id: 'prov-a', name: 'provider-a', display_name: 'Provider A', kind: 'openai',
    base_url: 'https://a.test', enabled: true, priority: 1, weight: 1,
    timeout_ms: 10000, max_retries: 2, consecutive_failures: 0, created_at: '', updated_at: '',
  },
  {
    id: 'prov-b', name: 'provider-b', display_name: 'Provider B', kind: 'openai',
    base_url: 'https://b.test', enabled: true, priority: 2, weight: 1,
    timeout_ms: 10000, max_retries: 2, consecutive_failures: 0, created_at: '', updated_at: '',
  },
];

const RULE: RoutingRule = {
  id: 'r-1',
  name: 'aturan-multi',
  description: '[model_only] aturan uji',
  priority: 100,
  match_model_id: 'm-1',
  match_capabilities: [],
  strategy: 'priority',
  max_attempts: 4,
  backoff_ms: 200,
  failure_threshold: 5,
  open_duration_ms: 30000,
  half_open_probes: 2,
  enabled: true,
  created_at: '',
  updated_at: '',
  providers: [
    { provider_id: 'prov-a', position: 0, weight: 5 },
    { provider_id: 'prov-b', position: 1, weight: 7 },
  ],
};

describe('EditRuleDrawer — ikatan provider & max_attempts (V-FF-01/V-FF-10)', () => {
  beforeEach(() => {
    payloadStore.update = null;
  });

  it('edit nama saja mempertahankan ikatan multi-provider, bobot, dan max_attempts', async () => {
    render(
      <EditRuleDrawer
        isOpen={true}
        onClose={vi.fn()}
        rule={RULE}
        models={[MODEL]}
        providers={PROVIDERS}
        onSuccess={vi.fn()}
      />
    );

    const nama = await screen.findByDisplayValue('aturan-multi');
    fireEvent.change(nama, { target: { value: 'aturan-multi-baru' } });

    fireEvent.click(screen.getByRole('button', { name: /simpan perubahan/i }));

    await waitFor(() => {
      expect(payloadStore.update).not.toBeNull();
    });
    expect(payloadStore.update).toMatchObject({
      name: 'aturan-multi-baru',
      provider_ids: ['prov-a', 'prov-b'],
      weights: { 'prov-a': 5, 'prov-b': 7 },
      max_attempts: 4,
    });
  });
});
