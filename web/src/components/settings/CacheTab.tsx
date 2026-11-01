import React, { useCallback, useEffect, useState } from 'react';
import { Zap, Trash2, CheckCircle2 } from 'lucide-react';
import { api, ApiError } from '../../api/client';
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
// Tab ini jujur soal keadaan itu: tidak ada tombol aktifkan yang pasti
// gagal. Yang ditawarkan hanya operasi yang benar-benar bekerja —
// melihat statistik, mengatur TTL (dipersistensikan dan dipakai saat
// kebijakan aktivasi tiba), menonaktifkan bila sedang menyala, dan
// mengosongkan cache. Penolakan cache_policy_unsafe dari server tetap
// ditangkap dan ditampilkan sebagai penjelasan, bukan toast gagal.

const TTL_MAKS_MENIT = 1440; // Batas server: TTLSeconds ≤ 86400 (24 jam).

export const CacheTab: React.FC = () => {
  const { toast, confirmModal } = useToast();
  const [cache, setCache] = useState<ResponseCacheStats | null>(null);
  const [memuat, setMemuat] = useState<boolean>(true);
  const [gagalMuat, setGagalMuat] = useState<string | null>(null);
  const [ttlMinutes, setTtlMinutes] = useState<number>(60);
  const [menyimpan, setMenyimpan] = useState<boolean>(false);
  const [mengosongkan, setMengosongkan] = useState<boolean>(false);
  const [penolakanKebijakan, setPenolakanKebijakan] = useState<string | null>(null);

  const muat = useCallback(async () => {
    setMemuat(true);
    try {
      const stats = await api.system.cacheStats();
      setCache(stats);
      setTtlMinutes(Math.max(1, Math.round((stats.ttl_seconds || 0) / 60) || 60));
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

  // Tangani kegagalan simpan: penolakan kebijakan tampil sebagai
  // penjelasan menetap; kegagalan lain cukup toast.
  const tanganiGagalSimpan = (err: unknown, konteks: string) => {
    if (err instanceof ApiError && err.code === 'cache_policy_unsafe') {
      setPenolakanKebijakan(err.message);
      return;
    }
    toast.error(`${konteks}: ${getErrorMessage(err)}`);
  };

  const handleSimpanTTL = async () => {
    if (!cache) return;
    const ttl = Math.min(TTL_MAKS_MENIT, Math.max(1, Math.floor(Number(ttlMinutes) || 60)));
    setMenyimpan(true);
    setPenolakanKebijakan(null);
    try {
      // enabled selalu dikirim apa adanya dari statistik terakhir; server
      // menolak enabled=true (cache_policy_unsafe) dan menerjemahkannya
      // di tanganiGagalSimpan menjadi penjelasan.
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
    setPenolakanKebijakan(null);
    try {
      await api.system.updateCacheSettings(false, cache?.ttl_seconds ?? ttlMinutes * 60);
      toast.info('Cache respons dinonaktifkan.', 'Pengaturan Cache');
      void muat();
    } catch (err: unknown) {
      tanganiGagalSimpan(err, 'Gagal menonaktifkan cache');
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
          {cache?.enabled && (
            <Button variant="danger" size="sm" isLoading={menyimpan} onClick={() => void handleNonaktifkan()}>
              Nonaktifkan Cache
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

      {/* Penjelasan jujur: aktivasi ditahan kebijakan keamanan server.
          Ringkasan selalu tampil; penjelasan panjang di balik Detail
          (InfoBanner, Fase 3 P5) — bukan lagi dinding teks. */}
      {!cache?.enabled && (
        <InfoBanner variant="warning" className="mt-4" summary="Cache belum dapat diaktifkan dari panel ini.">
          Server menahan aktivasi sampai kebijakan fingerprint tenant, routing, dan policy diterapk ...[truncated 4518 chars]an (keputusan keamanan audit F3 — kunci cache belum membedakan konteks tersebut, jadi respons bisa tersaji lintas konteks). TTL di bawah tetap berlaku dan tersimpan; statistik tetap terpantau dari sini.
        </InfoBanner>
      )}

      {/* Penolakan kebijakan dari server, ditampilkan apa adanya. */}
      {penolakanKebijakan && (
        <div className="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3">
          <p className="text-[11px] leading-relaxed text-red-300">
            Server menolak perubahan ini: {penolakanKebijakan}
          </p>
        </div>
      )}

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
                value={ttlMinutes}
                onChange={(e) => setTtlMinutes(Math.max(1, parseInt(e.target.value) || 1))}
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
          <span className="text-[10px] text-text-muted mt-1 block">
            {ttlMinutes >= 60
              ? `Sekitar ${(ttlMinutes / 60).toFixed(1)} jam masa berlaku (maks ${TTL_MAKS_MENIT / 60} jam)`
              : `Sekitar ${ttlMinutes} menit masa berlaku`}
          </span>
        </div>
      </div>
    </div>
  );
};
