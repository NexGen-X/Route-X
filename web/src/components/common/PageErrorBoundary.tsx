import { Component, ErrorInfo, ReactNode } from 'react';
import { AlertTriangle, RefreshCw, Home, RotateCcw, ChevronDown, ChevronUp } from 'lucide-react';
import { Button } from './Button';

interface Props {
  pageName: string;
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
  showDetails: boolean;
}

/**
 * PageErrorBoundary mengisolasi kegagalan rendering pada level halaman tunggal.
 * Alasan: React unmount seluruh pohon komponen jika TypeError tidak tertangkap di boundary,
 * menyebabkan layar putih kosong pada seluruh dashboard admin. Dengan boundary per-halaman,
 * kegagalan satu komponen atau format data hanya merender kartu error lokal, sementara
 * sidebar navigasi, header status, dan halaman lainnya tetap berfungsi secara normal.
 */
export class PageErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null, showDetails: false };
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { hasError: true, error };
  }

  private isChunkLoadError(error: Error | null): boolean {
    if (!error) return false;
    const msg = (error.message || '').toLowerCase();
    return (
      msg.includes('failed to fetch dynamically imported module') ||
      msg.includes('loading chunk') ||
      msg.includes('importing a module script failed') ||
      msg.includes('error loading dynamically imported module')
    );
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    console.error(`[PageErrorBoundary] Kesalahan render pada halaman ${this.props.pageName}:`, error, errorInfo);

    if (this.isChunkLoadError(error)) {
      const reloadKey = `route_x_chunk_reload_${this.props.pageName}`;
      const lastReload = sessionStorage.getItem(reloadKey);
      const now = Date.now();
      if (!lastReload || now - parseInt(lastReload, 10) > 10000) {
        sessionStorage.setItem(reloadKey, now.toString());
        console.warn(`[PageErrorBoundary] Chunk load error terdeteksi pada ${this.props.pageName}, memuat ulang halaman...`);
        window.location.reload();
      }
    }
  }

  componentDidUpdate(prevProps: Props): void {
    // Reset status error ketika pengguna berpindah ke halaman lain
    if (prevProps.pageName !== this.props.pageName && this.state.hasError) {
      this.setState({ hasError: false, error: null, showDetails: false });
    }
  }

  render(): ReactNode {
    if (this.state.hasError) {
      const isChunk = this.isChunkLoadError(this.state.error);
      const errorMessage = this.state.error?.message || 'Terjadi kesalahan internal saat merender komponen ini.';

      return (
        <div
          role="alert"
          aria-live="assertive"
          className="p-6 sm:p-8 rounded-card border border-rose-500/30 bg-rose-500/5 text-center my-6 max-w-2xl mx-auto shadow-lg shadow-black/40"
        >
          <div className="w-12 h-12 rounded-full bg-rose-500/10 border border-rose-500/20 text-rose-400 flex items-center justify-center mx-auto mb-3 shadow-sm">
            <AlertTriangle className="w-6 h-6" aria-hidden="true" />
          </div>

          <h3 className="text-base sm:text-lg font-bold text-white mb-1">
            {isChunk ? 'Pembaruan Versi Terdeteksi' : `Gagal Memuat Halaman ${this.props.pageName}`}
          </h3>

          <p className="text-xs text-text-muted mb-4 max-w-lg mx-auto font-mono">
            {isChunk
              ? 'Aset atau modul halaman telah diperbarui di server. Silakan muat ulang halaman untuk memuat versi terbaru.'
              : errorMessage}
          </p>

          {/* Collapsible Tech Stack */}
          {!isChunk && this.state.error?.stack && (
            <div className="mb-4 text-left">
              <button
                type="button"
                onClick={() => this.setState((prev) => ({ showDetails: !prev.showDetails }))}
                className="inline-flex items-center gap-1 text-[11px] text-text-muted hover:text-white transition-colors cursor-pointer mx-auto block"
                aria-expanded={this.state.showDetails}
              >
                {this.state.showDetails ? (
                  <>
                    <ChevronUp className="w-3 h-3" aria-hidden="true" />
                    <span>Sembunyikan Detail Kesalahan</span>
                  </>
                ) : (
                  <>
                    <ChevronDown className="w-3 h-3" aria-hidden="true" />
                    <span>Lihat Detail Kesalahan Teknis</span>
                  </>
                )}
              </button>
              {this.state.showDetails && (
                <pre className="mt-2 text-[10px] text-text-muted font-mono max-h-36 overflow-y-auto p-2.5 bg-black/40 rounded-lg border border-white/[0.06] whitespace-pre-wrap">
                  {this.state.error.stack}
                </pre>
              )}
            </div>
          )}

          {/* Action Recovery Buttons with >= 44x44px touch targets */}
          <div className="flex flex-wrap items-center justify-center gap-2.5 pt-2">
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                if (isChunk) {
                  window.location.reload();
                } else {
                  this.setState({ hasError: false, error: null, showDetails: false });
                }
              }}
              icon={<RotateCcw className="w-3.5 h-3.5" />}
              className="min-h-[44px] sm:min-h-[36px]"
              aria-label={isChunk ? 'Muat ulang versi terbaru' : `Coba muat ulang halaman ${this.props.pageName}`}
            >
              {isChunk ? 'Muat Ulang Versi Terbaru' : 'Coba Lagi'}
            </Button>

            {!isChunk && (
              <Button
                variant="secondary"
                size="sm"
                onClick={() => window.location.reload()}
                icon={<RefreshCw className="w-3.5 h-3.5" />}
                className="min-h-[44px] sm:min-h-[36px]"
                aria-label="Muat ulang browser"
              >
                Muat Ulang Browser
              </Button>
            )}

            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                window.location.hash = '#/';
                this.setState({ hasError: false, error: null, showDetails: false });
              }}
              icon={<Home className="w-3.5 h-3.5" />}
              className="min-h-[44px] sm:min-h-[36px]"
              aria-label="Kembali ke Dashboard"
            >
              Dashboard
            </Button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
