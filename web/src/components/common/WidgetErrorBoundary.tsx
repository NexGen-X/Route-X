import { Component, ErrorInfo, ReactNode } from 'react';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { Button } from './Button';

interface WidgetErrorBoundaryProps {
  title?: string;
  fallbackMessage?: string;
  onRetry?: () => void;
  children: ReactNode;
  className?: string;
}

interface WidgetErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

/**
 * WidgetErrorBoundary mengisolasi kegagalan rendering pada level komponen / widget lokal.
 * Mencegah satu widget (seperti chart Recharts atau feed) merusak seluruh halaman dashboard.
 */
export class WidgetErrorBoundary extends Component<WidgetErrorBoundaryProps, WidgetErrorBoundaryState> {
  constructor(props: WidgetErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): Partial<WidgetErrorBoundaryState> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    console.warn(`[WidgetErrorBoundary] Kegagalan widget "${this.props.title || 'Widget'}":`, error, errorInfo);
  }

  private handleRetry = (): void => {
    this.setState({ hasError: false, error: null });
    if (this.props.onRetry) {
      this.props.onRetry();
    }
  };

  render(): ReactNode {
    if (this.state.hasError) {
      const widgetTitle = this.props.title || 'Komponen';
      const fallbackText = this.props.fallbackMessage ||
        (this.state.error?.message ? `Gagal merender ${widgetTitle}: ${this.state.error.message}` : `Komponen ${widgetTitle} mengalami galat internal.`);

      return (
        <div
          role="alert"
          aria-live="polite"
          className={`p-4 sm:p-5 rounded-card border border-rose-500/20 bg-rose-500/5 text-left flex flex-col justify-between gap-3 ${this.props.className || ''}`}
        >
          <div className="flex items-start gap-3 min-w-0">
            <div className="p-1.5 rounded-lg bg-rose-500/10 text-rose-400 border border-rose-500/20 shrink-0 mt-0.5">
              <AlertCircle className="w-4 h-4" aria-hidden="true" />
            </div>
            <div className="min-w-0 flex-1">
              <h4 className="text-xs sm:text-sm font-semibold text-white truncate">
                {widgetTitle} Gagal Dimuat
              </h4>
              <p className="text-xs text-text-muted mt-1 font-mono break-words line-clamp-2">
                {fallbackText}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-end pt-2 border-t border-rose-500/10">
            <Button
              variant="secondary"
              size="sm"
              onClick={this.handleRetry}
              icon={<RefreshCw className="w-3.5 h-3.5" />}
              className="min-h-[44px] sm:min-h-[32px]"
              aria-label={`Coba muat ulang ${widgetTitle}`}
            >
              Coba Lagi
            </Button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
