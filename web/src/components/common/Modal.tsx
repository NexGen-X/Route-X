import React, { useEffect } from 'react';
import FocusTrap from 'focus-trap-react';
import { X } from 'lucide-react';

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  subtitle?: string;
  children: React.ReactNode;
  maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '4xl';
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  subtitle,
  children,
  maxWidth = 'md',
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
        aria-labelledby="modal-title"
        onClick={onClose}
        className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm animate-in fade-in duration-200 cursor-pointer"
      >
        <div
          className={`w-full ${maxW} bg-bg-surface border border-border rounded-card shadow-2xl flex flex-col max-h-[90vh] overflow-hidden cursor-default`}
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between px-5 py-4 border-b border-border bg-bg-surface-2/40">
            <div>
              <h2 id="modal-title" className="text-base font-semibold text-text-primary">{title}</h2>
              {subtitle && <p className="text-xs text-text-secondary mt-0.5">{subtitle}</p>}
            </div>
            <button
              onClick={onClose}
              aria-label="Tutup modal"
              className="text-text-muted hover:text-text-primary p-1 rounded-inner hover:bg-bg-surface-2 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
          <div className="p-5 overflow-y-auto flex-1">{children}</div>
        </div>
      </div>
    </FocusTrap>
  );
};
