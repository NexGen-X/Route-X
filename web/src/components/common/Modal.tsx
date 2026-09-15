import React, { useEffect, useId } from 'react';
import FocusTrap from 'focus-trap-react';
import { X } from 'lucide-react';
import { Tooltip } from './Tooltip';

// Kunci body bersama lintas Modal/Drawer agar lapisan bertumpuk tidak saling melepas kunci.
function __acquireBodyLock(): void {
  const g = window as any;
  g.__routexBodyLockCount = (g.__routexBodyLockCount || 0) + 1;
  if (g.__routexBodyLockCount === 1) {
    g.__routexBodyLockPrevOverflow = document.body.style.overflow;
  }
  document.body.style.overflow = 'hidden';
}

function __releaseBodyLock(): void {
  const g = window as any;
  g.__routexBodyLockCount = Math.max(0, (g.__routexBodyLockCount || 1) - 1);
  if (g.__routexBodyLockCount === 0) {
    document.body.style.overflow = g.__routexBodyLockPrevOverflow ?? '';
  }
}

interface ModalProps {
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
        aria-describedby={ariaDescribedBy}
        onClick={onClose}
        className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm animate-in fade-in duration-200 cursor-pointer"
      >
        <div
          className={`w-full ${maxW} bg-bg-surface border border-border rounded-card shadow-2xl flex flex-col max-h-[90vh] overflow-hidden cursor-default`}
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between px-5 py-4 border-b border-border bg-bg-surface-2/40">
            <div>
              <h2 id={titleId} className="text-base font-semibold text-text-primary">{title}</h2>
              {subtitle && <p className="text-xs text-text-secondary mt-0.5">{subtitle}</p>}
            </div>
            <Tooltip content="Tutup (Esc)" position="left">
              <button
                type="button"
                onClick={onClose}
                aria-label="Tutup modal"
                className="text-text-muted hover:text-text-primary p-1 rounded-inner hover:bg-bg-surface-2 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </Tooltip>
          </div>
          <div className="p-5 overflow-y-auto flex-1 scrollbar-thin">{children}</div>
          {footer && (
            <div className="px-5 py-4 border-t border-border bg-bg-surface-2/60 flex-shrink-0">
              {footer}
            </div>
          )}
        </div>
      </div>
    </FocusTrap>
  );
};
