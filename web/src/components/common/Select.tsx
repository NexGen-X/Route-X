import React, { useState, useRef, useEffect, useId } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, Check, Search, X } from 'lucide-react';

export interface SelectOption {
  value: string;
  label: string;
  description?: string;
  icon?: React.ReactNode;
  disabled?: boolean;
}

export interface SelectProps {
  options: SelectOption[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  label?: string;
  disabled?: boolean;
  error?: string;
  required?: boolean;
  searchable?: boolean;
  className?: string;
  id?: string;
  variant?: 'form' | 'compact';
  'aria-label'?: string;
}

export const Select: React.FC<SelectProps> = ({
  options,
  value,
  onChange,
  placeholder = '-- Pilih Opsi --',
  label,
  disabled = false,
  error,
  required = false,
  searchable,
  className = '',
  id,
  variant = 'form',
  'aria-label': ariaLabel,
}) => {
  const generatedId = useId();
  const selectId = id || generatedId;
  const [isOpen, setIsOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [highlightedIndex, setHighlightedIndex] = useState(-1);
  const [isMobile, setIsMobile] = useState<boolean>(() => {
    if (typeof window !== 'undefined') {
      return window.innerWidth < 640;
    }
    return false;
  });

  const containerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  // Timer fokus input pencarian; disimpan agar bisa dibatalkan bila menu
  // ditutup atau komponen unmount sebelum 50ms berlalu.
  const focusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Batalkan timer fokus yang tersisa saat unmount.
  useEffect(() => {
    return () => {
      if (focusTimerRef.current) {
        clearTimeout(focusTimerRef.current);
        focusTimerRef.current = null;
      }
    };
  }, []);

  // Aktifkan pencarian jika eksplisit atau jumlah opsi > 5
  const isSearchable = searchable ?? options.length > 5;

  const selectedOption = options.find((opt) => opt.value === value);

  // Filter opsi berdasarkan query pencarian
  const filteredOptions = options.filter((opt) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    return (
      opt.label.toLowerCase().includes(q) ||
      (opt.description && opt.description.toLowerCase().includes(q)) ||
      opt.value.toLowerCase().includes(q)
    );
  });

  // Listener resize untuk mendeteksi mobile viewport (< 640px)
  useEffect(() => {
    const handleResize = () => {
      setIsMobile(window.innerWidth < 640);
    };
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  // Kunci scroll body saat bottom sheet mobile terbuka
  useEffect(() => {
    if (isOpen && isMobile) {
      const originalOverflow = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
      return () => {
        document.body.style.overflow = originalOverflow;
      };
    }
  }, [isOpen, isMobile]);

  // Tangani klik di luar untuk menutup menu pada desktop
  useEffect(() => {
    if (!isOpen || isMobile) return;

    const handleOutsideClick = (e: MouseEvent | TouchEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
        setSearchQuery('');
      }
    };

    document.addEventListener('mousedown', handleOutsideClick);
    document.addEventListener('touchstart', handleOutsideClick);
    return () => {
      document.removeEventListener('mousedown', handleOutsideClick);
      document.removeEventListener('touchstart', handleOutsideClick);
    };
  }, [isOpen, isMobile]);

  // Fokuskan input pencarian saat menu terbuka
  useEffect(() => {
    if (isOpen && isSearchable && searchInputRef.current) {
      if (focusTimerRef.current) clearTimeout(focusTimerRef.current);
      focusTimerRef.current = setTimeout(() => {
        focusTimerRef.current = null;
        searchInputRef.current?.focus();
      }, 50);
    } else if (!isOpen && focusTimerRef.current) {
      clearTimeout(focusTimerRef.current);
      focusTimerRef.current = null;
    }
    if (isOpen) {
      const idx = filteredOptions.findIndex((opt) => opt.value === value);
      setHighlightedIndex(idx >= 0 ? idx : 0);
    }
  }, [isOpen, isSearchable]);

  // Navigasi keyboard
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (disabled) return;

    if (!isOpen) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        setIsOpen(true);
      }
      return;
    }

