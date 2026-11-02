import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { LiveRequestsFeed } from './LiveRequestsFeed';
import type { RequestLog } from '../../types';

const buatRequest = (overrides: Partial<RequestLog>): RequestLog => ({
  id: 'req-1',
  request_id: 'req-1',
  model_id: 'gpt-5',
  requested_model: 'gpt-5',
  status_code: 200,
  duration_ms: 120,
  prompt_tokens: 10,
  completion_tokens: 5,
  total_tokens: 15,
  cost_usd: '0.001',
  client_ip: '127.0.0.1',
  is_stream: false,
  created_at: new Date().toISOString(),
  ...overrides,
});

describe('LiveRequestsFeed (audit3 B-006 nested-interactive)', () => {
  it('setiap baris adalah SATU <button> asli tanpa kontrol interaktif bersarang', () => {
    const onNavigate = vi.fn();
    const { container } = render(
      <LiveRequestsFeed
        recentRequests={[
          buatRequest({ id: 'a', request_id: 'a', requested_model: 'gpt-5' }),
          buatRequest({ id: 'b', request_id: 'b', requested_model: 'claude-x', status_code: 500 }),
        ]}
        onNavigate={onNavigate}
      />
    );

    const barisA = screen.getByRole('button', { name: /Inspeksi request gpt-5/ });
    const barisB = screen.getByRole('button', { name: /Inspeksi request claude-x/ });
    expect(barisA.tagName).toBe('BUTTON');
    expect(barisB.tagName).toBe('BUTTON');

    // Tidak ada elemen role="button" non-tombol (pola lama div role=button)
    // dan tidak ada kontrol interaktif apa pun di dalam baris.
    expect(container.querySelectorAll('[role="button"]')).toHaveLength(0);
    for (const baris of [barisA, barisB]) {
      expect(baris.querySelectorAll('button, a, input, select, textarea, [tabindex]')).toHaveLength(0);
    }
  });

  it('klik baris tetap membuka halaman inspeksi request', () => {
    const onNavigate = vi.fn();
    render(
      <LiveRequestsFeed
        recentRequests={[buatRequest({})]}
        onNavigate={onNavigate}
      />
    );
    fireEvent.click(screen.getByRole('button', { name: /Inspeksi request gpt-5/ }));
    expect(onNavigate).toHaveBeenCalledWith('/requests');
  });
});
