import React, { useState } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { GlobalErrorBoundary } from './GlobalErrorBoundary';
import { WidgetErrorBoundary } from './WidgetErrorBoundary';
import { PageErrorBoundary } from './PageErrorBoundary';

// Komponen sengaja melempar error untuk pengujian boundary
const Bomb: React.FC<{ shouldThrow?: boolean; message?: string }> = ({
  shouldThrow = true,
  message = 'Sengaja crash untuk pengujian',
}) => {
  if (shouldThrow) {
    throw new Error(message);
  }
  return <div data-testid="bomb-safe">Komponen Berjalan Normal</div>;
};

describe('Frontend Error Boundary & Fallback Suite', () => {
  // Matikan console.error & console.warn sementara agar tidak mengotori log tes
  const originalError = console.error;
  const originalWarn = console.warn;

  beforeEach(() => {
    console.error = vi.fn();
    console.warn = vi.fn();
  });

  afterEach(() => {
    console.error = originalError;
    console.warn = originalWarn;
    vi.restoreAllMocks();
  });

  describe('GlobalErrorBoundary', () => {
    it('merender children saat tidak terjadi kesalahan', () => {
      render(
        <GlobalErrorBoundary>
          <div data-testid="child-content">Konten Aplikasi Sehat</div>
        </GlobalErrorBoundary>
      );

      expect(screen.getByTestId('child-content')).toBeInTheDocument();
      expect(screen.getByText('Konten Aplikasi Sehat')).toBeInTheDocument();
    });

    it('menangkap error uncaught dan menampilkan kartu pemulihan tanpa crash WSoD', () => {
      render(
        <GlobalErrorBoundary>
          <Bomb message="Database upstream 500 internal server error" />
        </GlobalErrorBoundary>
      );

      const alert = screen.getByRole('alert');
      expect(alert).toBeInTheDocument();
      expect(screen.getByText('Gagal Memuat Antarmuka Route-X')).toBeInTheDocument();
      expect(screen.getByText('Database upstream 500 internal server error')).toBeInTheDocument();
      expect(screen.getByText('Muat Ulang Halaman')).toBeInTheDocument();
      expect(screen.getByText('Coba Pulihkan')).toBeInTheDocument();
      expect(screen.getByText('Kembali ke Beranda')).toBeInTheDocument();
    });

    it('menampilkan panduan khusus saat mendeteksi kegagalan chunk load Vite', () => {
      render(
        <GlobalErrorBoundary>
          <Bomb message="Failed to fetch dynamically imported module: /assets/Dashboard-123.js" />
        </GlobalErrorBoundary>
      );

      expect(screen.getByText('Pembaruan Modul Sistem Terdeteksi')).toBeInTheDocument();
      expect(screen.getByText(/Versi frontend baru telah dideploy/i)).toBeInTheDocument();
    });

    it('dapat mencoba pemulihan state saat tombol Coba Pulihkan ditekan', () => {
      const RecoverableContainer = () => {
        const [hasError, setHasError] = useState(true);
        return (
          <GlobalErrorBoundary>
            {hasError ? (
              <Bomb message="Kesalahan sementara" />
            ) : (
              <div data-testid="recovered-content">Berhasil Pulih</div>
            )}
            <button data-testid="fix-btn" onClick={() => setHasError(false)}>
              Perbaiki
            </button>
          </GlobalErrorBoundary>
        );
      };

      render(<RecoverableContainer />);
      expect(screen.getByText('Gagal Memuat Antarmuka Route-X')).toBeInTheDocument();

      // Klik pulihkan
      const retryBtn = screen.getByRole('button', { name: /Coba pulihkan/i });
      fireEvent.click(retryBtn);
    });
  });

  describe('WidgetErrorBoundary', () => {
    it('merender konten widget dengan normal tanpa error', () => {
      render(
        <WidgetErrorBoundary title="Grafik Latensi">
          <div data-testid="widget-normal">Data Latensi 12ms</div>
        </WidgetErrorBoundary>
      );

      expect(screen.getByTestId('widget-normal')).toBeInTheDocument();
    });

    it('mengisolasi kegagalan satu widget sehingga tidak merusak sisa halaman', () => {
      render(
        <div>
          <div data-testid="unaffected-part">Bagian Halaman Lain Tetap Aman</div>
          <WidgetErrorBoundary title="Grafik Analitik">
            <Bomb message="Recharts SVG NaN calculation failure" />
          </WidgetErrorBoundary>
        </div>
      );

      // Bagian lain tetap muncul
      expect(screen.getByTestId('unaffected-part')).toBeInTheDocument();

      // Widget menampilkan fallback lokal
      expect(screen.getByText('Grafik Analitik Gagal Dimuat')).toBeInTheDocument();
      expect(screen.getByText(/Recharts SVG NaN calculation failure/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /Coba muat ulang Grafik Analitik/i })).toBeInTheDocument();
    });

    it('memanggil handler onRetry saat tombol Coba Lagi diklik', () => {
      const onRetryMock = vi.fn();
      render(
        <WidgetErrorBoundary title="Feed Permintaan" onRetry={onRetryMock}>
          <Bomb message="Koneksi WebSocket terputus" />
        </WidgetErrorBoundary>
      );

      const retryBtn = screen.getByRole('button', { name: /Coba muat ulang Feed Permintaan/i });
      fireEvent.click(retryBtn);
      expect(onRetryMock).toHaveBeenCalledTimes(1);
    });
  });

  describe('PageErrorBoundary', () => {
    it('merender halaman normal saat tidak ada error', () => {
      render(
        <PageErrorBoundary pageName="Observabilitas">
          <div data-testid="page-content">Halaman Observabilitas</div>
        </PageErrorBoundary>
      );

      expect(screen.getByTestId('page-content')).toBeInTheDocument();
    });

    it('menangkap error level halaman dan menyediakan tombol aksi aksesibel', () => {
      render(
        <PageErrorBoundary pageName="Routing Rules">
          <Bomb message="Gagal sinkronisasi upstream rules" />
        </PageErrorBoundary>
      );

      expect(screen.getByRole('alert')).toBeInTheDocument();
      expect(screen.getByText('Gagal Memuat Halaman Routing Rules')).toBeInTheDocument();
      expect(screen.getByText('Gagal sinkronisasi upstream rules')).toBeInTheDocument();

      const retryBtn = screen.getByRole('button', { name: /Coba muat ulang halaman Routing Rules/i });
      expect(retryBtn).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /Muat ulang browser/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /Kembali ke Dashboard/i })).toBeInTheDocument();
    });

    it('mereset status error secara otomatis ketika berpindah rute (pageName berubah)', () => {
      const RoutingContainer = ({ page }: { page: string }) => (
        <PageErrorBoundary pageName={page}>
          {page === 'Halaman Rusak' ? (
            <Bomb message="Error halaman rusak" />
          ) : (
            <div data-testid="halaman-sehat">Selamat datang di {page}</div>
          )}
        </PageErrorBoundary>
      );

      const { rerender } = render(<RoutingContainer page="Halaman Rusak" />);
      expect(screen.getByText('Gagal Memuat Halaman Halaman Rusak')).toBeInTheDocument();

      // Pindah rute ke Halaman Sehat
      rerender(<RoutingContainer page="Halaman Sehat" />);
      expect(screen.getByTestId('halaman-sehat')).toBeInTheDocument();
      expect(screen.getByText('Selamat datang di Halaman Sehat')).toBeInTheDocument();
    });
  });
});
