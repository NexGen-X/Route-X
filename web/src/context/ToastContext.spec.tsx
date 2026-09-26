import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';
import { ToastProvider, useToast } from './ToastContext';

const TestConsumer: React.FC = () => {
  const { toast, confirmModal } = useToast();
  return (
    <div>
      <button onClick={() => toast.success('Operasi berhasil!', 'Sukses')}>Trigger Success</button>
      <button onClick={() => toast.error('Operasi gagal!', 'Gagal')}>Trigger Error</button>
      <button
        onClick={async () => {
          await confirmModal({
            title: 'Konfirmasi Aksi',
            message: 'Yakin ingin melanjutkan?',
          });
        }}
      >
        Trigger Confirm
      </button>
    </div>
  );
};

describe('ToastContext & ToastNotificationCard', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  it('merender kontainer dengan z-[9999] dan mobile safe-area', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );

    const container = screen.getByTestId('toast-container');
    expect(container).toBeInTheDocument();
    expect(container).toHaveClass('z-[9999]');
    expect(container?.className).toContain('top-[max(1rem,env(safe-area-inset-top))]');
  });

  it('menampilkan notifikasi toast dengan benar dan auto-dismiss setelah durasi waktu', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );

    const triggerBtn = screen.getByText('Trigger Success');
    fireEvent.click(triggerBtn);

    expect(screen.getByText('Operasi berhasil!')).toBeInTheDocument();
    expect(screen.getByText('Sukses')).toBeInTheDocument();

    // Maju 4000ms
    act(() => {
      vi.advanceTimersByTime(4000);
    });

    expect(screen.queryByText('Operasi berhasil!')).not.toBeInTheDocument();
  });

  it('menghentikan (pause) timer countdown saat cursor di-hover dan melanjutkan saat mouse leave', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Trigger Error'));
    const toastEl = screen.getByText('Operasi gagal!').closest('[role="alert"]') as HTMLElement;
    expect(toastEl).toBeInTheDocument();

    // Maju 2000ms (separuh durasi)
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByText('Operasi gagal!')).toBeInTheDocument();

    // Hover (onMouseEnter)
    fireEvent.mouseEnter(toastEl);

    // Maju 5000ms saat di-hover: toast TIDAK boleh hilang
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(screen.getByText('Operasi gagal!')).toBeInTheDocument();

    // Lepas hover (onMouseLeave)
    fireEvent.mouseLeave(toastEl);

    // Maju sisa waktu ~2000ms: toast sekarang harus hilang
    act(() => {
      vi.advanceTimersByTime(2100);
    });
    expect(screen.queryByText('Operasi gagal!')).not.toBeInTheDocument();
  });

  it('menghapus notifikasi saat tombol X diklik', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Trigger Success'));
    expect(screen.getByText('Operasi berhasil!')).toBeInTheDocument();

    const closeBtn = screen.getByRole('button', { name: 'Tutup notifikasi' });
    fireEvent.click(closeBtn);

    expect(screen.queryByText('Operasi berhasil!')).not.toBeInTheDocument();
  });
});
