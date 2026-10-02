import React from 'react';
import { Loader2 } from 'lucide-react';
import { cn } from '../../utils/cn';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  size?: 'sm' | 'md' | 'lg';
  isLoading?: boolean;
  icon?: React.ReactNode;
}

export const Button: React.FC<ButtonProps> = ({
  children,
  variant = 'secondary',
  size = 'md',
  isLoading = false,
  icon,
  className = '',
  disabled,
  type = 'button',
  ...props
}) => {
  const base =
    'inline-flex items-center justify-center gap-1.5 font-medium transition-all duration-150 ease-out active:scale-[0.98] disabled:active:scale-100 disabled:opacity-50 disabled:cursor-not-allowed select-none rounded-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-bg-base cursor-pointer';

  const variants = {
    primary:
      'bg-accent text-black hover:bg-accent-hover font-semibold shadow-[0_0_12px_rgba(190,242,100,0.15)] hover:shadow-[0_0_16px_rgba(190,242,100,0.25)] border border-transparent',
    secondary:
      'bg-white/[0.03] hover:bg-white/[0.07] border border-white/[0.08] text-zinc-300 hover:text-white',
    danger:
      'bg-rose-500/10 text-rose-400 border border-rose-500/20 hover:bg-rose-500/20 hover:text-rose-300',
    ghost:
      'text-zinc-400 hover:text-zinc-200 hover:bg-white/[0.04]',
  };

  const sizes = {
    sm: 'text-xs px-2.5 py-1.5',
    md: 'text-xs sm:text-sm px-3.5 py-2',
    lg: 'text-sm sm:text-base px-5 py-2.5',
  };

  return (
    <button
      type={type}
      aria-busy={isLoading}
      className={cn(base, variants[variant], sizes[size], className)}
      disabled={disabled || isLoading}
      {...props}
    >
      {isLoading ? <Loader2 className="w-4 h-4 animate-spin shrink-0" aria-hidden="true" /> : icon}
      {children}
    </button>
  );
};
