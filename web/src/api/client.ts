// Route-X API Client
import type {
  Principal,
  Provider,
  Credential,
  OAuthSession,
  Model,
  ModelAlias,
  ProviderModel,
  Price,
  ReferencePrice,
  EgressPool,
  EgressProbeResult,
  RoutingRule,
  RateLimit,
  Budget,
  ContentFilter,
  Ban,
  CircuitBreakerStatus,
  APIKey,
  User,
  UserDetail,
  SetupHintResponse,
  Session,
  Webhook,
  WebhookDelivery,
  RequestLog,
  RequestEvent,
  RequestPayload,
  ObservabilitySummary,
  TimeSeriesPoint,
  BreakdownItem,
  BackgroundJob,
  AuditLogEntry,
  Diagnostics,
  SystemOverview,
  ResponseCacheStats,
  CLIDetectedResponse,
  CLIConfigureRequest,
  CLIConfigureResponse,
  CLIExportScriptResponse,
  DomainConfig,
  DomainUpdateRequest,
  BackupStatusResponse,
  ProviderHealthCheck,
} from '../types';

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public param?: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

let cachedCsrfToken = '';

export function setCsrfToken(token: string) {
  if (!token) {
    cachedCsrfToken = '';
    return;
  }
  cachedCsrfToken = token;
}

// Ambil CSRF token dari cookie double-submit atau cache memori sesi halaman.
export function getCsrfToken(): string {
  const hostMatch = document.cookie.match(/(?:^|;\s*)__Host-routex_csrf=([^;]+)/);
  if (hostMatch) return decodeURIComponent(hostMatch[1]);
  
  const match = document.cookie.match(/(?:^|;\s*)routex_csrf=([^;]+)/);
  if (match) return decodeURIComponent(match[1]);
  return cachedCsrfToken;
}

// ---------------------------------------------------------------------------
// Inti klien (audit3 grup 13 — B-057). Perilaku yang dulu cacat dan kini
// diperbaiki:
// - V-FF-23: tidak ada timeout — fetch yang macet (proxy diam, server
//   setengah mati) menggantung selamanya. Sekarang setiap request punya
//   tenggat (bawaan 30 detik) yang digabung dengan signal pemanggil.
// - V-FF-27: normalisasi error rusak — badan error non-JSON (mis. halaman
//   502 dari proxy) dilempar mentah sebagai pesan toast, dan kegagalan
//   parse JSON menutupi status HTTP sebenarnya. Sekarang status selalu
//   diperiksa dan error selalu berbentuk ApiError bermakna.
// - V-FF-26: pengecualian 401 membandingkan string path PERSIS, sehingga
//   '/api/auth/me?x=1' lolos dan memicu logout; perbandingan kini atas
//   pathname tanpa query string.
// - V-FF-31: 403 csrf_invalid tidak pernah dipulihkan — token basi membuat
//   SEMUA mutasi gagal sampai muat ulang manual. Sekarang token disegarkan
//   dari /api/auth/me dan request diulang tepat satu kali.
// - V-FF-32: tidak ada dedup — efek ganda/polling tumpang tindih mengirim
//   GET identik berkali-kali. GET identik yang sedang terbang kini berbagi
//   satu promise.
// - V-FF-28: fetchAllPages berhenti DIAM-DIAM di batas pengaman — kini
//   hasilnya membawa penanda `terpotong` (dan peringatan konsol), termasuk
//   bila kursor berputar.
// ---------------------------------------------------------------------------

const TIMEOUT_BAWAAN_MS = 30_000;
const METODE_MUTASI = ['POST', 'PUT', 'DELETE', 'PATCH'];

export interface OpsiRequest extends RequestInit {
  /** Tenggat per request dalam milidetik; bawaan 30 detik. */
  timeoutMs?: number;
}

interface SinyalGabungan {
  signal: AbortSignal;
  kenaTimeout: () => boolean;
  bersihkan: () => void;
}

// Menggabungkan tenggat internal dengan AbortSignal milik pemanggil:
// mana pun yang lebih dulu membatalkan fetch.
function buatSinyalGabungan(timeoutMs: number, sinyalPemanggil?: AbortSignal | null): SinyalGabungan {
  const controller = new AbortController();
  let timeout = false;
  const timer = setTimeout(() => {
    timeout = true;
    controller.abort();
  }, timeoutMs);
  const onBatalPemanggil = () => controller.abort();
  if (sinyalPemanggil) {
    if (sinyalPemanggil.aborted) {
      controller.abort();
    } else {
      sinyalPemanggil.addEventListener('abort', onBatalPemanggil, { once: true });
    }
  }
  return {
    signal: controller.signal,
    kenaTimeout: () => timeout,
    bersihkan: () => {
      clearTimeout(timer);
      sinyalPemanggil?.removeEventListener('abort', onBatalPemanggil);
    },
  };
}

