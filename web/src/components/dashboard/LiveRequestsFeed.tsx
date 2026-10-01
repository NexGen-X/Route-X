import React from 'react';
import { Activity } from 'lucide-react';
import { Button } from '../common/Button';
import { formatTimeAgo } from './utils';
import { useRecentRequests } from './hooks';

export interface LiveRequestsFeedProps {
  onNavigate: (path: string) => void;
}

export const LiveRequestsFeed: React.FC<LiveRequestsFeedProps> = ({
  onNavigate,
}) => {
  const { data: reqRes } = useRecentRequests();
  const recentRequests = reqRes?.items || [];

  return (
    <div className="bg-bg-surface/40 backdrop-blur-md ring-1 ring-white/5 rounded-2xl shadow-lg p-4 sm:p-5  flex flex-col justify-between">
      <div>
        <div className="flex items-center gap-2 mb-3">
          <span className="w-1.5 h-1.5 rounded-full bg-status-success animate-[shimmer_1.5s_infinite] bg-gradient-to-r from-bg-surface-2 via-bg-surface-3 to-bg-surface-2 bg-[length:400%_100%]" />
          <h3 className="text-sm font-semibold text-white tracking-tight">Live Requests Feed</h3>
        </div>

        <div className="mt-3 space-y-2.5">
          {recentRequests.length === 0 ? (
            <div className="py-12 text-center text-xs text-text-muted space-y-2">
              <Activity className="w-5 h-5 mx-auto text-text-muted" />
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
                  className="p-2.5 rounded-xl bg-bg-surface-2/60 backdrop-blur-sm ring-1 ring-white/5 border-transparent hover:ring-white/20 hover:bg-bg-surface-3 transition-all cursor-pointer group"
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      <span
                        className={`px-1.5 py-0.5 rounded text-[11px] font-mono font-medium ${
                          isOk
                            ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                            : isErr
                            ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                            : 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
                        }`}
                      >
                        {req.status_code}
                      </span>
                      <span className="text-xs font-mono font-medium text-white truncate max-w-[140px] group-hover:text-accent transition-colors">
                        {req.requested_model || req.model_id || 'unknown'}
                      </span>
                    </div>
                    <span className="text-[11px] text-text-muted font-mono whitespace-nowrap">
                      {formatTimeAgo(req.created_at)}
                    </span>
                  </div>

                  <div className="mt-1.5 flex items-center justify-between text-[11px] font-mono text-text-secondary">
                    <span className="text-text-muted flex items-center gap-1">
                      {req.provider_name || 'auto-routed'}
                      {req.is_stream && (
                        <span className="text-[10px] px-1 rounded bg-bg-surface-3 text-text-secondary font-medium">
                          SSE
                        </span>
                      )}
                    </span>
                    <div className="flex items-center gap-2">
                      <span className="text-text-primary">{req.duration_ms}ms</span>
                      <span className="text-border">·</span>
                      <span className="text-text-muted">{req.total_tokens || 0} tok</span>
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      <div className="mt-3">
        <button
          type="button"
          onClick={() => onNavigate('/requests')}
          className="w-full py-1.5 text-center text-xs text-text-muted hover:text-white font-mono rounded-nav bg-bg-surface-2/60 backdrop-blur-sm ring-1 ring-white/5 border-transparent hover:ring-white/20 transition-colors cursor-pointer"
        >
          Inspeksi Seluruh Jejak Audit Request →
        </button>
      </div>
    </div>
  );
};
