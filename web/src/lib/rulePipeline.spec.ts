import { describe, it, expect } from 'vitest';
import {
  parsePipeline,
  parsePipelineDariTag,
  modeAturan,
  normalisasiModels,
  validasiPipeline,
  validasiAlias,
  bangunPipeline,
  bangunPayloadUpdate,
  pipelineKosong,
  ATTEMPTS_MIN,
  ATTEMPTS_MAX,
  MODELS_MAX,
  LABEL_STRATEGI,
  type ComboPipeline,
} from './rulePipeline';

const pipelineSah: ComboPipeline = {
  strategy: 'round_robin',
  attempts: 3,
  models: ['gpt-5', 'gemini-2-5-pro'],
};

describe('parsePipeline', () => {
  it('mengembalikan pipeline sah dari kolom jsonb', () => {
    expect(parsePipeline({ pipeline: pipelineSah })).toEqual(pipelineSah);
  });

  it('null untuk aturan Model Only (pipeline tidak ada)', () => {
    expect(parsePipeline({ pipeline: null })).toBeNull();
    expect(parsePipeline({})).toBeNull();
  });

  it('null untuk jsonb cacat, bukan melempar (tabel tetap merender)', () => {
    // Cast ke ComboPipeline karena kita sengaja menguji guard runtime terhadap data
    // yang tidak sesuai skema — TypeScript sah menolak literal ini.
    const cacat = (p: unknown) => parsePipeline({ pipeline: p as ComboPipeline | null });
    expect(cacat({ strategy: 'priority' })).toBeNull();
    expect(cacat({ strategy: 'priority', attempts: 'bukan-angka', models: [] })).toBeNull();
    expect(cacat({ strategy: 'priority', attempts: 2, models: 'x' })).toBeNull();
  });
});

describe('parsePipelineDariTag (kompatibilitas raute-x)', () => {
  it('membaca resep dari tag description lama, tier-1 dari match_model_id', () => {
    // Format produksi: pipeTag = `[combo:pipeline=${JSON.stringify(tiers)}]`,
    // yaitu array JSON dibungkus kurung siku — ada `]` penutup ganda di akhir.
    const p = parsePipelineDariTag(
      '[combo:alias=murah-cerdas] [combo:tier2=gemini-2-5-pro] [combo:pipeline=[{"tier":2,"model":"gemini-2-5-pro"}]] cascade',
      'gpt-5',
    );
    expect(p).toEqual({ strategy: 'priority', attempts: 4, models: ['gpt-5', 'gemini-2-5-pro'] });
  });

  it('membaca resep dari tag produksi yang TIDAK ditutup ] (regresi raute-x)', () => {
    // Aturan raute-x sungguhan di produksi: tag pipeline berakhir "}]" lalu
    // langsung diikuti teks biasa — tidak ada "]" penutup tag. Regex lama
    // (/\[combo:pipeline=(\[.*?\])\]/) meminta "]" penutup, jadi gagal membuang
    // seluruh JSON dan parsePipelineDariTag mengembalikan null. Akibatnya kartu
    // menampilkan combo tapi rantai kosong, dan edit membuka resep kosong.
    const p = parsePipelineDariTag(
      '[combo:alias=raute-x] [combo:tier2=gemini-2.5-flash] [combo:pipeline=[{"tier":2,"model":"gemini-2.5-flash","providers":["70cdcf7a","b632034b"]}] Smart Tiered Cascade: hemat -> flagship',
      'gpt-5',
    );
    expect(p).toEqual({
      strategy: 'priority',
      attempts: 4,
      models: ['gpt-5', 'gemini-2.5-flash'],
    });
  });

  it('null bila tidak ada tag pipeline', () => {
    expect(parsePipelineDariTag('[model_only] passthrough langsung', 'gpt-5')).toBeNull();
    expect(parsePipelineDariTag(undefined, 'gpt-5')).toBeNull();
  });

  it('null bila json tag cacat', () => {
    expect(parsePipelineDariTag('[combo:pipeline=[tidak-valid]] x', 'gpt-5')).toBeNull();
  });

  it('null bila tidak ada model sama sekali (tag tanpa tier dan tanpa model utama)', () => {
    expect(parsePipelineDariTag('[combo:pipeline=[]] x')).toBeNull();
  });
});

