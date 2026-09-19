// Route-X Domain Types

export interface User {
  id: string;
  email: string;
  display_name: string;
  status: 'active' | 'disabled' | 'locked';
  must_change_password: boolean;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
  roles?: string[];
}

export interface Principal {
  user: User;
  session_id?: string;
  roles: string[];
  permissions: string[];
  session: {
    id: string;
    ip: string;
    user_agent: string;
    expires_at: string;
    created_at: string;
  };
  csrf_token?: string;
  must_change_password?: boolean;
}

export interface Provider {
  id: string;
  name: string;
  display_name: string;
  kind: string;
  base_url: string;
  enabled: boolean;
  priority: number;
  weight: number;
  timeout_ms: number;
  max_retries: number;
  consecutive_failures: number;
  last_health_status?: string;
  last_health_at?: string;
  last_latency_ms?: number;
  rate_limit_rpm?: number;
  rate_limit_tpm?: number;
  max_concurrent?: number;
  egress_pool_id?: string;
  credential_strategy?: 'round_robin' | 'priority';
  created_at: string;
  updated_at: string;
}

export interface Credential {
  id: string;
  provider_id: string;
  label: string;
  masked_hint: string;
  enabled: boolean;
  priority?: number;
  weight: number;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}

export interface OAuthSession {
  id: string;
  provider_id: string;
  credential_id: string;
  account_email: string;
  account_name?: string;
  client_id: string;
  redirect_uri: string;
  scopes: string[];
  last_refreshed_at?: string;
  last_refresh_error?: string;
  enabled: boolean;
  expires_at?: string;
  masked_hint?: string;
  created_at: string;
  updated_at: string;
}

export interface ModelProviderSummary {
  provider_id: string;
  provider_name: string;
  display_name: string;
  upstream_model_name: string;
}

export interface Model {
  id: string;
  model_id: string;
  display_name: string;
  family?: string;
  context_window?: number;
  max_output_tokens?: number;
  capabilities: string[];
  enabled: boolean;
  routing_priority: number;
  routing_strategy?: string;
  created_at: string;
  updated_at: string;
  providers?: ModelProviderSummary[];
}

export interface ModelAlias {
  id: string;
  alias: string;
  model_id: string;
  created_at: string;
}

export interface ProviderModel {
  id: string;
  model_id: string;
  provider_id: string;
  upstream_model_name: string;
  priority?: number;
  weight?: number;
  enabled: boolean;
  created_at: string;
}

export interface Price {
  id: string;
  provider_model_id: string;
  input_per_1m_usd: string;
  output_per_1m_usd: string;
  cached_input_per_1m_usd?: string;
  currency: string;
  effective_from: string;
  created_at: string;
}

export interface EgressPool {
  id: string;
  name: string;
  kind: string;
  masked_hint?: string;
  enabled: boolean;
  weight: number;
  region?: string;
  last_health_status?: 'healthy' | 'unhealthy' | string;
  last_health_at?: string;
  last_latency_ms?: number;
  created_at: string;
  updated_at?: string;
}

export interface EgressProbeResult {
  status: 'healthy' | 'unhealthy';
  latency_ms?: number;
  exit_ip?: string;
  country?: string;
  datacenter?: string;
  checked_at: string;
  message: string;
  error?: string;
}

export interface RoutingRule {
  id: string;
  name: string;
  description?: string;
  priority: number;
  match_model_id?: string;
  match_api_key_id?: string;
  match_capabilities: string[];
  strategy: string;
  max_attempts: number;
  backoff_ms: number;
  failure_threshold: number;
  open_duration_ms: number;
  half_open_probes: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  providers?: {
    provider_id: string;
    position: number;
    weight?: number;
  }[];
}

