import { Component, ErrorInfo, ReactNode } from 'react';
import { AlertTriangle, RefreshCw } from 'lucide-react';

interface Props {
  pageName: string;
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
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
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // Mencatat detail error komponen ke konsol browser untuk keperluan inspeksi dan debugging
    console.error(`[PageErrorBoundary] Kesalahan render pada halaman ${this.props.pageName}:`, error, errorInfo);
  }

  componentDidUpdate(prevProps: Props) {
    // Reset status error ketika pengguna berpindah ke halaman lain
    if (prevProps.pageName !== this.props.pageName && this.state.hasError) {
      this.setState({ hasError: false, error: null });
    }
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="p-8 rounded-card border border-rose-500/30 bg-rose-500/5 text-center my-6">
          <div className="w-12 h-12 rounded-full bg-rose-500/10 border border-rose-500/20 text-rose-400 flex items-center justify-center mx-auto mb-3">
            <AlertTriangle className="w-6 h-6" />
          </div>
          <h3 className="text-base font-bold text-white mb-1">
            Gagal Memuat Halaman {this.props.pageName}
          </h3>
          <p className="text-xs text-text-muted mb-4 max-w-lg mx-auto font-mono">
            {this.state.error?.message || 'Terjadi kesalahan internal saat merender komponen ini.'}
          </p>
          <button
            onClick={() => this.setState({ hasError: false, error: null })}
            className="inline-flex items-center gap-2 px-4 py-2 text-xs font-semibold rounded-button bg-rose-500 hover:bg-rose-600 text-white transition-colors"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            Coba Muat Ulang Halaman
          </button>
        </div>
      );
    }

    return this.props.children;
  }
}
