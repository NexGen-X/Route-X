/**
 * Utilitas pembantu untuk format runtime setting dan validasi
 */

/**
 * Format runtime setting value (unknown) ke string representasi tampilan
 */
export function formatSettingValue(val: unknown): string {
  if (val === null || val === undefined) return '';
  if (typeof val === 'object') {
    try {
      return JSON.stringify(val, null, 2);
    } catch {
      return String(val);
    }
  }
  return String(val);
}

/**
 * Label manusiawi untuk kunci setelan yang dikelola subsistem Route-X
 * sendiri (Fase 2 R3). Tab Lanjutan selama ini hanya menampilkan kunci
 * mentah seperti "system:response_cache"; label ini menjelaskan milik
 * siapa kunci tersebut. Kunci asing tetap null (ditampilkan apa adanya).
 */
const LABEL_KUNCI_DIKENAL: Record<string, string> = {
  'system:response_cache': 'Cache Respons — dikelola tab Performa & Cache',
  'system:domain:config': 'Konfigurasi Domain & HTTPS — dikelola tab Domain',
  'system:public_url': 'URL Publik Gateway',
};

export function labelSettingDikenal(key: string): string | null {
  return LABEL_KUNCI_DIKENAL[key] ?? null;
}

/**
 * Cek apakah sebuah kunci parameter dicadangkan untuk subsistem internal Route-X
 */
export function isReservedSetting(key: string): boolean {
  return key.startsWith('cli:config:') || key.startsWith('system:');
}

/**
 * Type guard aman untuk mengekstrak pesan error dari objek unknown
 */
export function getErrorMessage(err: unknown, fallback = 'Terjadi kesalahan sistem'): string {
  if (err instanceof Error) {
    return err.message;
  }
  if (typeof err === 'string') {
    return err;
  }
  if (typeof err === 'object' && err !== null && 'message' in err) {
    const msg = (err as Record<string, unknown>).message;
    if (typeof msg === 'string') {
      return msg;
    }
  }
  return fallback;
}
