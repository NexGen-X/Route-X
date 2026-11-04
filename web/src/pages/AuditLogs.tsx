import React, { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type { AuditLogEntry } from '../types';
import { getErrorMessage } from '../utils/error';
import { Card } from '../components/common/Card';
import { DataTableShell, kelasSelKepala, kelasSelIsi } from '../components/common/DataTableShell';
import { Button } from '../components/common/Button';
import { PageHeader } from '../components/common/PageHeader';
import { QueryError } from '../components/common/QueryError';
import { EmptyState } from '../components/common/EmptyState';
import { Drawer } from '../components/common/Drawer';
import { RefreshCw, ScrollText, Search } from 'lucide-react';

// Halaman Log Audit (B-108): endpoint GET /api/admin/system/audit-logs dan
// klien `api.system.auditLogs` sudah lama ada, tetapi tidak ada UI yang
// mengonsumsinya — jejak audit hanya bisa dibaca lewat API mentah.
//
// Filter yang ditawarkan persis yang didukung handler-nya: `action`
// (kecocokan persis), `resource_type` (kecocokan persis), dan rentang
// waktu (`window` atau `from` eksplisit). Handler TIDAK membaca filter
// aktor, jadi halaman ini tidak mengarangnya.

type JendelaAudit = '24h' | '7d' | '30d' | 'semua';

const PILIHAN_JENDELA: Array<{ nilai: JendelaAudit; label: string }> = [
  { nilai: '24h', label: '24 Jam' },
  { nilai: '7d', label: '7 Hari' },
  { nilai: '30d', label: '30 Hari' },
  { nilai: 'semua', label: 'Semua Waktu' },
];

// Awal rentang untuk pilihan "Semua Waktu": dikirim sebagai from eksplisit
// agar backend tidak diam-diam memakai bawaan 24 jamnya (pola yang sama
// dengan halaman Permintaan).
const AWAL_SEMUA_WAKTU = '1970-01-01T00:00:00Z';

const formatWaktu = (iso: string): string => {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
};

const labelAktor = (e: AuditLogEntry): string =>
  e.actor_email || e.actor_user_id || 'Sistem / tidak tercatat';

export const AuditLogs: React.FC = () => {
  const [entries, setEntries] = useState<AuditLogEntry[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [jendela, setJendela] = useState<JendelaAudit>('24h');
  const [aksi, setAksi] = useState('');
  const [jenisSumberDaya, setJenisSumberDaya] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [selected, setSelected] = useState<AuditLogEntry | null>(null);

  // Penjaga race daftar: abaikan respons basi bila filter sudah berganti.
  const listReqRef = useRef(0);

  const loadEntries = async (cursor?: string) => {
    const reqId = ++listReqRef.current;
    if (cursor) setIsLoadingMore(true);
    else setIsLoading(true);
    try {
      const params: Record<string, string> = {};
      if (cursor) params.cursor = cursor;
      if (jendela === 'semua') params.from = AWAL_SEMUA_WAKTU;
      else params.window = jendela;
      if (aksi.trim()) params.action = aksi.trim();
      if (jenisSumberDaya.trim()) params.resource_type = jenisSumberDaya.trim();

      const res = await api.system.auditLogs(params);
      if (listReqRef.current !== reqId) return;
      if (cursor) {
        setEntries((prev) => [...prev, ...(res.items || [])]);
      } else {
        setEntries(res.items || []);
      }
      setNextCursor(res.next_cursor);
      setLoadError(null);
    } catch (err: unknown) {
      if (listReqRef.current !== reqId) return;
      setLoadError(getErrorMessage(err, 'Gagal memuat log audit.'));
    } finally {
      if (listReqRef.current === reqId) {
        setIsLoading(false);
        setIsLoadingMore(false);
      }
    }
  };

  // Muat ulang dari halaman pertama setiap kali jendela berubah; filter
  // teks diterapkan lewat tombol Terapkan Filter / Enter (bukan per ketik)
  // karena keduanya kecocokan persis di backend.
  useEffect(() => {
    void loadEntries();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jendela]);

  const terapkanFilter = () => {
    void loadEntries();
  };

  return (
    <div className="space-y-4">
      <PageHeader
        title={
          <>
            <ScrollText className="w-5 h-5 text-accent-text" aria-hidden="true" />
            Log Audit
          </>
        }
        description="Jejak aksi administratif pada panel ini: siapa melakukan apa, pada sumber daya mana, dan kapan. Data dibaca dari endpoint audit yang sama yang dicatat setiap mutasi admin."
        actions={
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void loadEntries()}
            isLoading={isLoading}
            icon={<RefreshCw className="w-3.5 h-3.5" aria-hidden="true" />}
          >
            Segarkan
          </Button>
        }
      />

      {/* Bilah filter: jendela waktu + kecocokan persis aksi & jenis sumber daya */}
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end">
        <div
          role="group"
          aria-label="Pilih rentang waktu log audit"
          className="flex items-center overflow-x-auto max-w-full bg-bg-surface-1 p-0.5 rounded-md border border-border text-[11px] font-mono scrollbar-none self-start"
        >
          {PILIHAN_JENDELA.map((p) => (
            <button
              key={p.nilai}
              type="button"
              onClick={() => setJendela(p.nilai)}
              aria-pressed={jendela === p.nilai}
              className={`px-2.5 py-1 rounded transition-colors whitespace-nowrap focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/40 cursor-pointer ${
                jendela === p.nilai
                  ? 'bg-bg-surface-2 text-accent-text font-medium border border-accent/30'
                  : 'text-text-secondary hover:text-white'
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>

        <form
          className="flex flex-col sm:flex-row gap-2 flex-1"
          onSubmit={(e) => {
            e.preventDefault();
            terapkanFilter();
          }}
        >
          <label className="flex-1">
            <span className="sr-only">Filter aksi (kecocokan persis)</span>
            <input
              type="text"
              value={aksi}
              onChange={(e) => setAksi(e.target.value)}
              placeholder="Aksi persis, mis. provider.create"
              className="w-full bg-bg-surface-1 border border-border rounded-md px-3 py-2 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/50"
            />
          </label>
          <label className="flex-1">
            <span className="sr-only">Filter jenis sumber daya (kecocokan persis)</span>
            <input
              type="text"
              value={jenisSumberDaya}
              onChange={(e) => setJenisSumberDaya(e.target.value)}
              placeholder="Jenis sumber daya, mis. provider"
              className="w-full bg-bg-surface-1 border border-border rounded-md px-3 py-2 text-xs text-text-primary placeholder:text-text-muted focus:outline-none focus-visible:ring-1 focus-visible:ring-accent/50"
            />
          </label>
          <Button
            variant="secondary"
            size="sm"
            type="submit"
            icon={<Search className="w-3.5 h-3.5" aria-hidden="true" />}
            className="min-h-[36px]"
          >
            Terapkan Filter
          </Button>
        </form>
      </div>

      {loadError ? (
        <QueryError message={loadError} onRetry={() => void loadEntries()} />
      ) : isLoading ? (
        <Card className="p-10 text-center text-xs text-text-muted">
          <span role="status" aria-live="polite">
            Memuat log audit...
          </span>
        </Card>
      ) : entries.length === 0 ? (
        <EmptyState
          icon={<ScrollText className="w-5 h-5" aria-hidden="true" />}
          title="Belum ada catatan audit"
          description="Tidak ada catatan audit pada rentang dan filter ini. Coba rentang waktu yang lebih panjang atau longgarkan filter."
        />
      ) : (
        <Card className="rounded-box bg-bg-surface-1 border border-border/60">
          <DataTableShell testId="tabel-log-audit">
            <thead>
              <tr className="border-b border-border/60">
                <th scope="col" className={kelasSelKepala}>Waktu</th>
                <th scope="col" className={kelasSelKepala}>Aktor</th>
                <th scope="col" className={kelasSelKepala}>Aksi</th>
                <th scope="col" className={kelasSelKepala}>Sumber Daya</th>
                <th scope="col" className={kelasSelKepala}>IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/50">
              {entries.map((e) => (
                <tr
                  key={e.id}
                  className="hover:bg-bg-surface-2/50 cursor-pointer"
                  onClick={() => setSelected(e)}
                >
                  <td className={`${kelasSelIsi} whitespace-nowrap text-text-secondary`}>
                    {formatWaktu(e.occurred_at)}
                  </td>
                  <td className={kelasSelIsi}>
                    <span className="block max-w-[220px] truncate text-text-primary" title={labelAktor(e)}>
                      {labelAktor(e)}
                    </span>
                    {e.actor_role && (
                      <span className="block text-[10px] text-text-muted font-mono">{e.actor_role}</span>
                    )}
                  </td>
                  <td className={`${kelasSelIsi} font-mono text-text-primary`}>{e.action}</td>
                  <td className={kelasSelIsi}>
                    <span className="font-mono text-text-secondary">{e.resource_type}</span>
                    {e.resource_id && (
                      <span className="block max-w-[200px] truncate text-[10px] text-text-muted font-mono" title={e.resource_id}>
                        {e.resource_id}
                      </span>
                    )}
                  </td>
                  <td className={`${kelasSelIsi} font-mono text-text-secondary`}>{e.ip || '—'}</td>
                </tr>
              ))}
            </tbody>
          </DataTableShell>

          {nextCursor && (
            <div className="p-3 border-t border-border/60 flex justify-center">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => void loadEntries(nextCursor)}
                isLoading={isLoadingMore}
              >
                Muat Lebih Banyak
              </Button>
            </div>
          )}
        </Card>
      )}

      {/* Detail satu catatan: metadata lengkap lewat drawer (Calm Console) */}
      <Drawer
        isOpen={selected !== null}
        onClose={() => setSelected(null)}
        title="Detail Catatan Audit"
        subtitle={selected ? formatWaktu(selected.occurred_at) : undefined}
        maxWidth="md"
      >
        {selected && (
          <dl className="space-y-3 text-xs">
            <div>
              <dt className="text-text-muted font-mono text-[10px] uppercase tracking-wide">Aksi</dt>
              <dd className="mt-0.5 font-mono text-text-primary break-all">{selected.action}</dd>
            </div>
            <div>
              <dt className="text-text-muted font-mono text-[10px] uppercase tracking-wide">Aktor</dt>
              <dd className="mt-0.5 text-text-primary break-all">
                {labelAktor(selected)}
                {selected.actor_role ? ` · ${selected.actor_role}` : ''}
              </dd>
            </div>
            <div>
              <dt className="text-text-muted font-mono text-[10px] uppercase tracking-wide">Sumber Daya</dt>
              <dd className="mt-0.5 font-mono text-text-primary break-all">
                {selected.resource_type}
                {selected.resource_id ? ` · ${selected.resource_id}` : ''}
              </dd>
            </div>
            {selected.ip && (
              <div>
                <dt className="text-text-muted font-mono text-[10px] uppercase tracking-wide">Alamat IP</dt>
                <dd className="mt-0.5 font-mono text-text-primary">{selected.ip}</dd>
              </div>
            )}
            {selected.request_id && (
              <div>
                <dt className="text-text-muted font-mono text-[10px] uppercase tracking-wide">ID Permintaan</dt>
                <dd className="mt-0.5 font-mono text-text-primary break-all">{selected.request_id}</dd>
              </div>
            )}
            {selected.user_agent && (
              <div>
                <dt className="text-text-muted font-mono text-[10px] uppercase tracking-wide">User Agent</dt>
                <dd className="mt-0.5 text-text-secondary break-all">{selected.user_agent}</dd>
              </div>
            )}
            {selected.metadata && Object.keys(selected.metadata).length > 0 && (
              <div>
                <dt className="text-text-muted font-mono text-[10px] uppercase tracking-wide">Metadata</dt>
                <dd className="mt-0.5">
                  <pre className="bg-bg-surface-2/60 border border-border/60 rounded-md p-3 font-mono text-[11px] text-text-secondary overflow-x-auto whitespace-pre-wrap break-all">
                    {JSON.stringify(selected.metadata, null, 2)}
                  </pre>
                </dd>
              </div>
            )}
          </dl>
        )}
      </Drawer>
    </div>
  );
};
