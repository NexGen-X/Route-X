import { Component, ErrorInfo, ReactNode } from 'react';
import { AlertOctagon, RefreshCw, RotateCcw, Home, Copy, Check, ChevronDown, ChevronUp } from 'lucide-react';
import { Button } from './Button';

interface GlobalErrorBoundaryProps {
  children: ReactNode;
}

interface GlobalErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
  copied: boolean;
  showDetails: boolean;
}

/**
 * GlobalErrorBoundary menangkap error rendering tingkat paling atas aplikasi.
 * Mencegah White Screen of Death (WSoD) jika terjadi crash pada shell utama,
 * provider konteks, atau dependensi root React.
 */
export class GlobalErrorBoundary extends Component<GlobalErrorBoundaryProps, GlobalErrorBoundaryState> {
  constructor(props: GlobalErrorBoundaryProps) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
      copied: false,
      showDetails: false,
    };
  }

  static getDerivedStateFromError(error: Error): Partial<GlobalErrorBoundaryState> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    console.error('[GlobalErrorBoundary] Uncaught application error:', error, errorInfo);
    this.setState({ errorInfo });
  }

  private handleCopyDetails = async (): Promise<void> => {
    const errorDetails = [
      `Route-X Console Crash Report`,
      `Timestamp: ${new Date().toISOString()}`,
      `Error: ${this.state.error?.name || 'Error'}: ${this.state.error?.message || 'Unknown error'}`,
      `Stack:\n${this.state.error?.stack || 'No stack trace available'}`,
      `Component Stack:\n${this.state.errorInfo?.componentStack || 'No component stack available'}`,
    ].join('\n\n');

    try {
      await navigator.clipboard.writeText(errorDetails);
      this.setState({ copied: true });
      setTimeout(() => this.setState({ copied: false }), 2000);
    } catch {
      // Fallback copy
      const textarea = document.createElement('textarea');
      textarea.value = errorDetails;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      this.setState({ copied: true });
      setTimeout(() => this.setState({ copied: false }), 2000);
    }
  };

  private handleResetCacheAndHome = (): void => {
    try {
      localStorage.removeItem('routex_token');
      sessionStorage.clear();
    } catch {
      // Abaikan jika storage diblokir
    }
    window.location.href = '/';
  };

  render(): ReactNode {
    if (this.state.hasError) {
      const errorMessage = this.state.error?.message || 'Terjadi kesalahan sistem internal yang tidak tertangani.';
      const isChunk = errorMessage.toLowerCase().includes('dynamically imported module') ||
        errorMessage.toLowerCase().includes('loading chunk');

      return (
        <div
          role="alert"
          aria-live="assertive"
          className="min-h-screen bg-[#080B11] text-text-primary flex flex-col items-center justify-center p-4 sm:p-6 font-sans select-text"
        >
          {/* Subtle Ambient Radial Glow */}
          <div
            className="fixed inset-0 pointer-events-none bg-[radial-gradient(circle_at_50%_20%,rgba(59,130,246,0.12),transparent_70%)]"
            aria-hidden="true"
          />

          <div className="relative w-full max-w-2xl bg-[#0D121F]/95 backdrop-blur-xl border border-white/[0.08] rounded-2xl shadow-2xl shadow-black/80 p-6 sm:p-8 flex flex-col items-center text-center">
            {/* Header Icon */}
            <div className="w-16 h-16 rounded-2xl bg-rose-500/10 border border-rose-500/20 text-rose-400 flex items-center justify-center mb-5 shadow-lg shadow-rose-500/10 ring-1 ring-rose-500/20">
              <AlertOctagon className="w-8 h-8" aria-hidden="true" />
            </div>

            {/* Badge System */}
            <div className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-rose-500/10 border border-rose-500/20 text-rose-400 text-xs font-mono mb-3">
              <span className="w-1.5 h-1.5 rounded-full bg-rose-400 animate-pulse" aria-hidden="true" />
              <span>Route-X Core Gateway Recovery</span>
            </div>

            <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-white mb-2">
              {isChunk ? 'Pembaruan Modul Sistem Terdeteksi' : 'Gagal Memuat Antarmuka Route-X'}
            </h1>

            <p className="text-xs sm:text-sm text-text-secondary max-w-lg mb-6 leading-relaxed">
              {isChunk
                ? 'Versi frontend baru telah dideploy ke server. Silakan muat ulang halaman untuk mendapatkan modul terbaru.'
                : 'Sistem menangkap pengecualian rendering tak terduga. Dashboard dicegah dari crash total (White Screen of Death) dan data lokal Anda tetap aman.'}
            </p>

            {/* Error Message Box */}
            <div className="w-full bg-[#080B11]/80 border border-white/[0.06] rounded-xl p-3.5 mb-6 text-left">
              <div className="flex items-center justify-between text-[11px] text-text-muted font-mono mb-1.5">
                <span>Pesan Kesalahan:</span>
                <button
                  type="button"
                  onClick={this.handleCopyDetails}
                  className="inline-flex items-center gap-1 text-accent hover:text-blue-400 cursor-pointer transition-colors p-1"
                  aria-label="Salin detail kesalahan sistem"
                >
                  {this.state.copied ? (
                    <>
                      <Check className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" />
                      <span className="text-emerald-400">Tersalin</span>
                    </>
                  ) : (
                    <>
                      <Copy className="w-3.5 h-3.5" aria-hidden="true" />
                      <span>Salin Detail</span>
                    </>
                  )}
                </button>
              </div>
              <p className="text-xs text-rose-300 font-mono break-all line-clamp-3">
                {errorMessage}
              </p>

              {/* Collapsible Tech Stack */}
              <div className="mt-3 pt-2.5 border-t border-white/[0.06]">
                <button
                  type="button"
                  onClick={() => this.setState((prev) => ({ showDetails: !prev.showDetails }))}
                  className="inline-flex items-center gap-1 text-[11px] text-text-muted hover:text-white transition-colors cursor-pointer"
                  aria-expanded={this.state.showDetails}
                >
                  {this.state.showDetails ? (
                    <>
                      <ChevronUp className="w-3.5 h-3.5" aria-hidden="true" />
                      <span>Sembunyikan Stack Trace</span>
                    </>
                  ) : (
                    <>
                      <ChevronDown className="w-3.5 h-3.5" aria-hidden="true" />
                      <span>Lihat Stack Trace Teknis</span>
                    </>
                  )}
                </button>

                {this.state.showDetails && (
                  <pre className="mt-2 text-[10px] text-text-muted font-mono max-h-48 overflow-y-auto overflow-x-auto p-2.5 bg-black/40 rounded-lg whitespace-pre-wrap">
                    {this.state.error?.stack || 'Stack trace tidak tersedia.'}
                  </pre>
                )}
              </div>
            </div>

            {/* Action Recovery Buttons */}
            <div className="flex flex-wrap items-center justify-center gap-3 w-full">
              <Button
                variant="primary"
                size="md"
                onClick={() => window.location.reload()}
                icon={<RefreshCw className="w-4 h-4" />}
                className="w-full sm:w-auto min-h-[44px]"
                aria-label="Muat ulang halaman browser"
              >
                Muat Ulang Halaman
              </Button>

              <Button
                variant="secondary"
                size="md"
                onClick={() => this.setState({ hasError: false, error: null, errorInfo: null })}
                icon={<RotateCcw className="w-4 h-4" />}
                className="w-full sm:w-auto min-h-[44px]"
                aria-label="Coba pulihkan komponen antarmuka"
              >
                Coba Pulihkan
              </Button>

              <Button
                variant="ghost"
                size="md"
                onClick={this.handleResetCacheAndHome}
                icon={<Home className="w-4 h-4" />}
                className="w-full sm:w-auto min-h-[44px]"
                aria-label="Kembali ke Beranda Route-X"
              >
                Kembali ke Beranda
              </Button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
