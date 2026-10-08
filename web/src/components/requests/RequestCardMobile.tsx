import React from 'react';
import { ChevronRight } from 'lucide-react';
import { StatusDot } from '../common/StatusDot';
import type { RequestLog } from '../../types';
import {
  getHttpStatusDotVariant,
  getLatencyPillStyle,
  getHttpStatusColor,
} from './utils';

export interface RequestCardMobileProps {
  request: RequestLog;
  onInspect: (req: RequestLog) => void;
}

export const RequestCardMobile: React.FC<RequestCardMobileProps> = ({
  request,
  onInspect,
}) => {
  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onInspect(request);
    }
  };

  const latencyStyle = getLatencyPillStyle(request.duration_ms);
  const statusColor = getHttpStatusColor(request.status_code);

  return (
    <div
      role="button"
      tabIndex={0}
      onKeyDown={handleKeyDown}
      onClick={() => onInspect(request)}
      aria-label={`Buka detail request ${request.request_id}`}
      className="p-3.5 hover:bg-bg-surface-2/60 active:bg-bg-surface-2 cursor-pointer transition-colors space-y-2.5 focus:outline-none focus-visible:bg-bg-surface-2 focus-visible:ring-1 focus-visible:ring-accent min-h-[44px]"
      data-testid={`request-card-mobile-${request.request_id}`}
    >
      {/* Baris 1: Status Code Pill, Model, & Waktu */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <StatusDot
            status={getHttpStatusDotVariant(request.status_code)}
            label={String(request.status_code)}
          />
          <span
            className={`font-mono font-bold text-xs px-1.5 py-0.5 rounded border ${statusColor.bg} ${statusColor.text} ${statusColor.border}`}
          >
            {request.status_code}
          </span>
          <span className="font-mono text-xs font-semibold text-white truncate max-w-[130px]">
            {request.requested_model || request.model_id || 'unknown'}
          </span>
        </div>
        <span className="text-[10px] text-text-muted font-mono whitespace-nowrap">
          {new Date(request.created_at).toLocaleTimeString()}
        </span>
      </div>

      {/* Baris 2: Provider, Latency Pill, dan Token */}
      <div className="flex items-center justify-between text-[11px] font-mono text-text-secondary gap-2">
        <span className="text-text-muted truncate max-w-[140px] flex items-center gap-1">
          <span>{request.provider_name || request.provider_id || '-'}</span>
          {request.is_stream && (
            <span className="text-[9px] px-1 py-0.2 rounded bg-bg-surface-3 text-accent border border-border/50">
              SSE
            </span>
          )}
        </span>
        <div className="flex items-center gap-2 shrink-0">
          {/* Latency Pill */}
          <span
            className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded-chip text-[9px] font-mono font-bold border ${latencyStyle.bg} ${latencyStyle.text} ${latencyStyle.border}`}
          >
            <span className={`w-1 h-1 rounded-full ${latencyStyle.dot}`} aria-hidden="true" />
            <span>{request.duration_ms}ms</span>
          </span>
          <span className="text-text-muted">·</span>
          {/* Monospaced Token Count */}
          <span className="text-emerald-400 font-semibold font-mono">
            {request.total_tokens != null ? `${request.total_tokens} tok` : '-'}
          </span>
        </div>
      </div>

      {/* Baris 3: ID Request & Tombol Aksi */}
      <div className="flex items-center justify-between text-[10px] text-text-muted font-mono pt-1.5 border-t border-border/60">
        <span className="truncate max-w-[200px]">ID: {request.request_id}</span>
        <span className="text-accent flex items-center gap-0.5 font-medium min-h-[32px] items-center">
          Detail <ChevronRight className="w-3 h-3" aria-hidden="true" />
        </span>
      </div>
    </div>
  );
};
