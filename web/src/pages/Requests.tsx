import React, { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { RequestLog, RequestEvent, RequestPayload } from '../types';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { QueryError } from '../components/common/QueryError';
import { RefreshCw, Activity } from 'lucide-react';
import {
  RequestFilterBar,
  RequestTableSkeleton,
  RequestTableRow,
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
              className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium border transition-all duration-150 ${
                isLiveFeed
                  ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
                  : 'bg-white/[0.03] text-zinc-300 border-white/[0.08] hover:text-white'
              }`}
            >
              <span
                className={`w-2 h-2 rounded-full shrink-0 transition-opacity ${
                  isLiveFeed ? 'bg-emerald-400 ring-2 ring-emerald-400/20 animate-pulse' : 'bg-zinc-500'
                }`}
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

      <div className="space-y-4">
        {/* Filter Chips & Search Bar */}
        <div className="p-3 sm:p-4 bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] rounded-xl shadow-sm">
          <RequestFilterBar
            activeTab={activeTab}
            onTabChange={setActiveTab}
            search={search}
            onSearchChange={setSearch}
            onSearchSubmit={() => void loadRequests()}
          />
        </div>

        {/* Requests Table */}
        {isLoading ? (
          <RequestTableSkeleton />
        ) : requests.length === 0 ? (
          <div className="py-12 px-4 text-center space-y-3 bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] rounded-xl shadow-sm">
            <Activity className="w-8 h-8 mx-auto text-zinc-600" />
            <div className="text-sm font-semibold text-white">
              Tidak ada catatan permintaan
            </div>
            <p className="text-xs text-zinc-400 max-w-sm mx-auto">
              Belum ada permintaan inferensi yang cocok dengan kriteria filter atau kata kunci pencarian.
            </p>
          </div>
        ) : (
          <div className="bg-bg-surface/50 backdrop-blur-sm border border-white/[0.06] rounded-xl overflow-hidden shadow-sm">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs whitespace-nowrap">
                <thead>
                  <tr className="border-b border-white/[0.06] bg-white/[0.02] text-zinc-400 font-medium text-xs">
                    <th className="py-2.5 px-4 font-semibold">Timestamp</th>
                    <th className="py-2.5 px-4 font-semibold">Method</th>
                    <th className="py-2.5 px-4 font-semibold">Path</th>
                    <th className="py-2.5 px-4 font-semibold">Status</th>
                    <th className="py-2.5 px-4 font-semibold">Model</th>
                    <th className="py-2.5 px-4 font-semibold">Latency</th>
                    <th className="py-2.5 px-4 font-semibold">Cost</th>
                    <th className="py-2.5 px-4 text-right font-semibold">Aksi</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
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
          </div>
        )}

        {nextCursor && (
          <div className="flex justify-center pt-4 pb-8">
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
      </div>

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
