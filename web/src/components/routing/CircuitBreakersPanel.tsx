import React, { useEffect, useState } from 'react';
import type { CircuitBreakerStatus, Model, Provider, RoutingRule } from '../../types';
import { keadaanMuat, teksHitungKeadaan } from '../../lib/keadaanMuat';
import { Button } from '../common/Button';
import { Badge } from '../common/Badge';
import { QueryError } from '../common/QueryError';
import {
  ZapOff,
  ShieldCheck,
  RefreshCw,
  CheckCircle2,
  RotateCcw,
  AlertTriangle,
  Radio,
} from 'lucide-react';

export interface CircuitBreakersPanelProps {
  breakers: CircuitBreakerStatus[];
  providers: Provider[];
  // Rule + katalog model dari halaman Routing (sudah dimuat di sana) untuk
  // gema konfigurasi efektif per breaker — tanpa endpoint baru (R2 Fase 2).
  rules: RoutingRule[];
  models: Model[];
  isBreakersLoading: boolean;
  breakersError: string | null;
  loadBreakers: () => Promise<void>;
  handleResetBreaker: (providerId: string, model: string) => Promise<void>;
  // B-449: daftar provider (sumber terpisah dari breaker) gagal
  // dimuat halaman induk — chip hitungan provider belum terukur.
  providersGagal?: boolean;
  // B-491: daftar rule/model (sumber terpisah lain) gagal dimuat —
  // label "Bawaan server" di panel ini diturunkan dari rule/model
  // itu, jadi kegagalannya membuat label itu belum terukur, bukan
  // klaim definitif tentang bawaan server.
  rulesGagal?: boolean;
  modelsGagal?: boolean;
}

// Konfigurasi efektif yang ditampilkan untuk satu breaker: rule aktif
// prioritas tertinggi yang cakupannya memuat provider breaker ini dan
// (bila rule membatasi model) model breaker ini. Ini GEMA untuk dibaca
// operator, bukan sumber kebenaran mesin: pada runtime, rule dicocokkan
// per request (model + API key + kapabilitas), jadi label "+n rule lain"
// menandai bila ada rule lain yang juga mencakup pasangan ini.
export interface KonfigurasiEfektif {
  rule: RoutingRule;
  jumlahRuleLain: number;
}

export function konfigurasiEfektif(
  breaker: CircuitBreakerStatus,
  rules: RoutingRule[],
  models: Model[],
): KonfigurasiEfektif | null {
  const mencakup = rules.filter((r) => {
    if (!r.enabled) return false;
    const cakupanProvider = r.providers ?? [];
    if (cakupanProvider.length > 0 && !cakupanProvider.some((p) => p.provider_id === breaker.provider_id)) {
      return false;
    }
    if (r.match_model_id) {
      // Rule khusus model hanya mendefinisikan breaker model itu.
      if (!breaker.model) return false;
      const model = models.find((m) => m.id === r.match_model_id);
      if (!model || model.model_id !== breaker.model) return false;
    }
    return true;
  });
  if (mencakup.length === 0) return null;
  const terurut = [...mencakup].sort((a, b) => b.priority - a.priority);
  return { rule: terurut[0], jumlahRuleLain: terurut.length - 1 };
}

// Format durasi terbuka rule: milidetik → label ringkas yang terbaca.
export function labelDurasiBuka(ms: number): string {
  if (!ms || ms <= 0) return '0 dtk';
  if (ms < 1000) return `${ms} ms`;
  const dtk = Math.round(ms / 1000);
  if (dtk < 60) return `${dtk} dtk`;
  return `${(dtk / 60).toFixed(dtk % 60 === 0 ? 0 : 1)} mnt`;
}

