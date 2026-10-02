import React from 'react';
import { ChevronRight } from 'lucide-react';
import { formatUSD } from '../../utils/money';
import type { RequestLog } from '../../types';

export interface RequestTableRowProps {
  request: RequestLog;
  onInspect: (req: RequestLog) => void;
}

export const RequestTableRow: React.FC<RequestTableRowProps> = ({
  request,
  onInspect,
}) => {
  const handleKeyDown = (e: React.KeyboardEvent<HTMLTableRowElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onInspect(request);
    }
  };

  const getStatusBadgeClass = (code: number) => {
    if (code >= 200 && code < 300) {
      return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20';
    }
    if (code >= 400 && code < 500) {
      return 'bg-amber-500/10 text-amber-400 border-amber-500/20';
    }
    return 'bg-rose-500/10 text-rose-400 border-rose-500/20';
  };

  const formattedTime = new Date(request.created_at).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });

  const method = request.method || 'POST';
  const path = request.path || '/v1/chat/completions';
  const modelName = request.requested_model || request.model_id || '-';
  const providerName = request.provider_name || request.provider_id;

  return (
    <tr
      role="button"
      tabIndex={0}
      onKeyDown={handleKeyDown}
      onClick={() => onInspect(request)}
      className="hover:bg-bg-surface-2/60 cursor-pointer transition-colors group select-none text-xs"
      data-testid={`request-row-${request.request_id}`}
    >
      {/* 1. Timestamp & Request ID */}
      <td className="py-2.5 px-4 font-mono text-text-primary whitespace-nowrap">
        <div>{formattedTime}</div>
        <div className="text-[10px] text-text-muted truncate max-w-[120px]" title={request.request_id}>
          {request.request_id}
        </div>
      </td>

      {/* 2. Method */}
      <td className="py-2.5 px-4 whitespace-nowrap">
        <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold uppercase tracking-wider bg-blue-500/10 text-blue-400 border border-blue-500/20">
          {method}
        </span>
      </td>

      {/* 3. Path & Type */}
      <td className="py-2.5 px-4 whitespace-nowrap">
        <div className="font-mono text-text-primary text-xs">{path}</div>
        <div className="text-[10px] text-text-muted font-mono">
          {request.is_stream ? 'Stream (SSE)' : 'Unary HTTP'}
        </div>
      </td>

      {/* 4. Status Badge */}
      <td className="py-2.5 px-4 whitespace-nowrap">
        <span
          className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-inner text-[11px] font-mono font-semibold border ${getStatusBadgeClass(
            request.status_code
          )}`}
        >
          <span
            className={`w-1.5 h-1.5 rounded-full ${
              request.status_code < 300
                ? 'bg-emerald-400'
                : request.status_code < 500
                ? 'bg-amber-400'
                : 'bg-rose-400'
            }`}
          />
          {request.status_code}
        </span>
      </td>

      {/* 5. Model & Upstream */}
      <td className="py-2.5 px-4 whitespace-nowrap">
        <div className="font-medium text-text-primary truncate max-w-[180px]" title={modelName}>
          {modelName}
        </div>
        {providerName && (
          <div className="text-[10px] text-text-muted truncate max-w-[180px]" title={providerName}>
            {providerName}
          </div>
        )}
      </td>

      {/* 6. Latency & TTFT */}
      <td className="py-2.5 px-4 whitespace-nowrap font-mono">
        <div className="text-text-primary">{request.duration_ms} ms</div>
        {request.ttft_ms ? (
          <div className="text-[10px] text-text-muted">TTFT: {request.ttft_ms} ms</div>
        ) : null}
      </td>

      {/* 7. Cost & Tokens */}
      <td className="py-2.5 px-4 whitespace-nowrap font-mono">
        <div className="text-text-primary">{formatUSD(request.cost_usd, 6)}</div>
        {request.total_tokens != null && (
          <div className="text-[10px] text-text-muted">
            {request.total_tokens.toLocaleString()} tok
          </div>
        )}
      </td>

      {/* 8. Inspect Action */}
      <td className="py-2.5 px-4 text-right whitespace-nowrap">
        <div className="inline-flex items-center gap-1 text-[11px] text-text-muted group-hover:text-accent transition-colors font-medium">
          <span>Detail</span>
          <ChevronRight className="w-3.5 h-3.5 group-hover:translate-x-0.5 transition-transform" />
        </div>
      </td>
    </tr>
  );
};
