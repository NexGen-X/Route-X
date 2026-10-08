import React from 'react';
import { cn } from '../../utils/cn';

export type StatusDotVariant =
  | 'healthy'
  | 'degraded'
  | 'unhealthy'
  | 'neutral'
  | 'active'
  | 'disabled';

export interface StatusDotProps {
  status: StatusDotVariant;
  label?: string;
  latencyMs?: number;
  className?: string;
  ping?: boolean;
}

const STATUS_CONFIG: Record<
  StatusDotVariant,
  { dot: string; ping?: string; label: string; glow: string }
> = {
  healthy: {
    dot: 'bg-emerald-400',
    ping: 'bg-emerald-400',
    glow: 'shadow-[0_0_8px_rgba(52,211,153,0.8)]',
    label: 'Sehat',
  },
  active: {
    dot: 'bg-emerald-400',
    ping: 'bg-emerald-400',
    glow: 'shadow-[0_0_8px_rgba(52,211,153,0.8)]',
    label: 'Aktif',
  },
  degraded: {
    dot: 'bg-amber-400',
    ping: 'bg-amber-400',
    glow: 'shadow-[0_0_8px_rgba(251,191,36,0.8)]',
    label: 'Terdegradasi',
  },
  unhealthy: {
    dot: 'bg-rose-400',
    ping: 'bg-rose-400',
    glow: 'shadow-[0_0_8px_rgba(251,113,133,0.8)]',
    label: 'Terganggu',
  },
  disabled: {
    dot: 'bg-zinc-600',
    glow: '',
    label: 'Nonaktif',
  },
  neutral: {
    dot: 'bg-zinc-400',
    glow: '',
    label: 'Netral',
  },
};

/**
 * Komponen StatusDot bergaya Linear/Vercel-grade untuk menggantikan badge tebal.
 * Menampilkan bulatan halus dengan animasi ping glow mikro, label semantik, dan latensi monospaced.
 */
export const StatusDot: React.FC<StatusDotProps> = ({
  status,
  label,
  latencyMs,
  className = '',
  ping,
}) => {
  const config = STATUS_CONFIG[status] || STATUS_CONFIG.neutral;
  const displayLabel = label ?? config.label;
  const shouldPing = ping ?? (status === 'healthy' || status === 'active');

  return (
    <div
      className={cn(
        'inline-flex items-center gap-2 text-xs text-text-secondary select-none',
        className
      )}
    >
      <span className="relative flex h-2 w-2 items-center justify-center shrink-0">
        {shouldPing && config.ping && (
          <span
            className={cn(
              'absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping-subtle',
              config.ping
            )}
            aria-hidden="true"
          />
        )}
        <span
          className={cn(
            'relative inline-flex h-1.5 w-1.5 rounded-full shrink-0',
            config.dot,
            config.glow
          )}
          aria-hidden="true"
        />
      </span>
      {displayLabel && (
        <span className="text-xs text-text-secondary font-medium tracking-tight font-sans">
          {displayLabel}
        </span>
      )}
      {typeof latencyMs === 'number' && latencyMs >= 0 && (
        <span className="text-[11px] text-text-muted font-mono">{latencyMs}ms</span>
      )}
    </div>
  );
};
