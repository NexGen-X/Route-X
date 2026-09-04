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
    try {
      sessionStorage.removeItem('routex_csrf');
    } catch {}
    return;
  }
  cachedCsrfToken = token;
  try {
    sessionStorage.setItem('routex_csrf', token);
  } catch {}
}

// Ambil CSRF token dari cookie routex_csrf atau fallback cache / sessionStorage
export function getCsrfToken(): string {
  const match = document.cookie.match(/(?:^|;\s*)(?:__Host-)?routex_csrf=([^;]+)/);
  if (match) return decodeURIComponent(match[1]);
  if (cachedCsrfToken) return cachedCsrfToken;
  try {
    return sessionStorage.getItem('routex_csrf') || '';
  } catch {
    return '';
  }
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
  });

  if (res.status === 204) {
    return {} as T;
  }

  let data: any;
  const contentType = res.headers.get('content-type') || '';
  if (contentType.includes('application/json')) {
    data = await res.json();
  } else {
    data = await res.text();
  }

  // Tangkap CSRF token jika dikembalikan di respons
  if (data && typeof data === 'object' && typeof data.csrf_token === 'string') {
    setCsrfToken(data.csrf_token);
  }

  if (!res.ok) {
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
      request<ObservabilitySummary>(`/api/admin/observability/summary?window=${window}`),
    series: (metric = 'requests', window = '24h') =>
      request<{ points: TimeSeriesPoint[] }>(
        `/api/admin/observability/series?metric=${metric}&window=${window}`
      ),
    breakdown: (by = 'provider', window = '24h') =>
      request<{ items: BreakdownItem[] }>(
        `/api/admin/observability/breakdown?by=${by}&window=${window}`
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
    get: (id: string) => request<RequestLog>(`/api/admin/requests/${id}`),
    events: (id: string) => request<{ events: RequestEvent[] }>(`/api/admin/requests/${id}/events`),
    payload: (id: string) => request<RequestPayload>(`/api/admin/requests/${id}/payload`),
  },

  // Upstreams
  providers: {
    list: () => request<{ items: Provider[] }>('/api/admin/upstreams/providers'),
    get: (id: string) => request<Provider>(`/api/admin/upstreams/providers/${id}`),
    create: (data: Partial<Provider>) =>
      request<Provider>('/api/admin/upstreams/providers', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<Provider>) =>
      request<Provider>(`/api/admin/upstreams/providers/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/upstreams/providers/${id}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<Provider>(`/api/admin/upstreams/providers/${id}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    probe: (id: string) =>
      request<{ status: string; latency_ms: number; error?: string }>(
        `/api/admin/upstreams/providers/${id}/probe`,
        { method: 'POST' }
      ),
    healthChecks: (id: string) =>
      request<{ items: any[] }>(`/api/admin/upstreams/providers/${id}/health-checks`),
  },

  credentials: {
    list: (providerId: string) =>
      request<{ items: Credential[] }>(`/api/admin/upstreams/providers/${providerId}/credentials`),
    create: (providerId: string, data: { label: string; api_key: string; expires_at?: string }) =>
      request<Credential>(`/api/admin/upstreams/providers/${providerId}/credentials`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    delete: (providerId: string, id: string) =>
      request<void>(`/api/admin/upstreams/providers/${providerId}/credentials/${id}`, {
        method: 'DELETE',
      }),
  },

  models: {
    list: () => request<{ items: Model[] }>('/api/admin/upstreams/models'),
    get: (id: string) =>
      request<{ model: Model; aliases: ModelAlias[]; mappings: ProviderModel[] }>(
        `/api/admin/upstreams/models/${id}`
      ),
    create: (data: Partial<Model>) =>
      request<Model>('/api/admin/upstreams/models', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<Model>) =>
      request<Model>(`/api/admin/upstreams/models/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/upstreams/models/${id}`, { method: 'DELETE' }),
    addAlias: (id: string, alias: string) =>
      request<ModelAlias>(`/api/admin/upstreams/models/${id}/aliases`, {
        method: 'POST',
        body: JSON.stringify({ alias }),
      }),
    deleteAlias: (id: string, alias: string) =>
      request<void>(`/api/admin/upstreams/models/${id}/aliases/${alias}`, {
        method: 'DELETE',
      }),
    attachProvider: (id: string, data: Partial<ProviderModel>) =>
      request<ProviderModel>(`/api/admin/upstreams/models/${id}/mappings`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    detachProvider: (_id: string, mappingId: string) =>
      request<void>(`/api/admin/upstreams/models/mappings/${mappingId}`, {
        method: 'DELETE',
      }),
    setPrice: (mappingId: string, data: { input_per_1m_usd: string; output_per_1m_usd: string; cached_input_per_1m_usd?: string }) =>
      request<Price>(`/api/admin/upstreams/models/mappings/${mappingId}/pricing`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    pricingHistory: (mappingId: string) =>
      request<{ items: Price[] }>(`/api/admin/upstreams/models/mappings/${mappingId}/pricing/history`),
  },

  egress: {
    list: () => request<{ items: EgressPool[] }>('/api/admin/upstreams/egress-pools'),
    create: (data: Partial<EgressPool> & { proxy_url: string }) =>
      request<EgressPool>('/api/admin/upstreams/egress-pools', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<EgressPool> & { proxy_url?: string }) =>
      request<EgressPool>(`/api/admin/upstreams/egress-pools/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/upstreams/egress-pools/${id}`, { method: 'DELETE' }),
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
      request<RoutingRule>(`/api/admin/gateway/routing-rules/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/routing-rules/${id}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<RoutingRule>(`/api/admin/gateway/routing-rules/${id}/toggle`, {
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
      request<RateLimit>(`/api/admin/gateway/rate-limits/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/rate-limits/${id}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<RateLimit>(`/api/admin/gateway/rate-limits/${id}/toggle`, {
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
      request<Budget>(`/api/admin/gateway/budgets/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/budgets/${id}`, { method: 'DELETE' }),
    reset: (id: string) =>
      request<Budget>(`/api/admin/gateway/budgets/${id}/reset`, { method: 'POST' }),
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
      request<ContentFilter>(`/api/admin/gateway/content-filters/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/gateway/content-filters/${id}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<ContentFilter>(`/api/admin/gateway/content-filters/${id}/toggle`, {
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
      request<Ban>(`/api/admin/gateway/bans/${id}/lift`, { method: 'POST' }),
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
    get: (id: string) => request<APIKey>(`/api/admin/access/api-keys/${id}`),
    create: (data: Partial<APIKey>) =>
      request<APIKey>('/api/admin/access/api-keys', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<APIKey>) =>
      request<APIKey>(`/api/admin/access/api-keys/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    rotate: (id: string) =>
      request<APIKey>(`/api/admin/access/api-keys/${id}/rotate`, { method: 'POST' }),
    revoke: (id: string, reason?: string) =>
      request<void>(`/api/admin/access/api-keys/${id}/revoke`, {
        method: 'POST',
        body: JSON.stringify({ reason: reason || 'dicabut oleh admin' }),
      }),
    toggle: (id: string, enabled: boolean) =>
      request<APIKey>(`/api/admin/access/api-keys/${id}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
  },

  users: {
    list: (params?: Record<string, any>) => {
      const q = new URLSearchParams(params || {});
      return request<{ items: User[]; next_cursor?: string }>(`/api/admin/access/users?${q.toString()}`);
    },
    get: (id: string) => request<User>(`/api/admin/access/users/${id}`),
    create: (data: { email: string; name: string; password?: string; role_ids: string[]; must_change_password?: boolean }) =>
      request<User>('/api/admin/access/users', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: { name?: string; status?: string }) =>
      request<User>(`/api/admin/access/users/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    forceResetPassword: (id: string, tempPassword?: string) =>
      request<{ temp_password: string }>(`/api/admin/access/users/${id}/force-reset-password`, {
        method: 'POST',
        body: JSON.stringify({ temp_password: tempPassword }),
      }),
    revokeSessions: (id: string) =>
      request<void>(`/api/admin/access/users/${id}/revoke-sessions`, { method: 'POST' }),
  },

  roles: {
    list: () => request<{ items: Role[] }>('/api/admin/access/roles'),
    permissions: () => request<{ items: { key: string; description: string }[] }>('/api/admin/access/permissions'),
    setPermissions: (roleId: string, permissions: string[]) =>
      request<Role>(`/api/admin/access/roles/${roleId}/permissions`, {
        method: 'PUT',
        body: JSON.stringify({ permissions }),
      }),
  },

  sessions: {
    list: () => request<{ items: Session[] }>('/api/admin/access/sessions'),
    revoke: (id: string) =>
      request<void>(`/api/admin/access/sessions/${id}`, { method: 'DELETE' }),
  },

  // Automation
  webhooks: {
    list: () => request<{ items: Webhook[] }>('/api/admin/automation/webhooks'),
    get: (id: string) => request<Webhook>(`/api/admin/automation/webhooks/${id}`),
    create: (data: { name: string; url: string; events: string[]; secret?: string; enabled?: boolean }) =>
      request<Webhook>('/api/admin/automation/webhooks', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: string, data: Partial<Webhook> & { secret?: string }) =>
      request<Webhook>(`/api/admin/automation/webhooks/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: string) =>
      request<void>(`/api/admin/automation/webhooks/${id}`, { method: 'DELETE' }),
    toggle: (id: string, enabled: boolean) =>
      request<Webhook>(`/api/admin/automation/webhooks/${id}/toggle`, {
        method: 'POST',
        body: JSON.stringify({ enabled }),
      }),
    test: (id: string) =>
      request<{ status: number; duration_ms: number; response?: string }>(
        `/api/admin/automation/webhooks/${id}/test`,
        { method: 'POST' }
      ),
    deliveries: (webhookId: string) =>
      request<{ items: WebhookDelivery[] }>(`/api/admin/automation/webhooks/${webhookId}/deliveries`),
  },

  // System
  system: {
    settings: () => request<{ items: { key: string; value: string; description?: string }[] }>('/api/admin/system/settings'),
    updateSetting: (key: string, value: string) =>
      request<void>(`/api/admin/system/settings/${key}`, {
        method: 'PUT',
        body: JSON.stringify({ value }),
      }),
    jobs: () => request<{ items: BackgroundJob[] }>('/api/admin/system/jobs'),
    triggerJob: (name: string) =>
      request<{ status: string; job: string; message: string }>(`/api/admin/system/jobs/${name}/run`, { method: 'POST' }),
    auditLogs: (params?: Record<string, any>) => {
      const q = new URLSearchParams(params || {});
      return request<{ items: AuditLogEntry[]; next_cursor?: string }>(`/api/admin/system/audit-logs?${q.toString()}`);
    },
    diagnostics: () => request<Diagnostics>('/api/admin/system/diagnostics'),
  },
};
