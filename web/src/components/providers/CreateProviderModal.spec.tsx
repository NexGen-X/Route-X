import React from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { CreateProviderModal } from './CreateProviderModal';
import type { api as ApiClient } from '../../api/client';

// B-488: modal buat provider menerima gerbang galat sumbernya —
// saat daftar provider/pool gagal dimuat, modal tidak boleh
// bekerja atas daftar kosong-palsu (validasi nama ganda dan
// pencocokan pool belum terukur): banner tampil dan simpan mati.

vi.mock('focus-trap-react', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

// Modal memanggil useToast sendiri (di luar prop toast).
vi.mock('../../context/ToastContext', () => ({
  useToast: () => ({
    toast: { success: vi.fn(), error: vi.fn(), warn: vi.fn(), info: vi.fn() },
    confirmModal: vi.fn(async () => true),
  }),
}));

const toastStub = {
  success: vi.fn(),
  error: vi.fn(),
  warn: vi.fn(),
  info: vi.fn(),
};

function renderModal(tambahan: Partial<React.ComponentProps<typeof CreateProviderModal>> = {}) {
  return render(
    <CreateProviderModal
      isOpen={true}
      onClose={vi.fn()}
      selectedPreset={null}
      providers={[]}
      egressPools={[]}
      loadData={vi.fn(async () => undefined)}
      handleOpenDrawer={vi.fn()}
      toast={toastStub}
      api={{} as unknown as typeof ApiClient}
      {...tambahan}
    />
  );
}

describe('CreateProviderModal — gerbang galat sumber (B-488)', () => {
  it('daftar provider gagal → banner belum-terukur tampil dan simpan nonaktif', () => {
    renderModal({ isProvidersError: true });
    expect(screen.getByText(/pemeriksaan nama ganda dan pilihan pool di bawah belum terukur/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Daftarkan Provider/ })).toBeDisabled();
  });

  it('daftar pool gagal → simpan nonaktif juga', () => {
    renderModal({ isPoolsError: true });
    expect(screen.getByText(/daftar egress pool/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Daftarkan Provider/ })).toBeDisabled();
  });

  it('kedua sumber sehat → tidak ada banner dan simpan aktif', () => {
    renderModal();
    expect(screen.queryByText(/belum terukur, bukan daftar kosong/)).toBeNull();
    expect(screen.getByRole('button', { name: /Daftarkan Provider/ })).not.toBeDisabled();
  });
});
