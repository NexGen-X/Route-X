import React, { useEffect, useState } from 'react';
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
} from 'lucide-react';

export const Requests: React.FC = () => {
  const [requests, setRequests] = useState<RequestLog[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [activeTab, setActiveTab] = useState<'all' | 'success' | 'errors'>('all');
  const [search, setSearch] = useState('');
  const [selectedReq, setSelectedReq] = useState<RequestLog | null>(null);
  const [reqEvents, setReqEvents] = useState<RequestEvent[]>([]);
  const [reqPayload, setReqPayload] = useState<RequestPayload | null>(null);
  const [isInspectorOpen, setIsInspectorOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  const loadRequests = async (cursor?: string) => {
    setIsLoading(true);
    try {
      const params: Record<string, any> = { limit: 25 };
      if (cursor) params.cursor = cursor;
      if (activeTab === 'errors') params.status_class = 'error';
      if (activeTab === 'success') params.status_class = '2xx';
      if (search) params.search = search;

      const res = await api.requests.list(params);
      if (cursor) {
        setRequests((prev) => [...prev, ...(res.items || [])]);
      } else {
        setRequests(res.items || []);
      }
      setNextCursor(res.next_cursor);
    } catch (err) {
      console.error('Failed to load requests:', err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    loadRequests();
  }, [activeTab]);

  const handleInspect = async (req: RequestLog) => {
    setSelectedReq(req);
    setIsInspectorOpen(true);
    try {
      const [eventsRes, payloadRes] = await Promise.all([
        api.requests.events(req.request_id).catch(() => ({ events: [] })),
        api.requests.payload(req.request_id).catch(() => null),
      ]);
      setReqEvents(eventsRes.events || []);
      setReqPayload(payloadRes);
    } catch (err) {
      console.error(err);
    }
  };

  const getStatusBadge = (code: number) => {
    if (code >= 200 && code < 300) return <Badge variant="success">{code} OK</Badge>;
    if (code >= 400 && code < 500) return <Badge variant="warn">{code} WARN</Badge>;
    return <Badge variant="error">{code} ERR</Badge>;
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

      <Card>
        {/* Filter Chips & Search Bar */}
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 pb-4 border-b border-border">
          <div className="flex items-center gap-2">
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

        {/* Requests Table: Two Lines Per Cell */}
        <div className="overflow-x-auto">
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
              {requests.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-text-muted">
                    {isLoading ? 'Memuat log permintaan...' : 'Tidak ada catatan permintaan yang sesuai filter.'}
                  </td>
                </tr>
              ) : (
                requests.map((r) => (
                  <tr
                    key={r.id || r.request_id}
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
                        ${parseFloat(r.cost_usd || '0').toFixed(6)}
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
                ))
              )}
            </tbody>
          </table>
        </div>

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
                  ${parseFloat(selectedReq.cost_usd || '0').toFixed(6)}
                </div>
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
