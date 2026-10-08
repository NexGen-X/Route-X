import React, { useEffect, useId } from 'react';
import FocusTrap from 'focus-trap-react';
import { X } from 'lucide-react';
import { Tooltip } from './Tooltip';

declare global {
  interface Window {
    __routexBodyLockCount?: number;
    __routexBodyLockPrevOverflow?: string;
  }
}

// Kunci body bersama lintas Modal/Drawer agar lapisan bertumpuk tidak saling melepas kunci.
function __acquireBodyLock(): void {
  window.__routexBodyLockCount = (window.__routexBodyLockCount || 0) + 1;
  if (window.__routexBodyLockCount === 1) {
    window.__routexBodyLockPrevOverflow = document.body.style.overflow;
  }
  document.body.style.overflow = 'hidden';
}

function __releaseBodyLock(): void {
  window.__routexBodyLockCount = Math.max(0, (window.__routexBodyLockCount || 1) - 1);
  if (window.__routexBodyLockCount === 0) {
    document.body.style.overflow = window.__routexBodyLockPrevOverflow ?? '';
  }
}

export interface DrawerProps {
  isOpen: boolean;
  onClose: () => void;
  title: React.ReactNode;
  subtitle?: React.ReactNode;
  headerExtra?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl';
  ariaDescribedBy?: string;
}

export const Drawer: React.FC<DrawerProps> = ({
  isOpen,
  onClose,
  title,
  subtitle,
  headerExtra,
  children,
  footer,
  maxWidth = '2xl',
  ariaDescribedBy,
}) => {
  // ID unik untuk heading agar aria-labelledby selalu menunjuk target yang benar.
  const generatedTitleId = useId();
  const titleId = `drawer-title-${generatedTitleId.replace(/:/g, '')}`;
  const subtitleId = `drawer-subtitle-${generatedTitleId.replace(/:/g, '')}`;

  // Kunci body bersama lintas Modal/Drawer; restore nilai overflow asli.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    if (isOpen) {
      __acquireBodyLock();
      window.addEventListener('keydown', handleKeyDown);
      return () => {
        window.removeEventListener('keydown', handleKeyDown);
        __releaseBodyLock();
      };
    }
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const maxW = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    '2xl': 'max-w-2xl',
    '3xl': 'max-w-3xl',
    '4xl': 'max-w-4xl',
  }[maxWidth];

  return (
    <FocusTrap active={isOpen} focusTrapOptions={{ clickOutsideDeactivates: true }}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={subtitle ? subtitleId : ariaDescribedBy}
        className="fixed inset-0 z-50 overflow-hidden"
      >
        {/* Backdrop */}
        <div
          className="fixed inset-0 bg-black/60 backdrop-blur-xl transition-opacity animate-in fade-in duration-200"
          onClick={onClose}
        />

        {/* Sheet dinamis: mobile bottom-sheet mengambang dengan safe-area, desktop dialog tengah */}
        <div className="fixed inset-0 flex items-end justify-center sm:items-center px-2 pt-2 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:p-6 pointer-events-none">
          <div
            className={`pointer-events-auto w-full sm:w-screen ${maxW} bg-bg-surface/95 border border-white/[0.08] shadow-surface-elevated flex flex-col h-auto max-h-[92dvh] sm:max-h-[88vh] min-h-0 animate-in slide-in-from-bottom sm:slide-in-from-right sm:zoom-in-95 duration-300 ease-out rounded-card overflow-hidden backdrop-blur-xl`}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Drawer Header */}
            <div className="flex items-start justify-between px-4 py-3.5 sm:px-6 sm:py-4 border-b border-white/[0.06] bg-bg-surface-1/50 flex-shrink-0">
              <div className="space-y-1 flex-1 min-w-0 pr-3 sm:pr-4">
                <div className="flex items-center gap-3">
                  <h2 id={titleId} className="text-sm sm:text-base font-semibold text-white tracking-tight truncate">
                    {title}
                  </h2>
                </div>
                {subtitle && (
                  <p id={subtitleId} className="text-xs text-text-muted mt-0.5 leading-relaxed">
                    {subtitle}
                  </p>
                )}
                {headerExtra && <div className="pt-0.5">{headerExtra}</div>}
              </div>

              <Tooltip content="Tutup Panel (Esc)" position="left">
                <button
                  type="button"
                  onClick={onClose}
                  aria-label="Tutup panel"
                  className="text-text-muted hover:text-white p-2 rounded-xl hover:bg-bg-surface-2/80 border border-transparent hover:border-white/[0.08] transition-all flex-shrink-0 cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent min-h-[44px] min-w-[44px] inline-flex items-center justify-center"
                >
                  <X className="w-4 h-4" aria-hidden="true" />
                </button>
              </Tooltip>
            </div>

            {/* Drawer Body */}
            <div className="p-4 pb-5 sm:p-6 overflow-y-auto overscroll-contain min-h-0 flex-1 space-y-4 sm:space-y-5 scrollbar-thin">
              {children}
            </div>

            {/* Drawer Footer (Sticky) */}
            {footer && (
              <div className="px-4 py-3.5 sm:px-6 sm:py-4 border-t border-white/[0.06] bg-bg-surface-1/80 backdrop-blur-md flex-shrink-0 sticky bottom-0 z-10">
                {footer}
              </div>
            )}
          </div>
        </div>
      </div>
    </FocusTrap>
  );
};