export interface RateLimit {
  id: string;
  scope: string;
  scope_id: string;
  requests_per_second?: number;
  requests_per_minute?: number;
  tokens_per_minute?: number;
  daily_request_limit?: number;
  monthly_request_limit?: number;
  daily_token_limit?: number;
  monthly_token_limit?: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Budget {
  id: string;
  name: string;
  description?: string;
  scope: string;
  scope_id?: string;
  period: string;
  max_spend_usd: string;
  limit_usd?: string;
  spent_usd: string;
  alert_threshold: number;
  alert_threshold_pct?: number;
  action: string;
  action_on_exceed?: string;
  period_start: string;
  period_end: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ContentFilter {
  id: string;
  name: string;
  description?: string;
  kind: string;
  priority: number;
  applies_to: string;
  action: string;
  pattern?: string;
  pattern_type?: string;
  case_sensitive: boolean;
  max_eval_ms: number;
  model_id?: string;
  provider_id?: string;
  max_request_bytes?: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Ban {
  id: string;
  subject_kind: string;
  subject: string;
  reason: string;
  expires_at?: string;
  created_at: string;
  created_by?: string;
}

export interface CircuitBreakerStatus {
  provider_id: string;
  model: string;
  state: 'closed' | 'open' | 'half-open';
  failure_count: number;
  last_failure_at?: string;
  next_probe_at?: string;
}

export interface APIKey {
  id: string;
  name: string;
  masked_key: string;
  scopes: string[];
  allowed_models: string[];
  allowed_providers: string[];
  rpm_limit?: number;
  tpm_limit?: number;
  rate_limit_rps?: number;
  rate_limit_rpm?: number;
  rate_limit_tpm?: number;
  model_ids?: string[];
  provider_ids?: string[];
  budget_limit_usd?: string;
  enabled: boolean;
  expires_at?: string;
  created_at: string;
  updated_at: string;
  last_used_at?: string;
  raw_key?: string; // Hanya saat pertama dibuat atau dirotasi
}

export interface SetupHintResponse {
  has_default_admin: boolean;
  default_email?: string;
  default_password?: string;
  message?: string;
}

export interface UserDetail {
  user: User;
  roles: string[];
  permissions: string[];
}

export interface Session {
  id: string;
  user_id: string;
  user_email: string;
  user_name: string;
  ip: string;
  user_agent: string;
  expires_at: string;
  created_at: string;
  last_seen_at: string;
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  events: string[];
  masked_hint: string;
  enabled: boolean;
  max_retries: number;
  timeout_ms: number;
  consecutive_failures: number;
  last_delivery_at?: string;
  last_delivery_status?: string;
  created_at: string;
  updated_at: string;
}

export interface WebhookDelivery {
  id: number;
  webhook_id: string;
  event: string;
  status: string;
  attempt_count: number;
  next_attempt_at: string;
  response_status_code?: number;
  error_message?: string;
  created_at: string;
  delivered_at?: string;
}

export interface RequestLog {
  id: string;
  request_id: string;
  provider_id?: string;
  provider_name?: string;
  model_id?: string;
  requested_model?: string;
  api_key_id?: string;
  api_key_name?: string;
  status_code: number;
  duration_ms: number;
  ttft_ms?: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost_usd: string;
  error_type?: string;
  error_message?: string;
  client_ip: string;
  is_stream: boolean;
  created_at: string;
}

// Kontrak mengikuti TrafficEventDTO (internal/admin/dto.go:264): field utama
// label event adalah `kind`, bukan `event_type`.
export interface RequestEvent {
  seq: number;
  kind: string;
  provider_id?: string;
  provider_name?: string;
  upstream_model?: string;
  status_code?: number;
  latency_ms?: number;
  error_kind?: string;
  error_message?: string;
  detail?: unknown;
  created_at: string;
}

// Kontrak mengikuti TrafficPayloadDTO (internal/admin/dto.go:289): bodies dikirim
// sebagai JSON mentah (json.RawMessage), bukan teks prompt/response siap pakai.
export interface RequestPayload {
  request_headers?: unknown;
  request_body?: unknown;
  response_headers?: unknown;
  response_body?: unknown;
  truncated?: boolean;
  size_bytes?: number;
}

export interface ObservabilitySummary {
  window: string;
  total_requests: number;
  success_requests: number;
  error_requests: number;
  error_rate: number;
  total_tokens: number;
  prompt_tokens: number;
  completion_tokens: number;
  total_cost_usd: string;
  avg_latency_ms: number;
  p95_latency_ms: number;
  active_providers: number;
  circuit_breakers_open: number;
}

// Kontrak mengikuti TrafficPointDTO (internal/admin/dto.go:129): biaya dikirim
// sebagai string presisi desimal, BUKAN number — harus dikonversi saat plot.
export interface TimeSeriesPoint {
  timestamp: string;
  requests: number;
  errors: number;
  tokens: number;
  cost_usd: string;
  p50_latency_ms: number;
  p95_latency_ms: number;
}

export interface BreakdownItem {
  id: string;
  name: string;
  requests: number;
  tokens: number;
  cost_usd: string;
  percentage: number;
}

export interface BackgroundJob {
  name: string;
  interval: string;
  last_run_at?: string;
  last_status: 'ok' | 'failed' | 'running';
  last_duration_ms?: number;
  last_error?: string;
}

export interface AuditLogEntry {
  id: number;
  occurred_at: string;
  actor_user_id?: string;
  actor_email?: string;
  actor_role?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  ip?: string;
  user_agent?: string;
  request_id?: string;
  metadata?: Record<string, any>;
}

export interface DiagnosticsMemory {
  alloc_bytes: number;
  total_alloc_bytes: number;
  sys_bytes: number;
  num_gc: number;
}

export interface Diagnostics {
  version?: string;
  commit?: string;
  built_at?: string;
  uptime_seconds: number;
  go_version: string;
  num_goroutine: number;
  num_cpu?: number;
  memory?: DiagnosticsMemory;
  memory_allocated_mb?: number;
  memory_total_alloc_mb?: number;
  memory_sys_mb?: number;
  num_gc?: number;
  db_pool?: {
    total_conns: number;
    idle_conns: number;
    acquired_conns: number;
    max_conns: number;
  };
}

export interface SystemOverview {
  pid: number;
  os_arch: string;
  uptime_seconds: number;
  go_version: string;
  in_flight_requests: number;
  process_rss_bytes: number;
  host_ram_total_bytes: number;
  host_ram_used_bytes: number;
  container_ram_bytes: number;
  go_heap_bytes: number;
  go_sys_bytes: number;
  stack_inuse_bytes: number;
  num_gc: number;
  net_total_bytes: number;
  net_recv_bytes: number;
  net_sent_bytes: number;
  net_rate_mb_s: number;
  net_recv_rate_mb_s: number;
  net_sent_rate_mb_s: number;
  egress_total_routes: number;
  egress_xray_count: number;
  egress_http_count: number;
  egress_active_mode: string;
  container_cpu_cap: number;
  proxy_cpu_pct: number;
  host_cpu_pct: number;
  num_goroutine: number;
}

export interface ResponseCacheStats {
  enabled: boolean;
  ttl_seconds: number;
  hits: number;
  misses: number;
  total_entries: number;
}

export interface CLITool {
  id: string;
  name: string;
  description: string;
  category: string;
  installed: boolean;
  path?: string;
  version?: string;
  config_path?: string;
  supported_modes: string[];
  active_mode: 'model_only' | 'routing' | 'combo';
  active_target: string;
  env_vars: Record<string, string>;
  env_var_api_key?: string;
  export_snippet: string;
  updated_at?: string;
}

export interface CLIDetectedResponse {
  tools: CLITool[];
  total_detected: number;
  total_available: number;
  gateway_url: string;
  default_env_file: string;
}

export interface CLIConfigureRequest {
  tool_id: string;
  mode: 'model_only' | 'routing' | 'combo';
  target: string;
  api_key?: string;
  gateway_url?: string;
}

export interface CLIConfigureResponse {
  success: boolean;
  tool: CLITool;
  message: string;
  config_file?: string;
  export_snippet: string;
}

export interface CLIExportScriptResponse {
  content: string;
  file_path: string;
}

export interface XrayProtocolItem {
  id: string;
  name: string;
  protocol: string;
  transport: string;
  security: string;
  port: number;
  path_or_sni: string;
  share_link: string;
  egress_url: string;
  description: string;
}

export interface DomainConfig {
  domain: string;
  mode: 'letsencrypt' | 'cloudflare';
  status: 'unconfigured' | 'verifying' | 'active' | 'warning' | 'error';
  public_url: string;
  base_url: string;
  server_ip: string;
  resolved_ips?: string[];
  dns_matched: boolean;
  last_checked?: string;
  message?: string;
  xray_enabled?: boolean;
  xray_uuid?: string;
  xray_vless_reality?: string;
  xray_protocols?: XrayProtocolItem[];
}

export interface DomainUpdateRequest {
  domain: string;
  mode: 'letsencrypt' | 'cloudflare';
}

export interface BackupStatusResponse {
  database_name: string;
  database_size: string;
  database_bytes: number;
  total_tables: number;
  models_count: number;
  providers_count: number;
  routing_rules_count: number;
  api_keys_count: number;
  last_server_backup?: {
    file_name: string;
    file_size: string;
    created_at: string;
  };
}



