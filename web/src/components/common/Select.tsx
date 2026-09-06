import React, { useState, useRef, useEffect, useId } from 'react';
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
}) => {
  const generatedId = useId();
  const selectId = id || generatedId;
  const [isOpen, setIsOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [highlightedIndex, setHighlightedIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

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

  // Tangani klik di luar untuk menutup menu
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent | TouchEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
        setSearchQuery('');
      }
    };

    if (isOpen) {
      document.addEventListener('mousedown', handleOutsideClick);
      document.addEventListener('touchstart', handleOutsideClick);
    }
    return () => {
      document.removeEventListener('mousedown', handleOutsideClick);
      document.removeEventListener('touchstart', handleOutsideClick);
    };
  }, [isOpen]);

  // Fokuskan input pencarian saat menu terbuka
  useEffect(() => {
    if (isOpen && isSearchable && searchInputRef.current) {
      setTimeout(() => {
        searchInputRef.current?.focus();
      }, 50);
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

  return (
    <div className={`relative w-full text-left ${className}`} ref={containerRef} onKeyDown={handleKeyDown}>
      {label && (
        <label htmlFor={selectId} className="block text-xs font-semibold text-text-secondary uppercase mb-1">
          {label} {required && <span className="text-status-error">*</span>}
        </label>
      )}

      {/* Trigger Button - In-App Styled */}
      <button
        id={selectId}
        type="button"
        disabled={disabled}
        onClick={() => {
          if (!disabled) {
            setIsOpen((prev) => !prev);
            setSearchQuery('');
          }
        }}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        className={`w-full flex items-center justify-between gap-2 px-3 py-2 bg-bg-surface-2 border rounded-nav text-xs transition-all duration-150 text-left focus:outline-none focus:ring-1 focus:ring-accent ${
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
          className={`w-4 h-4 text-text-muted transition-transform duration-200 flex-shrink-0 ${
            isOpen ? 'rotate-180 text-accent' : ''
          }`}
        />
      </button>

      {error && <span className="text-[11px] text-status-error mt-1 block">{error}</span>}

      {/* Floating Custom Dropdown List (100% In-App Dark Glassmorphism) */}
      {isOpen && (
        <div
          className="absolute z-50 left-0 right-0 mt-1 bg-[#141416] border border-border/90 rounded-xl shadow-2xl backdrop-blur-xl overflow-hidden animate-in fade-in zoom-in-95 duration-150 flex flex-col max-h-64"
          style={{ minWidth: '100%' }}
        >
          {/* Kolom Pencarian Cepat Inline */}
          {isSearchable && (
            <div className="p-2 border-b border-border/60 bg-bg-surface-2/40 sticky top-0 z-10 flex items-center gap-1.5">
              <Search className="w-3.5 h-3.5 text-text-muted flex-shrink-0" />
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
                  className="p-0.5 text-text-muted hover:text-white rounded"
                >
                  <X className="w-3 h-3" />
                </button>
              )}
            </div>
          )}

          {/* Opsi Listbox */}
          <div
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
    </div>
  );
};
