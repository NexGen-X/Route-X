import React, { createContext, useContext, useState, useCallback, useId, useRef, useEffect } from 'react';
import { CheckCircle2, AlertCircle, AlertTriangle, Info, X, Trash2, HelpCircle } from 'lucide-react';
import { Modal } from '../components/common/Modal';

export type ToastType = 'success' | 'error' | 'warn' | 'info';

export interface ToastItem {
  id: string;
  type: ToastType;
  title?: string;
  message: string;
  duration?: number;
}

export interface ConfirmOptions {
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  danger?: boolean;
}

interface ToastContextType {
  showToast: (type: ToastType, message: string, title?: string, duration?: number) => void;
  toast: {
    success: (message: string, title?: string) => void;
    error: (message: string, title?: string) => void;
    warn: (message: string, title?: string) => void;
    info: (message: string, title?: string) => void;
  };
  confirmModal: (options: ConfirmOptions) => Promise<boolean>;
}

const ToastContext = createContext<ToastContextType | null>(null);

const typeConfigs: Record<
  ToastType,
  {
    border: string;
    bg: string;
    icon: React.ReactNode;
    titleColor: string;
    progressBar: string;
  }
> = {
  success: {
    border: 'border-emerald-500/30',
    bg: 'bg-bg-surface-1',
    icon: <CheckCircle2 className="w-5 h-5 text-emerald-400 flex-shrink-0" />,
    titleColor: 'text-emerald-400',
    progressBar: 'bg-emerald-400',
  },
  error: {
    border: 'border-rose-500/30',
    bg: 'bg-bg-surface-1',
    icon: <AlertCircle className="w-5 h-5 text-rose-400 flex-shrink-0" />,
    titleColor: 'text-rose-400',
    progressBar: 'bg-rose-500',
  },
  warn: {
    border: 'border-amber-500/30',
    bg: 'bg-bg-surface-1',
    icon: <AlertTriangle className="w-5 h-5 text-amber-400 flex-shrink-0" />,
    titleColor: 'text-amber-400',
    progressBar: 'bg-amber-400',
  },
  info: {
    border: 'border-cyan-500/30',
    bg: 'bg-bg-surface-1',
    icon: <Info className="w-5 h-5 text-cyan-400 flex-shrink-0" />,
    titleColor: 'text-cyan-400',
    progressBar: 'bg-cyan-400',
  },
};

interface ToastNotificationCardProps {
  toast: ToastItem;
  onClose: (id: string) => void;
}

