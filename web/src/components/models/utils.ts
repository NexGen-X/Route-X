import type { Model } from '../../types';

export const MODEL_FAMILIES = [
  'Claude',
  'GPT',
  'DeepSeek',
  'Gemini',
  'Qwen',
  'GLM',
  'Kimi',
  'Grok',
  'Mimo',
] as const;

export type ModelFamilyPreset = (typeof MODEL_FAMILIES)[number];

/**
 * Mendeteksi kelompok keluarga model berdasarkan field family atau prefix model_id.
 */
export const getModelFamily = (m: Pick<Model, 'family' | 'model_id'>): string => {
  if (m.family && m.family.trim()) {
    const f = m.family.toLowerCase().trim();
    if (f.includes('claude')) return 'Claude';
    if (f.includes('gpt') || f.includes('o1') || f.includes('o3') || f.includes('openai')) return 'GPT';
    if (f.includes('deepseek')) return 'DeepSeek';
    if (f.includes('gemini')) return 'Gemini';
    if (f.includes('qwen')) return 'Qwen';
    if (f.includes('glm')) return 'GLM';
    if (f.includes('kimi') || f.includes('moonshot')) return 'Kimi';
    if (f.includes('grok')) return 'Grok';
    if (f.includes('mimo')) return 'Mimo';
    return m.family;
  }
  const id = (m.model_id || '').toLowerCase();
  if (id.startsWith('claude')) return 'Claude';
  if (id.startsWith('gpt') || id.startsWith('o1') || id.startsWith('o3') || id.startsWith('text-embedding')) return 'GPT';
  if (id.startsWith('deepseek')) return 'DeepSeek';
  if (id.startsWith('gemini')) return 'Gemini';
  if (id.startsWith('qwen')) return 'Qwen';
  if (id.startsWith('glm')) return 'GLM';
  if (id.startsWith('kimi') || id.startsWith('moonshot')) return 'Kimi';
  if (id.startsWith('grok')) return 'Grok';
  if (id.startsWith('mimo')) return 'Mimo';
  return 'Other';
};

/**
 * Type guard aman untuk mengekstrak pesan error dari nilai tipe unknown.
 */
export const getErrorMessage = (err: unknown): string => {
  if (err instanceof Error) {
    return err.message;
  }
  if (typeof err === 'string') {
    return err;
  }
  if (err && typeof err === 'object' && 'message' in err && typeof (err as { message: unknown }).message === 'string') {
    return (err as { message: string }).message;
  }
  return String(err);
};

/**
 * Menghitung agregat jumlah model per rumpun keluarga model.
 */
export const calculateFamilyCounts = (models: Model[]): Record<string, number> => {
  const counts: Record<string, number> = { all: models.length };
  models.forEach((m) => {
    const fam = getModelFamily(m);
    counts[fam] = (counts[fam] || 0) + 1;
  });
  return counts;
};

/**
 * Menghasilkan urutan rumpun keluarga model yang tersedia berdasarkan prioritas.
 */
export const getAvailableFamilies = (familyCounts: Record<string, number>): string[] => {
  const priority = [...MODEL_FAMILIES] as string[];
  const present = Object.keys(familyCounts).filter((k) => k !== 'all' && familyCounts[k] > 0);
  present.sort((a, b) => {
    const idxA = priority.indexOf(a);
    const idxB = priority.indexOf(b);
    if (idxA !== -1 && idxB !== -1) return idxA - idxB;
    if (idxA !== -1) return -1;
    if (idxB !== -1) return 1;
    return a.localeCompare(b);
  });
  return ['all', ...present];
};

/**
 * Memfilter daftar model berdasarkan keluarga terpilih dan kata kunci pencarian.
 */
export const filterModels = (
  models: Model[],
  searchQuery: string,
  selectedFamily: string
): Model[] => {
  let result = models;
  if (selectedFamily !== 'all') {
    result = result.filter((m) => getModelFamily(m).toLowerCase() === selectedFamily.toLowerCase());
  }
  if (searchQuery.trim()) {
    const q = searchQuery.toLowerCase().trim();
    result = result.filter((m) => {
      return (
        m.model_id.toLowerCase().includes(q) ||
        m.display_name.toLowerCase().includes(q) ||
        (m.family && m.family.toLowerCase().includes(q)) ||
        getModelFamily(m).toLowerCase().includes(q) ||
        (m.providers &&
          m.providers.some(
            (p) =>
              p.provider_name.toLowerCase().includes(q) ||
              p.display_name.toLowerCase().includes(q) ||
              p.upstream_model_name.toLowerCase().includes(q)
          ))
      );
    });
  }
  return result;
};

/**
 * Memvalidasi apakah input string merupakan representasi nilai USD yang valid (angka non-negatif).
 */
export const isValidUSD = (value: string): boolean => {
  return value.trim() !== '' && Number.isFinite(Number(value)) && Number(value) >= 0;
};
