import React, { useCallback, useEffect, useRef, useState } from 'react';
import { DollarSign } from 'lucide-react';
import type { Price, Provider, ProviderModel, ReferencePrice } from '../../types';
import { api } from '../../api/client';
import { useToast } from '../../context/ToastContext';
import { getErrorMessage } from '../../utils/error';
import { formatUSD } from '../../utils/money';
import { Drawer } from '../common/Drawer';
import { Button } from '../common/Button';
import { QueryError } from '../common/QueryError';

// hargaEfektifSekarang memilih baris harga yang sedang menagih dari riwayat
// yang dikembalikan GET .../pricing/history, memakai predikat yang sama
// dengan PricingRepo.Current di backend (internal/database/repo/upstream/
// pricing.go): effective_from <= sekarang DAN (effective_to kosong ATAU
// effective_to > sekarang); bila beberapa baris memenuhi, yang
// effective_from-nya paling baru yang menang. Baris yang hanya "terbuka"
// (effective_to kosong) tetapi effective_from-nya masih di masa depan BUKAN
// harga efektif — backend pun tidak menagihnya.
export function hargaEfektifSekarang(items: Price[], sekarang: Date = new Date()): Price | undefined {
  const t = sekarang.getTime();
  let terpilih: Price | undefined;
  for (const p of items) {
    const dari = new Date(p.effective_from).getTime();
    if (Number.isNaN(dari) || dari > t) continue;
    if (p.effective_to) {
      const sampai = new Date(p.effective_to).getTime();
      if (!Number.isNaN(sampai) && sampai <= t) continue;
    }
    if (!terpilih || dari > new Date(terpilih.effective_from).getTime()) terpilih = p;
  }
  return terpilih;
}

function formatTanggal(iso?: string | null): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('id-ID');
}

function BarisHarga({ label, nilai }: { label: string; nilai?: string | null }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1">
      <span className="text-xs text-text-secondary">{label}</span>
      <span className="font-mono text-xs text-text-primary">
        {/* formatUSD 6 digit pecahan: kolom harga numeric(14,6), jadi tidak
            ada presisi tersimpan yang terpotong oleh tampilan. */}
        {nilai === undefined || nilai === null || nilai === '' ? '— (ikuti input)' : `${formatUSD(nilai, 6)} / 1M token`}
      </span>
    </div>
  );
}

export interface PricingDrawerProps {
  mapping: ProviderModel | null;
  provider: Provider | null;
  onClose: () => void;
}

