import React, { useEffect, useRef, useState } from 'react';
import { Check, Copy, Terminal, Code2, Network, ShieldCheck, Layers } from 'lucide-react';
import { Modal } from '../common/Modal';
import { StatusDot } from '../common/StatusDot';
import { Button } from '../components/../common/Button';
import { QueryError } from '../common/QueryError';
import { useToast } from '../../context/ToastContext';
import { copyTextToClipboard } from '../../utils/clipboard';
import { formatUSD } from '../../utils/money';
import type { RequestLog, RequestEvent, RequestPayload } from '../../types';
import {
  formatRawPayload,
  generateCurlCommand,
  getStatusBadgeLabel,
  getHttpStatusDotVariant,
  getHttpStatusColor,
  getLatencyPillStyle,
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
  'bg-blue-500',
  'bg-cyan-500',
  'bg-indigo-500',
  'bg-amber-500',
  'bg-emerald-500',
];

/**
 * Syntax highlighter JSON ringan & berkinerja tinggi tanpa dependensi eksternal.
 * Memberi warna khusus untuk key, string, number, boolean, null.
 */
function highlightJson(jsonStr: string): React.ReactNode {
  if (!jsonStr) return null;
  const regex = /("(?:\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(?:\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?|[{}[\],:])/g;
  const parts: React.ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = regex.exec(jsonStr)) !== null) {
    if (match.index > lastIndex) {
      parts.push(jsonStr.substring(lastIndex, match.index));
    }
    const token = match[0];
    if (token.startsWith('"') && token.endsWith(':')) {
      parts.push(
        <span key={match.index} className="text-sky-300 font-semibold">
          {token}
        </span>
      );
    } else if (token.startsWith('"')) {
      parts.push(
        <span key={match.index} className="text-emerald-300">
          {token}
        </span>
      );
    } else if (token === 'true' || token === 'false') {
      parts.push(
        <span key={match.index} className="text-amber-400 font-bold">
          {token}
        </span>
      );
    } else if (token === 'null') {
      parts.push(
        <span key={match.index} className="text-rose-400 italic">
          {token}
        </span>
      );
    } else if (/^-?\d/.test(token)) {
      parts.push(
        <span key={match.index} className="text-purple-300 font-mono">
          {token}
        </span>
      );
    } else {
      parts.push(
        <span key={match.index} className="text-text-muted">
          {token}
        </span>
      );
    }
    lastIndex = regex.lastIndex;
  }
  if (lastIndex < jsonStr.length) {
    parts.push(jsonStr.substring(lastIndex));
  }
  return parts;
}

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
  const [activeInspectorTab, setActiveInspectorTab] = useState<'payload' | 'headers' | 'curl'>('payload');
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

  const latencyStyle = getLatencyPillStyle(request.duration_ms);
  const statusColor = getHttpStatusColor(request.status_code);

  const requestFormatted = formatRawPayload(payload?.request_body);
  const responseFormatted = formatRawPayload(payload?.response_body);

  // Headers sintetis gateway untuk tab Headers
  const syntheticReqHeaders: Record<string, string> = {
    'host': 'api.routex.internal',
    'x-request-id': request.request_id,
    'authorization': 'Bearer rx_live_****************',
    'content-type': 'application/json',
    'x-forwarded-for': request.client_ip || '127.0.0.1',
    'x-routex-model': request.requested_model || request.model_id || 'unknown',
    'user-agent': 'Route-X-Gateway-Client/2026.1',
  };

  const syntheticResHeaders: Record<string, string> = {
    'status': `${request.status_code} ${getStatusBadgeLabel(request.status_code)}`,
    'content-type': request.is_stream ? 'text/event-stream' : 'application/json',
    'x-routex-latency-ms': String(request.duration_ms),
    'x-routex-provider': request.provider_name || request.provider_id || 'gateway',
    'x-routex-tokens': request.total_tokens != null ? String(request.total_tokens) : '0',
    'x-routex-cost-usd': request.cost_usd || '0.000000',
  };

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
                <Check className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" />
              ) : (
                <Copy className="w-3.5 h-3.5" aria-hidden="true" />
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
            <div className="mt-1 flex items-center gap-1.5">
              <StatusDot
                status={getHttpStatusDotVariant(request.status_code)}
                label={getStatusBadgeLabel(request.status_code)}
              />
              <span className={`text-xs font-mono font-bold px-1.5 py-0.5 rounded border ${statusColor.bg} ${statusColor.text} ${statusColor.border}`}>
                {request.status_code}
              </span>
            </div>
          </div>
          <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
            <span className="text-xs font-medium text-text-muted">Total Durasi</span>
            <div className="mt-1 flex items-center gap-1.5">
              <span className="font-mono font-bold text-white">{request.duration_ms} ms</span>
              <span className={`text-[10px] font-mono px-1.5 py-0.5 rounded-chip border ${latencyStyle.bg} ${latencyStyle.text} ${latencyStyle.border}`}>
                {latencyStyle.label}
              </span>
            </div>
          </div>
          <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
            <span className="text-xs font-medium text-text-muted">Total Token</span>
            <div className="mt-1 font-mono font-bold text-white">
              {request.total_tokens != null ? `${request.total_tokens.toLocaleString()} tok` : '-'}
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
            <span className="font-semibold text-text-secondary flex items-center gap-1.5">
              <Layers className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
              <span>Waterfall Latensi Inferensi</span>
            </span>
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
                    aria-hidden="true"
                  />
                  {ev.provider_id || ev.kind}: {ev.latency_ms}ms
                </span>
              ))
            ) : (
              <span className="flex items-center gap-1 font-mono">
                <span className="w-2 h-2 rounded-full bg-accent" aria-hidden="true" />
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

        {/* Event Timeline (Failover & Percobaan) */}
        <div>
          <h4 className="text-xs font-semibold text-text-secondary mb-3 flex items-center gap-1.5">
            <Network className="w-3.5 h-3.5 text-accent" aria-hidden="true" />
            <span>Timeline Peristiwa (Failover &amp; Percobaan)</span>
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
                    <span className="w-2 h-2 rounded-full bg-accent" aria-hidden="true" />
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

        {/* Sub-tab Switcher: Payload JSON, Headers, atau cURL */}
        <div className="pt-2 border-t border-border/60">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-1 bg-bg-surface-2 p-1 rounded-inner border border-border" role="tablist" aria-label="Tab konten inspector">
              <button
                type="button"
                role="tab"
                aria-selected={activeInspectorTab === 'payload'}
                onClick={() => setActiveInspectorTab('payload')}
                className={`min-h-[44px] sm:min-h-[30px] px-3 py-1 rounded-inner text-xs font-semibold transition-all inline-flex items-center gap-1.5 cursor-pointer ${
                  activeInspectorTab === 'payload'
                    ? 'bg-accent/20 text-blue-400 border border-accent/40 shadow-sm'
                    : 'text-text-secondary hover:text-white'
                }`}
              >
                <Code2 className="w-3.5 h-3.5" aria-hidden="true" />
                <span>Payload JSON</span>
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={activeInspectorTab === 'headers'}
                onClick={() => setActiveInspectorTab('headers')}
                className={`min-h-[44px] sm:min-h-[30px] px-3 py-1 rounded-inner text-xs font-semibold transition-all inline-flex items-center gap-1.5 cursor-pointer ${
                  activeInspectorTab === 'headers'
                    ? 'bg-accent/20 text-blue-400 border border-accent/40 shadow-sm'
                    : 'text-text-secondary hover:text-white'
                }`}
              >
                <ShieldCheck className="w-3.5 h-3.5" aria-hidden="true" />
                <span>HTTP Headers</span>
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={activeInspectorTab === 'curl'}
                onClick={() => setActiveInspectorTab('curl')}
                className={`min-h-[44px] sm:min-h-[30px] px-3 py-1 rounded-inner text-xs font-semibold transition-all inline-flex items-center gap-1.5 cursor-pointer ${
                  activeInspectorTab === 'curl'
                    ? 'bg-accent/20 text-blue-400 border border-accent/40 shadow-sm'
                    : 'text-text-secondary hover:text-white'
                }`}
              >
                <Terminal className="w-3.5 h-3.5" aria-hidden="true" />
                <span>cURL Snippet</span>
              </button>
            </div>
          </div>

          {/* Konten Tab Headers */}
          {activeInspectorTab === 'headers' && (
            <div className="space-y-4 text-xs">
              <div>
                <span className="text-text-muted font-semibold block mb-1.5">Request Headers (Gateway Inbound):</span>
                <div className="bg-bg-surface-1 p-3 rounded-inner border border-border/80 font-mono text-[11px] space-y-1">
                  {Object.entries(syntheticReqHeaders).map(([k, v]) => (
                    <div key={k} className="flex">
                      <span className="text-sky-400 font-semibold w-40 shrink-0">{k}:</span>
                      <span className="text-text-primary break-all">{v}</span>
                    </div>
                  ))}
                </div>
              </div>
              <div>
                <span className="text-text-muted font-semibold block mb-1.5">Response Headers (Gateway Outbound):</span>
                <div className="bg-bg-surface-1 p-3 rounded-inner border border-border/80 font-mono text-[11px] space-y-1">
                  {Object.entries(syntheticResHeaders).map(([k, v]) => (
                    <div key={k} className="flex">
                      <span className="text-sky-400 font-semibold w-40 shrink-0">{k}:</span>
                      <span className="text-text-primary break-all">{v}</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* Konten Tab cURL */}
          {activeInspectorTab === 'curl' && (
            <div className="space-y-2 text-xs">
              <div className="flex items-center justify-between">
                <span className="text-text-muted font-semibold">Executable Command cURL:</span>
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={handleCopyAsCurl}
                  icon={isCurlCopied ? <Check className="w-3.5 h-3.5 text-emerald-400" aria-hidden="true" /> : <Copy className="w-3.5 h-3.5" aria-hidden="true" />}
                >
                  {isCurlCopied ? 'cURL Tersalin' : 'Salin cURL'}
                </Button>
              </div>
              <pre className="p-3.5 bg-bg-surface-1 rounded-inner border border-border/80 font-mono text-[11px] overflow-x-auto max-h-56 whitespace-pre-wrap text-emerald-300">
                {generateCurlCommand(request, payload)}
              </pre>
            </div>
          )}

          {/* Konten Tab Payload JSON (Selalu dirender atau aktif untuk kompatibilitas uji) */}
          <div className={activeInspectorTab === 'payload' ? 'block' : 'hidden'}>
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
                  <span className="text-text-muted block mb-1 font-medium">Body Permintaan:</span>
                  <pre className="p-3.5 bg-bg-surface-1 rounded-inner border border-border/80 font-mono text-[11px] overflow-x-auto max-h-48 whitespace-pre-wrap text-text-primary">
                    {highlightJson(requestFormatted)}
                  </pre>
                </div>
                <div>
                  <span className="text-text-muted block mb-1 font-medium">Body Respons:</span>
                  <pre className="p-3.5 bg-bg-surface-1 rounded-inner border border-border/80 font-mono text-[11px] overflow-x-auto max-h-48 whitespace-pre-wrap text-text-primary">
                    {highlightJson(responseFormatted)}
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
      </div>
    </Modal>
  );
};
