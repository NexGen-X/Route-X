import React, { useCallback, useEffect, useState } from 'react';
import { Zap, Trash2, CheckCircle2 } from 'lucide-react';
import { api } from '../../api/client';
import type { ResponseCacheStats } from '../../types';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { Tooltip } from '../common/Tooltip';
import { useToast } from '../../context/ToastContext';
import { getErrorMessage } from './utils';
import { InfoBanner } from '../common/InfoBanner';

// Tab "Performa & Cache" di Settings (Fase 2 R3) — rumah tunggal kontrol
// cache respons. Sebelumnya kontrol ini tinggal di halaman Diagnostics,
// jauh dari pengaturan lain, dan tombol "Aktifkan Cache"-nya selalu gagal
// diam-diam membingungkan: server menolak aktivasi dengan kode
// cache_policy_unsafe (keputusan keamanan audit F3 — cache belum boleh
// menyala sampai kebijakan fingerprint tenant/routing/policy diterapkan,
// karena kunci cache belum membedakan konteks tersebut).
//
// Sejak Batch C1 kebijakan itu diterapkan di server — kunci cache
// terisolasi per API key (identitas ter-hash), memuat permukaan dan
// sidik rute, dan tahap awal hanya menyimpan respons sukses
// non-streaming tanpa tool — jadi aktivasi kini DITERIMA dan tab ini
// menawarkan kontrol yang berfungsi penuh: aktifkan, nonaktifkan
// (berlaku seketika), atur TTL, lihat statistik, dan kosongkan cache.
// Batas tahap awal dinyatakan apa adanya di banner, bukan disembunyikan.

const TTL_MAKS_MENIT = 1440; // Batas server: TTLSeconds ≤ 86400 (24 jam).

