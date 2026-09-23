import type { RoutingRule } from '../types';

// Pustaka murni untuk resep combo pipeline routing.
//
// Kontrak backend (internal/router/rules.go ComboPipelineDTO + admin handler):
//   - pipeline null = Model Only (jalur lama, passthrough 1:1)
//   - pipeline terisi = Combo Model: strategi + anggaran attempts + daftar model
//   - 4 strategi UI: priority, round_robin, lowest_latency, lowest_cost
//   - attempts = ANGGARAN TOTAL lintas seluruh model (bukan per model), rentang 1-20
//   - models: 1-8 elemen, tanpa duplikat, tanpa elemen kosong/whitespace
//
// Validasi di sini HARUS identik dengan backend agar input ditolak di UI, bukan
// memancing error 500 dari constraint database.

export type StrategiCombo = 'priority' | 'round_robin' | 'lowest_latency' | 'lowest_cost';

export const STRATEGI_COMBO: StrategiCombo[] = [
  'priority',
  'round_robin',
  'lowest_latency',
  'lowest_cost',
];

export const ATTEMPTS_MIN = 1;
export const ATTEMPTS_MAX = 20;
export const MODELS_MIN = 1;
export const MODELS_MAX = 8;

export interface ComboPipeline {
  strategy: StrategiCombo;
  attempts: number;
  models: string[];
}

export type ModeAturan = 'model_only' | 'combo';

// Tipe parameter modeAturan: pipeline adalah sumber kebenaran, description cadangan
// untuk aturan produksi belum dikonversi, max_attempts disertakan hanya karena
// pemanggil (RoutingRules) meneruskan rule utuh — TIDAK dipakai untuk menentukan mode.
export type InputModeAturan = Partial<
  Pick<RoutingRule, 'pipeline' | 'description' | 'max_attempts'>
>;

// LABEL_STRATEGI memetakan kode strategi ke label dan ringkasan Bahasa Indonesia
// untuk ditampilkan di UI (dropdown ComboBuilder + badge tabel aturan).
export const LABEL_STRATEGI: Record<StrategiCombo, { label: string; ringkas: string }> = {
  priority: {
    label: 'Prioritas',
    ringkas: 'Urutkan kandidat gabungan per prioritas provider.',
  },
  round_robin: {
    label: 'Round Robin',
    ringkas: 'Bagi beban merata ke seluruh kandidat yang sehat.',
  },
  lowest_latency: {
    label: 'Latensi Terendah',
    ringkas: 'Utamakan kandidat dengan latensi terukur terendah.',
  },
  lowest_cost: {
    label: 'Biaya Terendah',
    ringkas: 'Utamakan kandidat dengan biaya per token terendah.',
  },
};

// pipelineKosong mengembalikan resek default untuk ComboBuilder baru: satu model,
// anggaran minimum, strategi prioritas. models sengaja kosong agar UI memaksa
// operator memilih tier pertama sebelum menyimpan (validasi menolak models < 1).
export function pipelineKosong(): ComboPipeline {
  return { strategy: 'priority', attempts: ATTEMPTS_MIN, models: [] };
}

// parsePipeline dari field jsonb aturan (kolom pipeline migrasi 0014). Mengembalikan
// null bila aturan adalah Model Only atau jsonb cacat — tidak pernah melempar, agar
// tabel aturan tetap merender saat ada data produksi yang bentuknya tidak terduga.
export function parsePipeline(rule: {
  pipeline?: ComboPipeline | null;
}): ComboPipeline | null {
  const p = rule?.pipeline;
  if (!p || typeof p !== 'object') return null;
  if (!Array.isArray(p.models) || typeof p.attempts !== 'number' || typeof p.strategy !== 'string') {
    return null;
  }
  return p as ComboPipeline;
}

// parsePipelineDariTag membaca resep combo dari tag [combo:pipeline=...] di description.
// Dipakai untuk aturan produksi lama (mis. raute-x) yang belum dikonversi ke kolom
// jsonb — PR #5 memakainya agar operator tetap melihat rule lama sebagai combo.
//
// Format tag lama adalah array tier: [{"tier":2,"model":"x","providers":[...]}].
// Tier 1 TIDAK ada di tag (hanya match_model_id), jadi model utama digabung dari
// parameter terpisah. Mengembalikan null bila tidak ada tag.
export function parsePipelineDariTag(
  description: string | undefined | null,
  modelUtama?: string | null,
): ComboPipeline | null {
  const desc = description || '';
  const cocok = desc.match(/\[combo:pipeline=(\[.*?\])\]/);
  if (!cocok) return null;

  let tier: Array<{ tier: number; model: string }>;
  try {
    tier = JSON.parse(cocok[1]);
  } catch {
    return null;
  }
  if (!Array.isArray(tier)) return null;

  const models = [
    ...(modelUtama ? [modelUtama] : []),
    ...tier.map((t) => t?.model).filter((m): m is string => typeof m === 'string' && m !== ''),
  ];
  if (models.length === 0) return null;

  return { strategy: 'priority', attempts: models.length * 2, models };
}