// PricingDrawer menampilkan, untuk SATU pemetaan provider–model:
//   1. harga EFEKTIF yang sedang menagih (diturunkan dari riwayat harga),
//   2. harga ACUAN katalog bawaan untuk pemetaan itu (endpoint reference),
//   3. formulir pasang harga manual (endpoint tulis POST pricing), dan
//   4. riwayat harga.
// Ketiganya sengaja TIDAK dicampur: harga acuan hanyalah tawaran awal dari
// katalog dan tidak pernah menagih; yang menagih adalah harga efektif.
// Endpoint reference bersifat per-pemetaan — backend tidak menyediakan
// daftar katalog acuan global, jadi tidak ada tabel katalog di sini.
export const PricingDrawer: React.FC<PricingDrawerProps> = ({ mapping, provider, onClose }) => {
  const { toast } = useToast();
  const [riwayat, setRiwayat] = useState<Price[]>([]);
  const [acuan, setAcuan] = useState<ReferencePrice | null>(null);
  const [memuat, setMemuat] = useState(false);
  const [galat, setGalat] = useState<string | null>(null);
  const [input, setInput] = useState('');
  const [output, setOutput] = useState('');
  const [cached, setCached] = useState('');
  const [menyimpan, setMenyimpan] = useState(false);
  const [galatForm, setGalatForm] = useState<string | null>(null);

  const mappingId = mapping?.id;

  // B-486 (sapuan penjaga urutan): token urutan muat — berpindah
  // pemetaan dengan cepat dulu membuat respons pemetaan lama yang
  // tiba terakhir menimpa riwayat/acuan pemetaan yang sedang
  // dibuka, dan tombol "pakai acuan" lalu mengisi formulir harga
  // MILIK pemetaan lain ke pemetaan ini.
  const muatReqRef = useRef(0);

  const muat = useCallback(async () => {
    if (!mappingId) return;
    const reqId = ++muatReqRef.current;
    setMemuat(true);
    setGalat(null);
    try {
      const [hist, ref] = await Promise.all([
        api.models.pricingHistory(mappingId),
        api.models.pricingReference(mappingId),
      ]);
      if (muatReqRef.current !== reqId) return;
      setRiwayat(hist.items ?? []);
      setAcuan(ref);
    } catch (err) {
      if (muatReqRef.current !== reqId) return;
      setGalat(getErrorMessage(err, 'Gagal memuat harga pemetaan ini.'));
    } finally {
      if (muatReqRef.current === reqId) setMemuat(false);
    }
  }, [mappingId]);

  useEffect(() => {
    if (mappingId) void muat();
  }, [mappingId, muat]);

  if (!mapping) return null;

  const efektif = hargaEfektifSekarang(riwayat);

  const pakaiAcuan = () => {
    if (!acuan?.available) return;
    setInput(acuan.input_per_1m_usd ?? '');
    setOutput(acuan.output_per_1m_usd ?? '');
    setCached(acuan.cached_input_per_1m_usd ?? '');
    setGalatForm(null);
  };

  const simpan = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || !output.trim()) {
      setGalatForm('Harga input dan output wajib diisi (USD per 1M token).');
      return;
    }
    setMenyimpan(true);
    setGalatForm(null);
    try {
      await api.models.setPrice(mapping.id, {
        input_per_1m_usd: input.trim(),
        output_per_1m_usd: output.trim(),
        // Kosong SENGAJA tidak dikirim sebagai "0": backend menafsirkan
        // cached yang absen/kosong sebagai "ikuti harga input".
        ...(cached.trim() ? { cached_input_per_1m_usd: cached.trim() } : {}),
      });
      toast.success(`Harga ${mapping.upstream_model_name} diperbarui dan langsung berlaku.`);
      setInput('');
      setOutput('');
      setCached('');
      await muat();
    } catch (err) {
      setGalatForm(getErrorMessage(err, 'Gagal memasang harga baru.'));
    } finally {
      setMenyimpan(false);
    }
  };

  const inputKelas =
    'w-full px-2.5 py-1.5 text-xs bg-bg-surface-2 border border-border rounded-lg text-white font-mono placeholder:text-text-muted outline-none focus:border-accent';

  return (
    <Drawer
      isOpen={!!mapping}
      onClose={onClose}
      title={
        <span className="inline-flex items-center gap-2">
          <DollarSign className="w-4 h-4 text-accent-text" aria-hidden="true" />
          Harga — {mapping.upstream_model_name}
        </span>
      }
      subtitle={
        provider
          ? `Pemetaan ke ${provider.display_name || provider.name} (${provider.kind}). Harga dalam USD per 1M token.`
          : 'Harga dalam USD per 1M token.'
      }
    >
      {memuat ? (
        <p className="text-xs text-text-secondary py-4">Memuat harga…</p>
      ) : galat ? (
        <QueryError message={galat} onRetry={() => void muat()} />
      ) : (
        <div className="space-y-6">
          {/* Harga efektif — satu-satunya yang menagih request. */}
          <section aria-label="Harga efektif">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-text-secondary">Harga efektif saat ini</h3>
            {efektif ? (
              <div className="mt-1.5">
                <BarisHarga label="Input" nilai={efektif.input_per_1m_usd} />
                <BarisHarga label="Output" nilai={efektif.output_per_1m_usd} />
                <BarisHarga label="Input cache" nilai={efektif.cached_input_per_1m_usd} />
                <BarisHarga label="Reasoning" nilai={efektif.reasoning_per_1m_usd} />
                <p className="text-[11px] text-text-muted mt-1.5">
                  Berlaku sejak {formatTanggal(efektif.effective_from)} · sumber: {efektif.source || 'manual'}
                </p>
              </div>
            ) : (
              <p className="text-xs text-text-secondary mt-1.5">
                Belum ada harga yang berlaku untuk pemetaan ini. Permintaan lewat pemetaan ini tercatat{' '}
                <span className="text-amber-300">tanpa biaya (unpriced)</span> sampai harga dipasang di bawah.
              </p>
            )}
          </section>

          {/* Harga acuan katalog — tawaran awal, BUKAN harga efektif. */}
          <section aria-label="Harga acuan">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-text-secondary">Harga acuan (reference)</h3>
            {acuan?.available ? (
              <div className="mt-1.5">
                <BarisHarga label="Input" nilai={acuan.input_per_1m_usd} />
                <BarisHarga label="Output" nilai={acuan.output_per_1m_usd} />
                <BarisHarga label="Input cache" nilai={acuan.cached_input_per_1m_usd} />
                <p className="text-[11px] text-text-muted mt-1.5">
                  Sumber: {acuan.source || '—'} · diambil {acuan.fetched_on || '—'}. Harga acuan hanya tawaran awal
                  dari katalog bawaan — ia tidak menagih apa pun sampai dipasang lewat formulir di bawah.
                </p>
                <div className="mt-2">
                  <Button variant="secondary" size="sm" onClick={pakaiAcuan}>
                    Pakai harga acuan sebagai isian
                  </Button>
                </div>
              </div>
            ) : (
              <p className="text-xs text-text-secondary mt-1.5">
                Katalog acuan tidak punya harga yang bisa dipertanggungjawabkan untuk{' '}
                {acuan ? `${acuan.provider_kind} / ${acuan.upstream_model}` : 'pemetaan ini'}. Gateway tidak
                mengarang angka pengganti — isi harga secara manual di bawah.
              </p>
            )}
          </section>

          {/* Pasang harga manual — endpoint tulis POST pricing (models:write). */}
          <section aria-label="Pasang harga baru">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-text-secondary">Pasang harga baru</h3>
            <form onSubmit={simpan} className="mt-2 space-y-2.5">
              <div>
                <label htmlFor="harga-input" className="block text-xs text-text-secondary mb-1">
                  Input (USD / 1M token)
                </label>
                <input
                  id="harga-input"
                  type="text"
                  inputMode="decimal"
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  placeholder="mis. 2"
                  className={inputKelas}
                />
              </div>
              <div>
                <label htmlFor="harga-output" className="block text-xs text-text-secondary mb-1">
                  Output (USD / 1M token)
                </label>
                <input
                  id="harga-output"
                  type="text"
                  inputMode="decimal"
                  value={output}
                  onChange={(e) => setOutput(e.target.value)}
                  placeholder="mis. 10"
                  className={inputKelas}
                />
              </div>
              <div>
                <label htmlFor="harga-cached" className="block text-xs text-text-secondary mb-1">
                  Input cache (USD / 1M token, opsional — kosong berarti mengikuti harga input)
                </label>
                <input
                  id="harga-cached"
                  type="text"
                  inputMode="decimal"
                  value={cached}
                  onChange={(e) => setCached(e.target.value)}
                  placeholder="kosongkan untuk mengikuti input"
                  className={inputKelas}
                />
              </div>
              {galatForm && (
                <p role="alert" className="text-xs text-rose-300">
                  {galatForm}
                </p>
              )}
              <Button type="submit" variant="primary" size="sm" isLoading={menyimpan}>
                Pasang harga
              </Button>
              <p className="text-[11px] text-text-muted">
                Harga baru langsung berlaku dan menutup harga sebelumnya; riwayat lama tetap tersimpan di bawah.
              </p>
            </form>
          </section>

          {/* Riwayat harga dari endpoint history. */}
          <section aria-label="Riwayat harga">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-text-secondary">Riwayat harga</h3>
            {riwayat.length > 0 ? (
              <ul className="mt-1.5 divide-y divide-border">
                {riwayat.map((p) => (
                  <li key={p.id} className="py-2">
                    <p className="font-mono text-xs text-text-primary">
                      {formatUSD(p.input_per_1m_usd, 6)} in · {formatUSD(p.output_per_1m_usd, 6)} out
                      {p.cached_input_per_1m_usd ? ` · ${formatUSD(p.cached_input_per_1m_usd, 6)} cache` : ''}
                    </p>
                    <p className="text-[11px] text-text-muted mt-0.5">
                      {formatTanggal(p.effective_from)} → {p.effective_to ? formatTanggal(p.effective_to) : 'masih terbuka'} ·
                      sumber: {p.source || 'manual'}
                    </p>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-xs text-text-secondary mt-1.5">Belum ada riwayat harga untuk pemetaan ini.</p>
            )}
          </section>
        </div>
      )}
    </Drawer>
  );
};
