import React from 'react';
import { ChevronRight } from 'lucide-react';
import { StatusDot } from '../common/StatusDot';
import { formatUSD } from '../../utils/money';
import type { RequestLog } from '../../types';
import {
  getHttpStatusDotVariant,
  getLatencyPillStyle,
  getHttpStatusColor,
} from './utils';

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

  const latencyStyle = getLatencyPillStyle(request.duration_ms);
  const statusColor = getHttpStatusColor(request.status_code);

  return (
    <tr
      role="button"
      tabIndex={0}
      onKeyDown={handleKeyDown}
      onClick={() => onInspect(request)}
      aria-label={`Lihat detail permintaan ${request.request_id}, status ${request.status_code}`}
      className="hover:bg-bg-surface-2/60 active:bg-bg-surface-2 cursor-pointer transition-colors group focus:outline-none focus-visible:bg-bg-surface-2 focus-visible:ring-1 focus-visible:ring-accent"
      data-testid={`request-row-${request.request_id}`}
    >
      {/* Kolom 1: Status & Request ID */}
      <td className="py-3 px-4">
        <div className="flex items-center gap-2">
          <StatusDot
            status={getHttpStatusDotVariant(request.status_code)}
            label={String(request.status_code)}
          />
          <span
            className={`font-mono font-bold text-xs px-1.5 py-0.5 rounded border ${statusColor.bg} ${statusColor.text} ${statusColor.border}`}
          >
            {request.status_code}
          </span>
          <span className="font-mono font-medium text-text-primary truncate max-w-[140px]">
            {request.request_id}
          </span>
        </div>
        <div className="text-[11px] text-text-muted font-mono mt-1 flex items-center gap-1.5">
          <span className="px-1.5 py-0.5 rounded bg-bg-surface-3/60 text-text-secondary border border-border/50 text-[10px]">
            {request.is_stream ? 'Stream (SSE)' : 'Unary HTTP'}
          </span>
        </div>
      </td>

      {/* Kolom 2: Model & Provider */}
      <td className="py-3 px-4">
        <div className="font-semibold text-text-primary flex items-center gap-1.5">
          <span>{request.requested_model || request.model_id || '-'}</span>
        </div>
        <div className="text-[11px] text-text-muted font-mono mt-0.5 truncate max-w-[160px]">
          {request.provider_name || request.provider_id || '-'}
        </div>
      </td>

      {/* Kolom 3: Token & Cost */}
      <td className="py-3 px-4">
        <div className="font-mono text-text-primary font-medium">
          {request.total_tokens != null ? `${request.total_tokens.toLocaleString()} tok` : '-'}
        </div>
        <div className="text-[11px] text-accent font-mono mt-0.5">
          {formatUSD(request.cost_usd, 6)}
        </div>
      </td>

      {/* Kolom 4: Durasi & Latency Pill & TTFT */}
      <td className="py-3 px-4">
        <div className="flex items-center gap-1.5">
          <span className="font-mono text-text-primary font-semibold">{request.duration_ms} ms</span>
          {/* Latency Pill (<100ms hijau, <300ms biru, >500ms kuning) */}
          <span
            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-chip text-[10px] font-mono font-bold border ${latencyStyle.bg} ${latencyStyle.text} ${latencyStyle.border}`}
            title={`Latensi ${request.duration_ms}ms (${latencyStyle.label})`}
          >
            <span className={`w-1.5 h-1.5 rounded-full ${latencyStyle.dot}`} aria-hidden="true" />
            <span>{latencyStyle.label}</span>
          </span>
        </div>
        <div className="text-[11px] text-text-muted font-mono mt-0.5">
          {request.ttft_ms ? `TTFT: ${request.ttft_ms} ms` : '-'}
        </div>
      </td>

      {/* Kolom 5: Waktu & Klien */}
      <td className="py-3 px-4">
        <div className="text-text-primary font-mono text-[11px]">
          {new Date(request.created_at).toLocaleTimeString()}
        </div>
        <div className="text-[11px] text-text-muted font-mono mt-0.5 truncate max-w-[120px]">
          {request.client_ip || '-'}
        </div>
      </td>

      {/* Kolom 6: Aksi */}
      <td className="py-3 px-4 text-right">
        <span className="inline-flex items-center gap-1 text-accent opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity font-medium">
          Detail <ChevronRight className="w-3.5 h-3.5" aria-hidden="true" />
        </span>
      </td>
    </tr>
  );
};
