/**
 * Route-X Centralized Error Message Helper
 *
 * Fungsi ini mengekstrak string pesan kesalahan yang aman dan ramah pengguna
 * dari tipe `unknown`. Aman terhadap objek non-Error, struktur JSON berantai,
 * respons Axios / Fetch HTTP, array, nilai null/undefined, serta objek dengan
 * referensi sirkular (circular reference).
 */

interface ErrorWithResponse {
  response?: {
    data?: unknown;
    statusText?: string;
  };
}

interface CommonErrorPayload {
  message?: unknown;
  error?: unknown;
  detail?: unknown;
  details?: unknown;
  description?: unknown;
  reason?: unknown;
  code?: unknown;
  param?: unknown;
  msg?: unknown;
}

/**
 * Helper internal untuk mengekstrak pesan dari objek JavaScript dengan pelacakan
 * siklus referensi menggunakan WeakSet agar tidak terjadi rekursi tak terbatas.
 */
function extractFromObject(obj: unknown, visited = new WeakSet<object>()): string | null {
  if (obj === null || typeof obj !== 'object') {
    return null;
  }

  // Lindungi terhadap struktur sirkular
  if (visited.has(obj)) {
    return null;
  }
  visited.add(obj);

  // Jika berupa Array: ekstrak setiap elemen dan gabungkan
  if (Array.isArray(obj)) {
    if (obj.length === 0) return null;
    const parts = obj
      .map((item) => extractFromObject(item, visited) || (typeof item === 'string' ? item.trim() : null))
      .filter((s): s is string => Boolean(s && s.length > 0));
    return parts.length > 0 ? parts.join(', ') : null;
  }

  const payload = obj as CommonErrorPayload;

  // 1. Cek properti 'message' (jika string atau objek bersarang)
  if (typeof payload.message === 'string' && payload.message.trim().length > 0) {
    return payload.message.trim();
  }
  if (payload.message && typeof payload.message === 'object') {
    const nested = extractFromObject(payload.message, visited);
    if (nested) return nested;
  }

  // 2. Cek properti 'error' (bisa string atau objek seperti { error: { message: ... } })
  if (typeof payload.error === 'string' && payload.error.trim().length > 0) {
    return payload.error.trim();
  }
  if (payload.error && typeof payload.error === 'object') {
    const nested = extractFromObject(payload.error, visited);
    if (nested) return nested;
  }

  // 3. Cek properti 'detail' / 'details' (pola umum FastAPI / REST API)
  if (typeof payload.detail === 'string' && payload.detail.trim().length > 0) {
    return payload.detail.trim();
  }
  if (payload.detail && typeof payload.detail === 'object') {
    const nested = extractFromObject(payload.detail, visited);
    if (nested) return nested;
  }
  if (typeof payload.details === 'string' && payload.details.trim().length > 0) {
    return payload.details.trim();
  }
  if (payload.details && typeof payload.details === 'object') {
    const nested = extractFromObject(payload.details, visited);
    if (nested) return nested;
  }

  // 4. Cek properti 'description' / 'reason' / 'msg'
  if (typeof payload.description === 'string' && payload.description.trim().length > 0) {
    return payload.description.trim();
  }
  if (typeof payload.reason === 'string' && payload.reason.trim().length > 0) {
    return payload.reason.trim();
  }
  if (typeof payload.msg === 'string' && payload.msg.trim().length > 0) {
    return payload.msg.trim();
  }

  // 5. Cek pasangan 'code' dan 'param'
  const code = typeof payload.code === 'string' && payload.code.trim().length > 0 ? payload.code.trim() : null;
  const param = typeof payload.param === 'string' && payload.param.trim().length > 0 ? payload.param.trim() : null;

  if (code && param) {
    return `${code} (${param})`;
  }
  if (code) {
    return code;
  }
  if (param) {
    return param;
  }

  return null;
}

/**
 * Mengekstrak pesan kesalahan yang ramah pengguna dari input unknown.
 *
 * @param err Objek kesalahan, string, error HTTP, atau tipe data apa pun
 * @param fallback Pesan default jika ekstraksi gagal (default: 'Terjadi kesalahan sistem')
 * @returns Pesan string yang siap ditampilkan ke pengguna
 */
export function getErrorMessage(
  err: unknown,
  fallback = 'Terjadi kesalahan sistem'
): string {
  if (err === null || err === undefined) {
    return fallback;
  }

  // 1. Tipe string mentah
  if (typeof err === 'string') {
    const trimmed = err.trim();
    if (!trimmed) {
      return fallback;
    }
    // Jika string berupa JSON stringified, coba parse secara aman
    if ((trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
      try {
        const parsed = JSON.parse(trimmed);
        const extracted = extractFromObject(parsed);
        if (extracted) return extracted;
      } catch {
        // Bukan JSON valid, gunakan string langsung
      }
    }
    return trimmed;
  }

  // 2. Instance Error (Error standar, ApiError, TypeError, AxiosError, dll.)
  if (err instanceof Error) {
    // Periksa apakah ada objek respons gaya Axios (err.response.data)
    const errWithResp = err as unknown as ErrorWithResponse;
    if (errWithResp.response && typeof errWithResp.response === 'object') {
      const respData = errWithResp.response.data;
      if (typeof respData === 'string' && respData.trim().length > 0) {
        return respData.trim();
      }
      const extracted = extractFromObject(respData);
      if (extracted) return extracted;

      if (errWithResp.response.statusText && errWithResp.response.statusText.trim().length > 0) {
        return errWithResp.response.statusText.trim();
      }
    }

    // Ambil message dari Error instance
    if (err.message && err.message.trim().length > 0) {
      return err.message.trim();
    }
    return fallback;
  }

  // 3. Objek atau Array biasa
  if (typeof err === 'object') {
    // Cek apakah ada wrapper response gaya Axios langsung pada plain object
    const objWithResp = err as ErrorWithResponse;
    if (objWithResp.response && typeof objWithResp.response === 'object') {
      const respData = objWithResp.response.data;
      if (typeof respData === 'string' && respData.trim().length > 0) {
        return respData.trim();
      }
      const extractedResp = extractFromObject(respData);
      if (extractedResp) return extractedResp;
      if (objWithResp.response.statusText && objWithResp.response.statusText.trim().length > 0) {
        return objWithResp.response.statusText.trim();
      }
    }

    const extracted = extractFromObject(err);
    if (extracted) return extracted;

    // Upaya fallback representasi JSON untuk objek tanpa properti standar
    try {
      const jsonStr = JSON.stringify(err);
      if (jsonStr && jsonStr !== '{}' && jsonStr !== '[]') {
        return jsonStr;
      }
    } catch {
      // Objek sirkular atau tidak bisa di-stringify, fallback aman
      return fallback;
    }

    return fallback;
  }

  // 4. Primitif angka, boolean, bigint
  if (typeof err === 'number' || typeof err === 'boolean' || typeof err === 'bigint') {
    return String(err);
  }

  return fallback;
}
