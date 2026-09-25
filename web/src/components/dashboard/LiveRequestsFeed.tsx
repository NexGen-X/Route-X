import React from 'react';
import { Activity } from 'lucide-react';
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
    <div className="bg-[#121316] border border-[#20242D] rounded-xl p-5 shadow-lg flex flex-col justify-between">
      <div>
        <div className="flex items-center justify-between pb-3 border-b border-[#1C2029]">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            <h3 className="text-sm font-semibold text-white tracking-tight">Live Requests Feed</h3>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onNavigate('/requests')}
            className="text-xs text-primary hover:text-primary-hover p-0 h-auto"
          >
            Semua &rarr;
          </Button>
        </div>

        <div className="mt-3 space-y-2.5">
          {recentRequests.length === 0 ? (
            <div className="py-12 text-center text-xs text-text-muted space-y-2">
              <Activity className="w-5 h-5 mx-auto text-[#2A2F3D]" />
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
                  className="p-2.5 rounded-lg bg-[#181A20] border border-[#232732] hover:border-primary/40 hover:bg-[#1D2028] transition-all cursor-pointer group"
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      <span
                        className={`px-1.5 py-0.5 rounded text-[10px] font-mono font-bold ${
                          isOk
                            ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                            : isErr
                            ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                            : 'bg-amber-500/10 text-amber-400'
                        }`}
                      >
                        {req.status_code}
                      </span>
                      <span className="text-xs font-mono font-semibold text-white truncate max-w-[140px] group-hover:text-primary transition-colors">
                        {req.requested_model || req.model_id || 'unknown'}
                      </span>
                    </div>
                    <span className="text-[10px] text-text-muted font-mono whitespace-nowrap">
                      {formatTimeAgo(req.created_at)}
                    </span>
                  </div>

                  <div className="mt-1.5 flex items-center justify-between text-[11px] font-mono text-text-secondary">
                    <span className="text-text-muted flex items-center gap-1">
                      {req.provider_name || 'auto-routed'}
                      {req.is_stream && (
                        <span className="text-[9px] px-1 rounded bg-[#252A36] text-primary">
                          SSE
                        </span>
                      )}
                    </span>
                    <div className="flex items-center gap-2">
                      <span className="text-text-primary">{req.duration_ms}ms</span>
                      <span className="text-text-muted">·</span>
                      <span className="text-text-muted">{req.total_tokens || 0} tok</span>
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      <div className="mt-4 pt-3 border-t border-[#1C2029]">
        <button
          type="button"
          onClick={() => onNavigate('/requests')}
          className="w-full py-1.5 text-center text-xs text-text-secondary hover:text-white font-mono rounded bg-[#16181F] border border-[#262B37] hover:border-[#383E4F] transition-colors"
        >
          Inspeksi Seluruh Jejak Audit Request &rarr;
        </button>
      </div>
    </div>
  );
};
