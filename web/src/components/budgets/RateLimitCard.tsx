import React from 'react';
import { Card } from '../common/Card';
import { Badge } from '../common/Badge';
import { Tooltip } from '../common/Tooltip';
import { Gauge, Trash2 } from 'lucide-react';
import type { RateLimitCardProps } from './types';

export const RateLimitCard: React.FC<RateLimitCardProps> = ({
  limit,
  scopeDisplay,
  onDelete,
}) => {
  return (
    <Card className="p-5 flex flex-col justify-between" data-testid={`rate-limit-card-${limit.id}`}>
      <div>
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-full bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center shrink-0">
              <Gauge className="w-5 h-5" />
            </div>
            <div>
              <h4 className="text-sm font-bold text-white uppercase tracking-wider">{limit.scope}</h4>
              <span className="text-xs text-text-muted font-mono">
                {scopeDisplay.label}
              </span>
            </div>
          </div>
          <Badge variant="info">Aktif</Badge>
        </div>

        <div className="mt-5 grid grid-cols-3 gap-2 p-3 bg-bg-surface-2 rounded-lg border border-border/50 text-center">
          <div>
            <span className="block text-[10px] text-text-muted uppercase">Req / Mnt</span>
            <span className="font-mono text-sm font-bold text-white mt-0.5 block">
              {(limit.requests_per_minute ?? 0) > 0 ? (limit.requests_per_minute ?? 0).toLocaleString() : '∞'}
            </span>
          </div>
          <div>
            <span className="block text-[10px] text-text-muted uppercase">Tok / Mnt</span>
            <span className="font-mono text-sm font-bold text-accent mt-0.5 block">
              {(limit.tokens_per_minute ?? 0) > 0 ? (limit.tokens_per_minute ?? 0).toLocaleString() : '∞'}
            </span>
          </div>
          <div>
            <span className="block text-[10px] text-text-muted uppercase">Burst / Dtk</span>
            <span className="font-mono text-sm font-bold text-white mt-0.5 block">
              {(limit.requests_per_second ?? 0) > 0 ? `${limit.requests_per_second}/s` : '∞'}
            </span>
          </div>
        </div>
      </div>

      <div className="mt-5 pt-3 border-t border-border flex justify-end">
        <Tooltip content="Hapus Aturan Limit" position="left">
          <button
            type="button"
            onClick={() => void onDelete(limit.id)}
            className="p-1.5 rounded-md text-text-muted hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
            aria-label="Hapus limit"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        </Tooltip>
      </div>
    </Card>
  );
};