export const ToastNotificationCard: React.FC<ToastNotificationCardProps> = ({ toast, onClose }) => {
  const { id, type, title, message, duration = 4000 } = toast;
  const config = typeConfigs[type] || typeConfigs.info;

  const [isPaused, setIsPaused] = useState(false);
  const remainingTimeRef = useRef<number>(duration);
  const startTimeRef = useRef<number>(Date.now());
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const startTimer = useCallback(() => {
    if (duration <= 0 || remainingTimeRef.current <= 0) return;
    startTimeRef.current = Date.now();
    timerRef.current = setTimeout(() => {
      onClose(id);
    }, remainingTimeRef.current);
  }, [id, duration, onClose]);

  const pauseTimer = useCallback(() => {
    if (duration <= 0) return;
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    const elapsed = Date.now() - startTimeRef.current;
    remainingTimeRef.current = Math.max(0, remainingTimeRef.current - elapsed);
    setIsPaused(true);
  }, [duration]);

  const resumeTimer = useCallback(() => {
    if (duration <= 0 || remainingTimeRef.current <= 0) return;
    setIsPaused(false);
    startTimer();
  }, [duration, startTimer]);

  useEffect(() => {
    startTimer();
    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [startTimer]);

  return (
    <div
      role={type === 'error' || type === 'warn' ? 'alert' : 'status'}
      aria-atomic="true"
      onMouseEnter={pauseTimer}
      onMouseLeave={resumeTimer}
      className={`pointer-events-auto rounded-xl border ${config.border} ${config.bg} p-4 shadow-2xl backdrop-blur-md flex flex-col relative overflow-hidden transition-all transform translate-y-0 opacity-100`}
    >
      <div className="flex items-start gap-3">
        <div className="mt-0.5">{config.icon}</div>
        <div className="flex-1 min-w-0 pr-2">
          {title && (
            <h4 className={`text-xs font-semibold tracking-wide ${config.titleColor} mb-0.5`}>
              {title}
            </h4>
          )}
          <p className="text-xs text-text-secondary leading-relaxed break-words">{message}</p>
        </div>
        <button
          type="button"
          aria-label="Tutup notifikasi"
          onClick={() => onClose(id)}
          className="p-1 rounded-md text-text-muted hover:text-text-primary hover:bg-border transition-colors -mr-1 -mt-1"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {/* Indikator progress bar countdown dengan animasi menyusut dan pause on hover */}
      {duration > 0 && (
        <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-border">
          <div
            className={`h-full ${config.progressBar}`}
            style={{
              width: '100%',
              animation: `toast-progress ${duration}ms linear forwards`,
              animationPlayState: isPaused ? 'paused' : 'running',
            }}
          />
        </div>
      )}
    </div>
  );
};

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [confirmState, setConfirmState] = useState<{
    isOpen: boolean;
    options: ConfirmOptions;
    resolve: (val: boolean) => void;
  } | null>(null);
  const confirmDescriptionId = useId();

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const showToast = useCallback(
    (type: ToastType, message: string, title?: string, duration = 4000) => {
      const id = Math.random().toString(36).substring(2, 9);
      const newToast: ToastItem = { id, type, title, message, duration };
      setToasts((prev) => [...prev, newToast]);
    },
    []
  );

  const toast = {
    success: (message: string, title?: string) => showToast('success', message, title),
    error: (message: string, title?: string) => showToast('error', message, title || 'Terjadi Kesalahan'),
    warn: (message: string, title?: string) => showToast('warn', message, title || 'Peringatan'),
    info: (message: string, title?: string) => showToast('info', message, title),
  };

  const confirmModal = useCallback((options: ConfirmOptions): Promise<boolean> => {
    return new Promise((resolve) => {
      setConfirmState({
        isOpen: true,
        options,
        resolve: (val: boolean) => {
          setConfirmState(null);
          resolve(val);
        },
      });
    });
  }, []);

  const handleConfirmClose = (result: boolean) => {
    if (confirmState) {
      confirmState.resolve(result);
    }
  };

  return (
    <ToastContext.Provider value={{ showToast, toast, confirmModal }}>
      {children}

      {/* Kontainer Notifikasi Toast dengan Safe Area Berponi & Z-Index Tinggi */}
      <div
        data-testid="toast-container"
        aria-live="polite"
        aria-relevant="additions"
        aria-atomic="false"
        className="fixed top-[max(1rem,env(safe-area-inset-top))] left-1/2 -translate-x-1/2 sm:left-auto sm:translate-x-0 sm:right-4 z-[9999] flex flex-col gap-2.5 w-[calc(100%-2rem)] max-w-sm pointer-events-none"
      >
        {toasts.map((t) => (
          <ToastNotificationCard key={t.id} toast={t} onClose={removeToast} />
        ))}
      </div>

      {confirmState && (
        <Modal
          isOpen={confirmState.isOpen}
          onClose={() => handleConfirmClose(false)}
          title={confirmState.options.title}
          maxWidth="md"
          ariaDescribedBy={confirmDescriptionId}
        >
          <div>
            <div className="flex items-start gap-4 mb-4">
              <div
                className={`p-3 rounded-xl flex-shrink-0 ${
                  confirmState.options.danger
                    ? 'bg-rose-500/10 border border-rose-500/20 text-rose-400'
                    : 'bg-accent/10 border border-accent/20 text-accent'
                }`}
              >
                {confirmState.options.danger ? (
                  <Trash2 className="w-6 h-6" />
                ) : (
                  <HelpCircle className="w-6 h-6" />
                )}
              </div>
              <div className="flex-1 min-w-0">
                <p id={confirmDescriptionId} className="text-xs text-text-secondary mt-1.5 leading-relaxed">
                  {confirmState.options.message}
                </p>
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 mt-4 pt-4 border-t border-border-subtle">
              <button
                type="button"
                onClick={() => handleConfirmClose(false)}
                className="px-4 py-2 rounded-lg bg-bg-surface-2 hover:bg-bg-surface-3 text-xs font-semibold text-text-primary border border-border transition-all"
              >
                {confirmState.options.cancelText || 'Batal'}
              </button>
              <button
                type="button"
                onClick={() => handleConfirmClose(true)}
                className={`px-4 py-2 rounded-lg text-xs font-semibold transition-all shadow-md ${
                  confirmState.options.danger
                    ? 'bg-rose-600 hover:bg-rose-500 text-white shadow-rose-900/30'
                    : 'bg-accent hover:bg-accent-hover text-black shadow-accent/20'
                }`}
              >
                {confirmState.options.confirmText || 'Ya, Lanjutkan'}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </ToastContext.Provider>
  );
};

export const useToast = (): ToastContextType => {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return ctx;
};
