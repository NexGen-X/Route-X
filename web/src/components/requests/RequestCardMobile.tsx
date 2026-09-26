import React from 'react';
import { ChevronRight } from 'lucide-react';
import { StatusDot } from '../common/StatusDot';
import type { RequestLog } from '../../types';
import { getHttpStatusDotVariant } from './utils';

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

  return (
    <div
      role="button"
      tabIndex={0}
      onKeyDown={handleKeyDown}
      onClick={() => onInspect(request)}
      className="p-3.5 hover:bg-bg-surface-2/60 active:bg-bg-surface-2 cursor-pointer transition-colors space-y-2"
      data-testid={`request-card-mobile-${request.request_id}`}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 min-w-0">
          <StatusDot
            status={getHttpStatusDotVariant(request.status_code)}
            label={String(request.status_code)}
          />
          <span className="font-mono text-xs font-semibold text-white truncate max-w-[130px]">
            {request.requested_model || request.model_id || 'unknown'}
          </span>
        </div>
        <span className="text-[10px] text-text-muted font-mono whitespace-nowrap">
          {new Date(request.created_at).toLocaleTimeString()}
        </span>
      </div>

      <div className="flex items-center justify-between text-[11px] font-mono text-text-secondary">
        <span className="text-text-muted truncate max-w-[140px]">
          {request.provider_name || request.provider_id || '-'}
          {request.is_stream && (
            <span className="ml-1 text-[9px] px-1 rounded bg-bg-surface-3 text-accent border border-border/50">
              SSE
            </span>
          )}
        </span>
        <div className="flex items-center gap-2">
          <span className="text-white font-semibold">{request.duration_ms}ms</span>
          <span className="text-text-muted">·</span>
          <span className="text-emerald-400 font-semibold">
            {request.total_tokens != null ? `${request.total_tokens} tok` : '-'}
          </span>
        </div>
      </div>

      <div className="flex items-center justify-between text-[10px] text-text-muted font-mono pt-1 border-t border-border/60">
        <span className="truncate max-w-[180px]">ID: {request.request_id}</span>
        <span className="text-accent flex items-center gap-0.5 font-medium">
          Detail <ChevronRight className="w-3 h-3" />
        </span>
      </div>
    </div>
  );
};
