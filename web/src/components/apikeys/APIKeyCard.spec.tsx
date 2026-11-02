import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { APIKeyCard } from './APIKeyCard';
import { ToastProvider } from '../../context/ToastContext';
import type { APIKey } from '../../types';

const buatKunci = (overrides: Partial<APIKey>): APIKey => ({
  id: 'key-1',
  name: 'Kunci Uji',
  masked_key: 'sk-••••1234',
  scopes: [],
  allowed_models: [],
  allowed_providers: [],
  enabled: true,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  ...overrides,
});

describe('APIKeyCard (audit3 B-007 heading-order)', () => {
  it('nama kunci memakai <h3>, bukan <h4>', () => {
    const { container } = render(
      <ToastProvider>
        <APIKeyCard
          apiKey={buatKunci({})}
          onOpenAllowed={vi.fn()}
          onRotate={vi.fn()}
          onRevoke={vi.fn()}
        />
      </ToastProvider>
    );
    const judul = screen.getByRole('heading', { name: 'Kunci Uji' });
    expect(judul.tagName).toBe('H3');
    expect(container.querySelector('h4')).toBeNull();
  });
});
