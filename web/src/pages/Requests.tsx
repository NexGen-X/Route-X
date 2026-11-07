import React, { useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../api/client';
import type { RequestLog, RequestEvent, RequestPayload } from '../types';
import { getErrorMessage } from '../utils/error';
import { Card } from '../components/common/Card';
import { DataTableShell, kelasSelIsi } from '../components/common/DataTableShell';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { QueryError } from '../components/common/QueryError';
import { keadaanMuat } from '../lib/keadaanMuat';
import { RefreshCw, Activity } from 'lucide-react';
import {
  RequestFilterBar,
  RequestTableSkeleton,
  RequestTableRow,
  RequestCardMobile,
  RequestInspectorModal,
  type RequestTabFilter,
} from '../components/requests';

// Awal rentang untuk pilihan "Semua Waktu": dikirim sebagai from eksplisit
// agar backend tidak diam-diam memakai bawaan 24 jamnya di balik label itu
// (audit3 V-FJ-03). Epoch cukup lama untuk mencakup seluruh riwayat.
const AWAL_SEMUA_WAKTU = '1970-01-01T00:00:00Z';

export const Requests: React.FC = () => {
  const [requests, setRequests] = useState<RequestLog[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [activeTab, setActiveTab] = useState<RequestTabFilter>('all');
  const [search, setSearch] = useState('');
  const [providerFilter, setProviderFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [timeRangeFilter, setTimeRangeFilter] = useState('');
  // Opsi dropdown provider adalah provider NYATA milik operator dari API —
  // bukan daftar slug vendor karangan yang tidak pernah cocok dengan
  // provider_id mana pun (audit3 V-FJ-03).
  const [providers, setProviders] = useState<Array<{ id: string; name: string }>>([]);
  // B-454: kegagalan daftar provider untuk filter dicatat — dulu
  // ditelan sehingga "tidak ada provider" dan "gagal dimuat" tidak
  // dapat dibedakan operator.
  const [providersError, setProvidersError] = useState<string | null>(null);
  const [selectedReq, setSelectedReq] = useState<RequestLog | null>(null);
  const [reqEvents, setReqEvents] = useState<RequestEvent[]>([]);
  const [reqPayload, setReqPayload] = useState<RequestPayload | null>(null);
  // Payload yang tidak tersimpan (404 dari endpoint payload) adalah keadaan
  // normal by design — Route-X tidak merekam body request/respons — bukan
  // kegagalan muat, jadi dibedakan dari error inspector sungguhan (B-078).
  const [payloadTidakTersimpan, setPayloadTidakTersimpan] = useState(false);
  const [isInspectorOpen, setIsInspectorOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isLiveFeed, setIsLiveFeed] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [inspectorError, setInspectorError] = useState<string | null>(null);
  // B-593: penanda kegagalan EVENTS secara terpisah dari `inspectorError`
  // gabungan (events dan/atau payload). Bila HANYA payload gagal sementara
  // events kosong secara sah, klaim kosong timeline tetap benar — jadi
  // gerbangnya harus penanda khusus ini, bukan `inspectorError`.
  const [eventsGagal, setEventsGagal] = useState(false);

  // Penjaga race inspector: abaikan respons basi bila user sudah klik request lain.
  const inspectorReqRef = useRef(0);
  // Penjaga race daftar: abaikan respons basi bila user sudah ganti tab/cari.
  const listReqRef = useRef(0);

  // Debounce pencarian: mengetik tidak langsung menembak API, tunggu 400ms diam.
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
      // provider_id adalah filter kanonis backend; nilai dropdown kini ID
      // provider operator, bukan slug vendor (audit3 V-FJ-03).
      if (providerFilter) params.provider_id = providerFilter;
      if (statusFilter) params.status_code = statusFilter;
      if (timeRangeFilter) {
        params.time_range = timeRangeFilter;
      } else {
        // "Semua Waktu" harus benar-benar tanpa batas bawah.
        params.from = AWAL_SEMUA_WAKTU;
      }

      const res = await api.requests.list(params);
      if (listReqRef.current !== reqId) return;
      if (silent) {
        // Mode Live (audit3 V-FJ-14): GABUNGKAN halaman pertama yang segar
        // ke daftar yang sudah tampil, jangan menggantinya — versi lama
        // mereset daftar ke 25 baris setiap 4 detik dan membuang halaman
        // "Muat Lebih Banyak" yang sudah dimuat operator. Kursor lama
        // dipertahankan: ia menunjuk sesudah halaman terdalam yang dimuat,
        // dan halaman segar hanya berisi item yang lebih baru darinya.
        const fresh = res.items || [];
        setRequests((prev) => {
          const kunci = (x: RequestLog) => x.id || x.request_id;
          const freshKeys = new Set(fresh.map(kunci));
          return [...fresh, ...prev.filter((x) => !freshKeys.has(kunci(x)))];
        });
      } else if (cursor) {
        setRequests((prev) => [...prev, ...(res.items || [])]);
        setNextCursor(res.next_cursor);
      } else {
        setRequests(res.items || []);
        setNextCursor(res.next_cursor);
      }
      setLoadError(null);
    } catch (err: unknown) {
      if (listReqRef.current !== reqId) return;
      setLoadError(err instanceof Error ? err.message : String(err));
    } finally {
      if (listReqRef.current === reqId && !silent) setIsLoading(false);
    }
  };

  // Muat daftar provider operator untuk dropdown filter (sekali).
  useEffect(() => {
    let aktif = true;
    api.providers
      .list({ limit: 100 })
      .then((res) => {
        if (aktif) {
          setProviders((res.items || []).map((p) => ({ id: p.id, name: p.name })));
          setProvidersError(null);
        }
      })
      .catch((err: unknown) => {
        // B-454: dropdown tetap tinggal "Semua Provider" (bukan
        // daftar karangan), tetapi kegagalannya kini DITANDAI di
        // filter lewat providersError.
        if (aktif) setProvidersError(getErrorMessage(err));
      });
    return () => {
      aktif = false;
    };
  }, []);

  useEffect(() => {
    void loadRequests();
  }, [activeTab, providerFilter, statusFilter, timeRangeFilter]);

  useEffect(() => {
    if (!isLiveFeed) return;
    const interval = setInterval(() => {
      void loadRequests(undefined, true);
    }, 4000);
    return () => clearInterval(interval);
  }, [isLiveFeed, activeTab, search, providerFilter, statusFilter, timeRangeFilter]);

  const handleInspect = async (req: RequestLog) => {
    const reqId = ++inspectorReqRef.current;
    setSelectedReq(req);
    // Bersihkan state lama agar modal tidak menampilkan payload request sebelumnya.
    setReqEvents([]);
    setReqPayload(null);
    setPayloadTidakTersimpan(false);
    setIsInspectorOpen(true);
    setInspectorError(null);
    setEventsGagal(false);
    try {
      const targetId = req.id || req.request_id;
      // allSettled TANPA catch penelan (audit F7): versi lama menangkap
      // kegagalan events/payload menjadi daftar kosong / null, sehingga
      // inspector menampilkan "tidak ada events" untuk request yang
      // datanya GAGAL dimuat — keadaan kosong karangan. Kini kegagalan
      // dilaporkan lewat inspectorError sementara bagian yang berhasil
      // tetap ditampilkan.
      const [eventsRes, payloadRes] = await Promise.allSettled([
        api.requests.events(targetId, req.created_at),
        api.requests.payload(targetId, req.created_at),
      ]);
      if (inspectorReqRef.current !== reqId) return;
      const gagal: string[] = [];
      if (eventsRes.status === 'fulfilled') {
        setReqEvents(eventsRes.value.events || []);
        setEventsGagal(false);
      } else {
        setReqEvents([]);
        setEventsGagal(true);
        gagal.push('events: ' + getErrorMessage(eventsRes.reason));
      }
      if (payloadRes.status === 'fulfilled') {
        setReqPayload(payloadRes.value);
      } else if (payloadRes.reason instanceof ApiError && payloadRes.reason.status === 404) {
        // Endpoint payload menjawab 404 (kode not_found) bila baris payload
        // tidak ada. Karena SavePayload tidak pernah dipanggil di produksi,
        // inilah keadaan normal untuk SEMUA request: payload memang tidak
        // disimpan, bukan gagal dimuat — jangan laporkan sebagai error
        // inspector (B-078). Kegagalan lain (5xx, jaringan) tetap error.
        setReqPayload(null);
        setPayloadTidakTersimpan(true);
      } else {
        setReqPayload(null);
        gagal.push('payload: ' + getErrorMessage(payloadRes.reason));
      }
      if (gagal.length > 0) {
        setInspectorError('Sebagian data inspector gagal dimuat — ' + gagal.join('; '));
      }
    } catch (err: unknown) {
      if (inspectorReqRef.current !== reqId) return;
      setReqEvents([]);
      setReqPayload(null);
      setPayloadTidakTersimpan(false);
      setEventsGagal(true);
      setInspectorError(err instanceof Error ? err.message : String(err));
    }
  };

  const handleResetFilters = () => {
    setActiveTab('all');
    setSearch('');
    setProviderFilter('');
    setStatusFilter('');
    setTimeRangeFilter('');
  };

  return (
    <div className="space-y-6">
      {/* Header & Filter Bar */}
      <PageHeader
        title="Permintaan"
        actions={
          <>
            <button
              type="button"
              onClick={() => setIsLiveFeed(!isLiveFeed)}
              className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-inner text-xs font-semibold border transition-all cursor-pointer min-h-[44px] sm:min-h-[36px] ${
                isLiveFeed
                  ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
                  : 'bg-bg-surface-2 text-text-secondary border-border hover:text-white'
              }`}
            >
              <span
                className={`w-2 h-2 rounded-full shrink-0 transition-opacity ${
                  isLiveFeed ? 'bg-emerald-400 ring-2 ring-emerald-400/20' : 'bg-text-muted/60'
                }`}
              />
              <span>{isLiveFeed ? 'Live Polling Aktif' : 'Live Stream'}</span>
            </button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void loadRequests()}
              isLoading={isLoading}
              icon={<RefreshCw className="w-3.5 h-3.5" aria-hidden="true" />}
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
          providerFilter={providerFilter}
          onProviderFilterChange={setProviderFilter}
          providers={providers}
          providersError={providersError}
          statusFilter={statusFilter}
          onStatusFilterChange={setStatusFilter}
          timeRangeFilter={timeRangeFilter}
          onTimeRangeFilterChange={setTimeRangeFilter}
          onResetFilters={handleResetFilters}
        />

        {/* Requests List: Card List on Mobile, Table on Desktop */}
        {isLoading ? (
          <RequestTableSkeleton />
        ) : keadaanMuat({ memuat: isLoading, gagal: loadError != null, jumlah: requests.length }) === 'gagal' ? (
          // B-410: muatan gagal — daftar kosong berarti belum
          // terukur, bukan "tidak ada catatan". QueryError di atas
          // kartu membawa rincian + coba lagi.
          <div className="py-12 px-4 text-center space-y-3">
            <Activity className="w-8 h-8 mx-auto text-text-muted/40" aria-hidden="true" />
            <div className="text-sm font-semibold text-white">
              Catatan permintaan belum dapat dimuat
            </div>
            <p className="text-xs text-text-muted max-w-sm mx-auto">
              Daftar permintaan belum terukur karena muatan gagal. Gunakan tombol coba lagi di pesan galat di atas.
            </p>
          </div>
        ) : requests.length === 0 ? (
          <div className="py-12 px-4 text-center space-y-3">
            <Activity className="w-8 h-8 mx-auto text-text-muted/40" aria-hidden="true" />
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
            <DataTableShell className="hidden sm:block">
                <thead>
                  <tr className="border-b border-border text-text-muted font-medium text-xs bg-bg-surface-2/40">
                    <th className={`${kelasSelIsi} font-semibold`}>Status &amp; Request ID</th>
                    <th className={`${kelasSelIsi} font-semibold`}>Model &amp; Provider</th>
                    <th className={`${kelasSelIsi} font-semibold`}>Token &amp; Cost</th>
                    <th className={`${kelasSelIsi} font-semibold`}>Durasi &amp; TTFT</th>
                    <th className={`${kelasSelIsi} font-semibold`}>Waktu &amp; Klien</th>
                    <th className={`${kelasSelIsi} text-right`}>Aksi</th>
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
            </DataTableShell>
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
        payloadTidakTersimpan={payloadTidakTersimpan}
        eventsGagal={eventsGagal}
        error={inspectorError}
        onRetry={() => {
          if (selectedReq) void handleInspect(selectedReq);
        }}
      />
    </div>
  );
};
