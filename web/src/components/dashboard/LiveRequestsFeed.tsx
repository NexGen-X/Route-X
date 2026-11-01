import React from 'react';
import { Activity, ExternalLink, Eye } from 'lucide-react';
import { Button } from '../common/Button';
import type { RequestLog } from '../../types';
import { formatTimeAgo } from './utils';

export interface LiveRequestsFeedProps {
  recentRequests: RequestLog[];
  onNavigate: (path: string) => void;
}

export const LiveRequestsFeed: React.FC<LiveRequestsFeedProps> = ({
  recentRequests,
  onNavigate,
}) => {
  return (
    <div className="flex flex-col justify-between h-full py-1">
      <div>
        {/* Header — clean minimal dot + title */}
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" aria-hidden="true" />
            <h3 className="text-sm font-medium text-white tracking-tight">
              Live Requests Feed
            </h3>
          </div>
          <span className="text-[10px] font-mono text-text-muted">
            Auto-refresh (5s)
          </span>
        </div>

        {/* Tabular List of recent requests */}
        {recentRequests.length === 0 ? (
          <div className="py-10 text-center text-xs text-text-muted space-y-2">
            <Activity className="w-4 h-4 mx-auto text-text-muted" aria-hidden="true" />
            <p>Belum ada permintaan inferensi yang tercatat.</p>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => onNavigate('/cli-integrations')}
              className="mt-2 text-xs"
            >
              Buka Panduan Integrasi
            </Button>
          </div>
        ) : (
          <div className="divide-y divide-border/70 border-y border-border/60">
            {recentRequests.map((req) => {
              const is2xx = req.status_code >= 200 && req.status_code < 300;
              const is4xx = req.status_code >= 400 && req.status_code < 500;
              const is5xx = req.status_code >= 500;
              const statusBadgeClass = is2xx
                ? 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20'
                : is4xx
                ? 'text-amber-400 bg-amber-500/10 border-amber-500/20'
                : is5xx
                ? 'text-rose-400 bg-rose-500/10 border-rose-500/20'
                : 'text-text-secondary bg-bg-surface-2 border-border';

              return (
                <div
                  key={req.id || req.request_id}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      onNavigate('/requests');
                    }
                  }}
                  onClick={() => onNavigate('/requests')}
                  className="py-2 px-1 flex items-center justify-between gap-2 text-xs hover:bg-bg-surface-2/30 transition-colors cursor-pointer group"
                >
                  {/* Kiri: Status code, Model Name, Protocol */}
                  <div className="flex items-center gap-2 min-w-0">
                    <span className={`font-mono font-medium text-[11px] tabular-nums shrink-0 px-1.5 py-0.5 rounded border ${statusBadgeClass}`}>
                      {req.status_code}
                    </span>
                    <span className="font-mono text-xs text-text-primary truncate group-hover:text-white transition-colors">
                      {req.requested_model || req.model_id || 'unknown'}
                    </span>
                    {req.is_stream && (
                      <span className="text-[9px] font-mono px-1.5 py-0.5 rounded text-cyan-400 bg-cyan-500/10 border border-cyan-500/20 shrink-0">
                        SSE
                      </span>
                    )}
                  </div>

                  {/* Kanan: Latency, Tokens, Time */}
                  <div className="flex items-center gap-2 shrink-0 font-mono text-[11px] text-text-secondary">
                    <span className="tabular-nums">{req.duration_ms}ms</span>
                    <span className="text-text-muted select-none" aria-hidden="true">·</span>
                    <span className="text-text-muted tabular-nums hidden sm:inline">{req.total_tokens || 0} tok</span>
                    <span className="text-text-muted select-none hidden sm:inline" aria-hidden="true">·</span>
                    <span className="text-text-muted text-[10px] whitespace-nowrap">
                      {formatTimeAgo(req.created_at)}
                    </span>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        onNavigate('/requests');
                      }}
                      aria-label={`Inspeksi request ${req.requested_model || req.model_id}`}
                      className="p-1 text-text-muted hover:text-white rounded hover:bg-bg-surface-2 transition-colors"
                      title="Inspeksi jejak audit request"
                    >
                      <Eye className="w-3.5 h-3.5" aria-hidden="true" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Footer — inspeksi button */}
      <div className="mt-3 pt-2">
        <button
          type="button"
          onClick={() => onNavigate('/requests')}
          className="w-full py-1.5 text-center text-xs text-accent-text hover:text-white font-medium rounded-md transition-colors cursor-pointer flex items-center justify-center gap-1.5"
        >
          <span>Inspeksi Seluruh Jejak Audit Request</span>
          <ExternalLink className="w-3 h-3 text-text-muted" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
};