// fetch dengan tenggat; semua kegagalan jaringan/abort dinormalisasi
// menjadi ApiError agar pemanggil menerima satu bentuk error saja.
async function fetchBertenggat(url: string, init: RequestInit, timeoutMs: number): Promise<Response> {
  const gabungan = buatSinyalGabungan(timeoutMs, init.signal);
  try {
    return await fetch(url, { ...init, signal: gabungan.signal });
  } catch (err) {
    if (err instanceof Error && err.name === 'AbortError') {
      if (gabungan.kenaTimeout()) {
        throw new ApiError(
          0,
          'request_timeout',
          `Permintaan ke server melewati batas waktu ${Math.round(timeoutMs / 1000)} detik. Server mungkin sibuk — coba lagi.`
        );
      }
      throw new ApiError(0, 'request_aborted', 'Permintaan dibatalkan.');
    }
    // Jaringan mati/DNS gagal/CORS: fetch melempar TypeError mentah. Bungkus
    // menjadi ApiError agar seluruh pemanggil mendapat bentuk error yang sama
    // dan bisa membedakan gangguan jaringan dari respons server.
    throw new ApiError(0, 'network_error', 'Tidak dapat menghubungi server Route-X. Periksa koneksi jaringan.');
  } finally {
    gabungan.bersihkan();
  }
}

function ekstrakError(data: unknown, status: number): { code: string; message: string; param?: string } {
  const errObj = (data && typeof data === 'object' && 'error' in data && data.error && typeof data.error === 'object')
    ? (data.error as Record<string, unknown>)
    : undefined;
  const code = typeof errObj?.code === 'string' ? errObj.code : 'unknown_error';
  if (typeof errObj?.message === 'string' && errObj.message !== '') {
    return { code, message: errObj.message, param: typeof errObj?.param === 'string' ? errObj.param : undefined };
  }
  // Badan non-JSON singkat yang bukan markup boleh ditampilkan apa adanya;
  // halaman error HTML dari proxy/upstream TIDAK — dulu teks mentahnya
  // menjadi pesan toast (V-FF-27).
  if (typeof data === 'string') {
    const ringkas = data.trim();
    if (ringkas !== '' && ringkas.length <= 200 && !ringkas.startsWith('<')) {
      return { code, message: ringkas };
    }
  }
  return {
    code,
    message: status >= 500
      ? 'Terjadi kesalahan sistem.'
      : `Permintaan gagal dengan status ${status}.`,
  };
}

// Menyegarkan token CSRF dari /api/auth/me memakai fetch mentah (bukan
// request()) agar tidak rekursif dan tidak memicu penanganan 401 global.
async function segarkanCsrfToken(): Promise<boolean> {
  try {
    const res = await fetchBertenggat('/api/auth/me', { credentials: 'same-origin' }, 10_000);
    if (!res.ok) return false;
    const data: unknown = await res.json();
    if (data && typeof data === 'object' && 'csrf_token' in data) {
      const token = (data as Record<string, unknown>).csrf_token;
      if (typeof token === 'string' && token !== '') {
        setCsrfToken(token);
        return true;
      }
    }
    return false;
  } catch {
    return false;
  }
}

async function lakukanRequest<T>(path: string, options: OpsiRequest, sudahUlangCsrf: boolean): Promise<T> {
  const { timeoutMs = TIMEOUT_BAWAAN_MS, ...init } = options;
  const headers = new Headers(init.headers);
  if (!headers.has('Content-Type') && init.body && typeof init.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }

  // Double-submit CSRF protection untuk metode mutasi
  const method = (init.method || 'GET').toUpperCase();
  const mutasi = METODE_MUTASI.includes(method);
  if (mutasi) {
    const csrf = getCsrfToken();
    if (csrf) {
      headers.set('X-CSRF-Token', csrf);
    }
  }

  const res = await fetchBertenggat(path, { ...init, headers, credentials: 'same-origin' }, timeoutMs);

  if (res.status === 204) {
    return {} as T;
  }

  let data: unknown;
  let jsonCacat = false;
  const contentType = res.headers.get('content-type') || '';
  if (contentType.includes('application/json')) {
    try {
      data = await res.json();
    } catch {
      jsonCacat = true;
      data = undefined;
    }
  } else {
    data = await res.text();
  }

  // Tangkap CSRF token jika dikembalikan di respons
  if (data && typeof data === 'object' && 'csrf_token' in data) {
    const csrfToken = (data as Record<string, unknown>).csrf_token;
    if (typeof csrfToken === 'string') {
      setCsrfToken(csrfToken);
    }
  }

  if (!res.ok) {
    const galat = ekstrakError(data, res.status);

    // Pemulihan CSRF (V-FF-31): token basi (rotasi sesi, muat ulang sebagian)
    // tidak boleh membunuh semua mutasi sampai akhir sesi. Segarkan token
    // lalu ulangi request yang sama tepat satu kali.
    if (res.status === 403 && galat.code === 'csrf_invalid' && mutasi && !sudahUlangCsrf) {
      if (await segarkanCsrfToken()) {
        return lakukanRequest<T>(path, options, true);
      }
    }

    // Penanganan 401 global: beri tahu AuthContext, kecuali untuk endpoint
    // identitas/login sendiri supaya tidak memicu loop logout. Perbandingan
    // memakai pathname — query string tidak boleh mengelabui pengecualian
    // ini (V-FF-26).
    const pathname = path.split('?')[0];
    if (res.status === 401 && pathname !== '/api/auth/me' && pathname !== '/api/auth/login') {
      window.dispatchEvent(new CustomEvent('routex:unauthorized'));
    }
    throw new ApiError(res.status, galat.code, galat.message, galat.param);
  }

  if (jsonCacat) {
    // Status sukses tetapi badan JSON tidak terbaca dan tidak kosong:
    // datanya tidak bisa dikarang, jadi ini tetap kesalahan — bedanya dari
    // versi lama, kesalahan pada respons ERROR kini selalu melaporkan
    // status/kode aslinya, bukan invalid_json_response (V-FF-27).
    throw new ApiError(res.status, 'invalid_json_response', 'Respons server bukan JSON yang valid.');
  }

  return data as T;
}

