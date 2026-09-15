import React, { useId } from 'react';
import { Check } from 'lucide-react';

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
      className={`inline-flex items-start gap-2.5 select-none cursor-pointer ${
        disabled ? 'opacity-40 cursor-not-allowed' : ''
      } ${className}`}
    >
      <div className="relative flex items-center justify-center shrink-0 mt-0.5">
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
          className={`w-4 h-4 rounded border transition-all duration-150 flex items-center justify-center ${
            checked
              ? 'bg-accent border-accent text-black shadow-[0_0_8px_rgba(163,230,53,0.35)]'
              : 'bg-bg-surface-2 border-border/80 hover:border-accent/60'
          } peer-focus-visible:ring-2 peer-focus-visible:ring-accent peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-bg-base`}
        >
          {checked && <Check className="w-3 h-3 stroke-[3]" />}
        </div>
      </div>
      {(label || description) && (
        <div className="flex flex-col">
          {label && <span className="text-xs text-text-primary font-medium">{label}</span>}
          {description && <span className="text-[11px] text-text-muted mt-0.5">{description}</span>}
        </div>
      )}
    </label>
  );
};
