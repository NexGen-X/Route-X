import React, { useEffect, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { Modal } from '../common/Modal';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { QueryError } from '../common/QueryError';
import { useToast } from '../../context/ToastContext';
import { copyTextToClipboard } from '../../utils/clipboard';
import { formatUSD } from '../../utils/money';
import type { RequestLog, RequestEvent, RequestPayload } from '../../types';
import {
  formatRawPayload,
  generateCurlCommand,
  getStatusBadgeLabel,
  getStatusBadgeVariant,
} from './utils';

export interface RequestInspectorModalProps {
  isOpen: boolean;
  onClose: () => void;
  request: RequestLog | null;
  events: RequestEvent[];
  payload: RequestPayload | null;
  error?: string | null;
  onRetry?: () => void;
}

const WATERFALL_COLORS = [
  'bg-accent',
  'bg-cyan-500',
  'bg-purple-500',
  'bg-amber-500',
  'bg-emerald-500',
];

export const RequestInspectorModal: React.FC<RequestInspectorModalProps> = ({
  isOpen,
  onClose,
  request,
  events,
  payload,
  error,
  onRetry,
}) => {
  const { toast } = useToast();
  const [isCurlCopied, setIsCurlCopied] = useState(false);
  const curlTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (curlTimerRef.current) {
        clearTimeout(curlTimerRef.current);
        curlTimerRef.current = null;
      }
    };
  }, []);

  if (!request) return null;

  const handleCopyAsCurl = async () => {
    const curlCmd = generateCurlCommand(request, payload);
    try {
      await copyTextToClipboard(curlCmd);
      setIsCurlCopied(true);
      toast.success('Perintah cURL disalin ke clipboard.');
      if (curlTimerRef.current) clearTimeout(curlTimerRef.current);
      curlTimerRef.current = setTimeout(() => {
        curlTimerRef.current = null;
        setIsCurlCopied(false);
      }, 2000);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`Gagal menyalin perintah cURL: ${msg}`);
    }
  };

  const totalEventLatency =
    events.reduce((sum, ev) => sum + (ev.latency_ms || 0), 0) || 1;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Detail Request & Timeline Event"
      maxWidth="2xl"
      footer={
        <div className="flex items-center justify-between w-full">
          <Button
            size="sm"
            variant={isCurlCopied ? 'primary' : 'secondary'}
            onClick={handleCopyAsCurl}
            title={isCurlCopied ? 'Tersalin!' : 'Salin sebagai perintah cURL'}
            aria-label={isCurlCopied ? 'Tersalin' : 'Salin sebagai cURL'}
            icon={
              isCurlCopied ? (
                <Check className="w-3.5 h-3.5 text-emerald-400" />
              ) : (
                <Copy className="w-3.5 h-3.5" />
              )
            }
          >
            {isCurlCopied ? 'cURL Tersalin' : 'Salin cURL'}
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Tutup
          </Button>
        </div>
      }
    >
      <div className="space-y-6">
        {error && <QueryError message={error} onRetry={onRetry} />}

        {/* Quick Metrics Grid */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
            <span className="text-xs font-medium text-text-muted">Status HTTP</span>
            <div className="mt-1">
              <Badge variant={getStatusBadgeVariant(request.status_code)}>
                {getStatusBadgeLabel(request.status_code)}
              </Badge>
            </div>
          </div>
          <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
            <span className="text-xs font-medium text-text-muted">Total Durasi</span>
            <div className="mt-1 font-mono font-bold text-white">{request.duration_ms} ms</div>
          </div>
          <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
            <span className="text-xs font-medium text-text-muted">Total Token</span>
            <div className="mt-1 font-mono font-bold text-white">
              {request.total_tokens != null ? request.total_tokens.toLocaleString() : '-'}
            </div>
          </div>
          <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
            <span className="text-xs font-medium text-text-muted">Biaya USD</span>
            <div className="mt-1 font-mono font-bold text-accent">
              {formatUSD(request.cost_usd, 6)}
            </div>
          </div>
        </div>

        {/* Latency Waterfall Bar */}
        <div className="p-3.5 bg-bg-surface-2 rounded-inner border border-border space-y-2">
          <div className="flex items-center justify-between text-xs">
            <span className="font-semibold text-text-secondary">Waterfall Latensi Inferensi</span>
            <span className="font-mono text-accent font-bold">{request.duration_ms} ms</span>
          </div>
          <div className="w-full h-3 bg-bg-base rounded-full overflow-hidden flex border border-border/50">
            {events.length > 0 ? (
              events.map((ev, idx) => {
                const pct = Math.min(
                  100,
                  Math.max(4, Math.round(((ev.latency_ms || 0) / totalEventLatency) * 100))
                );
                return (
                  <div
                    key={idx}
                    style={{ width: `${pct}%` }}
                    title={`${ev.provider_id || ev.kind}: ${ev.latency_ms}ms (${pct}%)`}
                    className={`${
                      WATERFALL_COLORS[idx % WATERFALL_COLORS.length]
                    } hover:opacity-80 transition-all border-r border-black/30 first:rounded-l-full last:rounded-r-full`}
                  />
                );
              })
            ) : (
              <div
                style={{ width: '100%' }}
                className="bg-accent rounded-full"
                title={`Upstream Latency: ${request.duration_ms}ms`}
              />
            )}
          </div>
          <div className="flex flex-wrap items-center gap-3 text-[11px] text-text-muted pt-0.5">
            {events.length > 0 ? (
              events.map((ev, idx) => (
                <span key={idx} className="flex items-center gap-1 font-mono">
                  <span
                    className={`w-2 h-2 rounded-full ${
                      WATERFALL_COLORS[idx % WATERFALL_COLORS.length]
                    }`}
                  />
                  {ev.provider_id || ev.kind}: {ev.latency_ms}ms
                </span>
              ))
            ) : (
              <span className="flex items-center gap-1 font-mono">
                <span className="w-2 h-2 rounded-full bg-accent" />
                Upstream roundtrip: {request.duration_ms}ms
              </span>
            )}
          </div>
        </div>

        {/* Error Message if any */}
        {request.error_message && (
          <div className="p-3.5 rounded-inner bg-status-error/10 border border-status-error/20 text-status-error text-xs">
            <div className="font-semibold mb-1">Pesan Kesalahan:</div>
            <div className="font-mono">{request.error_message}</div>
          </div>
        )}

        {/* Event Timeline */}
        <div>
          <h4 className="text-xs font-semibold text-text-secondary mb-3">
            Timeline Peristiwa (Failover &amp; Percobaan)
          </h4>
          <div className="space-y-2">
            {events.length === 0 ? (
              <p className="text-xs text-text-muted">Tidak ada rekaman event sekunder.</p>
            ) : (
              events.map((ev, idx) => (
                <div
                  key={idx}
                  className="p-3 bg-bg-surface-2/40 border border-border rounded-inner flex items-center justify-between text-xs"
                >
                  <div className="flex items-center gap-3">
                    <span className="w-2 h-2 rounded-full bg-accent" />
                    <div>
                      <span className="font-semibold text-white">{ev.kind}</span>
                      <span className="text-text-muted ml-2 font-mono">
                        {ev.provider_id || '-'}
                      </span>
                    </div>
                  </div>
                  <span className="font-mono text-text-secondary">{ev.latency_ms} ms</span>
                </div>
              ))
            )}
          </div>
        </div>

        {/* Request Payload Viewer */}
        <div>
          <h4 className="text-xs font-semibold text-text-secondary mb-2">
            Muatan Permintaan &amp; Respons
          </h4>
          {payload ? (
            <div className="space-y-3 text-xs">
              {payload.truncated && (
                <div className="text-[11px] text-amber-500">
                  Payload terpotong saat perekaman ({payload.size_bytes ?? 0} byte).
                </div>
              )}
              <div>
                <span className="text-text-muted block mb-1">Body Permintaan:</span>
                <pre className="p-3 bg-black/60 rounded-inner border border-border font-mono text-[11px] overflow-x-auto max-h-40 whitespace-pre-wrap text-text-primary">
                  {formatRawPayload(payload.request_body)}
                </pre>
              </div>
              <div>
                <span className="text-text-muted block mb-1">Body Respons:</span>
                <pre className="p-3 bg-black/60 rounded-inner border border-border font-mono text-[11px] overflow-x-auto max-h-40 whitespace-pre-wrap text-text-primary">
                  {formatRawPayload(payload.response_body)}
                </pre>
              </div>
            </div>
          ) : (
            <p className="text-xs text-text-muted italic">
              Payload tidak disimpan untuk retensi keamanan.
            </p>
          )}
        </div>
      </div>
    </Modal>
  );
};