// GET identik yang sedang dalam penerbangan berbagi satu promise (V-FF-32).
const getSedangTerbang = new Map<string, Promise<unknown>>();

async function request<T>(path: string, options: OpsiRequest = {}): Promise<T> {
  const method = (options.method || 'GET').toUpperCase();
  const bisaDedup = method === 'GET' && !options.body && !options.signal;
  if (!bisaDedup) {
    return lakukanRequest<T>(path, options, false);
  }
  const kunci = `${method} ${path}`;
  const ada = getSedangTerbang.get(kunci);
  if (ada) {
    return ada as Promise<T>;
  }
  const janji = lakukanRequest<T>(path, options, false);
  getSedangTerbang.set(kunci, janji as Promise<unknown>);
  const bersihkan = () => {
    if (getSedangTerbang.get(kunci) === (janji as Promise<unknown>)) {
      getSedangTerbang.delete(kunci);
    }
  };
  void janji.then(bersihkan, bersihkan);
  return janji;
}

// fetchAllPages mengikuti rantai next_cursor sampai habis (audit F7):
// daftar admin di backend terpotong pada batas halaman bawaan (50), jadi
// halaman yang hanya membaca halaman pertama diam-diam menyembunyikan
// data ke-51 dan seterusnya. Batas 25 halaman x 200 baris (5.000 item)
// adalah pengaman, bukan perilaku normal untuk instalasi pribadi.
// Audit3 V-FF-28: mencapai pengaman itu tidak lagi senyap — hasil membawa
// penanda `terpotong: true` (dan peringatan konsol), termasuk bila kursor
// dari server berputar ke nilai yang sudah pernah dipakai.
export interface HasilSemuaHalaman<T> {
  items: T[];
  terpotong?: boolean;
}

async function fetchAllPages<T>(
  fetchPage: (cursor?: string) => Promise<{ items: T[]; next_cursor?: string }>
): Promise<HasilSemuaHalaman<T>> {
  const items: T[] = [];
  const kursorPernah = new Set<string>();
  let cursor: string | undefined;
  let terpotong = false;
  for (let page = 0; page < 25; page++) {
    const res = await fetchPage(cursor);
    items.push(...(res.items || []));
    if (!res.next_cursor) break;
    if (kursorPernah.has(res.next_cursor)) {
      terpotong = true;
      break;
    }
    kursorPernah.add(res.next_cursor);
    cursor = res.next_cursor;
    if (page === 24) {
      terpotong = true;
    }
  }
  if (terpotong) {
    console.warn(
      `fetchAllPages berhenti sebelum seluruh data terbaca (terkumpul ${items.length} item) — daftar mungkin tidak lengkap.`
    );
  }
  return { items, terpotong: terpotong || undefined };
}