export const CircuitBreakersPanel: React.FC<CircuitBreakersPanelProps> = ({
  breakers,
  providers,
  rules,
  models,
  isBreakersLoading,
  breakersError,
  loadBreakers,
  handleResetBreaker,
  providersGagal = false,
  rulesGagal = false,
  modelsGagal = false,
}) => {
  const totalAbnormalBreakers = breakers.filter((b) => b.state !== 'closed').length;
  // B-312: klaim sehat hanya sah dari data yang benar-benar tiba.
  // Array kosong saat muatan pertama/penyegaran berjalan atau saat
  // fetch gagal berarti "belum diketahui", bukan "semua normal" —
  // daftar kosong dari respons SUKSES tetap sah sebagai sehat.
  const statusDiketahui = !isBreakersLoading && !breakersError;
  const klaimSehat = statusDiketahui && totalAbnormalBreakers === 0;
  // B-555: klaim TERGANGGU pun hanya sah dari data terukur —
  // versi lama menggantungkan lencana "{n} Sirkuit Terganggu"
  // hanya pada hitungan daftar, jadi saat refetch GAGAL (daftar
  // lama dipertahankan halaman induk + breakersError terisi)
  // lencana merah definitif tetap tampil dari data basi, padahal
  // halaman yang sama menampilkan QueryError. Kini cabang itu
  // ikut statusDiketahui seperti klaim sehat B-312.
  const klaimTerganggu = statusDiketahui && totalAbnormalBreakers > 0;
  const tampilNetral = totalAbnormalBreakers === 0 && !statusDiketahui;

  // Jam lokal untuk hitung mundur probe: berdetak tiap detik hanya selama
  // ada breaker yang menunggu probe, agar panel diam saat semua normal.
  const adaProbeMenunggu = breakers.some(
    (b) => b.state !== 'closed' && b.next_probe_at && new Date(b.next_probe_at).getTime() > Date.now(),
  );
  const [sekarang, setSekarang] = useState<number>(() => Date.now());
  useEffect(() => {
    if (!adaProbeMenunggu) return;
    const id = setInterval(() => setSekarang(Date.now()), 1000);
    return () => clearInterval(id);
  }, [adaProbeMenunggu]);

  // Segarkan otomatis tiap 15 detik hanya selama ada breaker terganggu
  // dan tab terlihat — panel tidak mem-polling server saat semua sehat.
  useEffect(() => {
    if (totalAbnormalBreakers === 0) return;
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') {
        void loadBreakers();
      }
    }, 15000);
    return () => clearInterval(id);
  }, [totalAbnormalBreakers, loadBreakers]);

  // Ambang bawaan pada banner sehat: dari rule global prioritas tertinggi
  // (tanpa cakupan provider/model), bukan angka karangan di UI.
  // B-518: angka ini hanya sah dari daftar yang segar — bila muat-ulang
  // rule gagal (rulesGagal), daftar ini BASI dan chip di bawah
  // menampilkannya sebagai netral '—', bukan angka definitif.
  const ruleGlobalTeratas = [...rules]
    .filter((r) => r.enabled && (r.providers ?? []).length === 0 && !r.match_model_id)
    .sort((a, b) => b.priority - a.priority)[0];

  return (
    <div className="pt-6 border-t border-border/40">
      <div className="rounded-box bg-bg-surface-1 border border-border/60 p-4 sm:p-5 shadow-sm space-y-4">
        {/* Header Row: Responsive & Unbroken on Mobile */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div className="flex items-start gap-3">
            <div
              className={`w-10 h-10 rounded-xl flex items-center justify-center flex-shrink-0 mt-0.5 ${
                klaimTerganggu
                  ? 'bg-rose-500/10 text-rose-400 border border-rose-500/30 shadow-sm'
                  : klaimSehat
                    ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 shadow-sm'
                    : 'bg-bg-surface-2 text-text-muted border border-border shadow-sm'
              }`}
            >
              {klaimTerganggu ? (
                <ZapOff className="w-5 h-5" aria-hidden="true" />
              ) : klaimSehat ? (
                <ShieldCheck className="w-5 h-5" aria-hidden="true" />
              ) : isBreakersLoading ? (
                <RefreshCw className="w-5 h-5 animate-spin" aria-hidden="true" />
              ) : (
                <AlertTriangle className="w-5 h-5" aria-hidden="true" />
              )}
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-base font-bold text-white tracking-tight">
                  Pemutus Sirkuit (Circuit Breakers)
                </h3>
                {klaimTerganggu ? (
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-rose-500/15 text-rose-400 border border-rose-500/30">
                    <span className="w-2 h-2 rounded-full bg-rose-400 animate-ping" aria-hidden="true" />
                    {totalAbnormalBreakers} Sirkuit Terganggu
                  </span>
                ) : klaimSehat ? (
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30">
                    <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" aria-hidden="true" />
                    Semua Beroperasi Normal
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-bg-surface-2 text-text-muted border border-border">
                    <span className="w-2 h-2 rounded-full bg-text-muted" aria-hidden="true" />
                    {isBreakersLoading ? 'Memuat Status Sirkuit…' : 'Status Sirkuit Tidak Tersedia'}
                  </span>
                )}
              </div>
              <p className="text-xs text-text-secondary mt-0.5 leading-relaxed">
                Proteksi isolasi otomatis yang mengkarantina provider saat terjadi lonjakan kegagalan dan failover darurat.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 self-end sm:self-auto flex-shrink-0">
            <Button
              variant="secondary"
              size="sm"
              onClick={loadBreakers}
              isLoading={isBreakersLoading}
              className="min-h-[38px] sm:min-h-[34px] px-3 text-xs"
              aria-label="Segarkan status circuit breakers"
            >
              <RefreshCw className={`w-3.5 h-3.5 mr-1.5 ${isBreakersLoading ? 'animate-spin' : ''}`} aria-hidden="true" />
              Segarkan
            </Button>
          </div>
        </div>

        {breakersError && <QueryError message={breakersError} onRetry={() => void loadBreakers()} />}

        {/* Body: Unknown / Healthy State vs Tripped State */}
        {tampilNetral ? (
          <div className="p-4 sm:p-5 rounded-xl bg-bg-surface-2/60 border border-border/40">
            <h4 className="text-xs font-bold text-white">
              {isBreakersLoading ? 'Memuat status sirkuit…' : 'Status sirkuit tidak tersedia'}
            </h4>
            <p className="text-xs text-text-secondary mt-0.5 leading-relaxed max-w-xl">
              {isBreakersLoading
                ? 'Status breaker sedang dimuat; belum ada keadaan yang bisa dinyatakan.'
                : 'Status breaker gagal dimuat; keadaan sirkuit belum bisa dinyatakan normal maupun terganggu.'}
            </p>
          </div>
        ) : breakers.length === 0 || totalAbnormalBreakers === 0 ? (
          <div className="p-4 sm:p-5 rounded-xl bg-bg-surface-2/60 border border-border/40 flex flex-col md:flex-row items-start md:items-center justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-lg bg-emerald-500/10 text-emerald-400 flex-shrink-0 mt-0.5">
                <CheckCircle2 className="w-4 h-4" aria-hidden="true" />
              </div>
              <div>
                <h4 className="text-xs font-bold text-white">
                  Seluruh Sirkuit Provider Normal (Closed)
                </h4>
                <p className="text-xs text-text-secondary mt-0.5 leading-relaxed max-w-xl">
                  Proteksi isolasi otomatis aktif. Tidak ada upstream yang mengalami lonjakan kegagalan beruntun atau terisolasi dari perutean model.
                </p>
              </div>
            </div>

            {/* 3 Telemetry Chips */}
            <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 w-full md:w-auto flex-shrink-0 pt-2 md:pt-0 border-t md:border-t-0 border-border/40">
              <div className="px-3 py-2 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Provider Dipantau</span>
                <span className="text-xs font-bold font-mono text-white">
                  {teksHitungKeadaan(
                    keadaanMuat({ memuat: false, gagal: providersGagal, jumlah: providers.length }),
                    providers.length,
                  )}{' '}
                  Terhubung
                </span>
              </div>
              <div className="px-3 py-2 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Sirkuit Terputus</span>
                <span className="text-xs font-bold font-mono text-emerald-400">
                  0 (Aman)
                </span>
              </div>
              <div className="col-span-2 sm:col-span-1 px-3 py-2 rounded-lg bg-bg-surface-1 border border-border/60 text-center">
                <span className="text-[10px] text-text-muted block font-sans">Ambang Isolasi</span>
                <span className="text-xs font-bold font-mono text-sky-400">
                  {ruleGlobalTeratas && !rulesGagal
                    ? `${ruleGlobalTeratas.failure_threshold}x Gagal`
                    : rulesGagal || modelsGagal
                      ? '—'
                      : 'Bawaan Server'}
                </span>
              </div>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 pt-1">
            {breakers.map((b, idx) => {
              const provObj = providers.find((p) => p.id === b.provider_id);
              const providerName = provObj?.display_name || provObj?.name || b.provider_id;
              const isTrip = b.state === 'open';
              const isHalf = b.state === 'half-open';
              const konf = konfigurasiEfektif(b, rules, models);
              // B-511: bila daftar model GAGAL dimuat, rule
              // khusus-model yang cakupannya memuat breaker ini
              // tidak dapat dinilai sama sekali — rule generik
              // yang terpilih bisa jadi bukan pemenang
              // sesungguhnya (dengan daftar model terukur, rule
              // khusus-model berprioritas lebih tinggi akan
              // menang). Labelnya karena itu "belum terukur",
              // bukan angka definitif dari rule yang salah.
              // B-518: rulesGagal ikut serta TANPA syarat — konf
              // diturunkan dari daftar rule itu sendiri, jadi
              // daftar rule BASI (muat-ulang gagal dan daftar
              // sukses sebelumnya bertahan di state) membuat
              // setiap konf meragukan; gerbang B-511 lama hanya
              // menutup cabang daftar kosong, bukan cabang basi.
              const konfMeragukan =
                konf != null &&
                (rulesGagal ||
                  (modelsGagal &&
                    rules.some((r) => {
                      if (!r.enabled || !r.match_model_id) return false;
                      const cakupan = r.providers ?? [];
                      return (
                        cakupan.length === 0 ||
                        cakupan.some((p) => p.provider_id === b.provider_id)
                      );
                    })));
              const sisaProbeDtk = b.next_probe_at
                ? Math.max(0, Math.round((new Date(b.next_probe_at).getTime() - sekarang) / 1000))
                : null;

              return (
                <div
                  key={idx}
                  className={`p-4 rounded-xl border flex flex-col justify-between transition-colors shadow-xs ${
                    isTrip
                      ? 'bg-rose-500/5 border-rose-500/30'
                      : isHalf
                      ? 'bg-amber-500/5 border-amber-500/30'
                      : 'bg-bg-surface-2 border-border/60'
                  }`}
                  data-testid={`breaker-${b.provider_id}-${b.model || 'all'}`}
                >
                  <div>
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <span className="text-[10px] text-text-muted font-sans uppercase tracking-wider block font-semibold">
                          Provider Upstream
                        </span>
                        <h4 className="text-xs font-bold text-white truncate">
                          {providerName}
                        </h4>
                        <span className="text-[11px] font-mono text-sky-300 block truncate font-medium">
                          {b.model || 'Semua Model Provider'}
                        </span>
                      </div>
                      <Badge variant={isTrip ? 'error' : isHalf ? 'warn' : 'success'}>
                        {isTrip ? 'Terisolasi (Open)' : isHalf ? 'Uji Coba (Half-Open)' : 'Normal'}
                      </Badge>
                    </div>

                    <div className="mt-3 p-2.5 rounded-lg bg-bg-surface-1/90 border border-border/50 text-xs space-y-1.5 font-mono">
                      <div className="flex justify-between items-center text-text-muted text-[11px]">
                        <span className="flex items-center gap-1">
                          <AlertTriangle className="w-3 h-3 text-rose-400" aria-hidden="true" />
                          Kegagalan Konsekutif:
                        </span>
                        <span className={`font-bold px-1.5 py-0.2 rounded ${isTrip ? 'bg-rose-500/20 text-rose-300' : 'text-white'}`}>
                          {b.failure_count ?? 0}x
                        </span>
                      </div>
                      {b.next_probe_at && (
                        <div className="flex justify-between items-center text-text-muted text-[11px]">
                          <span className="flex items-center gap-1">
                            <Radio className="w-3 h-3 text-amber-400" aria-hidden="true" />
                            Jadwal Uji Probe:
                          </span>
                          <span className="text-amber-300 font-semibold">
                            {new Date(b.next_probe_at).toLocaleTimeString('id-ID', {
                              hour: '2-digit',
                              minute: '2-digit',
                              second: '2-digit',
                            })}
                            {sisaProbeDtk !== null && (
                              <span className="text-text-muted font-normal">
                                {' '}
                                ({sisaProbeDtk > 0 ? `${sisaProbeDtk} dtk lagi` : 'segera'})
                              </span>
                            )}
                          </span>
                        </div>
                      )}
                      <div className="flex justify-between items-center text-text-muted text-[11px]">
                        <span className="flex items-center gap-1">
                          <ShieldCheck className="w-3 h-3 text-sky-400" aria-hidden="true" />
                          Konfigurasi Efektif:
                        </span>
                        <span className="text-white font-semibold text-right">
                          {konf && !konfMeragukan
                            ? `Ambang ${konf.rule.failure_threshold}x · jeda ${labelDurasiBuka(konf.rule.open_duration_ms)} · probe ${konf.rule.half_open_probes}x`
                            : rulesGagal || modelsGagal
                              ? 'Belum terukur (daftar rule/model gagal dimuat)'
                              : 'Bawaan server'}
                        </span>
                      </div>
                      {konf && !konfMeragukan && (
                        <div className="text-[10px] text-text-muted text-right">
                          dari rule &quot;{konf.rule.name}&quot;
                          {konf.jumlahRuleLain > 0 && ` · +${konf.jumlahRuleLain} rule lain juga mencakup`}
                        </div>
                      )}
                    </div>
                  </div>

                  {b.state !== 'closed' && (
                    <div className="mt-4 pt-3 border-t border-border/40">
                      <Button
                        variant="secondary"
                        size="sm"
                        className="w-full text-xs min-h-[38px] border-border/70 hover:border-accent text-white font-semibold"
                        onClick={() => handleResetBreaker(b.provider_id, b.model || '')}
                        aria-label={`Pulihkan sirkuit untuk provider ${providerName}`}
                      >
                        <RotateCcw className="w-3.5 h-3.5 mr-1.5 text-accent-text" aria-hidden="true" />
                        Pulihkan Sirkuit Sekarang
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};
