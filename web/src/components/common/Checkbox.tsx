import React, { useId } from 'react';
import { Check } from 'lucide-react';
import { cn } from '../../utils/cn';

export interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: React.ReactNode;
  description?: React.ReactNode;
}

export const Checkbox: React.FC<CheckboxProps> = ({
  label,
  description,
  checked,
  onChange,
  disabled,
  className = '',
  id,
  ...props
}) => {
  const generatedId = useId();
  const inputId = id || generatedId;

  return (
    <label
      htmlFor={inputId}
      className={cn(
        'inline-flex items-center min-h-[44px] sm:min-h-[36px] py-1 gap-2.5 select-none cursor-pointer',
        disabled && 'opacity-40 cursor-not-allowed',
        className
      )}
    >
      <div className="relative flex items-center justify-center shrink-0">
        <input
          type="checkbox"
          id={inputId}
          checked={checked}
          onChange={onChange}
          disabled={disabled}
          className="peer sr-only"
          {...props}
        />
        <div
          className={cn(
            'w-4 h-4 rounded border transition-all duration-150 flex items-center justify-center',
            checked
              ? 'bg-accent border-accent text-white shadow-[0_0_10px_rgba(59,130,246,0.35)]'
              : 'bg-bg-surface-2/90 border-white/[0.12] hover:border-accent/60',
            'peer-focus-visible:ring-2 peer-focus-visible:ring-accent peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-bg-base'
          )}
        >
          {checked && <Check className="w-3 h-3 stroke-[3]" aria-hidden="true" />}
        </div>
      </div>
      {(label || description) && (
        <div className="flex flex-col min-w-0">
          {label && <span className="text-xs text-text-primary font-medium">{label}</span>}
          {description && <span className="text-[11px] text-text-muted mt-0.5 leading-relaxed">{description}</span>}
        </div>
      )}
    </label>
  );
};