// modeAturan menentukan mode dari pipeline jsonb, BUKAN dari max_attempts. Memakai
// max_attempts adalah bug K2: rule combo dengan anggaran 1 salah dibaca model_only,
// lalu simpan menghapus provider_ids/weights. Sumber kebenaran adalah kolom pipeline;
// tag description hanya cadangan untuk aturan produksi yang belum dikonversi.
export function modeAturan(rule: InputModeAturan): ModeAturan {
  if (parsePipeline(rule)) return 'combo';
  if (rule?.description?.includes('[combo:pipeline=')) return 'combo';
  return 'model_only';
}

// normalisasiModels membersihkan daftar model: trim, buang elemen kosong, hapus
// duplikat dengan mempertahankan kemunculan pertama. Dipakai sebelum validasi dan
// sebelum membangun payload.
export function normalisasiModels(models: string[] | undefined | null): string[] {
  if (!Array.isArray(models)) return [];
  const terlihat = new Set<string>();
  const hasil: string[] = [];
  for (const m of models) {
    if (typeof m !== 'string') continue;
    const bersih = m.trim();
    if (bersih === '' || terlihat.has(bersih)) continue;
    terlihat.add(bersih);
    hasil.push(bersih);
  }
  return hasil;
}

// validasiPipeline memeriksa resep terhadap constraint backend. Mengembalikan daftar
// pesan error (kosong = sah) — tidak melempar — supaya UI bisa menampilkan error
// per field tanpa try/catch di setiap pemanggilan.
export function validasiPipeline(p: ComboPipeline | null | undefined): string[] {
  if (!p) return ['Resep pipeline tidak boleh kosong.'];
  const errors: string[] = [];

  if (!STRATEGI_COMBO.includes(p.strategy)) {
    errors.push(`Strategi "${p.strategy}" tidak dikenal. Pilih salah satu: ${STRATEGI_COMBO.join(', ')}.`);
  }
  if (!Number.isInteger(p.attempts) || p.attempts < ATTEMPTS_MIN || p.attempts > ATTEMPTS_MAX) {
    errors.push(`Anggaran attempts harus bilangan bulat ${ATTEMPTS_MIN}-${ATTEMPTS_MAX}, dapat ${p.attempts}.`);
  }

  const models = normalisasiModels(p.models);
  if (models.length < MODELS_MIN) {
    errors.push(`Daftar model minimal ${MODELS_MIN} model (saat ini ${models.length}).`);
  }
  if (models.length > MODELS_MAX) {
    errors.push(`Daftar model maksimal ${MODELS_MAX} model (saat ini ${models.length}).`);
  }
  // Elemen kosong/duplikat sudah dibuang normalisasi; laporkan bila input asli
  // mengandunginya supaya operator tahu mengapa modelnya hilang saat disimpan.
  if (Array.isArray(p.models)) {
    const mentah = p.models.filter((m) => typeof m === 'string' && m.trim() === '');
    if (mentah.length > 0) {
      errors.push('Daftar model mengandung elemen kosong — elemen tersebut akan dibuang.');
    }
    const unik = new Set(p.models.filter((m): m is string => typeof m === 'string').map((m) => m.trim()));
    if (unik.size !== p.models.filter((m) => typeof m === 'string').length) {
      errors.push('Daftar model mengandung duplikat — duplikat akan dibuang.');
    }
  }
  return errors;
}

// validasiAlias memeriksa prasyarat "alias wajib punya pipeline" (backend menolak
// alias yatim dengan 400). Alias sendiri opsional — aturan combo tanpa virtual
// endpoint tetap sah.
export function validasiAlias(
  alias: string | undefined | null,
  pipeline: ComboPipeline | null | undefined,
): string | null {
  const aliasBersih = (alias || '').trim();
  if (aliasBersih === '') return null;
  if (!pipeline || validasiPipeline(pipeline).length > 0) {
    return 'Virtual endpoint (alias) hanya boleh dipasang pada aturan combo dengan resep pipeline sah.';
  }
  return null;
}

// bangunPipeline menghasilkan objek siap kirim ke POST/PUT setelah normalisasi.
// Model kosong/duplikat dibuang di sini agar payload selalu bersih; validasi
// panggilan terpisah bila ingin memblokir penyimpanan.
export function bangunPipeline(p: ComboPipeline): ComboPipeline {
  return {
    strategy: p.strategy,
    attempts: p.attempts,
    models: normalisasiModels(p.models),
  };
}

// bangunPayloadUpdate menyusun payload update yang MEMPERTAHANKAN field tak diedit.
// Ini perbaikan inti bug K2: handler lama menimpa seluruh payload per mode, sehingga
// operator yang hanya mengganti nama rule combo kehilangan provider_ids/weights.
// Field yang tidak disebut di edits tetap diambil dari existing.
export function bangunPayloadUpdate<T extends Record<string, unknown>>(
  existing: T,
  edits: Partial<T>,
): Partial<T> {
  return { ...existing, ...edits };
}