describe('modeAturan (perbaikan bug K2: sumber kebenaran adalah pipeline)', () => {
  it('combo bila pipeline jsonb terisi', () => {
    expect(modeAturan({ pipeline: pipelineSah, max_attempts: 1 })).toBe('combo');
  });

  // REGRESI K2: rule combo dengan anggaran 1 pernah salah dibaca model_only oleh
  // getRuleMode lama (memakai max_attempts===1), lalu simpan menghapus provider_ids.
  it('combo meskipun anggaran attempts 1 (bukan diturunkan dari max_attempts)', () => {
    expect(modeAturan({ pipeline: { ...pipelineSah, attempts: 1 }, max_attempts: 1 })).toBe('combo');
  });

  it('combo bila hanya ada tag description lama (aturan produksi belum dikonversi)', () => {
    expect(modeAturan({ description: '[combo:pipeline=[{"tier":2,"model":"x"}]]' })).toBe('combo');
  });
  it('model_only bila pipeline null dan tidak ada tag', () => {
    expect(modeAturan({ max_attempts: 1 })).toBe('model_only');
    expect(modeAturan({ max_attempts: 5 })).toBe('model_only');
  });
});

describe('normalisasiModels', () => {
  it('trim, buang kosong, hapus duplikat pertama-menang', () => {
    expect(normalisasiModels([' gpt-5 ', '', 'gemini-2-5-pro', 'gpt-5', '  '])).toEqual([
      'gpt-5',
      'gemini-2-5-pro',
    ]);
  });
  it('array non-array menjadi kosong', () => {
    expect(normalisasiModels(null)).toEqual([]);
    expect(normalisasiModels(undefined)).toEqual([]);
  });
});

describe('validasiPipeline (identik constraint backend)', () => {
  it('sah untuk resek batas bawah (attempts 1, models 1)', () => {
    expect(validasiPipeline({ strategy: 'priority', attempts: ATTEMPTS_MIN, models: ['gpt-5'] })).toEqual([]);
  });
  it('sah untuk batas atas (attempts 20, models 8)', () => {
    expect(
      validasiPipeline({
        strategy: 'round_robin',
        attempts: ATTEMPTS_MAX,
        models: Array.from({ length: MODELS_MAX }, (_, i) => `model-${i}`),
      }),
    ).toEqual([]);
  });
  it('menolak attempts 0 dan 21 (rentang 1-20)', () => {
    expect(validasiPipeline({ ...pipelineSah, attempts: 0 }).some((e) => e.includes('1-20'))).toBe(true);
    expect(validasiPipeline({ ...pipelineSah, attempts: 21 }).some((e) => e.includes('1-20'))).toBe(true);
  });
  it('menolak attempts bukan bilangan bulat', () => {
    expect(validasiPipeline({ ...pipelineSah, attempts: 2.5 }).length).toBeGreaterThan(0);
  });
  it('menolak lebih dari 8 model', () => {
    const models = Array.from({ length: 9 }, (_, i) => `model-${i}`);
    expect(validasiPipeline({ ...pipelineSah, models }).some((e) => e.includes('maksimal'))).toBe(true);
  });
  it('menolak daftar model kosong (minimal 1)', () => {
    expect(validasiPipeline({ ...pipelineSah, models: [] }).some((e) => e.includes('minimal'))).toBe(true);
  });
  it('melaporkan elemen kosong dan duplikat (dibuang normalisasi)', () => {
    const errors = validasiPipeline({ ...pipelineSah, models: ['gpt-5', '', 'gpt-5', 'gemini-2-5-pro'] });
    expect(errors.some((e) => e.includes('elemen kosong'))).toBe(true);
    expect(errors.some((e) => e.includes('duplikat'))).toBe(true);
  });
  it('menolak strategi tidak dikenal termasuk weighted/capability (valid di enum Go, bukan UI)', () => {
    expect(validasiPipeline({ ...pipelineSah, strategy: 'weighted' as ComboPipeline['strategy'] }).some((e) => e.includes('tidak dikenal'))).toBe(true);
    expect(validasiPipeline({ ...pipelineSah, strategy: 'capability' as ComboPipeline['strategy'] }).some((e) => e.includes('tidak dikenal'))).toBe(true);
  });
  it('menolak pipeline null/undefined', () => {
    expect(validasiPipeline(null).length).toBeGreaterThan(0);
    expect(validasiPipeline(undefined).length).toBeGreaterThan(0);
  });
});

