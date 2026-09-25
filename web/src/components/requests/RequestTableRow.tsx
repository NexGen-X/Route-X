import React from 'react';
import { ChevronRight } from 'lucide-react';
import { Badge } from '../common/Badge';
import { formatUSD } from '../../utils/money';
import type { RequestLog } from '../../types';
import { getStatusBadgeLabel, getStatusBadgeVariant } from './utils';

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

  return (
    <tr
      role="button"
      tabIndex={0}
      onKeyDown={handleKeyDown}
      onClick={() => onInspect(request)}
      className="hover:bg-bg-surface-2/60 cursor-pointer transition-colors group"
      data-testid={`request-row-${request.request_id}`}
    >
      <td className="py-3 px-4">
        <div className="flex items-center gap-2">
          <Badge variant={getStatusBadgeVariant(request.status_code)}>
            {getStatusBadgeLabel(request.status_code)}
          </Badge>
          <span className="font-mono font-medium text-text-primary truncate max-w-[140px]">
            {request.request_id}
          </span>
        </div>
        <div className="text-[11px] text-text-muted font-mono mt-0.5">
          {request.is_stream ? 'Stream (SSE)' : 'Unary HTTP'}
        </div>
      </td>

      <td className="py-3 px-4">
        <div className="font-semibold text-text-primary">
          {request.requested_model || request.model_id || '-'}
        </div>
        <div className="text-[11px] text-text-muted font-mono">
          {request.provider_name || request.provider_id || '-'}
        </div>
      </td>

      <td className="py-3 px-4">
        <div className="font-mono text-text-primary">
          {request.total_tokens != null ? request.total_tokens.toLocaleString() : '-'} tokens
        </div>
        <div className="text-[11px] text-text-muted font-mono">
          {formatUSD(request.cost_usd, 6)}
        </div>
      </td>

      <td className="py-3 px-4">
        <div className="font-mono text-text-primary">{request.duration_ms} ms</div>
        <div className="text-[11px] text-text-muted font-mono">
          {request.ttft_ms ? `TTFT: ${request.ttft_ms} ms` : '-'}
        </div>
      </td>

      <td className="py-3 px-4">
        <div className="text-text-primary">
          {new Date(request.created_at).toLocaleTimeString()}
        </div>
        <div className="text-[11px] text-text-muted font-mono">{request.client_ip || '-'}</div>
      </td>

      <td className="py-3 px-4 text-right">
        <span className="inline-flex items-center gap-1 text-accent opacity-0 group-hover:opacity-100 transition-opacity font-medium">
          Detail <ChevronRight className="w-3.5 h-3.5" />
        </span>
      </td>
    </tr>
  );
};
