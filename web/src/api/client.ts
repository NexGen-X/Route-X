// Route-X API Client
import type {
  Principal,
  Provider,
  Credential,
  Model,
  ModelAlias,
  ProviderModel,
  Price,
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
  Role,
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

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (!headers.has('Content-Type') && options.body && typeof options.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }

  // Double-submit CSRF protection untuk metode mutasi
  const method = (options.method || 'GET').toUpperCase();
  if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
    const csrf = getCsrfToken();
    if (csrf) {
      headers.set('X-CSRF-Token', csrf);
    }
  }

  const res = await fetch(path, {
    ...options,
    headers,
    credentials: 'same-origin',
  }).catch(() => {
    // Jaringan mati/DNS gagal/CORS: fetch melempar TypeError mentah. Bungkus
    // menjadi ApiError agar seluruh pemanggil mendapat bentuk error yang sama
    // dan bisa membedakan gangguan jaringan dari respons server.
    throw new ApiError(0, 'network_error', 'Tidak dapat menghubungi server Route-X. Periksa koneksi jaringan.');
  });

  if (res.status === 204) {
    return {} as T;
  }

  let data: any;
  const contentType = res.headers.get('content-type') || '';
  if (contentType.includes('application/json')) {
    try {
      data = await res.json();
    } catch {
      throw new ApiError(res.status, 'invalid_json_response', 'Respons server bukan JSON yang valid.');
    }
  } else {
    data = await res.text();
  }

  // Tangkap CSRF token jika dikembalikan di respons
  if (data && typeof data === 'object' && typeof data.csrf_token === 'string') {
    setCsrfToken(data.csrf_token);
  }

  if (!res.ok) {
    // Penanganan 401 global: beri tahu AuthContext agar logout, kecuali untuk
    // endpoint identitas/login sendiri supaya tidak memicu loop logout.
    if (
      res.status === 401 &&
      path !== '/api/auth/me' &&
      path !== '/api/auth/login'
    ) {
      window.dispatchEvent(new CustomEvent('routex:unauthorized'));
    }
    const errObj = data?.error || {};
    throw new ApiError(
      res.status,
      errObj.code || 'unknown_error',
      errObj.message || (typeof data === 'string' ? data : 'Terjadi kesalahan sistem.'),
      errObj.param
    );
  }

  return data as T;
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
    liveMetrics: () => request<any>('/api/admin/observability/metrics/live'),
  },

  // Requests
  requests: {
    list: (params: Record<string, any>) => {
      const q = new URLSearchParams();
      Object.entries(params).forEach(([k, v]) => {
        if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
      });
      return request<{ items: RequestLog[]; next_cursor?: string }>(`/api/admin/requests?${q.toString()}`);
    },
    get: (id: string) => request<RequestLog>(`/api/admin/requests/${encodeURIComponent(id)}`),
    events: (id: string) => request<{ events: RequestEvent[] }>(`/api/admin/requests/${encodeURIComponent(id)}/events`),
    payload: (id: string) => request<RequestPayload>(`/api/admin/requests/${encodeURIComponent(id)}/payload`),
  },

  // Upstreams
  providers: {
    list: () => request<{ items: Provider[] }>('/api/admin/upstreams/providers'),
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
      request<{ items: any[] }>(`/api/admin/upstreams/providers/${encodeURIComponent(id)}/health-checks`),
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
  },

  credentials: {
    list: (providerId: string) =>
      request<{ items: Credential[] }>(`/api/admin/upstreams/providers/${encodeURIComponent(providerId)}/credentials`),
    create: (providerId: string, data: { label: string; api_key: string; expires_at?: string }) =>
      request<Credential>(`/api/admin/upstreams/providers/${encodeURIComponent(providerId)}/credentials`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
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
    list: () => request<{ items: Model[] }>('/api/admin/upstreams/models'),
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
  },

  // Gateway Policies
  routing: {
    list: () => request<{ items: RoutingRule[] }>('/api/admin/gateway/routing-rules'),
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
    list: (params?: Record<string, any>) => {
      const q = new URLSearchParams(params || {});
      return request<{ items: APIKey[]; next_cursor?: string }>(`/api/admin/access/api-keys?${q.toString()}`);
    },
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
    revoke: (id: string, reason?: string) =>
      request<void>(`/api/admin/access/api-keys/${encodeURIComponent(id)}/revoke`, {
        method: 'POST',
        body: JSON.stringify({ reason: reason || 'dicabut oleh admin' }),
      }),
    toggle: (id: string, enabled: boolean) =>
      request<APIKey>(`/api/admin/access/api-keys/${encodeURIComponent(id)}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
  },

  users: {
    list: (params?: Record<string, any>) => {
      const q = new URLSearchParams(params || {});
      return request<{ items: User[]; next_cursor?: string }>(`/api/admin/access/users?${q.toString()}`);
    },
    get: (id: string) => request<User>(`/api/admin/access/users/${encodeURIComponent(id)}`),
    create: (data: { email: string; name: string; password?: string; role_ids: string[]; must_change_password?: boolean }) =>
      request<User>('/api/admin/access/users', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: { name?: string; status?: string }) =>
      request<User>(`/api/admin/access/users/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    forceResetPassword: (id: string, tempPassword?: string) =>
      request<{ temp_password: string }>(`/api/admin/access/users/${encodeURIComponent(id)}/force-reset-password`, {
        method: 'POST',
        body: JSON.stringify({ temp_password: tempPassword }),
      }),
    revokeSessions: (id: string) =>
      request<void>(`/api/admin/access/users/${encodeURIComponent(id)}/revoke-sessions`, { method: 'POST' }),
  },

  roles: {
    list: () => request<{ items: Role[] }>('/api/admin/access/roles'),
    permissions: () => request<{ items: { key: string; description: string }[] }>('/api/admin/access/permissions'),
    setPermissions: (roleId: string, permissions: string[]) =>
      request<Role>(`/api/admin/access/roles/${encodeURIComponent(roleId)}/permissions`, {
        method: 'PUT',
        body: JSON.stringify({ permissions }),
      }),
  },

  sessions: {
    list: () => request<{ items: Session[] }>('/api/admin/access/sessions'),
    revoke: (id: string) =>
      request<void>(`/api/admin/access/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
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
      request<{ status: number; duration_ms: number; response?: string }>(
        `/api/admin/automation/webhooks/${encodeURIComponent(id)}/test`,
        { method: 'POST' }
      ),
    deliveries: (webhookId: string) =>
      request<{ items: WebhookDelivery[] }>(`/api/admin/automation/webhooks/${encodeURIComponent(webhookId)}/deliveries`),
  },

  // System
  system: {
    settings: () => request<{ items: { key: string; value: string; description?: string }[] }>('/api/admin/system/settings'),
    updateSetting: (key: string, value: string) =>
      request<void>(`/api/admin/system/settings/${encodeURIComponent(key)}`, {
        method: 'PUT',
        body: JSON.stringify({ value }),
      }),
    jobs: () => request<{ items: BackgroundJob[] }>('/api/admin/system/jobs'),
    triggerJob: (name: string) =>
      request<{ status: string; job: string; message: string }>(`/api/admin/system/jobs/${encodeURIComponent(name)}/run`, { method: 'POST' }),
    auditLogs: (params?: Record<string, any>) => {
      const q = new URLSearchParams(params || {});
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
  },

  // CLI Integrations (Fitur 4)
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