describe('validasiAlias (mencegah alias yatim)', () => {
  it('alias dengan pipeline sah lolos', () => {
    expect(validasiAlias('murah-cerdas', pipelineSah)).toBeNull();
  });
  it('alias tanpa pipeline ditolak (backend 400 alias_tanpa_pipeline)', () => {
    expect(validasiAlias('murah-cerdas', null)).not.toBeNull();
    expect(validasiAlias('murah-cerdas', undefined)).not.toBeNull();
  });
  it('alias dengan pipeline cacat ditolak', () => {
    expect(validasiAlias('murah-cerdas', { ...pipelineSah, attempts: 99 })).not.toBeNull();
  });
  it('alias kosong/whitespace selalu lolos (alias opsional)', () => {
    expect(validasiAlias('', pipelineSah)).toBeNull();
    expect(validasiAlias('   ', null)).toBeNull();
    expect(validasiAlias(undefined, null)).toBeNull();
  });
});

describe('bangunPipeline', () => {
  it('menormalisasi model di payload', () => {
    expect(bangunPipeline({ ...pipelineSah, models: [' gpt-5 ', 'gpt-5', 'gemini-2-5-pro', ''] }).models).toEqual([
      'gpt-5',
      'gemini-2-5-pro',
    ]);
  });
});

describe('bangunPayloadUpdate (perbaikan bug K2 data-loss)', () => {
  // REGRESI K2: handler lama menimpa payload per mode, sehingga operator yang hanya
  // mengganti nama rule combo kehilangan provider_ids/weights/pipeline.
  it('mempertahankan provider_ids, weights, dan pipeline saat hanya nama diubah', () => {
    const existing = {
      name: 'combo-lama',
      strategy: 'priority',
      provider_ids: ['prov-a', 'prov-b'],
      weights: { 'prov-a': 7, 'prov-b': 3 },
      pipeline: pipelineSah,
      virtual_alias: 'murah-cerdas',
    };
    const payload = bangunPayloadUpdate(existing, { name: 'combo-baru' });
    expect(payload.name).toBe('combo-baru');
    expect(payload.provider_ids).toEqual(['prov-a', 'prov-b']);
    expect(payload.weights).toEqual({ 'prov-a': 7, 'prov-b': 3 });
    expect(payload.pipeline).toEqual(pipelineSah);
    expect(payload.virtual_alias).toBe('murah-cerdas');
  });
});

describe('pipelineKosong dan LABEL_STRATEGI', () => {
  it('pipelineKosong default strategi prioritas dan anggaran minimum, models kosong', () => {
    expect(pipelineKosong()).toEqual({ strategy: 'priority', attempts: ATTEMPTS_MIN, models: [] });
  });
  it('LABEL_STRATEGI meliputi keempat strategi UI', () => {
    expect(Object.keys(LABEL_STRATEGI).sort()).toEqual([
      'lowest_cost',
      'lowest_latency',
      'priority',
      'round_robin',
    ]);
    expect(LABEL_STRATEGI.priority.label).toBe('Prioritas');
  });
});
