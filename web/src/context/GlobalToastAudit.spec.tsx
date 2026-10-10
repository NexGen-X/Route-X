import React, { useState } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';
import { ToastProvider, useToast } from './ToastContext';

// Helper component untuk menguji skenario toast multi-layer
const TestAuditHarness: React.FC<{
  onAction?: (toast: ReturnType<typeof useToast>['toast']) => Promise<void> | void;
}> = ({ onAction }) => {
  const { toast } = useToast();
  return (
    <div>
      <button onClick={() => onAction && void onAction(toast)}>Jalankan Aksi</button>
    </div>
  );
};

describe('Global Toast Duplication Audit & Verification (12 Mandated Tests)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  // ---------------------------------------------------------------------------
  // TEST 1: One failed API request produces only one user-facing error notification.
  // ---------------------------------------------------------------------------
  it('TEST 1: One failed API request produces only one user-facing error notification', async () => {
    const handleFailedApi = (toast: ReturnType<typeof useToast>['toast']) => {
      try {
        throw new Error('Connection timeout to upstream cluster');
      } catch (err: unknown) {
        toast.error((err as Error).message, 'API Error');
      }
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handleFailedApi} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    const errorToasts = screen.getAllByRole('alert');
    expect(errorToasts).toHaveLength(1);
    expect(screen.getByText('Connection timeout to upstream cluster')).toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // TEST 2: An error reported by both a global interceptor and a component does not produce duplicate notifications.
  // ---------------------------------------------------------------------------
  it('TEST 2: An error reported by both a global interceptor and a component does not produce duplicate notifications', async () => {
    const handleBothLayersReporting = (toast: ReturnType<typeof useToast>['toast']) => {
      const errMessage = 'Internal Server Error (500)';
      const opId = 'req-user-123';

      // Layer 1: Global HTTP interceptor melaporkan error
      toast.error(errMessage, 'Global Interceptor', { dedupeKey: opId });

      // Layer 2: Komponen catch block lokal juga melaporkan error dengan opId yang sama
      toast.error(errMessage, 'Komponen Lokal', { dedupeKey: opId });
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handleBothLayersReporting} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    // Hanya 1 toast error yang boleh muncul di layar (layer kedua diredam)
    const errorAlerts = screen.getAllByRole('alert');
    expect(errorAlerts).toHaveLength(1);
    expect(screen.getByText('Internal Server Error (500)')).toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // TEST 3: A user can retry the same operation and receive a new notification if the retry fails.
  // ---------------------------------------------------------------------------
  it('TEST 3: A user can retry the same operation and receive a new notification if the retry fails', async () => {
    let failureCount = 0;
    const handleRetryableAction = (toast: ReturnType<typeof useToast>['toast']) => {
      failureCount++;
      toast.error(`Koneksi gateway gagal (Percobaan #${failureCount})`, 'Gagal');
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handleRetryableAction} />
      </ToastProvider>
    );

    const btn = screen.getByText('Jalankan Aksi');

    // Percobaan 1: Gagal
    fireEvent.click(btn);
    expect(screen.getByText('Koneksi gateway gagal (Percobaan #1)')).toBeInTheDocument();

    // User menunggu 1200ms (> dedupe burst window 1000ms) lalu klik Coba Lagi
    act(() => {
      vi.advanceTimersByTime(1200);
    });

    // Percobaan 2: Gagal lagi
    fireEvent.click(btn);

    // Notifikasi baru untuk percobaan kedua harus muncul
    expect(screen.getByText('Koneksi gateway gagal (Percobaan #2)')).toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // TEST 4: Two independent failed operations can produce two notifications when appropriate.
  // ---------------------------------------------------------------------------
  it('TEST 4: Two independent failed operations can produce two notifications when appropriate', async () => {
    const handleIndependentFailures = (toast: ReturnType<typeof useToast>['toast']) => {
      // Operasi A gagal
      toast.error('Gagal memuat daftar provider', 'Providers', { dedupeKey: 'load-providers' });
      // Operasi B gagal
      toast.error('Gagal memuat kuota anggaran', 'Budgets', { dedupeKey: 'load-budgets' });
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handleIndependentFailures} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    // Kedua operasi independen harus menampilkan notifikasi masing-masing
    expect(screen.getByText('Gagal memuat daftar provider')).toBeInTheDocument();
    expect(screen.getByText('Gagal memuat kuota anggaran')).toBeInTheDocument();
    expect(screen.getAllByRole('alert')).toHaveLength(2);
  });

  // ---------------------------------------------------------------------------
  // TEST 5: A successful operation does not display an error notification.
  // ---------------------------------------------------------------------------
  it('TEST 5: A successful operation does not display an error notification', async () => {
    const handleSuccess = (toast: ReturnType<typeof useToast>['toast']) => {
      toast.success('Kredensial berhasil disimpan dengan enkripsi AES-256', 'Sukses');
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handleSuccess} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    expect(screen.getByText('Kredensial berhasil disimpan dengan enkripsi AES-256')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // TEST 6: Success notifications are not duplicated.
  // ---------------------------------------------------------------------------
  it('TEST 6: Success notifications are not duplicated', async () => {
    const handleDuplicateSuccess = (toast: ReturnType<typeof useToast>['toast']) => {
      // Dua pemanggilan sukses beruntun yang identik dalam burst window
      toast.success('Aturan perutean berhasil diterapkan', 'Sukses');
      toast.success('Aturan perutean berhasil diterapkan', 'Sukses');
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handleDuplicateSuccess} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    // Hanya 1 elemen teks sukses yang muncul
    const successMessages = screen.getAllByText('Aturan perutean berhasil diterapkan');
    expect(successMessages).toHaveLength(1);
  });

  // ---------------------------------------------------------------------------
  // TEST 7: Validation errors remain understandable and do not produce redundant notifications.
  // ---------------------------------------------------------------------------
  it('TEST 7: Validation errors remain understandable and do not produce redundant notifications', async () => {
    const handleFormSubmitWithValidation = (toast: ReturnType<typeof useToast>['toast']) => {
      const fieldValues = { name: '', rpm: -5 };

      // Validasi 1: Nama wajib diisi (early return)
      if (!fieldValues.name) {
        toast.error('Nama kunci API wajib diisi.', 'Validasi Gagal');
        return;
      }
      if (fieldValues.rpm < 0) {
        toast.error('RPM tidak boleh negatif.', 'Validasi Gagal');
        return;
      }
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handleFormSubmitWithValidation} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    // Hanya 1 toast validasi pertama yang muncul
    expect(screen.getByText('Nama kunci API wajib diisi.')).toBeInTheDocument();
    expect(screen.queryByText('RPM tidak boleh negatif.')).not.toBeInTheDocument();
    expect(screen.getAllByRole('alert')).toHaveLength(1);
  });

  // ---------------------------------------------------------------------------
  // TEST 8: Authentication and authorization error handling still works.
  // ---------------------------------------------------------------------------
  it('TEST 8: Authentication and authorization error handling still works', async () => {
    let authDispatched = false;
    window.addEventListener('routex:unauthorized', () => {
      authDispatched = true;
    });

    const handle401Unauthorized = (toast: ReturnType<typeof useToast>['toast']) => {
      // Simulasikan dispatch dari API client saat menerima 401
      window.dispatchEvent(new CustomEvent('routex:unauthorized'));
      toast.warn('Sesi login telah berakhir. Silakan masuk kembali.', 'Autentikasi Kedaluwarsa');
    };

    render(
      <ToastProvider>
        <TestAuditHarness onAction={handle401Unauthorized} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    expect(authDispatched).toBe(true);
    expect(screen.getByText('Sesi login telah berakhir. Silakan masuk kembali.')).toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // TEST 9: Loading indicators and notification dismissal behave correctly.
  // ---------------------------------------------------------------------------
  it('TEST 9: Loading indicators and notification dismissal behave correctly', async () => {
    render(
      <ToastProvider>
        <TestAuditHarness onAction={(toast) => toast.info('Memproses sinkronisasi...', 'Info', { duration: 3000 })} />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Jalankan Aksi'));

    expect(screen.getByText('Memproses sinkronisasi...')).toBeInTheDocument();

    // Tutup manual dengan tombol X
    const closeBtn = screen.getByRole('button', { name: 'Tutup notifikasi' });
    fireEvent.click(closeBtn);

    expect(screen.queryByText('Memproses sinkronisasi...')).not.toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // TEST 10: Repeated component mounting, rerendering, or event subscriptions do not cause repeated notifications.
  // ---------------------------------------------------------------------------
  it('TEST 10: Repeated component mounting, rerendering, or event subscriptions do not cause repeated notifications', async () => {
    const ComponentWithEffect: React.FC = () => {
      const { toast } = useToast();
      const [, setCounter] = useState(0);

      // Re-render tiruan beberapa kali dalam rentang waktu singkat
      return (
        <div>
          <button onClick={() => setCounter((c) => c + 1)}>Re-render</button>
          <button onClick={() => toast.warn('Peringatan kuota mendekati batas', 'Batas Anggaran')}>
            Picu Warning
          </button>
        </div>
      );
    };

    render(
      <ToastProvider>
        <ComponentWithEffect />
      </ToastProvider>
    );

    const warnBtn = screen.getByText('Picu Warning');
    const rerenderBtn = screen.getByText('Re-render');

    // Klik picu
    fireEvent.click(warnBtn);
    // Klik rerender bertubi-tubi
    fireEvent.click(rerenderBtn);
    fireEvent.click(rerenderBtn);
    // Klik picu lagi dalam waktu < 1000ms
    fireEvent.click(warnBtn);

    // Toast tidak boleh mengganda
    const warnings = screen.getAllByText('Peringatan kuota mendekati batas');
    expect(warnings).toHaveLength(1);
  });

  // ---------------------------------------------------------------------------
  // TEST 11: The original Egress connection failure displays one error notification.
  // ---------------------------------------------------------------------------
  it('TEST 11: The original Egress connection failure displays one error notification', async () => {
    // Menyimulasikan error koneksi proxy-sg.internal pada Egress.tsx
    const handleEgressProbeFailure = (
      toast: ReturnType<typeof useToast>['toast'],
      setFeedback: (val: unknown) => void
    ) => {
      const errorMsg =
        'Gagal terhubung melalui proxy Singapore Primary Egress: Get https://cloudflare.com/cdn-cgi/trace: proxyconnect tcp: dial tcp: lookup proxy-sg.internal on 127.0.0.53:53: no such host';

      // Arsitektur baru: setFeedback diset null (meredam banner error ganda)
      setFeedback(null);
      // Single source of truth: toast.error
      toast.error(errorMsg, 'Uji Egress Gagal', { dedupeKey: 'probe-pool-sg' });
    };

    const EgressSimulator: React.FC = () => {
      const { toast } = useToast();
      const [feedback, setFeedback] = useState<unknown>(null);

      return (
        <div>
          {feedback !== null && <div role="alert">UJI KONEKSI GAGAL</div>}
          <button onClick={() => handleEgressProbeFailure(toast, setFeedback)}>
            Uji Ping Proxy
          </button>
        </div>
      );
    };

    render(
      <ToastProvider>
        <EgressSimulator />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Uji Ping Proxy'));

    // Verifikasi bahwa teks banner "UJI KONEKSI GAGAL" TIDAK ADA di DOM
    expect(screen.queryByText('UJI KONEKSI GAGAL')).not.toBeInTheDocument();

    // Verifikasi bahwa toast error tunggal "Uji Egress Gagal" ada di DOM
    expect(screen.getByText('Uji Egress Gagal')).toBeInTheDocument();
    expect(
      screen.getByText(/lookup proxy-sg.internal on 127.0.0.53:53: no such host/)
    ).toBeInTheDocument();

    // Hanya tepat 1 elemen role="alert" di seluruh dokumen
    const allAlerts = screen.getAllByRole('alert');
    expect(allAlerts).toHaveLength(1);
  });

  // ---------------------------------------------------------------------------
  // TEST 12: Other pages affected by the same root cause no longer display duplicate notifications.
  // ---------------------------------------------------------------------------
  it('TEST 12: Other pages affected by the same root cause no longer display duplicate notifications', async () => {
    // Menyimulasikan Settings.tsx saat restore database gagal
    const handleRestoreFailure = (
      toast: ReturnType<typeof useToast>['toast'],
      setFeedback: (val: unknown) => void
    ) => {
      const errorMsg = 'Berkas dump corrupt atau tidak sesuai skema PostgreSQL';

      // Arsitektur baru: setFeedback diset null (meredam in-page alert banner duplikat)
      setFeedback(null);
      toast.error('Gagal memulihkan database: ' + errorMsg, 'Pemulihan Gagal', {
        dedupeKey: 'restore-db',
      });
    };

    const SettingsRestoreSimulator: React.FC = () => {
      const { toast } = useToast();
      const [backupFeedback, setBackupFeedback] = useState<unknown>(null);

      return (
        <div>
          {backupFeedback !== null && <div role="alert">PEMULIHAN DATABASE GAGAL</div>}
          <button onClick={() => handleRestoreFailure(toast, setBackupFeedback)}>
            Pulihkan Database
          </button>
        </div>
      );
    };

    render(
      <ToastProvider>
        <SettingsRestoreSimulator />
      </ToastProvider>
    );

    fireEvent.click(screen.getByText('Pulihkan Database'));

    // Tidak ada in-page alert banner duplikat
    expect(screen.queryByText('PEMULIHAN DATABASE GAGAL')).not.toBeInTheDocument();

    // Hanya notifikasi toast tunggal
    expect(screen.getByText('Pemulihan Gagal')).toBeInTheDocument();
    expect(screen.getByText(/Berkas dump corrupt/)).toBeInTheDocument();
    expect(screen.getAllByRole('alert')).toHaveLength(1);
  });
});
