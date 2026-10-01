import React from 'react';
import { ChevronRight } from 'lucide-react';
import { StatusDot } from '../common/StatusDot';
import { formatUSD } from '../../utils/money';
import type { RequestLog } from '../../types';
import { getHttpStatusDotVariant } from './utils';

export interface RequestItemProps {
  request: RequestLog;
  onInspect: (req: RequestLog) => void;
}

export const RequestItem: React.FC<RequestItemProps> = ({ request, onInspect }) => {
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
      data-testid={`request-row-${request.request_id}`}
      onKeyDown={handleKeyDown}
      onClick={() => onInspect(request)}
      className="group relative flex flex-col sm:flex-row sm:items-center justify-between p-4 bg-bg-surface/40 backdrop-blur-md rounded-2xl border border-transparent hover:border-border/50 hover:bg-bg-surface hover:shadow-xl hover:shadow-black/20 hover:-translate-y-0.5 transition-all duration-300 cursor-pointer overflow-hidden"
    >
      {/* Decorative gradient blob on hover */}
      <div className="absolute inset-0 bg-gradient-to-r from-accent/0 via-accent/5 to-accent/0 opacity-0 group-hover:opacity-100 transition-opacity duration-500 pointer-events-none" />

      <div className="flex flex-col sm:flex-row gap-4 sm:gap-6 flex-1 relative z-10 w-full">
        {/* Status & ID */}
        <div className="flex flex-col flex-1 sm:min-w-[200px] w-full">
          <div className="flex items-center gap-2 mb-1">
            <StatusDot
              status={getHttpStatusDotVariant(request.status_code)}
              label={String(request.status_code)}
            />
            <span className="font-mono font-medium text-sm text-text-primary truncate max-w-[200px]">
              {request.request_id}
            </span>
          </div>
          <div className="text-xs text-text-muted font-mono flex items-center gap-2">
            <span>{request.is_stream ? 'Stream (SSE)' : 'Unary HTTP'}</span>
            <span className="w-1 h-1 rounded-full bg-border" />
            <span>{new Date(request.created_at).toLocaleTimeString()}</span>
          </div>
        </div>

        {/* Info Grid for Mobile, Row for Desktop */}
        <div className="grid grid-cols-2 sm:flex sm:flex-row gap-4 sm:gap-6 flex-1 w-full">
          {/* Model & Provider */}
          <div className="flex flex-col flex-1">
            <span className="text-sm font-semibold text-text-primary truncate" title={request.requested_model || request.model_id || '-'}>
              {request.requested_model || request.model_id || '-'}
            </span>
            <span className="text-xs text-text-muted truncate">
              {request.provider_name || request.provider_id || '-'}
            </span>
          </div>

          {/* Token & Cost */}
          <div className="flex flex-col flex-1">
            <span className="font-mono text-sm text-text-primary">
              {request.total_tokens != null ? `${request.total_tokens.toLocaleString()} tok` : '-'}
            </span>
            <span className="text-xs text-text-muted font-mono">
              {formatUSD(request.cost_usd, 6)}
            </span>
          </div>

          {/* Duration */}
          <div className="flex flex-col flex-1 col-span-2 sm:col-span-1">
            <span className="font-mono text-sm text-text-primary">
              {request.duration_ms} ms
            </span>
            <span className="text-xs text-text-muted font-mono">
              {request.ttft_ms ? `TTFT: ${request.ttft_ms} ms` : '-'}
            </span>
          </div>
        </div>
      </div>

      <div className="absolute sm:static top-4 right-4 sm:top-auto sm:right-auto sm:mt-0 flex justify-end shrink-0 relative z-10 sm:w-[80px]">
        <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-bg-surface-2 text-text-secondary sm:opacity-0 sm:group-hover:opacity-100 hover:bg-accent/10 hover:text-accent transition-all duration-300">
          <span className="text-xs font-semibold hidden sm:inline-block">Inspect</span>
          <ChevronRight className="w-4 h-4" />
        </div>
      </div>
    </div>
  );
};
