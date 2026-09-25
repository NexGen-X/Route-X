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
