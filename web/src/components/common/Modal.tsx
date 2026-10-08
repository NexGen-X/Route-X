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

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  subtitle?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '4xl';
  ariaDescribedBy?: string;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  subtitle,
  children,
  footer,
  maxWidth = 'md',
  ariaDescribedBy,
}) => {
  const generatedTitleId = useId();
  const titleId = `modal-title-${generatedTitleId.replace(/:/g, '')}`;
  const subtitleId = `modal-subtitle-${generatedTitleId.replace(/:/g, '')}`;

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
    '4xl': 'max-w-4xl',
  }[maxWidth];

  return (
    <FocusTrap active={isOpen}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={subtitle ? subtitleId : ariaDescribedBy}
        onClick={onClose}
        className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-xl animate-in fade-in duration-200 cursor-pointer"
      >
        <div
          className={`w-full ${maxW} bg-bg-surface/95 border border-white/[0.08] rounded-card shadow-surface-elevated flex flex-col max-h-[90vh] overflow-hidden cursor-default backdrop-blur-xl animate-in zoom-in-95 duration-200`}
          onClick={(e) => e.stopPropagation()}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-5 py-4 border-b border-white/[0.06] bg-bg-surface-1/50 shrink-0">
            <div className="min-w-0 pr-3">
              <h2 id={titleId} className="text-sm font-semibold text-text-primary tracking-tight">
                {title}
              </h2>
              {subtitle && (
                <p id={subtitleId} className="text-xs text-text-muted mt-0.5 leading-relaxed">
                  {subtitle}
                </p>
              )}
            </div>
            <Tooltip content="Tutup (Esc)" position="left">
              <button
                type="button"
                onClick={onClose}
                aria-label="Tutup modal"
                className="text-text-muted hover:text-white p-2 min-h-[44px] min-w-[44px] rounded-inner hover:bg-bg-surface-2/80 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent cursor-pointer flex items-center justify-center shrink-0"
              >
                <X className="w-4 h-4" aria-hidden="true" />
              </button>
            </Tooltip>
          </div>

          {/* Body */}
          <div className="p-5 overflow-y-auto flex-1 scrollbar-thin">{children}</div>

          {/* Sticky Footer */}
          {footer && (
            <div className="px-5 py-3.5 border-t border-white/[0.06] bg-bg-surface-1/70 backdrop-blur-md flex-shrink-0 sticky bottom-0 z-10">
              {footer}
            </div>
          )}
        </div>
      </div>
    </FocusTrap>
  );
};