export const api = {
  // Auth
  auth: {
    me: () => request<Principal>('/api/auth/me'),
    login: (credentials: { email: string; password: string; keep_signed_in?: boolean }) =>
      request<Principal>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify(credentials),
      }),
    logout: () => request<void>('/api/auth/logout', { method: 'POST' }),
    changePassword: (data: { current_password: string; new_password: string }) =>
      request<void>('/api/auth/change-password', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    setupHint: () => request<SetupHintResponse>('/api/auth/setup-hint'),
  },

  // Observability
  observability: {
    summary: (window = '24h') =>
      request<ObservabilitySummary>(`/api/admin/observability/summary?window=${encodeURIComponent(window)}`),
    series: (metric = 'requests', window = '24h') =>
      request<{ points: TimeSeriesPoint[] }>(
        `/api/admin/observability/series?metric=${encodeURIComponent(metric)}&window=${encodeURIComponent(window)}`
      ),
    breakdown: (by = 'provider', window = '24h') =>
      request<{ items: BreakdownItem[] }>(
        `/api/admin/observability/breakdown?by=${encodeURIComponent(by)}&window=${encodeURIComponent(window)}`
      ),
    health: () =>
      request<{ providers: { id: string; name: string; status: string; error?: string; checked_at?: string }[] }>(
        '/api/admin/observability/health'
      ),
    liveMetrics: () => request<Record<string, unknown>>('/api/admin/observability/metrics/live'),
  },

  // Requests
  requests: {
    list: (params: Record<string, string | number | boolean | undefined | null>) => {
      const q = new URLSearchParams();
      Object.entries(params).forEach(([k, v]) => {
        if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
      });
      return request<{ items: RequestLog[]; next_cursor?: string }>(`/api/admin/requests?${q.toString()}`);
    },
    get: (id: string) => request<RequestLog>(`/api/admin/requests/${encodeURIComponent(id)}`),
    events: (id: string, createdAt?: string) => {
      const q = createdAt ? `?created_at=${encodeURIComponent(createdAt)}` : '';
      return request<{ events: RequestEvent[] }>(`/api/admin/requests/${encodeURIComponent(id)}/events${q}`);
    },
    payload: (id: string, createdAt?: string) => {
      const q = createdAt ? `?created_at=${encodeURIComponent(createdAt)}` : '';
      return request<RequestPayload>(`/api/admin/requests/${encodeURIComponent(id)}/payload${q}`);
    },
  },

  // Upstreams
  providers: {
    list: (params?: { search?: string; kind?: string; enabled?: boolean; is_byok?: boolean; limit?: number; cursor?: string }) => {
      const q = new URLSearchParams();
      if (params?.search) q.set('search', params.search);
      if (params?.kind) q.set('kind', params.kind);
      if (params?.enabled !== undefined) q.set('enabled', String(params.enabled));
      if (params?.is_byok !== undefined) q.set('is_byok', String(params.is_byok));
      if (params?.limit) q.set('limit', String(params.limit));
      if (params?.cursor) q.set('cursor', params.cursor);
      const qs = q.toString();
      return request<{ items: Provider[]; next_cursor?: string }>(
        qs ? `/api/admin/upstreams/providers?${qs}` : '/api/admin/upstreams/providers'
      );
    },
    listAll: () =>
      fetchAllPages<Provider>((cursor) => api.providers.list({ limit: 200, cursor })),
    get: (id: string) => request<Provider>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}`),
    create: (data: Partial<Provider>) =>
      request<Provider>('/api/admin/upstreams/providers', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<Provider>) =>
      request<Provider>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<Provider>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    probe: (id: string) =>
      request<{ status: string; latency_ms: number; error?: string }>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/probe`,
        { method: 'POST' }
      ),
    healthChecks: (id: string) =>
      request<{ items: ProviderHealthCheck[]; next_cursor?: string }>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/health-checks`
      ),
    syncModels: (id: string) =>
      request<{ count: number; models: string[]; message: string }>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/sync-models`,
        { method: 'POST' }
      ),
    models: (id: string) =>
      request<{ items: ProviderModel[] }>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}/models`),
    addModel: (id: string, name: string) =>
      request<ProviderModel>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}/models`, {
        method: 'POST',
        body: JSON.stringify({ name }),
      }),
    deleteModel: (id: string, mappingId: string) =>
      request<void>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}/models/${encodeURIComponent(mappingId)}`, {
        method: 'DELETE',
      }),
    testModel: (id: string, model: string) =>
      request<{ status: string; latency_ms: number; message?: string; error?: string }>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/test-model`,
        {
          method: 'POST',
          body: JSON.stringify({ model }),
        }
      ),
    setCredentialStrategy: (id: string, strategy: 'round_robin' | 'priority') =>
      request<Provider>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}/credential-strategy`, {
        method: 'POST',
        body: JSON.stringify({ strategy }),
      }),
    oauthExchange: (id: string, data: { code: string; redirect_uri?: string; egress_pool_id?: string | null }) =>
      request<{ status: string; account_email: string; account_name?: string; expires_at: string; session: OAuthSession }>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/oauth/exchange`,
        {
          method: 'POST',
          body: JSON.stringify(data),
        }
      ),
    listOAuthSessions: (id: string) =>
      request<{ items: OAuthSession[] }>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}/oauth/sessions`),
    refreshOAuth: (id: string, sessionId?: string) =>
      request<{ status: string; refreshed_count: number }>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/oauth/refresh`,
        {
          method: 'POST',
          body: JSON.stringify({ session_id: sessionId }),
        }
      ),
    toggleOAuthSession: (id: string, sessionId: string, enabled: boolean) =>
      request<{ status: string; session_id: string; enabled: boolean }>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/oauth/sessions/${encodeURIComponent(sessionId)}/toggle`,
        {
          method: 'POST',
          body: JSON.stringify({ enabled }),
        }
      ),
    deleteOAuthSession: (id: string, sessionId: string) =>
      request<void>(
        `/api/admin/upstreams/providers/${encodeURIComponent(id)}/oauth/sessions/${encodeURIComponent(sessionId)}`,
        { method: 'DELETE' }
      ),
  },

  credentials: {
    list: (providerId: string) =>
      request<{ items: Credential[] }>(`/api/admin/upstreams/providers/${encodeURIComponent(providerId)}/credentials`),
    create: (providerId: string, data: { label: string; api_key: string; expires_at?: string; egress_pool_id?: string | null }) =>
      request<Credential>(`/api/admin/upstreams/providers/${encodeURIComponent(providerId)}/credentials`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    setEgressPool: (providerId: string, id: string, egress_pool_id: string | null) =>
      request<Credential>(
        `/api/admin/upstreams/providers/${encodeURIComponent(providerId)}/credentials/${encodeURIComponent(id)}/egress-pool`,
        {
          method: 'PUT',
          body: JSON.stringify({ egress_pool_id }),
        }
      ),
    delete: (providerId: string, id: string) =>
      request<void>(`/api/admin/upstreams/providers/${encodeURIComponent(providerId)}/credentials/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
    toggle: (providerId: string, id: string, enabled: boolean) =>
      request<Credential>(`/api/admin/upstreams/providers/${encodeURIComponent(providerId)}/credentials/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
  },

  models: {
    list: (params?: { search?: string; family?: string; enabled?: boolean; limit?: number; cursor?: string }) => {
      const q = new URLSearchParams();
      if (params?.search) q.set('search', params.search);
      if (params?.family) q.set('family', params.family);
      if (params?.enabled !== undefined) q.set('enabled', String(params.enabled));
      if (params?.limit) q.set('limit', String(params.limit));
      if (params?.cursor) q.set('cursor', params.cursor);
      const qs = q.toString();
      const path = '/api/admin/upstreams/models';
      return request<{ items: Model[]; next_cursor?: string }>(qs ? `${path}?${qs}` : path);
    },
    listAll: () =>
      fetchAllPages<Model>((cursor) => api.models.list({ limit: 200, cursor })),
    get: (id: string) =>
      request<{ model: Model; aliases: ModelAlias[]; mappings: ProviderModel[] }>(
        `/api/admin/upstreams/models/${encodeURIComponent(id)}`
      ),
    create: (data: Partial<Model>) =>
      request<Model>('/api/admin/upstreams/models', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<Model>) =>
      request<Model>(`/api/admin/upstreams/models/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/upstreams/models/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    addAlias: (id: string, alias: string) =>
      request<ModelAlias>(`/api/admin/upstreams/models/${encodeURIComponent(id)}/aliases`, {
        method: 'POST',
        body: JSON.stringify({ alias }),
      }),
    deleteAlias: (id: string, alias: string) =>
      request<void>(`/api/admin/upstreams/models/${encodeURIComponent(id)}/aliases/${encodeURIComponent(alias)}`, {
        method: 'DELETE',
      }),
    attachProvider: (id: string, data: Partial<ProviderModel>) =>
      request<ProviderModel>(`/api/admin/upstreams/models/${encodeURIComponent(id)}/mappings`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    detachProvider: (_id: string, mappingId: string) =>
      request<void>(`/api/admin/upstreams/models/mappings/${encodeURIComponent(mappingId)}`, {
        method: 'DELETE',
      }),
    setPrice: (mappingId: string, data: { input_per_1m_usd: string; output_per_1m_usd: string; cached_input_per_1m_usd?: string }) =>
      request<Price>(`/api/admin/upstreams/models/mappings/${encodeURIComponent(mappingId)}/pricing`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    pricingHistory: (mappingId: string) =>
      request<{ items: Price[] }>(`/api/admin/upstreams/models/mappings/${encodeURIComponent(mappingId)}/pricing/history`),
    pricingReference: (mappingId: string) =>
      request<ReferencePrice>(`/api/admin/upstreams/models/mappings/${encodeURIComponent(mappingId)}/pricing/reference`),
  },

  egress: {
    list: () => request<{ items: EgressPool[] }>('/api/admin/upstreams/egress-pools'),
    create: (data: Partial<EgressPool> & { proxy_url: string }) =>
      request<EgressPool>('/api/admin/upstreams/egress-pools', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<EgressPool> & { proxy_url?: string }) =>
      request<EgressPool>(`/api/admin/upstreams/egress-pools/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/upstreams/egress-pools/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    test: (id: string) =>
      request<EgressProbeResult>(`/api/admin/upstreams/egress-pools/${encodeURIComponent(id)}/test`, { method: 'POST' }),
    provisionCloudflare: (data: {
      account_id: string;
      api_token: string;
      gateway_id?: string;
      name?: string;
      collect_logs?: boolean;
      enable_cache?: boolean;
      region?: string;
    }) =>
      request<{
        pool: EgressPool;
        provider_type: string;
        target_url: string;
        already_exist?: boolean;
      }>('/api/admin/upstreams/egress-pools/provision/cloudflare', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    provisionDeno: (data: {
      access_token: string;
      project_name?: string;
      name?: string;
      region?: string;
    }) =>
      request<{
        pool: EgressPool;
        provider_type: string;
        target_url: string;
      }>('/api/admin/upstreams/egress-pools/provision/deno', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  },

  // Gateway Policies
  routing: {
    list: (params?: { limit?: number; cursor?: string }) => {
      const q = new URLSearchParams();
      if (params?.limit) q.set('limit', String(params.limit));
      if (params?.cursor) q.set('cursor', params.cursor);
      const qs = q.toString();
      return request<{ items: RoutingRule[]; next_cursor?: string }>(
        qs ? `/api/admin/gateway/routing-rules?${qs}` : '/api/admin/gateway/routing-rules'
      );
    },
    listAll: () =>
      fetchAllPages<RoutingRule>((cursor) => api.routing.list({ limit: 200, cursor })),
    create: (data: Partial<RoutingRule>) =>
      request<RoutingRule>('/api/admin/gateway/routing-rules', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<RoutingRule>) =>
      request<RoutingRule>(`/api/admin/gateway/routing-rules/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/routing-rules/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<RoutingRule>(`/api/admin/gateway/routing-rules/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    setProviders: (id: string, data: { provider_ids: string[]; weights: Record<string, number> }) =>
      request<RoutingRule>(`/api/admin/gateway/routing-rules/${encodeURIComponent(id)}/providers`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },

  rateLimits: {
    list: () => request<{ items: RateLimit[] }>('/api/admin/gateway/rate-limits'),
    create: (data: Partial<RateLimit>) =>
      request<RateLimit>('/api/admin/gateway/rate-limits', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<RateLimit>) =>
      request<RateLimit>(`/api/admin/gateway/rate-limits/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/rate-limits/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<RateLimit>(`/api/admin/gateway/rate-limits/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
  },

  budgets: {
    list: () => request<{ items: Budget[] }>('/api/admin/gateway/budgets'),
    create: (data: Partial<Budget>) =>
      request<Budget>('/api/admin/gateway/budgets', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<Budget>) =>
      request<Budget>(`/api/admin/gateway/budgets/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/budgets/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<Budget>(`/api/admin/gateway/budgets/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    reset: (id: string) =>
      request<Budget>(`/api/admin/gateway/budgets/${encodeURIComponent(id)}/reset`, { method: 'POST' }),
  },

  filters: {
    list: (cursor?: string) => {
      const q = cursor ? `?cursor=${encodeURIComponent(cursor)}` : '';
      return request<{ items: ContentFilter[]; next_cursor?: string }>(
        `/api/admin/gateway/content-filters${q}`
      );
    },
    create: (data: Partial<ContentFilter>) =>
      request<ContentFilter>('/api/admin/gateway/content-filters', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<ContentFilter>) =>
      request<ContentFilter>(`/api/admin/gateway/content-filters/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/content-filters/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<ContentFilter>(`/api/admin/gateway/content-filters/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
  },

  bans: {
    list: () => request<{ items: Ban[] }>('/api/admin/gateway/bans'),
    create: (data: { subject_kind: string; subject: string; reason: string; expires_at?: string }) =>
      request<Ban>('/api/admin/gateway/bans', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    lift: (id: string) =>
      request<Ban>(`/api/admin/gateway/bans/${encodeURIComponent(id)}/lift`, { method: 'POST' }),
  },

  breakers: {
    list: () => request<{ items: CircuitBreakerStatus[] }>('/api/admin/gateway/circuit-breakers'),
    reset: (providerId: string, model: string) =>
      request<void>('/api/admin/gateway/circuit-breakers/reset', {
        method: 'POST',
        body: JSON.stringify({ provider_id: providerId, model }),
      }),
  },

  // Access Control
  apiKeys: {
    list: (params?: Record<string, string | number | boolean | undefined | null>) => {
      const q = new URLSearchParams();
      if (params) {
        Object.entries(params).forEach(([k, v]) => {
          if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
        });
      }
      return request<{ items: APIKey[]; next_cursor?: string }>(`/api/admin/access/api-keys?${q.toString()}`);
    },
    listAll: () =>
      fetchAllPages<APIKey>((cursor) => api.apiKeys.list({ limit: 200, cursor })),
    get: (id: string) => request<APIKey>(`/api/admin/access/api-keys/${encodeURIComponent(id)}`),
    create: (data: Partial<APIKey>) =>
      request<APIKey>('/api/admin/access/api-keys', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<APIKey>) =>
      request<APIKey>(`/api/admin/access/api-keys/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    rotate: (id: string) =>
      request<APIKey>(`/api/admin/access/api-keys/${encodeURIComponent(id)}/rotate`, { method: 'POST' }),
    remove: (id: string) =>
      request<{ status: string }>(`/api/admin/access/api-keys/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
    toggle: (id: string, enabled: boolean) =>
      request<APIKey>(`/api/admin/access/api-keys/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    getAllowed: (id: string) =>
      request<{ model_ids: string[]; provider_ids: string[] }>(`/api/admin/access/api-keys/${encodeURIComponent(id)}/allowed`),
    setAllowed: (id: string, data: { model_ids: string[]; provider_ids: string[] }) =>
      request<APIKey>(`/api/admin/access/api-keys/${encodeURIComponent(id)}/allowed`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
  },

  users: {
    list: (params?: Record<string, string | number | boolean | undefined | null>) => {
      const q = new URLSearchParams();
      if (params) {
        Object.entries(params).forEach(([k, v]) => {
          if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
        });
      }
      return request<{ items: User[]; next_cursor?: string }>(`/api/admin/access/users?${q.toString()}`);
    },
    get: (id: string) => request<UserDetail>(`/api/admin/access/users/${encodeURIComponent(id)}`),
    create: (data: { email: string; name: string; password?: string; must_change_password?: boolean; role?: 'admin' | 'user' }) =>
      request<User>('/api/admin/access/users', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: { name?: string; status?: string; role?: 'admin' | 'user' }) =>
      request<User>(`/api/admin/access/users/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<{ status: string }>(`/api/admin/access/users/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
    forceResetPassword: (id: string, tempPassword?: string) =>
      // Backend menjawab {status:"password_reset"}; temp password diisi admin di request.
      request<{ status: string }>(`/api/admin/access/users/${encodeURIComponent(id)}/force-reset-password`, {
        method: 'POST',
        body: JSON.stringify({ temp_password: tempPassword }),
      }),
    revokeSessions: (id: string) =>
      request<{ status: string; count: number }>(
        `/api/admin/access/users/${encodeURIComponent(id)}/revoke-sessions`,
        { method: 'POST' }
      ),
    // Buka kunci login manual (audit3 B-076): backend menjawab User akhir
    // (idempoten untuk akun yang memang tidak terkunci).
    unlock: (id: string) =>
      request<User>(`/api/admin/access/users/${encodeURIComponent(id)}/unlock`, {
        method: 'POST',
      }),
  },

  sessions: {
    list: () => request<{ items: Session[] }>('/api/admin/access/sessions'),
    revoke: (id: string) =>
      request<void>(`/api/admin/access/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    // Saklar respons insiden: mencabut SELURUH sesi aktif semua pengguna,
    // termasuk sesi pemanggil sendiri (request berikutnya wajib masuk ulang).
    revokeAll: () =>
      request<{ status: string; count: number }>('/api/admin/access/sessions/revoke-all', {
        method: 'POST',
      }),
  },

  // Automation
  webhooks: {
    list: () => request<{ items: Webhook[] }>('/api/admin/automation/webhooks'),
    get: (id: string) => request<Webhook>(`/api/admin/automation/webhooks/${encodeURIComponent(id)}`),
    create: (data: { name: string; url: string; events: string[]; secret?: string; enabled?: boolean }) =>
      request<Webhook>('/api/admin/automation/webhooks', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<Webhook> & { secret?: string }) =>
      request<Webhook>(`/api/admin/automation/webhooks/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/automation/webhooks/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<Webhook>(`/api/admin/automation/webhooks/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    test: (id: string) =>
      request<{ status: string | number; status_code?: number; duration_ms: number; error?: string; response?: string }>(
        `/api/admin/automation/webhooks/${encodeURIComponent(id)}/test`,
        { method: 'POST' }
      ),
    getDeliveries: (webhookId: string) =>
      request<{ items: WebhookDelivery[] }>(`/api/admin/automation/webhooks/${encodeURIComponent(webhookId)}/deliveries`),
    getDelivery: (deliveryId: string) =>
      request<WebhookDelivery>(`/api/admin/automation/deliveries/${encodeURIComponent(deliveryId)}`),
  },

  // System
  system: {
    settings: () => request<{ items: { key: string; value: unknown; description?: string }[] }>('/api/admin/system/settings'),
    updateSetting: (key: string, value: unknown, description?: string) => {
      let rawVal = value;
      if (typeof value === 'string') {
        const trimmed = value.trim();
        if ((trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
          try {
            rawVal = JSON.parse(trimmed);
          } catch {
            rawVal = value;
          }
        }
      }
      return request<void>(`/api/admin/system/settings/${encodeURIComponent(key)}`, {
        method: 'PUT',
        body: JSON.stringify({ value: rawVal, description }),
      });
    },
    deleteSetting: (key: string) =>
      request<void>(`/api/admin/system/settings/${encodeURIComponent(key)}`, {
        method: 'DELETE',
      }),
    jobs: () => request<{ items: BackgroundJob[] }>('/api/admin/system/jobs'),
    triggerJob: (name: string) =>
      request<{ status: string; job: string; message: string }>(`/api/admin/system/jobs/${encodeURIComponent(name)}/run`, { method: 'POST' }),
    auditLogs: (params?: Record<string, string | number | boolean | undefined | null>) => {
      const q = new URLSearchParams();
      if (params) {
        Object.entries(params).forEach(([k, v]) => {
          if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
        });
      }
      return request<{ items: AuditLogEntry[]; next_cursor?: string }>(`/api/admin/system/audit-logs?${q.toString()}`);
    },
    diagnostics: () => request<Diagnostics>('/api/admin/system/diagnostics'),
    overview: () => request<SystemOverview>('/api/admin/system/overview'),
    cacheStats: () => request<ResponseCacheStats>('/api/admin/system/cache'),
    flushCache: () => request<{ deleted: number; message: string }>('/api/admin/system/cache/flush', { method: 'POST' }),
    updateCacheSettings: (enabled: boolean, ttlSeconds: number) =>
      request<{ enabled: boolean; ttl_seconds: number; message: string }>('/api/admin/system/cache/settings', {
        method: 'POST',
        body: JSON.stringify({ enabled, ttl_seconds: ttlSeconds }),
      }),
    domain: {
      get: () => request<DomainConfig>('/api/admin/system/domain'),
      update: (data: DomainUpdateRequest) =>
        request<DomainConfig>('/api/admin/system/domain', {
          method: 'POST',
          body: JSON.stringify(data),
        }),
      delete: () =>
        request<{ status: string }>('/api/admin/system/domain', {
          method: 'DELETE',
        }),
    },
    backup: {
      status: () => request<BackupStatusResponse>('/api/admin/system/backup/status'),
      // download mengunduh dump lewat POST — sejak audit3 B-045 ekspor bukan
      // lagi GET (dump penuh wajib melewati CSRF, tautan polos tidak boleh
      // memicunya). Respons biner disimpan sebagai berkas lewat object URL;
      // respons error JSON diurai menjadi ApiError, TIDAK terunduh sebagai
      // berkas "cadangan" berisi JSON.
      download: async (format: 'sql.gz' | 'sql' = 'sql.gz'): Promise<void> => {
        const headers = new Headers();
        const csrf = getCsrfToken();
        if (csrf) {
          headers.set('X-CSRF-Token', csrf);
        }
        // Ekspor cadangan bisa lama (dump penuh), jadi tenggatnya lebih
        // longgar dari request biasa — tetapi tetap ada (V-FF-23): unduhan
        // yang macet tidak boleh menggantung tanpa batas.
        const res = await fetchBertenggat(
          `/api/admin/system/backup/export?format=${format}`,
          {
            method: 'POST',
            headers,
            credentials: 'same-origin',
          },
          120_000
        );
        if (!res.ok) {
          if (res.status === 401) {
            window.dispatchEvent(new CustomEvent('routex:unauthorized'));
          }
          let code = 'unknown_error';
          let message = 'Terjadi kesalahan sistem.';
          try {
            const data: unknown = await res.json();
            const errObj = (data && typeof data === 'object' && 'error' in data && data.error && typeof data.error === 'object')
              ? (data.error as Record<string, unknown>)
              : undefined;
            if (typeof errObj?.code === 'string') code = errObj.code;
            if (typeof errObj?.message === 'string') message = errObj.message;
          } catch {
            // Badan error bukan JSON — pertahankan pesan generik.
          }
          throw new ApiError(res.status, code, message);
        }
        const blob = await res.blob();
        const disposition = res.headers.get('content-disposition') || '';
        const cocok = disposition.match(/filename="?([^";]+)"?/);
        const fileName = cocok ? cocok[1] : `routex_backup.${format}`;
        const objectUrl = URL.createObjectURL(blob);
        const tautan = document.createElement('a');
        tautan.href = objectUrl;
        tautan.download = fileName;
        document.body.appendChild(tautan);
        tautan.click();
        tautan.remove();
        URL.revokeObjectURL(objectUrl);
      },
      restore: async (file: File) => {
        const formData = new FormData();
        formData.append('file', file);
        return request<{ status: string }>('/api/admin/system/backup/restore', {
          method: 'POST',
          body: formData,
        });
      },
    },
  },

  // Integrasi CLI (Fitur 4)
  cli: {
    detected: () => request<CLIDetectedResponse>('/api/admin/cli/detected'),
    configure: (data: CLIConfigureRequest) =>
      request<CLIConfigureResponse>('/api/admin/cli/configure', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    exportScript: () => request<CLIExportScriptResponse>('/api/admin/cli/env-export'),
  },
};
