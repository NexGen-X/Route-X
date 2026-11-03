import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
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
          onToggleEnabled={vi.fn()}
          onDelete={vi.fn()}
        />
      </ToastProvider>
    );
    const judul = screen.getByRole('heading', { name: 'Kunci Uji' });
    expect(judul.tagName).toBe('H3');
    expect(container.querySelector('h4')).toBeNull();
  });
});

describe('APIKeyCard (B-080 siklus hidup: aktif/nonaktif + hapus)', () => {
  const renderKartu = (apiKey: APIKey, props?: Partial<React.ComponentProps<typeof APIKeyCard>>) =>
    render(
      <ToastProvider>
        <APIKeyCard
          apiKey={apiKey}
          onOpenAllowed={vi.fn()}
          onRotate={vi.fn()}
          onToggleEnabled={vi.fn()}
          onDelete={vi.fn()}
          {...props}
        />
      </ToastProvider>
    );

  it('kunci aktif menampilkan Nonaktifkan, dan tidak ada lagi tombol Cabut', () => {
    renderKartu(buatKunci({ enabled: true }));
    expect(screen.getByRole('button', { name: /Nonaktifkan kunci API/ })).toBeTruthy();
    expect(screen.queryByRole('button', { name: /Cabut/ })).toBeNull();
    expect(screen.getByRole('button', { name: /Hapus kunci API/ })).toBeTruthy();
  });

  it('kunci nonaktif menampilkan Aktifkan', () => {
    renderKartu(buatKunci({ enabled: false }));
    expect(screen.getByRole('button', { name: /Aktifkan kunci API/ })).toBeTruthy();
  });

  it('klik Nonaktifkan dan Hapus meneruskan kunci ke handler halaman', () => {
    const onToggleEnabled = vi.fn();
    const onDelete = vi.fn();
    const kunci = buatKunci({ enabled: true });
    renderKartu(kunci, { onToggleEnabled, onDelete });

    fireEvent.click(screen.getByRole('button', { name: /Nonaktifkan kunci API/ }));
    expect(onToggleEnabled).toHaveBeenCalledWith(kunci);

    fireEvent.click(screen.getByRole('button', { name: /Hapus kunci API/ }));
    expect(onDelete).toHaveBeenCalledWith(kunci);
  });
});