    switch (e.key) {
      case 'Escape':
        e.preventDefault();
        setIsOpen(false);
        setSearchQuery('');
        break;
      case 'ArrowDown':
        e.preventDefault();
        setHighlightedIndex((prev) => (prev < filteredOptions.length - 1 ? prev + 1 : 0));
        break;
      case 'ArrowUp':
        e.preventDefault();
        setHighlightedIndex((prev) => (prev > 0 ? prev - 1 : filteredOptions.length - 1));
        break;
      case 'Enter':
        e.preventDefault();
        if (highlightedIndex >= 0 && highlightedIndex < filteredOptions.length) {
          const target = filteredOptions[highlightedIndex];
          if (!target.disabled) {
            onChange(target.value);
            setIsOpen(false);
            setSearchQuery('');
          }
        }
        break;
      default:
        break;
    }
  };

  const handleSelect = (val: string, isOptDisabled?: boolean) => {
    if (isOptDisabled) return;
    onChange(val);
    setIsOpen(false);
    setSearchQuery('');
  };

  const widthClass = className.includes('w-') ? '' : 'w-full';

  return (
    <div className={`relative ${widthClass} text-left ${className}`} ref={containerRef} onKeyDown={handleKeyDown}>
      {label && (
        <label htmlFor={selectId} className={`block font-semibold text-text-secondary uppercase mb-1 ${variant === 'compact' ? 'text-[10px]' : 'text-xs'}`}>
          {label} {required && <span className="text-status-error">*</span>}
        </label>
      )}

      {/* Trigger Button - In-App Styled */}
      <button
        id={selectId}
        type="button"
        aria-label={ariaLabel}
        disabled={disabled}
        onClick={() => {
          if (!disabled) {
            setIsOpen((prev) => !prev);
            setSearchQuery('');
          }
        }}
        aria-haspopup="listbox"
        aria-controls={isOpen ? (isMobile ? `${selectId}-mobile-listbox` : `${selectId}-listbox`) : undefined}
        aria-expanded={isOpen}
        className={`w-full flex items-center justify-between gap-2 bg-bg-surface-2 border transition-all duration-150 text-left focus:outline-none focus:ring-1 focus:ring-accent ${
          variant === 'compact'
            ? 'px-2.5 py-1 text-[11px] rounded-nav'
            : 'px-3 py-2 text-xs rounded-nav'
        } ${
          error
            ? 'border-status-error text-status-error'
            : isOpen
            ? 'border-accent ring-1 ring-accent text-white shadow-sm'
            : 'border-border text-white hover:border-border-hover'
        } ${disabled ? 'opacity-50 cursor-not-allowed bg-bg-surface' : 'cursor-pointer'}`}
      >
        <div className="flex items-center gap-2 truncate min-w-0 flex-1">
          {selectedOption?.icon && <span className="flex-shrink-0">{selectedOption.icon}</span>}
          <span className={`truncate ${selectedOption ? 'text-white font-medium' : 'text-text-muted font-normal'}`}>
            {selectedOption ? selectedOption.label : placeholder}
          </span>
        </div>

        <ChevronDown
          className={`${variant === 'compact' ? 'w-3.5 h-3.5' : 'w-4 h-4'} text-text-muted transition-transform duration-200 flex-shrink-0 ${
            isOpen ? 'rotate-180 text-accent' : ''
          }`}
        />
      </button>

      {error && <span className="text-[11px] text-status-error mt-1 block">{error}</span>}

      {/* Floating Custom Dropdown List untuk Desktop (>= 640px) */}
      {isOpen && !isMobile && (
        <div
          className="absolute z-50 left-0 right-0 mt-1 bg-[#141416] border border-border/90 rounded-xl shadow-2xl backdrop-blur-xl overflow-hidden animate-in fade-in zoom-in-95 duration-150 flex flex-col max-h-64"
          style={{ minWidth: '100%' }}
        >
          {/* Kolom Pencarian Cepat Inline */}
          {isSearchable && (
            <div className="p-2 border-b border-border/60 bg-bg-surface-2/40 sticky top-0 z-10 flex items-center gap-1.5">
              <Search className="w-3.5 h-3.5 text-text-muted flex-shrink-0" />
              <input
                aria-activedescendant={isOpen && highlightedIndex >= 0 ? `${selectId}-opt-${highlightedIndex}` : undefined}
                ref={searchInputRef}
                type="text"
                value={searchQuery}
                onChange={(e) => {
                  setSearchQuery(e.target.value);
                  setHighlightedIndex(0);
                }}
                placeholder="Ketik untuk memfilter..."
                className="w-full bg-transparent text-xs text-white placeholder:text-text-muted focus:outline-none font-sans"
              />
              {searchQuery && (
                <button
                  type="button"
                  onClick={() => setSearchQuery('')}
                  className="p-0.5 text-text-muted hover:text-white rounded"
                >
                  <X className="w-3 h-3" />
                </button>
              )}
            </div>
          )}

          {/* Opsi Listbox Desktop */}
          <div
            id={`${selectId}-listbox`}
            ref={listRef}
            role="listbox"
            tabIndex={-1}
            className="overflow-y-auto p-1 divide-y divide-border/20 space-y-0.5"
          >
            {filteredOptions.length === 0 ? (
              <div className="py-4 px-3 text-center text-xs text-text-muted">
                Tidak ada opsi yang cocok dengan "{searchQuery}"
              </div>
            ) : (
              filteredOptions.map((opt, index) => {
                const isSelected = opt.value === value;
                const isHighlighted = index === highlightedIndex;

                return (
                  <div
                    key={opt.value || `empty-${index}`}
                    id={`${selectId}-opt-${index}`}
                    role="option"
                    aria-selected={isSelected}
                    onClick={() => handleSelect(opt.value, opt.disabled)}
                    onMouseEnter={() => setHighlightedIndex(index)}
                    className={`flex items-center justify-between gap-2 px-3 py-2 rounded-lg text-xs cursor-pointer transition-colors duration-100 ${
                      opt.disabled
                        ? 'opacity-40 cursor-not-allowed text-text-muted'
                        : isSelected
                        ? 'bg-accent/15 text-accent font-semibold border-l-2 border-accent'
                        : isHighlighted
                        ? 'bg-bg-surface-2 text-white'
                        : 'text-text-secondary hover:text-white hover:bg-bg-surface-2/50'
                    }`}
                  >
                    <div className="flex items-center gap-2 truncate min-w-0">
                      {opt.icon && <span className="flex-shrink-0">{opt.icon}</span>}
                      <div className="truncate">
                        <div className="truncate font-sans">{opt.label}</div>
                        {opt.description && (
                          <div className="text-[10px] text-text-muted font-mono truncate mt-0.5">
                            {opt.description}
                          </div>
                        )}
                      </div>
                    </div>

                    {isSelected && <Check className="w-3.5 h-3.5 text-accent flex-shrink-0" />}
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}

      {/* Custom Bottom Sheet untuk Mobile (< 640px) via React Portal */}
      {isOpen && isMobile && typeof document !== 'undefined' && createPortal(
        <div className="fixed inset-0 z-50 flex items-end justify-center select-none" role="dialog" aria-modal="true" aria-label={label || placeholder}>
          {/* Dark blur backdrop */}
          <div
            className="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity"
            onClick={() => {
              setIsOpen(false);
              setSearchQuery('');
            }}
            aria-hidden="true"
          />

          {/* Bottom Sheet Container */}
          <div className="relative w-full max-h-[85vh] bg-[#141416] border-t border-border rounded-t-2xl z-50 flex flex-col animate-in slide-in-from-bottom duration-200 pb-[max(1rem,env(safe-area-inset-bottom))] shadow-2xl">
            {/* Drag Handle */}
            <div className="w-12 h-1.5 bg-border rounded-full mx-auto my-3 shrink-0" />

            {/* Header dengan judul & tombol tutup */}
            <div className="flex items-center justify-between px-4 pb-3 border-b border-border/60">
              <div className="flex items-center gap-2 min-w-0">
                <span className="text-sm font-semibold text-white truncate">
                  {label || placeholder || 'Pilih Opsi'}
                </span>
                {required && <span className="text-status-error text-xs">*</span>}
              </div>
              <button
                type="button"
                onClick={() => {
                  setIsOpen(false);
                  setSearchQuery('');
                }}
                className="p-1 rounded-lg text-text-muted hover:text-white hover:bg-bg-surface-2 transition-colors cursor-pointer"
                aria-label="Tutup"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Kolom Pencarian Cepat Mobile jika isSearchable */}
            {isSearchable && (
              <div className="p-3 border-b border-border/60 bg-bg-surface-2/40 flex items-center gap-2">
                <Search className="w-4 h-4 text-text-muted flex-shrink-0" />
                <input
                  ref={searchInputRef}
                  type="text"
                  value={searchQuery}
                  onChange={(e) => {
                    setSearchQuery(e.target.value);
                    setHighlightedIndex(0);
                  }}
                  placeholder="Ketik untuk memfilter..."
                  className="w-full bg-transparent text-xs text-white placeholder:text-text-muted focus:outline-none font-sans"
                />
                {searchQuery && (
                  <button
                    type="button"
                    onClick={() => setSearchQuery('')}
                    className="p-1 text-text-muted hover:text-white rounded"
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
            )}

            {/* Daftar Opsi Mobile */}
            <div
              id={`${selectId}-mobile-listbox`}
              ref={listRef}
              role="listbox"
              tabIndex={-1}
              className="overflow-y-auto px-2 py-2 space-y-1 divide-y divide-border/20 max-h-[60vh]"
            >
              {filteredOptions.length === 0 ? (
                <div className="py-8 px-4 text-center text-xs text-text-muted">
                  Tidak ada opsi yang cocok dengan "{searchQuery}"
                </div>
              ) : (
                filteredOptions.map((opt, index) => {
                  const isSelected = opt.value === value;

                  return (
                    <div
                      key={opt.value || `mobile-empty-${index}`}
                      id={`${selectId}-mobile-opt-${index}`}
                      role="option"
                      aria-selected={isSelected}
                      onClick={() => handleSelect(opt.value, opt.disabled)}
                      className={`flex items-center justify-between gap-3 px-3 py-3 rounded-xl text-xs cursor-pointer transition-colors duration-100 ${
                        opt.disabled
                          ? 'opacity-40 cursor-not-allowed text-text-muted'
                          : isSelected
                          ? 'bg-accent/15 text-white font-medium'
                          : 'text-text-secondary hover:text-white hover:bg-bg-surface-2'
                      }`}
                    >
                      <div className="flex items-center gap-3 truncate min-w-0 flex-1">
                        {opt.icon && <span className="flex-shrink-0">{opt.icon}</span>}
                        <div className="truncate">
                          <div className={`truncate font-sans ${isSelected ? 'text-white font-semibold' : 'text-text-primary'}`}>
                            {opt.label}
                          </div>
                          {opt.description && (
                            <div className="text-[10px] text-text-muted font-mono truncate mt-0.5">
                              {opt.description}
                            </div>
                          )}
                        </div>
                      </div>

                      {/* Custom Route-X Radio Indicator (Accent Circle #BEF264) */}
                      <div
                        className={`w-4 h-4 rounded-full border flex items-center justify-center shrink-0 transition-colors ${
                          isSelected
                            ? 'border-accent bg-accent/20'
                            : 'border-border bg-bg-surface'
                        }`}
                      >
                        {isSelected && <div className="w-1.5 h-1.5 rounded-full bg-accent" />}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>,
        document.body
      )}
    </div>
  );
};