export const CacheTab: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [cache, setCache] = useState<ResponseCacheStats | null>(null);
  const [memuat, setMemuat] = useState<boolean>(true);
  const [gagalMuat, setGagalMuat] = useState<string | null>(null);
  // TTL dipegang sebagai TEKS mentah (audit3 V-FF-18): versi lama menjepit
  // nilai di luar rentang secara diam-diam saat simpan (ketik 2000 -> 1440)
  // dan mengosongkan kolom pun mustahil. Kini nilai di luar 1..1440 DITOLAK
  // dengan pesan galat yang terlihat; yang tersimpan selalu yang diketik.
  const [ttlTeks, setTtlTeks] = useState<string>('60');
  const [ttlGalat, setTtlGalat] = useState<string | null>(null);
  const [menyimpan, setMenyimpan] = useState<boolean>(false);
  const [mengosongkan, setMengosongkan] = useState<boolean>(false);

  const muat = useCallback(async () => {
    setMemuat(true);
    try {
      const stats = await api.system.cacheStats();
      setCache(stats);
      setTtlTeks(String(Math.max(1, Math.round((stats.ttl_seconds || 0) / 60) || 60)));
      setTtlGalat(null);
      setGagalMuat(null);
    } catch (err: unknown) {
      setGagalMuat(getErrorMessage(err));
    } finally {
      setMemuat(false);
    }
  }, []);

  useEffect(() => {
    void muat();
  }, [muat]);

  // Kegagalan simpan cukup toast: server tidak lagi menolak aktivasi
  // dengan kode kebijakan (Batch C1), jadi tidak ada penjelasan menetap
  // yang perlu ditampilkan.
  const tanganiGagalSimpan = (err: unknown, konteks: string) => {
    toast.error(`${konteks}: ${getErrorMessage(err)}`);
  };

  // Mengurai teks TTL menjadi menit yang sah, atau pesan galat. Tidak ada
  // penjepitan senyap: di luar rentang berarti ditolak (V-FF-18).
  const uraiTtl = (): { ttl: number } | { galat: string } => {
    const t = ttlTeks.trim();
    if (!/^\d+$/.test(t)) {
      return { galat: 'TTL harus berupa bilangan bulat menit (tanpa desimal).' };
    }
    const ttl = Number(t);
    if (ttl < 1 || ttl > TTL_MAKS_MENIT) {
      return { galat: `TTL harus antara 1 dan ${TTL_MAKS_MENIT} menit (maks 24 jam) — nilai ${ttl} menit tidak disimpan.` };
    }
    return { ttl };
  };

  const handleSimpanTTL = async () => {
    if (!cache) return;
    const hasil = uraiTtl();
    if ('galat' in hasil) {
      setTtlGalat(hasil.galat);
      toast.error(hasil.galat, 'TTL Tidak Valid');
      return;
    }
    const ttl = hasil.ttl;
    setTtlGalat(null);
    setMenyimpan(true);
    try {
      // enabled selalu dikirim apa adanya dari statistik terakhir —
      // menyimpan TTL tidak mengubah status aktif/nonaktif.
      await api.system.updateCacheSettings(cache.enabled, ttl * 60);
      toast.success(`TTL cache respons tersimpan: ${ttl} menit.`, 'TTL Tersimpan');
      void muat();
    } catch (err: unknown) {
      tanganiGagalSimpan(err, 'Gagal menyimpan TTL');
    } finally {
      setMenyimpan(false);
    }
  };

  const handleNonaktifkan = async () => {
    setMenyimpan(true);
    try {
      await api.system.updateCacheSettings(false, cache?.ttl_seconds ?? 3600);
      toast.info('Cache respons dinonaktifkan.', 'Pengaturan Cache');
      void muat();
    } catch (err: unknown) {
      tanganiGagalSimpan(err, 'Gagal menonaktifkan cache');
    } finally {
      setMenyimpan(false);
    }
  };

  // Mengaktifkan dengan TTL dari kolom yang sedang tampil (divalidasi
  // dengan pengurai yang sama seperti simpan TTL — tidak ada nilai yang
  // terkirim tanpa terlihat operator).
  const handleAktifkan = async () => {
    const hasil = uraiTtl();
    if ('galat' in hasil) {
      setTtlGalat(hasil.galat);
      toast.error(hasil.galat, 'TTL Tidak Valid');
      return;
    }
    setTtlGalat(null);
    setMenyimpan(true);
    try {
      await api.system.updateCacheSettings(true, hasil.ttl * 60);
      toast.success('Cache respons diaktifkan.', 'Pengaturan Cache');
      void muat();
    } catch (err: unknown) {
      tanganiGagalSimpan(err, 'Gagal mengaktifkan cache');
    } finally {
      setMenyimpan(false);
    }
  };

  const handleFlush = async () => {
    const ok = await confirmModal({
      title: 'Kosongkan Response Cache?',
      message:
        'Apakah Anda yakin ingin mengosongkan seluruh entri cache inferensi di Redis? Permintaan berikutnya akan langsung menuju ke upstream.',
      confirmText: 'Ya, Kosongkan',
      cancelText: 'Batal',
      danger: true,
    });
    if (!ok) return;
    setMengosongkan(true);
    try {
      const res = await api.system.flushCache();
      toast.success(res.message || 'Response cache berhasil dibersihkan.', 'Cache Dikosongkan');
      void muat();
    } catch (err: unknown) {
      toast.error('Gagal membersihkan cache: ' + getErrorMessage(err));
    } finally {
      setMengosongkan(false);
    }
  };

  const totalReq = (cache?.hits || 0) + (cache?.misses || 0);
  const hitRatio = totalReq > 0 ? (((cache?.hits || 0) / totalReq) * 100).toFixed(1) : '0.0';

  return (
    <div>
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-border/60">
        <div className="flex items-center gap-3">
          <Zap className="w-5 h-5 text-accent-text shrink-0" aria-hidden="true" />
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-base font-semibold text-text-primary">Cache Respons Redis</h3>
              <Badge variant={cache?.enabled ? 'success' : 'neutral'}>
                {cache?.enabled ? 'AKTIF' : 'NONAKTIF'}
              </Badge>
            </div>
            <p className="text-[11px] text-text-muted mt-0.5">
              Respons identik dilayani dari Redis tanpa memanggil upstream.
            </p>
          </div>
        </div>
        <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2 w-full sm:w-auto">
          {cache?.enabled ? (
            <Button variant="danger" size="sm" isLoading={menyimpan} onClick={() => void handleNonaktifkan()}>
              Nonaktifkan Cache
            </Button>
          ) : (
            <Button variant="primary" size="sm" isLoading={menyimpan} disabled={!cache} onClick={() => void handleAktifkan()}>
              Aktifkan Cache
            </Button>
          )}
          <Button
            variant="secondary"
            size="sm"
            isLoading={mengosongkan}
            onClick={() => void handleFlush()}
            className="text-red-400 hover:text-red-300 border-red-900/40 hover:bg-red-950/20"
            icon={<Trash2 className="w-3.5 h-3.5" aria-hidden="true" />}
          >
            Kosongkan Cache
          </Button>
        </div>
      </div>

      {/* Penjelasan perilaku & batas tahap awal aktivasi (Batch C1):
          ringkasan selalu tampil; rincian di balik Detail (InfoBanner). */}
      <InfoBanner
        variant="info"
        className="mt-4"
        summary={
          cache?.enabled
            ? 'Cache aktif — hanya respons sukses non-streaming tanpa tool yang disimpan.'
            : 'Cache nonaktif — aktifkan untuk melayani respons identik dari Redis.'
        }
      >
        Kunci cache terisolasi per API key: respons untuk satu kunci tidak pernah melayani kunci lain.
        Yang disimpan hanya respons sukses (2xx) non-streaming dari chat completions dan permukaan
        Anthropic; streaming, respons error, jawaban yang ditolak penyaring konten, dan permintaan
        yang mendeklarasikan tool tidak pernah di-cache. Perubahan provider, routing, atau pemetaan
        model membuat entri lama tidak terjangkau dan kedaluwarsa mengikuti TTL; perubahan kredensial
        mengosongkan cache. Menonaktifkan cache menghentikan penyajian dari cache seketika.
      </InfoBanner>


      {gagalMuat && (
        <p className="mt-4 text-xs text-red-400">
          Gagal memuat statistik cache: {gagalMuat}{' '}
          <button type="button" onClick={() => void muat()} className="underline cursor-pointer">
            Coba lagi
          </button>
        </p>
      )}

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mt-4">
        <div className="py-1">
          <span className="text-[11px] text-text-muted block">Rasio Hit Cache</span>
          <span className="text-xl font-bold font-mono text-emerald-400 mt-1 block">
            {memuat ? '…' : `${hitRatio}%`}
          </span>
          <span className="text-[10px] text-text-secondary block mt-0.5 font-mono">
            {cache?.hits || 0} hit / {totalReq} total
          </span>
        </div>

        <div className="py-1">
          <span className="text-[11px] text-text-muted block">Hit Cache (Hemat Biaya)</span>
          <span className="text-xl font-bold font-mono text-text-primary mt-1 block">
            {cache?.hits || 0}
          </span>
          <span className="text-[10px] text-emerald-400/80 block mt-0.5">
            Dilayani lokal, tanpa upstream
          </span>
        </div>

        <div className="py-1">
          <span className="text-[11px] text-text-muted block">Total Entri di Redis</span>
          <span className="text-xl font-bold font-mono text-accent-text mt-1 block">
            {cache?.total_entries || 0}
          </span>
          <span className="text-[10px] text-text-secondary block mt-0.5">
            Prompt aktif tersimpan
          </span>
        </div>

        <div className="py-1 flex flex-col justify-between">
          <div>
            <span className="text-[11px] text-text-muted block">Durasi Simpan (TTL)</span>
            <div className="flex items-center gap-1.5 mt-1">
              <input
                id="cache-ttl-minutes"
                aria-label="Durasi simpan cache dalam menit"
                type="number"
                min="1"
                max={TTL_MAKS_MENIT}
                value={ttlTeks}
                aria-invalid={ttlGalat !== null}
                onChange={(e) => {
                  setTtlTeks(e.target.value);
                  setTtlGalat(null);
                }}
                className="w-16 bg-bg-base border border-border/80 rounded px-1.5 py-0.5 text-xs text-text-primary font-mono focus:border-accent focus:outline-none"
              />
              <span className="text-xs text-text-secondary">menit</span>
              <Tooltip content="Simpan TTL" position="left">
                <button
                  type="button"
                  onClick={() => void handleSimpanTTL()}
                  disabled={menyimpan || !cache}
                  className="p-1.5 rounded bg-accent/20 hover:bg-accent/30 text-accent-text ml-auto text-xs min-w-[28px] min-h-[28px] flex items-center justify-center cursor-pointer focus:outline-none focus-visible:ring-1 focus-visible:ring-accent"
                  aria-label="Simpan TTL"
                >
                  <CheckCircle2 className="w-3.5 h-3.5" aria-hidden="true" />
                </button>
              </Tooltip>
            </div>
          </div>
          {ttlGalat ? (
            <span role="alert" className="text-[10px] text-red-400 mt-1 block">
              {ttlGalat}
            </span>
          ) : (
            <span className="text-[10px] text-text-muted mt-1 block">
              {(() => {
                const n = Number(ttlTeks);
                if (!Number.isInteger(n) || n < 1) {
                  return `Rentang sah 1–${TTL_MAKS_MENIT} menit (maks 24 jam)`;
                }
                return n >= 60
                  ? `Sekitar ${(n / 60).toFixed(1)} jam masa berlaku (rentang sah 1–${TTL_MAKS_MENIT} menit)`
                  : `Sekitar ${n} menit masa berlaku`;
              })()}
            </span>
          )}
        </div>
      </div>
    </div>
  );
};
