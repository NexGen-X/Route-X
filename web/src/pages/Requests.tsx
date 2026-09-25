import React, { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { RequestLog, RequestEvent, RequestPayload } from '../types';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { QueryError } from '../components/common/QueryError';
import { RefreshCw, Radio, Activity } from 'lucide-react';
import {
  RequestFilterBar,
  RequestTableSkeleton,
  RequestTableRow,
  RequestCardMobile,
  RequestInspectorModal,
  type RequestTabFilter,
} from '../components/requests';

export const Requests: React.FC = () => {
  const [requests, setRequests] = useState<RequestLog[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [activeTab, setActiveTab] = useState<RequestTabFilter>('all');
  const [search, setSearch] = useState('');
  const [selectedReq, setSelectedReq] = useState<RequestLog | null>(null);
  const [reqEvents, setReqEvents] = useState<RequestEvent[]>([]);
  const [reqPayload, setReqPayload] = useState<RequestPayload | null>(null);
  const [isInspectorOpen, setIsInspectorOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isLiveFeed, setIsLiveFeed] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [inspectorError, setInspectorError] = useState<string | null>(null);

  // Penjaga race inspector: abaikan respons basi bila user sudah klik request lain.
  const inspectorReqRef = useRef(0);
  // Penjaga race daftar: abaikan respons basi bila user sudah ganti tab/cari.
  const listReqRef = useRef(0);

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

  const loadRequests = async (cursor?: string, silent = false) => {
    const reqId = ++listReqRef.current;
    if (!silent) setIsLoading(true);
    try {
      const params: Record<string, string | number> = { limit: 25 };
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
    } catch (err: unknown) {
      if (listReqRef.current !== reqId) return;
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      if (listReqRef.current === reqId && !silent) setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadRequests();
  }, [activeTab]);

  useEffect(() => {
    if (!isLiveFeed) return;
    const interval = setInterval(() => {
      void loadRequests(undefined, true);
    }, 4000);
    return () => clearInterval(interval);
  }, [isLiveFeed, activeTab, search]);

  const handleInspect = async (req: RequestLog) => {
    const reqId = ++inspectorReqRef.current;
    setSelectedReq(req);
    // Bersihkan state lama agar modal tidak menampilkan payload request sebelumnya.
    setReqEvents([]);
    setReqPayload(null);
    setIsInspectorOpen(true);
    setInspectorError(null);
    try {
      const targetId = req.id || req.request_id;
      const [eventsRes, payloadRes] = await Promise.all([
        api.requests
          .events(targetId, req.created_at)
          .catch(() => ({ events: [] as RequestEvent[] })),
        api.requests.payload(targetId, req.created_at).catch(() => null),
      ]);
      if (inspectorReqRef.current !== reqId) return;
      setReqEvents(eventsRes.events || []);
      setReqPayload(payloadRes);
    } catch (err: unknown) {
      if (inspectorReqRef.current !== reqId) return;
      setReqEvents([]);
      setReqPayload(null);
      setInspectorError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <div className="space-y-6">
      {/* Header & Filter Bar */}
      <PageHeader
        title="Requests Inspector"
        actions={
          <>
            <button
              type="button"
              onClick={() => setIsLiveFeed(!isLiveFeed)}
              className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-inner text-xs font-semibold border transition-all ${
                isLiveFeed
                  ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/40 animate-pulse'
                  : 'bg-bg-surface-2 text-text-secondary border-border hover:text-white'
              }`}
            >
              <Radio
                className={`w-3.5 h-3.5 ${isLiveFeed ? 'text-emerald-400' : 'text-text-muted'}`}
              />
              <span>{isLiveFeed ? 'Live Polling Aktif' : 'Live Stream'}</span>
            </button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void loadRequests()}
              isLoading={isLoading}
              icon={<RefreshCw className="w-3.5 h-3.5" />}
            >
              Segarkan
            </Button>
          </>
        }
      />

      {loadError && (
        <QueryError message={loadError} onRetry={() => void loadRequests()} />
      )}

      <Card>
        {/* Filter Chips & Search Bar */}
        <RequestFilterBar
          activeTab={activeTab}
          onTabChange={setActiveTab}
          search={search}
          onSearchChange={setSearch}
          onSearchSubmit={() => void loadRequests()}
        />

        {/* Requests List: Card List on Mobile, Table on Desktop */}
        {isLoading ? (
          <RequestTableSkeleton />
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
                <RequestCardMobile
                  key={r.id || r.request_id}
                  request={r}
                  onInspect={handleInspect}
                />
              ))}
            </div>

            {/* Desktop Table (hidden sm:block) */}
            <div className="hidden sm:block overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-border text-text-muted font-medium text-xs bg-bg-surface-2/40">
                    <th className="py-3 px-4 font-semibold">Status &amp; Request ID</th>
                    <th className="py-3 px-4 font-semibold">Model &amp; Provider</th>
                    <th className="py-3 px-4 font-semibold">Token &amp; Cost</th>
                    <th className="py-3 px-4 font-semibold">Durasi &amp; TTFT</th>
                    <th className="py-3 px-4 font-semibold">Waktu &amp; Klien</th>
                    <th className="py-3 px-4 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/60">
                  {requests.map((r) => (
                    <RequestTableRow
                      key={r.id || r.request_id}
                      request={r}
                      onInspect={handleInspect}
                    />
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
              onClick={() => void loadRequests(nextCursor)}
              isLoading={isLoading}
            >
              Muat Lebih Banyak...
            </Button>
          </div>
        )}
      </Card>

      {/* Inspector Modal */}
      <RequestInspectorModal
        isOpen={isInspectorOpen}
        onClose={() => setIsInspectorOpen(false)}
        request={selectedReq}
        events={reqEvents}
        payload={reqPayload}
        error={inspectorError}
        onRetry={() => {
          if (selectedReq) void handleInspect(selectedReq);
        }}
      />
    </div>
  );
};
