import React, { useEffect, useRef, useState } from 'react';
import { formatUSD } from '../utils/money';
import { api } from '../api/client';
import type { RequestLog, RequestEvent, RequestPayload } from '../types';
import { Card } from '../components/common/Card';
import { Badge } from '../components/common/Badge';
import { Button } from '../components/common/Button';
import { Modal } from '../components/common/Modal';
import {
  Search,
  RefreshCw,
  ChevronRight,
  Copy,
  Check,
  Radio,
  Terminal,
  Activity,
} from 'lucide-react';
import { useToast } from '../context/ToastContext';
import { copyTextToClipboard } from '../utils/clipboard';
import { QueryError } from '../components/common/QueryError';

export const Requests: React.FC = () => {
  const { toast } = useToast();
  const [requests, setRequests] = useState<RequestLog[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [activeTab, setActiveTab] = useState<'all' | 'success' | 'errors'>('all');
  const [search, setSearch] = useState('');
  const [selectedReq, setSelectedReq] = useState<RequestLog | null>(null);
  const [reqEvents, setReqEvents] = useState<RequestEvent[]>([]);
  const [reqPayload, setReqPayload] = useState<RequestPayload | null>(null);
  const [isInspectorOpen, setIsInspectorOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isLiveFeed, setIsLiveFeed] = useState(false);
  const [isCurlCopied, setIsCurlCopied] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [inspectorError, setInspectorError] = useState<string | null>(null);
  // Penjaga race inspector: abaikan respons basi bila user sudah klik request lain.
  const inspectorReqRef = useRef(0);
  // Timer indikator salin cURL; disimpan agar bisa dibatalkan saat unmount.
  const curlTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (curlTimerRef.current) {
        clearTimeout(curlTimerRef.current);
        curlTimerRef.current = null;
      }
    };
  }, []);

  // Debounce pencarian: mengetik tidak langsung menembak API, tunggu 400ms diam.
  // Efek saat mount dilewati karena pemuatan awal sudah ditangani efek activeTab.
  const searchFirstRef = useRef(true);
  useEffect(() => {
    if (searchFirstRef.current) {
      searchFirstRef.current = false;
      return;
    }
    const timer = setTimeout(() => {
      void loadRequests();
    }, 400);
    return () => clearTimeout(timer);
  }, [search]);

  // Penjaga race daftar: abaikan respons basi bila user sudah ganti tab/cari.
  const listReqRef = useRef(0);

  const loadRequests = async (cursor?: string, silent = false) => {
    const reqId = ++listReqRef.current;
    if (!silent) setIsLoading(true);
    try {
      const params: Record<string, any> = { limit: 25 };
      if (cursor) params.cursor = cursor;
      if (activeTab === 'errors') params.status_class = 'error';
      if (activeTab === 'success') params.status_class = '2xx';
      if (search) params.search = search;

      const res = await api.requests.list(params);
      if (listReqRef.current !== reqId) return;
      if (cursor) {
        setRequests((prev) => [...prev, ...(res.items || [])]);
      } else {
        setRequests(res.items || []);
      }
      setNextCursor(res.next_cursor);
      setLoadError(null);
    } catch (err) {
      if (listReqRef.current !== reqId) return;
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      if (listReqRef.current === reqId && !silent) setIsLoading(false);
    }
  };

  useEffect(() => {
    loadRequests();
  }, [activeTab]);

  useEffect(() => {
    if (!isLiveFeed) return;
    const interval = setInterval(() => {
      void loadRequests(undefined, true);
    }, 4000);
    return () => clearInterval(interval);
  }, [isLiveFeed, activeTab, search]);

  const copyAsCurl = async () => {
    if (!selectedReq) return;
    const gwURL = `${window.location.protocol}//${window.location.host}/v1/chat/completions`;
    const promptText = reqPayload?.prompt_text || 'Hello Route-X';
    const curlCmd = `curl -X POST "${gwURL}" \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer rx_live_personal_gateway" \\\n  -d '{\n    "model": ${JSON.stringify(selectedReq.model_id)},\n    "messages": [{"role": "user", "content": ${JSON.stringify(promptText)}}]\n  }'`;
    try {
      await copyTextToClipboard(curlCmd);
      setIsCurlCopied(true);
      toast.success('Perintah cURL disalin ke clipboard.');
      if (curlTimerRef.current) clearTimeout(curlTimerRef.current);
      curlTimerRef.current = setTimeout(() => {
        curlTimerRef.current = null;
        setIsCurlCopied(false);
      }, 2000);
    } catch (err) {
      toast.error('Gagal menyalin perintah cURL: ' + (err instanceof Error ? err.message : String(err)));
    }
  };

  const handleInspect = async (req: RequestLog) => {
    const reqId = ++inspectorReqRef.current;
    setSelectedReq(req);
    // Bersihkan state lama agar modal tidak menampilkan payload request sebelumnya.
    setReqEvents([]);
    setReqPayload(null);
    setIsInspectorOpen(true);
    setInspectorError(null);
    try {
      const [eventsRes, payloadRes] = await Promise.all([
        api.requests.events(req.request_id),
        api.requests.payload(req.request_id),
      ]);
      if (inspectorReqRef.current !== reqId) return;
      setReqEvents(eventsRes.events || []);
      setReqPayload(payloadRes);
    } catch (err) {
      if (inspectorReqRef.current !== reqId) return;
      setReqEvents([]);
      setReqPayload(null);
      setInspectorError(err instanceof Error ? err.message : String(err));
    }
  };

  const getStatusBadge = (code: number) => {
    if (code >= 200 && code < 300) return <Badge variant="success">{code} OK</Badge>;
    if (code >= 400 && code < 500) return <Badge variant="warn">{code} WARN</Badge>;
    if (code >= 500) return <Badge variant="error">{code} ERR</Badge>;
    return <Badge variant="neutral">{code}</Badge>;
  };

  return (
    <div className="space-y-6">
      {/* Header & Filter Bar */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Requests Inspector</h2>
          <p className="text-xs text-text-secondary mt-1">
            Penelusuran audit lalu lintas inferensi AI, detail payload request/response, dan urutan failover.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setIsLiveFeed(!isLiveFeed)}
            className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-inner text-xs font-semibold border transition-all ${
              isLiveFeed
                ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/40 animate-pulse'
                : 'bg-bg-surface-2 text-text-secondary border-border hover:text-white'
            }`}
          >
            <Radio className={`w-3.5 h-3.5 ${isLiveFeed ? 'text-emerald-400' : 'text-text-muted'}`} />
            <span>{isLiveFeed ? 'Live Polling Aktif' : 'Live Stream'}</span>
          </button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => loadRequests()}
            isLoading={isLoading}
            icon={<RefreshCw className="w-3.5 h-3.5" />}
          >
            Segarkan
          </Button>
        </div>
      </div>

      {loadError && <QueryError message={loadError} onRetry={() => void loadRequests()} />}

      <Card>
        {/* Filter Chips & Search Bar */}
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 pb-4 border-b border-border">
          <div className="flex flex-wrap items-center gap-2">
            {[
              { id: 'all', label: 'All Requests' },
              { id: 'success', label: 'Success (2xx)' },
              { id: 'errors', label: 'Errors (4xx/5xx)' },
            ].map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id as any)}
                className={`px-3 py-1.5 rounded-chip text-xs font-semibold transition-all ${
                  activeTab === tab.id
                    ? 'bg-accent/10 text-accent border border-accent/30 shadow-sm'
                    : 'bg-bg-surface-2 text-text-secondary border border-border hover:text-white'
                }`}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <div className="relative w-full md:w-72">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
            <input
              type="text"
              placeholder="Cari ID request, model, IP..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && loadRequests()}
              className="w-full pl-9 pr-3 py-1.5 bg-bg-surface-2 border border-border rounded-nav text-xs text-text-primary focus:outline-none focus:border-accent"
            />
          </div>
        </div>

        {/* Requests List: Card List on Mobile, Table on Desktop */}
        {isLoading ? (
          <div className="p-4 sm:p-0">
            {/* Mobile skeleton */}
            <div className="sm:hidden space-y-3">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="p-3.5 bg-bg-surface-2/40 border border-border/40 rounded-lg animate-pulse space-y-2">
                  <div className="flex justify-between items-center">
                    <div className="h-5 bg-bg-surface-2 rounded w-28" />
                    <div className="h-4 bg-bg-surface-2 rounded w-16" />
                  </div>
                  <div className="h-4 bg-bg-surface-2 rounded w-40" />
                  <div className="h-3 bg-bg-surface-2 rounded w-24" />
                </div>
              ))}
            </div>
            {/* Desktop table skeleton */}
            <div className="hidden sm:block overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-border text-text-muted uppercase tracking-wider text-[11px] bg-bg-surface-2/40">
                    <th className="py-3 px-4 font-semibold">Status & Request ID</th>
                    <th className="py-3 px-4 font-semibold">Model & Provider</th>
                    <th className="py-3 px-4 font-semibold">Token & Cost</th>
                    <th className="py-3 px-4 font-semibold">Durasi & TTFT</th>
                    <th className="py-3 px-4 font-semibold">Waktu & Klien</th>
                    <th className="py-3 px-4 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/60">
                  {[...Array(6)].map((_, i) => (
                    <tr key={i} className="animate-pulse">
                      <td className="py-3 px-4"><div className="h-4 bg-bg-surface-2 rounded w-28" /></td>
                      <td className="py-3 px-4"><div className="h-4 bg-bg-surface-2 rounded w-36" /></td>
                      <td className="py-3 px-4"><div className="h-4 bg-bg-surface-2 rounded w-20" /></td>
                      <td className="py-3 px-4"><div className="h-4 bg-bg-surface-2 rounded w-16" /></td>
                      <td className="py-3 px-4"><div className="h-4 bg-bg-surface-2 rounded w-24" /></td>
                      <td className="py-3 px-4 text-right"><div className="h-4 bg-bg-surface-2 rounded w-12 ml-auto" /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : requests.length === 0 ? (
          <div className="py-12 px-4 text-center space-y-3">
            <Activity className="w-8 h-8 mx-auto text-text-muted/40" />
            <div className="text-sm font-semibold text-white">
              Tidak ada catatan permintaan
            </div>
            <p className="text-xs text-text-muted max-w-sm mx-auto">
              Belum ada permintaan inferensi yang cocok dengan kriteria filter atau kata kunci pencarian.
            </p>
          </div>
        ) : (
          <>
            {/* Mobile Card List (sm:hidden) */}
            <div className="sm:hidden divide-y divide-border/60">
              {requests.map((r) => (
                <div
                  key={r.id || r.request_id}
                  role="button"
                      tabIndex={0}
                      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleInspect(r); } }}
                      onClick={() => handleInspect(r)}
                  className="p-3.5 hover:bg-bg-surface-2/60 active:bg-bg-surface-2 cursor-pointer transition-colors space-y-2"
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      {getStatusBadge(r.status_code)}
                      <span className="font-mono text-xs font-semibold text-white truncate max-w-[130px]">
                        {r.requested_model || r.model_id || 'unknown'}
                      </span>
                    </div>
                    <span className="text-[10px] text-text-muted font-mono whitespace-nowrap">
                      {new Date(r.created_at).toLocaleTimeString()}
                    </span>
                  </div>

                  <div className="flex items-center justify-between text-[11px] font-mono text-text-secondary">
                    <span className="text-text-muted truncate max-w-[140px]">
                      {r.provider_name || r.provider_id || '-'}
                      {r.is_stream && <span className="ml-1 text-[9px] px-1 rounded bg-[#252A36] text-primary">SSE</span>}
                    </span>
                    <div className="flex items-center gap-2">
                      <span className="text-white font-semibold">{r.duration_ms}ms</span>
                      <span className="text-text-muted">·</span>
                      <span className="text-emerald-400 font-semibold">
                        {r.total_tokens != null ? `${r.total_tokens} tok` : '-'}
                      </span>
                    </div>
                  </div>

                  <div className="flex items-center justify-between text-[10px] text-text-muted font-mono pt-1 border-t border-[#1C2029]">
                    <span className="truncate max-w-[180px]">ID: {r.request_id}</span>
                    <span className="text-accent flex items-center gap-0.5 font-medium">
                      Detail <ChevronRight className="w-3 h-3" />
                    </span>
                  </div>
                </div>
              ))}
            </div>

            {/* Desktop Table (hidden sm:block) */}
            <div className="hidden sm:block overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-border text-text-muted uppercase tracking-wider text-[11px] bg-bg-surface-2/40">
                    <th className="py-3 px-4 font-semibold">Status & Request ID</th>
                    <th className="py-3 px-4 font-semibold">Model & Provider</th>
                    <th className="py-3 px-4 font-semibold">Token & Cost</th>
                    <th className="py-3 px-4 font-semibold">Durasi & TTFT</th>
                    <th className="py-3 px-4 font-semibold">Waktu & Klien</th>
                    <th className="py-3 px-4 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/60">
                  {requests.map((r) => (
                    <tr
                      key={r.id || r.request_id}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleInspect(r); } }}
                      onClick={() => handleInspect(r)}
                      className="hover:bg-bg-surface-2/60 cursor-pointer transition-colors group"
                    >
                      <td className="py-3 px-4">
                        <div className="flex items-center gap-2">
                          {getStatusBadge(r.status_code)}
                          <span className="font-mono font-medium text-text-primary truncate max-w-[140px]">
                            {r.request_id}
                          </span>
                        </div>
                        <div className="text-[11px] text-text-muted font-mono mt-0.5">
                          {r.is_stream ? 'Stream (SSE)' : 'Unary HTTP'}
                        </div>
                      </td>

                      <td className="py-3 px-4">
                        <div className="font-semibold text-text-primary">{r.requested_model || r.model_id || '-'}</div>
                        <div className="text-[11px] text-text-muted font-mono">{r.provider_name || r.provider_id || '-'}</div>
                      </td>

                      <td className="py-3 px-4">
                        <div className="font-mono text-text-primary">
                          {r.total_tokens != null ? r.total_tokens.toLocaleString() : '-'} tokens
                        </div>
                        <div className="text-[11px] text-text-muted font-mono">
                          {formatUSD(r.cost_usd, 6)}
                        </div>
                      </td>

                      <td className="py-3 px-4">
                        <div className="font-mono text-text-primary">{r.duration_ms} ms</div>
                        <div className="text-[11px] text-text-muted font-mono">
                          {r.ttft_ms ? `TTFT: ${r.ttft_ms} ms` : '-'}
                        </div>
                      </td>

                      <td className="py-3 px-4">
                        <div className="text-text-primary">{new Date(r.created_at).toLocaleTimeString()}</div>
                        <div className="text-[11px] text-text-muted font-mono">{r.client_ip || '-'}</div>
                      </td>

                      <td className="py-3 px-4 text-right">
                        <span className="inline-flex items-center gap-1 text-accent opacity-0 group-hover:opacity-100 transition-opacity font-medium">
                          Detail <ChevronRight className="w-3.5 h-3.5" />
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}

        {nextCursor && (
          <div className="p-4 border-t border-border flex justify-center">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => loadRequests(nextCursor)}
              isLoading={isLoading}
            >
              Muat Lebih Banyak...
            </Button>
          </div>
        )}
      </Card>

      {/* Inspector Modal */}
      {selectedReq && (
        <Modal
          isOpen={isInspectorOpen}
          onClose={() => setIsInspectorOpen(false)}
          title="Detail Request & Timeline Event"
          subtitle={`ID: ${selectedReq.request_id}`}
          maxWidth="2xl"
        >
          <div className="space-y-6">
            {inspectorError && (
              <QueryError message={inspectorError} onRetry={() => void handleInspect(selectedReq)} />
            )}
            {/* Quick Metrics Grid */}
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
                <span className="text-[10px] text-text-muted uppercase">Status HTTP</span>
                <div className="mt-1">{getStatusBadge(selectedReq.status_code)}</div>
              </div>
              <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
                <span className="text-[10px] text-text-muted uppercase">Total Durasi</span>
                <div className="mt-1 font-mono font-bold text-white">{selectedReq.duration_ms} ms</div>
              </div>
              <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
                <span className="text-[10px] text-text-muted uppercase">Total Token</span>
                <div className="mt-1 font-mono font-bold text-white">
                  {selectedReq.total_tokens != null ? selectedReq.total_tokens.toLocaleString() : '-'}
                </div>
              </div>
              <div className="p-3 bg-bg-surface-2 rounded-inner border border-border">
                <span className="text-[10px] text-text-muted uppercase">Biaya USD</span>
                <div className="mt-1 font-mono font-bold text-accent">
                  {formatUSD(selectedReq.cost_usd, 6)}
                </div>
              </div>
            </div>

            {/* Reproduce via cURL */}
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 p-3 bg-bg-surface-2/60 rounded-inner border border-border">
              <div className="flex items-center gap-2.5">
                <div className="w-7 h-7 rounded-full bg-accent/10 flex items-center justify-center text-accent">
                  <Terminal className="w-3.5 h-3.5" />
                </div>
                <div>
                  <span className="text-xs font-semibold text-white block">Reproduksi Inferensi</span>
                  <span className="text-[11px] text-text-muted">Klon request ini langsung sebagai perintah cURL terminal</span>
                </div>
              </div>
              <Button
                size="sm"
                variant={isCurlCopied ? 'primary' : 'secondary'}
                onClick={copyAsCurl}
                title={isCurlCopied ? 'Tersalin!' : 'Salin sebagai perintah cURL'}
                aria-label={isCurlCopied ? 'Tersalin' : 'Salin sebagai cURL'}
                icon={isCurlCopied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
              >
                {isCurlCopied ? 'Tersalin!' : 'cURL'}
              </Button>
            </div>

            {/* Latency Waterfall Bar */}
            <div className="p-3.5 bg-bg-surface-2 rounded-inner border border-border space-y-2">
              <div className="flex items-center justify-between text-xs">
                <span className="font-semibold text-text-secondary">Waterfall Latensi Inferensi</span>
                <span className="font-mono text-accent font-bold">{selectedReq.duration_ms} ms</span>
              </div>
              <div className="w-full h-3 bg-bg-base rounded-full overflow-hidden flex border border-border/50">
                {reqEvents.length > 0 ? (
                  (() => {
                    const colors = [
                      'bg-accent',
                      'bg-cyan-500',
                      'bg-purple-500',
                      'bg-amber-500',
                      'bg-emerald-500',
                    ];
                    const totalEv = reqEvents.reduce((s, ev) => s + (ev.latency_ms || 0), 0) || 1;
                    return reqEvents.map((ev, idx) => {
                      const pct = Math.min(100, Math.max(4, Math.round(((ev.latency_ms || 0) / totalEv) * 100)));
                      return (
                        <div
                          key={idx}
                          style={{ width: `${pct}%` }}
                          title={`${ev.provider_id || ev.event_type}: ${ev.latency_ms}ms (${pct}%)`}
                          className={`${colors[idx % colors.length]} hover:opacity-80 transition-all border-r border-black/30 first:rounded-l-full last:rounded-r-full`}
                        />
                      );
                    });
                  })()
                ) : (
                  <div
                    style={{ width: '100%' }}
                    className="bg-accent rounded-full"
                    title={`Upstream Latency: ${selectedReq.duration_ms}ms`}
                  />
                )}
              </div>
              <div className="flex flex-wrap items-center gap-3 text-[11px] text-text-muted pt-0.5">
                {reqEvents.length > 0 ? (
                  reqEvents.map((ev, idx) => (
                    <span key={idx} className="flex items-center gap-1 font-mono">
                      <span className={`w-2 h-2 rounded-full ${['bg-accent', 'bg-cyan-500', 'bg-purple-500', 'bg-amber-500', 'bg-emerald-500'][idx % 5]}`} />
                      {ev.provider_id || ev.event_type}: {ev.latency_ms}ms
                    </span>
                  ))
                ) : (
                  <span className="flex items-center gap-1 font-mono">
                    <span className="w-2 h-2 rounded-full bg-accent" />
                    Upstream roundtrip: {selectedReq.duration_ms}ms
                  </span>
                )}
              </div>
            </div>

            {/* Error Message if any */}
            {selectedReq.error_message && (
              <div className="p-3.5 rounded-inner bg-status-error/10 border border-status-error/20 text-status-error text-xs">
                <div className="font-semibold mb-1">Pesan Kesalahan:</div>
                <div className="font-mono">{selectedReq.error_message}</div>
              </div>
            )}

            {/* Event Timeline */}
            <div>
              <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-3">
                Timeline Peristiwa (Failover & Percobaan)
              </h4>
              <div className="space-y-2">
                {reqEvents.length === 0 ? (
                  <p className="text-xs text-text-muted">Tidak ada rekaman event sekunder.</p>
                ) : (
                  reqEvents.map((ev, idx) => (
                    <div
                      key={idx}
                      className="p-3 bg-bg-surface-2/40 border border-border rounded-inner flex items-center justify-between text-xs"
                    >
                      <div className="flex items-center gap-3">
                        <span className="w-2 h-2 rounded-full bg-accent" />
                        <div>
                          <span className="font-semibold text-white">{ev.event_type}</span>
                          <span className="text-text-muted ml-2 font-mono">{ev.provider_id || '-'}</span>
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
              <h4 className="text-xs font-semibold text-text-muted uppercase tracking-wider mb-2">
                Muatan Permintaan & Respons
              </h4>
              {reqPayload ? (
                <div className="space-y-3 text-xs">
                  <div>
                    <span className="text-text-muted block mb-1">Prompt Masukan:</span>
                    <pre className="p-3 bg-black/60 rounded-inner border border-border font-mono text-[11px] overflow-x-auto max-h-40 whitespace-pre-wrap text-text-primary">
                      {reqPayload.prompt_text || '(kosong atau tidak direkam)'}
                    </pre>
                  </div>
                  <div>
                    <span className="text-text-muted block mb-1">Respons Keluaran:</span>
                    <pre className="p-3 bg-black/60 rounded-inner border border-border font-mono text-[11px] overflow-x-auto max-h-40 whitespace-pre-wrap text-text-primary">
                      {reqPayload.response_text || '(kosong atau tidak direkam)'}
                    </pre>
                  </div>
                </div>
              ) : (
                <p className="text-xs text-text-muted italic">Payload tidak disimpan untuk retensi keamanan.</p>
              )}
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
};
