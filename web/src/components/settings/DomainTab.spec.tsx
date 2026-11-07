import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DomainTab } from './DomainTab';
import type { DomainTabProps } from './types';
import type { DomainConfig } from '../../types';

// B-546: cabang basi DomainTab berpenanda PERSISTEN — muatan
// berhasil sebelumnya masih tampil (data menang), tetapi
// penyegaran terakhir yang gagal tidak boleh hanya tampil
// sebagai toast sesaat di halaman induk sementara lencana
// "Aktif & Terlindungi"/"DNS Terverifikasi" tetap definitif.

const CONFIG_AKTIF: DomainConfig = {
  domain: 'ai.contoh.id',
  mode: 'letsencrypt',
  status: 'active',
  public_url: 'https://ai.contoh.id',
  base_url: 'https://ai.contoh.id/v1',
  server_ip: '203.0.113.10',
  dns_matched: true,
};

const propsDasar = (sebagian: Partial<DomainTabProps>): DomainTabProps => ({
  domainConfig: null,
  inputDomain: '',
  inputMode: 'letsencrypt',
  isDomainLoading: false,
  isDomainSaving: false,
  isDomainDeleting: false,
  domainFeedback: null,
  domainError: null,
  serverIP: '203.0.113.10',
  setInputDomain: vi.fn(),
  setInputMode: vi.fn(),
  onRefresh: vi.fn(),
  onSaveDomain: vi.fn((e: React.FormEvent) => e.preventDefault()),
  onDeleteDomain: vi.fn(),
  ...sebagian,
});

describe('DomainTab — penanda cabang basi (B-546)', () => {
  it('konfigurasi ada + segarkan terakhir gagal → penanda basi PERSISTEN tampil di samping lencana definitif', () => {
    render(
      <DomainTab
        {...propsDasar({
          domainConfig: CONFIG_AKTIF,
          inputDomain: 'ai.contoh.id',
          domainError: 'jaringan putus',
        })}
      />,
    );
    expect(screen.getByText('Aktif & Terlindungi')).toBeInTheDocument();
    expect(
      screen.getByText(/Segarkan terakhir gagal: jaringan putus Status di bawah adalah data terakhir yang berhasil dimuat/),
    ).toBeInTheDocument();
  });

  it('konfigurasi ada + tanpa galat → tidak ada penanda basi', () => {
    render(
      <DomainTab
        {...propsDasar({ domainConfig: CONFIG_AKTIF, inputDomain: 'ai.contoh.id' })}
      />,
    );
    expect(screen.queryByText(/Segarkan terakhir gagal/)).not.toBeInTheDocument();
    expect(screen.queryByText(/belum dapat dimuat/)).not.toBeInTheDocument();
  });

  it('tanpa konfigurasi + galat → banner gagal-muat B-451 tetap yang tampil (bukan penanda basi)', () => {
    render(
      <DomainTab {...propsDasar({ domainConfig: null, domainError: 'jaringan putus' })} />,
    );
    expect(screen.getByText(/Konfigurasi domain belum dapat dimuat/)).toBeInTheDocument();
    expect(screen.queryByText(/Segarkan terakhir gagal/)).not.toBeInTheDocument();
  });
});
