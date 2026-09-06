import React, { useEffect } from 'react';
import { X } from 'lucide-react';

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
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    if (isOpen) {
      document.body.style.overflow = 'hidden';
      window.addEventListener('keydown', handleKeyDown);
    }
    return () => {
      document.body.style.overflow = 'unset';
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
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="drawer-title"
      className="fixed inset-0 z-50 overflow-hidden"
    >
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity animate-in fade-in duration-200"
        onClick={onClose}
      />

      {/* Slide-over panel: Di layar ponsel (lebar < 640px) gunakan pl-0 dan w-full agar drawer mengisi layar penuh tanpa strip hitam canggung di sebelah kiri, sedangkan di desktop gunakan pl-6 dan batas maxW */}
      <div className="fixed inset-y-0 right-0 flex max-w-full pl-0 sm:pl-6 pointer-events-none w-full sm:w-auto">
        <div
          className={`pointer-events-auto w-full sm:w-screen ${maxW} bg-bg-surface border-l border-border shadow-2xl flex flex-col h-full animate-in slide-in-from-right duration-300 ease-out`}
          onClick={(e) => e.stopPropagation()}
        >
          {/* Drawer Header */}
          <div className="flex items-start justify-between px-4 py-3.5 sm:px-6 sm:py-5 border-b border-border bg-bg-surface-2/60">
            <div className="space-y-1.5 flex-1 min-w-0 pr-3 sm:pr-4">
              <div className="flex items-center gap-3">
                <div id="drawer-title" className="text-sm sm:text-base font-bold text-white tracking-tight truncate">
                  {title}
                </div>
              </div>
              {subtitle && (
                <div className="text-xs text-text-secondary leading-relaxed">
                  {subtitle}
                </div>
              )}
              {headerExtra && <div className="pt-1">{headerExtra}</div>}
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

          {/* Drawer Body */}
          <div className="p-3.5 sm:p-6 overflow-y-auto flex-1 space-y-4 sm:space-y-6 scrollbar-thin">
            {children}
          </div>
        </div>
      </div>
    </div>
  );
};
