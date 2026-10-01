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
      className="group relative flex flex-col p-3 bg-black rounded-xl border border-white/5 hover:border-white/10 hover:bg-white/[0.02] transition-colors cursor-pointer overflow-hidden"
    >
      {/* Header: Status and Path/Method */}
      <div className="flex items-start justify-between mb-2">
        <div className="flex items-center gap-2">
          <StatusDot
            status={getHttpStatusDotVariant(request.status_code)}
            label={String(request.status_code)}
          />
          <span className="font-mono font-medium text-xs text-white/90 truncate max-w-[200px]">
            {request.request_id}
          </span>
        </div>
        <div className="flex items-center gap-1.5 px-2 py-1 rounded bg-white/5 text-white/70 group-hover:bg-white/10 group-hover:text-white transition-colors">
          <span className="text-[10px] font-semibold uppercase tracking-wider">Inspect</span>
          <ChevronRight className="w-3 h-3" />
        </div>
      </div>

      {/* Grid Metadata */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2 mb-2">
        {/* Model */}
        <div className="flex flex-col bg-white/[0.03] rounded-lg p-2">
          <span className="text-[10px] text-white/50 uppercase tracking-wider mb-0.5">Model</span>
          <span className="text-xs font-semibold text-white/90 truncate" title={request.requested_model || request.model_id || '-'}>
            {request.requested_model || request.model_id || '-'}
          </span>
        </div>

        {/* Tokens & Cost */}
        <div className="flex flex-col bg-white/[0.03] rounded-lg p-2">
          <span className="text-[10px] text-white/50 uppercase tracking-wider mb-0.5">Cost & Usage</span>
          <div className="flex items-center gap-1.5 text-xs text-white/80 font-mono truncate">
            <span>{request.total_tokens != null ? `${request.total_tokens}tk` : '-'}</span>
            <span className="w-1 h-1 rounded-full bg-white/20" />
            <span>{formatUSD(request.cost_usd, 6)}</span>
          </div>
        </div>

        {/* Latency */}
        <div className="flex flex-col bg-white/[0.03] rounded-lg p-2">
          <span className="text-[10px] text-white/50 uppercase tracking-wider mb-0.5">Latency</span>
          <div className="flex items-center gap-1.5 text-xs text-white/80 font-mono truncate">
            <span>{request.duration_ms}ms</span>
            {request.ttft_ms ? (
              <>
                <span className="w-1 h-1 rounded-full bg-white/20" />
                <span className="text-white/50">ttft: {request.ttft_ms}ms</span>
              </>
            ) : null}
          </div>
        </div>

        {/* Type & Time */}
        <div className="flex flex-col bg-white/[0.03] rounded-lg p-2">
          <span className="text-[10px] text-white/50 uppercase tracking-wider mb-0.5">Time & Type</span>
          <div className="flex items-center gap-1.5 text-xs text-white/80 font-mono truncate">
            <span>{new Date(request.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
            <span className="w-1 h-1 rounded-full bg-white/20" />
            <span className="text-white/50">{request.is_stream ? 'SSE' : 'HTTP'}</span>
          </div>
        </div>
      </div>
    </div>
  );
};
