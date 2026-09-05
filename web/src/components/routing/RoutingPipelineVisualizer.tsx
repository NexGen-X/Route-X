import React, { useState } from 'react';
import {
  Terminal,
  ShieldAlert,
  Coins,
  GitFork,
  Database,
  Server,
  Zap,
  CheckCircle2,
} from 'lucide-react';

export const RoutingPipelineVisualizer: React.FC = () => {
  const [selectedStage, setSelectedStage] = useState<number>(4); // Default ke Routing Decision Engine

  const stages = [
    {
      id: 1,
      title: 'Inbound Request',
      subtitle: 'Klien Terminal / SDK',
      icon: Terminal,
      badge: 'HTTP POST /v1',
      description:
        'Request datang dari perkakas AI terminal (Antigravity, Claude CLI, OpenCode, Cursor, Aider, dll.) membawa API Key Bearer token.',
      details: [
        'Endpoint standar: /v1/chat/completions',
        'Streaming token SSE didukung tanpa buffer (flush_interval -1)',
        'Mendukung otentikasi dual: Master Key & Per-Tool Sub-Key',
      ],
    },
    {
      id: 2,
      title: 'Security & PII Guard',
      subtitle: 'Inspeksi Konten & SSRF',
      icon: ShieldAlert,
      badge: 'AES-256 & Guard',
      description:
        'Lapisan penyaringan konten dan perlindungan SSRF. Mencegah kebocoran PII (email, API key rahasia) dan memblokir injeksi berbahaya sebelum keluar jaringan.',
      details: [
        'Enkripsi kredensial AES-256-GCM dengan AAD terikat',
        'SSRF Policy: Menolak alamat private IP loopback yang tidak diizinkan',
        'Deteksi dan sensor otomatis data sensitif (PII Redaction)',
      ],
    },
    {
      id: 3,
      title: 'Quota & Limits',
      subtitle: 'Rate Limit & Anggaran',
      icon: Coins,
      badge: 'RPM / TPM & USD',
      description:
        'Pengecekan batas laju (Rate Limit) dan plafon anggaran keuangan (Budget USD) dengan presisi 8 desimal (1e-8 USD).',
      details: [
        'Presisi moneter murni integer upstream.USD (bebas floating error)',
        'Plafon harian/bulanan dengan pemblokiran instan saat kuota habis',
        'Algoritma Token Bucket terdistribusi via Redis 7',
      ],
    },
    {
      id: 4,
      title: 'Routing Decision Engine',
      subtitle: 'Model Only / Smart / Combo',
      icon: GitFork,
      badge: '3-Mode Core',
      description:
        'Jantung pemrosesan Route-X: menentukan jalur eksekusi model terbaik berdasarkan mode aktif yang Anda konfigurasikan.',
      details: [
        'Mode 1 (Model Only): Passthrough 1:1 langsung ke model pilihan tanpa intervensi.',
        'Mode 2 (Smart Routing): Prioritas latensi, cost optimization, & weighted load balancing.',
        'Mode 3 (Combo Routing): Otomatis failover dari Tier 1 (Primary) ke Tier 2 (Fallback Flagship).',
      ],
    },
    {
      id: 5,
      title: 'Response Cache Engine',
      subtitle: 'Akselerasi In-Memory',
      icon: Database,
      badge: 'Redis Sub-5ms',
      description:
        'Penyimpanan jawaban deterministik di Redis. Jika prompt yang sama pernah dieksekusi, jawaban dikembalikan seketika dengan latensi sub-5 milidetik dan biaya $0.',
      details: [
        'Hashing deterministik SHA-256 prompt masukan & model params',
        'Penghematan biaya token 100% pada prompt berulang (Cache Hit)',
        'TTL dinamis yang dapat di-flush kapan saja dari dashboard',
      ],
    },
    {
      id: 6,
      title: 'Upstream Provider Execution',
      subtitle: 'Anthropic / OpenAI / Google',
      icon: Server,
      badge: 'Circuit Breaker',
      description:
        'Permintaan diteruskan ke provider upstream resmi atau server lokal (Ollama). Dilengkapi circuit breaker cerdas untuk mengisolasi kegagalan provider.',
      details: [
        'Otomatis retry dengan exponential backoff jika rate-limit 429',
        'Circuit breaker auto-trip jika provider mengalami lonjakan error 5xx',
        'Pencatatan akuntansi token dan latensi real-time ke database',
      ],
    },
  ];

  const current = stages.find((s) => s.id === selectedStage) || stages[3];
  const CurrentIcon = current.icon;

  return (
    <div className="p-5 bg-bg-surface-1 border border-border rounded-2xl space-y-5">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-3 border-b border-border">
        <div>
          <h3 className="text-sm font-bold text-white flex items-center gap-2">
            <Zap className="w-4 h-4 text-accent" />
            Visualisasi Alur Eksekusi Inferensi (Pipeline Canvas)
          </h3>
          <p className="text-[11px] text-text-secondary mt-0.5">
            Jalur navigasi setiap permintaan inferensi dari perkakas terminal hingga ke provider AI upstream.
          </p>
        </div>
        <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-accent/15 text-accent border border-accent/30 self-start sm:self-auto font-semibold">
          LIVE ENGINE FLOW
        </span>
      </div>

      {/* Interactive Horizontal Pipeline Nodes */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-2">
        {stages.map((stage) => {
          const Icon = stage.icon;
          const isSelected = stage.id === selectedStage;
          return (
            <button
              key={stage.id}
              onClick={() => setSelectedStage(stage.id)}
              className={`p-3 rounded-xl text-left transition-all border flex flex-col justify-between min-h-[90px] relative overflow-hidden ${
                isSelected
                  ? 'bg-accent/15 border-accent shadow-md shadow-accent/10'
                  : 'bg-bg-surface-2/60 border-border hover:border-accent/40 hover:bg-bg-surface-2'
              }`}
            >
              <div className="flex items-center justify-between w-full">
                <div
                  className={`w-7 h-7 rounded-lg flex items-center justify-center ${
                    isSelected ? 'bg-accent text-black font-bold' : 'bg-bg-surface-1 text-accent'
                  }`}
                >
                  <Icon className="w-3.5 h-3.5" />
                </div>
                <span className="text-[9px] font-mono text-text-muted">0{stage.id}</span>
              </div>

              <div className="mt-2">
                <h4 className="text-[11px] font-bold text-white truncate">{stage.title}</h4>
                <span className="text-[10px] text-text-muted block truncate">{stage.subtitle}</span>
              </div>

              {isSelected && (
                <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-accent" />
              )}
            </button>
          );
        })}
      </div>

      {/* Stage Detail Card */}
      <div className="p-4 bg-bg-surface-2/80 rounded-xl border border-border flex flex-col sm:flex-row items-start gap-4">
        <div className="w-10 h-10 rounded-xl bg-accent/20 text-accent flex items-center justify-center shrink-0">
          <CurrentIcon className="w-5 h-5" />
        </div>

        <div className="flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs font-bold text-white">
              Tahap 0{current.id}: {current.title}
            </span>
            <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-bg-surface-1 text-accent border border-border">
              {current.badge}
            </span>
          </div>

          <p className="text-xs text-text-secondary leading-relaxed">{current.description}</p>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 pt-2">
            {current.details.map((detail, i) => (
              <div
                key={i}
                className="flex items-start gap-2 p-2 rounded-lg bg-bg-surface-1 border border-border text-[11px] text-text-muted"
              >
                <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400 shrink-0 mt-0.5" />
                <span>{detail}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
