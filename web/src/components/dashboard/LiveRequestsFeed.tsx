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
    <div className="bg-bg-surface border border-border/80 rounded-card p-4 sm:p-5 shadow-sm flex flex-col justify-between">
      <div>
        {/* Header — live pulse dot + title */}
        <div className="flex items-center justify-between mb-3.5">
          <div className="flex items-center gap-2">
            <span className="relative flex h-2 w-2">
              <span
                className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
                aria-hidden="true"
              />
              <span
                className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.8)]"
                aria-hidden="true"
              />
            </span>
            <h3 className="text-sm font-semibold text-white tracking-tight">
              Live Requests Feed
            </h3>
          </div>
          <span className="text-[10px] font-mono text-text-muted">
            Auto-refresh (5s)
          </span>
        </div>

        {/* List of recent requests */}
        <div className="space-y-2.5">
          {recentRequests.length === 0 ? (
            <div className="py-12 text-center text-xs text-text-muted space-y-2">
              <Activity className="w-5 h-5 mx-auto text-text-muted" aria-hidden="true" />
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
            recentRequests.map((req) => {
              const isOk = req.status_code >= 200 && req.status_code < 300;
              const isErr = req.status_code >= 400;
              const statusClass = isOk
                ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/25'
                : isErr
                ? 'bg-rose-500/10 text-rose-400 border-rose-500/25'
                : 'bg-amber-500/10 text-amber-400 border-amber-500/25';

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
                  className="p-2.5 sm:p-3 rounded-xl bg-bg-surface-2/60 border border-border/70 hover:border-border-hover hover:bg-bg-surface-2 transition-all cursor-pointer group"
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2 min-w-0">
                      {/* Badge status HTTP berkode warna */}
                      <span
                        className={`px-2 py-0.5 rounded-md text-[11px] font-mono font-bold border tabular-nums ${statusClass}`}
                      >
                        {req.status_code}
                      </span>
                      <span className="text-xs font-mono font-semibold text-white truncate max-w-[140px] group-hover:text-blue-400 transition-colors">
                        {req.requested_model || req.model_id || 'unknown'}
                      </span>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      <span className="text-[10px] text-text-muted font-mono whitespace-nowrap">
                        {formatTimeAgo(req.created_at)}
                      </span>
                      {/* Tombol inspect */}
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          onNavigate('/requests');
                        }}
                        aria-label={`Inspeksi request ${req.requested_model || req.model_id}`}
                        className="p-1 text-text-muted hover:text-white rounded-md hover:bg-bg-surface-3 transition-colors cursor-pointer"
                        title="Inspeksi jejak audit request"
                      >
                        <Eye className="w-3.5 h-3.5" aria-hidden="true" />
                      </button>
                    </div>
                  </div>

                  <div className="mt-2 flex items-center justify-between text-[11px] font-mono text-text-secondary">
                    <span className="text-text-muted flex items-center gap-1.5 truncate">
                      <span className="truncate">{req.provider_name || 'auto-routed'}</span>
                      {req.is_stream && (
                        <span className="text-[9px] px-1.5 py-0.2 rounded bg-blue-500/15 text-blue-400 font-semibold border border-blue-500/25">
                          SSE
                        </span>
                      )}
                    </span>

                    <div className="flex items-center gap-2 shrink-0">
                      {/* Latency pill */}
                      <span className="px-1.5 py-0.5 rounded bg-bg-surface-3 text-text-primary text-[10px] border border-border/60 tabular-nums font-semibold">
                        {req.duration_ms}ms
                      </span>
                      <span className="text-border" aria-hidden="true">·</span>
                      <span className="text-text-muted tabular-nums">
                        {req.total_tokens || 0} tok
                      </span>
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {/* Footer — inspeksi button */}
      <div className="mt-4 pt-2">
        <button
          type="button"
          onClick={() => onNavigate('/requests')}
          className="w-full min-h-[38px] py-2 text-center text-xs text-text-muted hover:text-white font-mono rounded-xl bg-bg-surface-2/60 hover:bg-bg-surface-2 border border-border/70 hover:border-blue-500/30 transition-colors cursor-pointer flex items-center justify-center gap-1.5"
        >
          <span>Inspeksi Seluruh Jejak Audit Request</span>
          <ExternalLink className="w-3.5 h-3.5 text-text-muted group-hover:text-blue-400" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
};
