import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ProviderCard } from './ProviderCard';
import type { Provider } from '../../types';

const buatProvider = (overrides: Partial<Provider>): Provider => ({
  id: 'prov-1',
  name: 'openai-main',
  display_name: 'OpenAI Production',
  kind: 'openai',
  base_url: 'https://api.openai.com/v1',
  enabled: true,
  priority: 100,
  weight: 100,
  timeout_ms: 30000,
  max_retries: 2,
  consecutive_failures: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  ...overrides,
});

const handlers = {
  onOpenDrawer: vi.fn(),
  onProbe: vi.fn(),
  onQuickAddAccount: vi.fn(),
  onCopyBaseUrl: vi.fn(),
  onToggle: vi.fn(),
};

describe('ProviderCard (audit3 B-075 & B-007)', () => {
  it('gagalMuat menampilkan penanda gagal-muat, BUKAN klaim belum ada kredensial', () => {
    render(
      <ProviderCard
        provider={buatProvider({})}
        poolSummary={{ credentials: [], oauthSessions: [], gagalMuat: true }}
        isCustom={false}
        {...handlers}
      />
    );
    expect(screen.getByText('Ringkasan kredensial gagal dimuat')).toBeInTheDocument();
    expect(screen.queryByText('Belum ada kredensial')).toBeNull();
    expect(screen.queryByText(/Kredensial Aktif/)).toBeNull();
  });

  it('ringkasan kosong yang BENAR-BENAR terbaca tetap menampilkan Belum ada kredensial', () => {
    render(
      <ProviderCard
        provider={buatProvider({})}
        poolSummary={{ credentials: [], oauthSessions: [] }}
        isCustom={false}
        {...handlers}
      />
    );
    expect(screen.getByText('Belum ada kredensial')).toBeInTheDocument();
    expect(screen.queryByText('Ringkasan kredensial gagal dimuat')).toBeNull();
  });

  it('nama provider memakai <h3>, bukan <h4> (heading-order)', () => {
    const { container } = render(
      <ProviderCard
        provider={buatProvider({})}
        poolSummary={{ credentials: [], oauthSessions: [] }}
        isCustom={false}
        {...handlers}
      />
    );
    const judul = screen.getByRole('heading', { name: 'OpenAI Production' });
    expect(judul.tagName).toBe('H3');
    expect(container.querySelector('h4')).toBeNull();
  });
});

describe('ProviderCard — saklar enable/disable provider (B-086)', () => {
  it('provider aktif: saklar tercentang dan klik memanggil onToggle dengan provider itu', async () => {
    const onToggle = vi.fn();
    render(
      <ProviderCard
        provider={buatProvider({ enabled: true })}
        poolSummary={{ credentials: [], oauthSessions: [] }}
        isCustom={false}
        {...handlers}
        onToggle={onToggle}
      />
    );
    const saklar = screen.getByRole('switch', { name: 'Nonaktifkan provider OpenAI Production' });
    expect(saklar).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(saklar);
    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(onToggle.mock.calls[0][0]).toMatchObject({ id: 'prov-1', enabled: true });
  });

  it('provider nonaktif: saklar tidak tercentang dengan label Aktifkan', () => {
    render(
      <ProviderCard
        provider={buatProvider({ enabled: false })}
        poolSummary={{ credentials: [], oauthSessions: [] }}
        isCustom={false}
        {...handlers}
      />
    );
    const saklar = screen.getByRole('switch', { name: 'Aktifkan provider OpenAI Production' });
    expect(saklar).toHaveAttribute('aria-checked', 'false');
  });

  it('saklar nonaktif selama toggle berjalan (isToggling)', () => {
    render(
      <ProviderCard
        provider={buatProvider({ enabled: true })}
        poolSummary={{ credentials: [], oauthSessions: [] }}
        isCustom={false}
        isToggling
        {...handlers}
      />
    );
    expect(screen.getByRole('switch')).toBeDisabled();
  });

  // B-377: entri ringkasan yang belum tiba (query halaman induk
  // masih berjalan) menampilkan keadaan memuat — bukan klaim
  // "Belum ada kredensial" dan bukan penanda gagal-muat.
  it('memuat menampilkan keadaan memuat ringkasan, BUKAN klaim kosong/gagal', () => {
    render(
      <ProviderCard
        provider={buatProvider({})}
        poolSummary={{ credentials: [], oauthSessions: [], memuat: true }}
        isCustom={false}
        {...handlers}
      />
    );
    expect(screen.getByText('Memuat ringkasan kredensial…')).toBeInTheDocument();
    expect(screen.queryByText('Belum ada kredensial')).toBeNull();
    expect(screen.queryByText('Ringkasan kredensial gagal dimuat')).toBeNull();
  });
});
