import React, { useEffect, useId } from 'react';
import FocusTrap from 'focus-trap-react';
import { X } from 'lucide-react';

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

interface DrawerProps {
  isOpen: boolean;
  onClose: () => void;
  title: React.ReactNode;
  subtitle?: React.ReactNode;
  headerExtra?: React.ReactNode;
  children: React.ReactNode;
  maxWidth?: 'md' | 'lg' | 'xl' | '2xl' | '3xl';
}

export const Drawer: React.FC<DrawerProps> = ({
  isOpen,
  onClose,
  title,
  subtitle,
  headerExtra,
  children,
  maxWidth = '2xl',
}) => {
  // ID unik untuk heading agar aria-labelledby selalu menunjuk target yang benar.
  const generatedTitleId = useId();
  const titleId = `drawer-title-${generatedTitleId.replace(/:/g, '')}`;
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
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    '2xl': 'max-w-2xl',
    '3xl': 'max-w-3xl',
  }[maxWidth];

  return (
    <FocusTrap active={isOpen} focusTrapOptions={{ clickOutsideDeactivates: true }}>
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      className="fixed inset-0 z-50 overflow-hidden"
    >
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity animate-in fade-in duration-200"
        onClick={onClose}
      />

      {/* Sheet dinamis: mobile bottom-sheet mengambang (ada jeda dari tepi
          viewport + safe-area), desktop dialog tengah. Tinggi mengikuti
          konten (h-auto) dengan batas max agar daftar panjang scroll di
          dalam body, bukan satu panel full-screen. */}
      <div className="fixed inset-0 flex items-end justify-center sm:items-center px-2 pt-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] sm:p-6 pointer-events-none">
        <div
          className={`pointer-events-auto w-full sm:w-screen ${maxW} bg-bg-surface border border-border shadow-2xl flex flex-col h-auto max-h-[92dvh] sm:max-h-[85vh] min-h-0 animate-in slide-in-from-bottom sm:slide-in-from-right sm:zoom-in-95 duration-300 ease-out rounded-2xl overflow-hidden`}
          onClick={(e) => e.stopPropagation()}
        >
          {/* Drawer Header: stabil, tidak ikut scroll */}
          <div className="flex items-start justify-between px-3 py-3 sm:px-6 sm:py-5 border-b border-border bg-bg-surface-2/60 flex-shrink-0">
            <div className="space-y-1 flex-1 min-w-0 pr-3 sm:pr-4">
              <div className="flex items-center gap-3">
                <h2 id={titleId} className="text-sm sm:text-base font-bold text-white tracking-tight truncate">
                  {title}
                </h2>
              </div>
              {subtitle && (
                <div className="text-xs text-text-secondary leading-relaxed">
                  {subtitle}
                </div>
              )}
              {headerExtra && <div className="pt-0.5">{headerExtra}</div>}
            </div>

            <button
              onClick={onClose}
              title="Tutup Panel (Esc)"
              aria-label="Tutup panel"
              className="text-text-muted hover:text-white p-1.5 rounded-xl hover:bg-bg-surface border border-transparent hover:border-border transition-all flex-shrink-0 cursor-pointer"
            >
              <X className="w-5 h-5" aria-hidden="true" />
            </button>
          </div>

          {/* Drawer Body: satu-satunya area scroll; pb-4 (16px) agar item
              terakhir tidak menempel ke tepi bawah panel. Desktop tetap p-6
              seragam (sm:p-6 menimpa pb-4). min-h-0 + flex-1: menyusut saat
              panel mencapai max-height, tidak meregang saat konten pendek. */}
          <div className="p-3 pb-4 sm:p-6 overflow-y-auto overscroll-contain min-h-0 flex-1 space-y-3 sm:space-y-6 scrollbar-thin">
            {children}
          </div>
        </div>
      </div>
    </div>
    </FocusTrap>
  );
};
