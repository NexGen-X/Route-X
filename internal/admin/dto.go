// Package admin menyediakan endpoint REST API administratif untuk Route-X.
// File ini mendefinisikan Data Transfer Object (DTO) untuk menjamin isolasi kontrak API publik
// dari skema dan struct internal repository database.
package admin

import (
	"encoding/json"
	"net/netip"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// dto adalah antarmuka penanda (marker interface) unexported.
// Penegakan dilakukan oleh compiler: hanya tipe struct yang dideklarasikan di paket admin
// dan secara eksplisit mengimplementasikan adalahDTO() yang dapat diteruskan ke respond().
// Keputusan ini mencegah pengembang di masa depan secara tidak sengaja mengembalikan struct repository.
type dto interface {
	adalahDTO()
}

// -----------------------------------------------------------------------------
// Envelope & Respons Umum
// -----------------------------------------------------------------------------

// ListEnvelope adalah pembungkus standar daftar data dengan paginasi kursor opsional.
type ListEnvelope[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func (ListEnvelope[T]) adalahDTO() {}

// StatusResponse adalah respons status sederhana untuk mutasi delete/lift/toggle.
type StatusResponse struct {
	Status string `json:"status"`
}

func (StatusResponse) adalahDTO() {}

// StatusCountResponse adalah respons mutasi yang memuat status dan jumlah entitas terdampak.
type StatusCountResponse struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

func (StatusCountResponse) adalahDTO() {}

// JobRunResponse adalah respons saat pemicuan background job manual berhasil.
type JobRunResponse struct {
	Status  string `json:"status"`
	Job     string `json:"job"`
	Message string `json:"message"`
}

func (JobRunResponse) adalahDTO() {}

// WebhookPingResponse adalah respons hasil tes koneksi langsung ke endpoint webhook.
type WebhookPingResponse struct {
	Status     string `json:"status"`
	StatusCode int    `json:"status_code,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Enqueued   int    `json:"enqueued,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (WebhookPingResponse) adalahDTO() {}

// ProbeProviderResponse adalah hasil pemeriksaan diagnostik langsung (probe) upstream provider.
type ProbeProviderResponse struct {
	Status    string `json:"status"`
	LatencyMS int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func (ProbeProviderResponse) adalahDTO() {}

// -----------------------------------------------------------------------------
// Domain Observability & Traffic
// -----------------------------------------------------------------------------

// DistributionDTO merangkum sebaran statistik dan persentil latensi milidetik.
// Persentil bertipe pointer float64: nil berarti tidak ada sampel pada periode tersebut (bukan 0 ms).
type DistributionDTO struct {
	Count int64    `json:"count"`
	SumMS int64    `json:"sum_ms"`
	MaxMS *int     `json:"max_ms"`
	P50   *float64 `json:"p50"`
	P90   *float64 `json:"p90"`
	P95   *float64 `json:"p95"`
	P99   *float64 `json:"p99"`
}

// TrafficStatsDTO adalah ringkasan performa dan volume lalu lintas untuk dashboard overview.
type TrafficStatsDTO struct {
	TotalRequests     int64           `json:"total_requests"`
	SuccessRequests   int64           `json:"success_requests"`
	ErrorRequests     int64           `json:"error_requests"`
	Timeouts          int64           `json:"timeouts"`
	Retries           int64           `json:"retries"`
	Failovers         int64           `json:"failovers"`
	TotalTokens       int64           `json:"total_tokens"`
	PromptTokens      int64           `json:"prompt_tokens"`
	CompletionTokens  int64           `json:"completion_tokens"`
	CachedInputTokens int64           `json:"cached_input_tokens"`
	ReasoningTokens   int64           `json:"reasoning_tokens"`
	TotalCostUSD      string          `json:"total_cost_usd"`
	CostUSD           string          `json:"cost_usd"`
	AvgLatencyMS      float64         `json:"avg_latency_ms"`
	P95LatencyMS      *float64        `json:"p95_latency_ms"`
	ErrorRate         float64         `json:"error_rate"`
	Availability      float64         `json:"availability"`
	From              time.Time       `json:"from"`
	To                time.Time       `json:"to"`
	Source            string          `json:"source"`
	Latency           DistributionDTO `json:"latency"`
	TTFT              DistributionDTO `json:"ttft"`
}

func (TrafficStatsDTO) adalahDTO() {}

// TrafficPointDTO mewakili satu titik data agregat pada grafik deret waktu.
type TrafficPointDTO struct {
	Timestamp    string          `json:"timestamp"`
	Bucket       time.Time       `json:"bucket"`
	Requests     int64           `json:"requests"`
	Success      int64           `json:"success"`
	Errors       int64           `json:"errors"`
	TotalTokens  int64           `json:"total_tokens"`
	Tokens       int64           `json:"tokens"`
	CostUSD      string          `json:"cost_usd"`
	Latency      DistributionDTO `json:"latency"`
	P50LatencyMS *float64        `json:"p50_latency_ms"`
	P95LatencyMS *float64        `json:"p95_latency_ms"`
}

// TrafficSeriesResponse adalah respons grafik deret waktu yang mendukung pembacaan items maupun points.
type TrafficSeriesResponse struct {
	Items  []TrafficPointDTO `json:"items"`
	Points []TrafficPointDTO `json:"points"`
}

func (TrafficSeriesResponse) adalahDTO() {}

// TrafficSliceDTO adalah satu irisan data breakdown pemakaian per provider/model/kunci.
type TrafficSliceDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Requests    int64           `json:"requests"`
	Success     int64           `json:"success"`
	Errors      int64           `json:"errors"`
	TotalTokens int64           `json:"total_tokens"`
	CostUSD     string          `json:"cost_usd"`
	Percentage  float64         `json:"percentage"`
	Latency     DistributionDTO `json:"latency"`
}

// TrafficBreakdownResponse adalah respons breakdown pemakaian berdasarkan dimensi tertentu.
type TrafficBreakdownResponse struct {
	Dimension string            `json:"dimension"`
	Items     []TrafficSliceDTO `json:"items"`
}

func (TrafficBreakdownResponse) adalahDTO() {}

// ProviderHealthDTO memuat status kesehatan upstream provider.
type ProviderHealthDTO struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	LastError     *string `json:"last_error,omitempty"`
	LastCheckedAt *string `json:"last_checked_at,omitempty"`
}

// HealthSummaryResponse adalah respons daftar kesehatan seluruh provider.
type HealthSummaryResponse struct {
	Providers []ProviderHealthDTO `json:"providers"`
}

func (HealthSummaryResponse) adalahDTO() {}

// DBPoolStatsDTO menyajikan telemetri koneksi pgxpool PostgreSQL.
type DBPoolStatsDTO struct {
	TotalConns       int32 `json:"total_conns"`
	IdleConns        int32 `json:"idle_conns"`
	AcquiredConns    int32 `json:"acquired_conns"`
	MaxConns         int32 `json:"max_conns"`
	AcquireCount     int64 `json:"acquire_count"`
	EmptyAcquireWait int64 `json:"empty_acquire_wait"`
}

// RedisStatsDTO menyajikan status ping dan konektivitas Redis.
type RedisStatsDTO struct {
	Status string `json:"status"`
	Ping   string `json:"ping"`
}

// LiveMetricsResponse adalah metrik real-time sistem database dan cache.
type LiveMetricsResponse struct {
	DBPool DBPoolStatsDTO `json:"db_pool"`
	Redis  *RedisStatsDTO `json:"redis,omitempty"`
}

func (LiveMetricsResponse) adalahDTO() {}

// TrafficRowDTO adalah ringkasan satu entri log inferensi HTTP.
type TrafficRowDTO struct {
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	RequestID        string    `json:"request_id"`
	APIKeyID         string    `json:"api_key_id"`
	APIKeyName       string    `json:"api_key_name"`
	Method           string    `json:"method"`
	Endpoint         string    `json:"endpoint"`
	RequestedModel   string    `json:"requested_model"`
	ModelName        string    `json:"model_name"`
	ProviderName     string    `json:"provider_name"`
	UpstreamModel    string    `json:"upstream_model"`
	StatusCode       int       `json:"status_code"`
	ErrorType        string    `json:"error_type"`
	ErrorCode        string    `json:"error_code"`
	Stream           bool      `json:"stream"`
	IsStream         bool      `json:"is_stream"`
	InputTokens      int       `json:"input_tokens"`
	PromptTokens     int       `json:"prompt_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	CostUSD          string    `json:"cost_usd"`
	LatencyMS        int       `json:"latency_ms"`
	DurationMS       int       `json:"duration_ms"`
	TTFTMS           *int      `json:"ttft_ms"`
	RetryCount       int       `json:"retry_count"`
	FailoverCount    int       `json:"failover_count"`
}

func (TrafficRowDTO) adalahDTO() {}

// TrafficDetailDTO adalah inspeksi mendalam satu permintaan inferensi AI.
type TrafficDetailDTO struct {
	TrafficRowDTO
	UserID            string          `json:"user_id"`
	ModelID           string          `json:"model_id"`
	ProviderID        string          `json:"provider_id"`
	ErrorMessage      string          `json:"error_message"`
	CachedInputTokens int             `json:"cached_input_tokens"`
	ReasoningTokens   int             `json:"reasoning_tokens"`
	UpstreamLatencyMS *int            `json:"upstream_latency_ms"`
	RoutingStrategy   string          `json:"routing_strategy"`
	RoutingDecision   json.RawMessage `json:"routing_decision"`
	ClientIP          string          `json:"client_ip"`
	UserAgent         string          `json:"user_agent"`
}

func (TrafficDetailDTO) adalahDTO() {}

// TrafficEventDTO adalah rekaman satu peristiwa dalam siklus hidup percobaan request.
type TrafficEventDTO struct {
	Seq           int             `json:"seq"`
	Kind          string          `json:"kind"`
	ProviderID    *string         `json:"provider_id"`
	ProviderName  *string         `json:"provider_name"`
	UpstreamModel *string         `json:"upstream_model"`
	StatusCode    *int            `json:"status_code"`
	LatencyMS     *int            `json:"latency_ms"`
	ErrorKind     *string         `json:"error_kind"`
	ErrorMessage  *string         `json:"error_message"`
	Detail        json.RawMessage `json:"detail"`
	CreatedAt     time.Time       `json:"created_at"`
}

func (TrafficEventDTO) adalahDTO() {}

// TrafficEventsResponse adalah respons daftar timeline event untuk satu request.
type TrafficEventsResponse struct {
	Items  []TrafficEventDTO `json:"items"`
	Events []TrafficEventDTO `json:"events"`
}

func (TrafficEventsResponse) adalahDTO() {}

// TrafficPayloadDTO memuat rekaman raw HTTP headers dan bodies (request & response).
type TrafficPayloadDTO struct {
	RequestHeaders  json.RawMessage `json:"request_headers"`
	RequestBody     json.RawMessage `json:"request_body"`
	ResponseHeaders json.RawMessage `json:"response_headers"`
	ResponseBody    json.RawMessage `json:"response_body"`
	Truncated       bool            `json:"truncated"`
	SizeBytes       int             `json:"size_bytes"`
}

func (TrafficPayloadDTO) adalahDTO() {}

// -----------------------------------------------------------------------------
// Domain Upstreams & Models
// -----------------------------------------------------------------------------

// ProviderDTO adalah konfigurasi upstream provider AI.
type ProviderDTO struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	DisplayName         string          `json:"display_name"`
	Kind                string          `json:"kind"`
	BaseURL             string          `json:"base_url"`
	Enabled             bool            `json:"enabled"`
	Priority            int             `json:"priority"`
	Weight              int             `json:"weight"`
	TimeoutMS           int             `json:"timeout_ms"`
	MaxRetries          int             `json:"max_retries"`
	RateLimitRPM        *int            `json:"rate_limit_rpm"`
	RateLimitTPM        *int            `json:"rate_limit_tpm"`
	MaxConcurrent       *int            `json:"max_concurrent"`
	EgressPoolID        *string         `json:"egress_pool_id"`
	IsBYOK              bool            `json:"is_byok"`
	OwnerUserID         *string         `json:"owner_user_id"`
	LastHealthStatus    *string         `json:"last_health_status"`
	LastHealthAt        *time.Time      `json:"last_health_at"`
	LastLatencyMS       *int            `json:"last_latency_ms"`
	ConsecutiveFailures int             `json:"consecutive_failures"`
	Metadata            json.RawMessage `json:"metadata"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	CreatedBy           *string         `json:"created_by"`
}

func (ProviderDTO) adalahDTO() {}

// HealthCheckDTO adalah riwayat hasil uji kesehatan provider.
type HealthCheckDTO struct {
	ID           int64     `json:"id"`
	ProviderID   string    `json:"provider_id"`
	Status       string    `json:"status"`
	LatencyMS    *int      `json:"latency_ms"`
	StatusCode   *int      `json:"status_code"`
	ErrorKind    *string   `json:"error_kind"`
	ErrorMessage *string   `json:"error_message"`
	CheckedAt    time.Time `json:"checked_at"`
	CreatedAt    time.Time `json:"created_at"`
}

func (HealthCheckDTO) adalahDTO() {}

// CredentialMetaDTO adalah metadata kredensial provider.
// Catatan Keamanan: encryption_key_id dan ciphertext DILARANG dipublikasikan ke API admin.
type CredentialMetaDTO struct {
	ID           string     `json:"id"`
	ProviderID   string     `json:"provider_id"`
	Label        string     `json:"label"`
	MaskedHint   string     `json:"masked_hint"`
	Enabled      bool       `json:"enabled"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	AuthFailures int        `json:"auth_failures"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CreatedBy    *string    `json:"created_by"`
}

func (CredentialMetaDTO) adalahDTO() {}

// ModelProviderSummaryDTO merangkum informasi provider upstream yang menyediakan sebuah model.
type ModelProviderSummaryDTO struct {
	ProviderID        string `json:"provider_id"`
	ProviderName      string `json:"provider_name"`
	DisplayName       string `json:"display_name"`
	UpstreamModelName string `json:"upstream_model_name"`
}

// ModelDTO adalah entri katalog model kanonik di registry.
type ModelDTO struct {
	ID              string                    `json:"id"`
	ModelID         string                    `json:"model_id"`
	DisplayName     string                    `json:"display_name"`
	Family          *string                   `json:"family"`
	ContextWindow   *int                      `json:"context_window"`
	MaxOutputTokens *int                      `json:"max_output_tokens"`
	Capabilities    []string                  `json:"capabilities"`
	Enabled         bool                      `json:"enabled"`
	RoutingPriority int                       `json:"routing_priority"`
	RoutingStrategy *string                   `json:"routing_strategy"`
	DeprecatedAt    *time.Time                `json:"deprecated_at"`
	Metadata        json.RawMessage           `json:"metadata"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
	Providers       []ModelProviderSummaryDTO `json:"providers"`
}

func (ModelDTO) adalahDTO() {}

// ModelAliasDTO adalah alias virtual nama model.
type ModelAliasDTO struct {
	ID        string    `json:"id"`
	ModelID   string    `json:"model_id"`
	Alias     string    `json:"alias"`
	CreatedAt time.Time `json:"created_at"`
}

func (ModelAliasDTO) adalahDTO() {}

// ModelMappingDTO adalah pemetaan keterhubungan antara model kanonik dan target model provider upstream.
type ModelMappingDTO struct {
	ID                string    `json:"id"`
	ModelID           string    `json:"model_id"`
	ProviderID        string    `json:"provider_id"`
	UpstreamModelName string    `json:"upstream_model_name"`
	Priority          *int      `json:"priority"`
	Weight            *int      `json:"weight"`
	MaxContextWindow  *int      `json:"max_context_window,omitempty"`
	SupportsStreaming bool      `json:"supports_streaming"`
	SupportsTools     bool      `json:"supports_tools"`
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (ModelMappingDTO) adalahDTO() {}

// ModelDetailResponse adalah detail model kanonik lengkap beserta alias dan mapping provider.
type ModelDetailResponse struct {
	Model    ModelDTO          `json:"model"`
	Aliases  []ModelAliasDTO   `json:"aliases"`
	Mappings []ModelMappingDTO `json:"mappings"`
}

func (ModelDetailResponse) adalahDTO() {}

// PriceDTO adalah penetapan harga per 1M token dalam string desimal moneter eksak.
type PriceDTO struct {
	ID                  string     `json:"id"`
	ProviderModelID     string     `json:"provider_model_id"`
	InputPer1MUSD       string     `json:"input_per_1m_usd"`
	OutputPer1MUSD      string     `json:"output_per_1m_usd"`
	CachedInputPer1MUSD *string    `json:"cached_input_per_1m_usd"`
	ReasoningPer1MUSD   *string    `json:"reasoning_per_1m_usd"`
	Currency            string     `json:"currency"`
	EffectiveFrom       time.Time  `json:"effective_from"`
	EffectiveTo         *time.Time `json:"effective_to"`
	Source              string     `json:"source"`
	CreatedAt           time.Time  `json:"created_at"`
	CreatedBy           *string    `json:"created_by"`
}

func (PriceDTO) adalahDTO() {}

// EgressPoolDTO adalah konfigurasi proxy pool keluar.
// Catatan Keamanan: encryption_key_id dan proxy_url_encrypted DILARANG dipublikasikan ke API admin.
type EgressPoolDTO struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Kind             string     `json:"kind"`
	MaskedHint       string     `json:"masked_hint"`
	Enabled          bool       `json:"enabled"`
	Weight           int        `json:"weight"`
	Region           *string    `json:"region"`
	LastHealthStatus *string    `json:"last_health_status"`
	LastHealthAt     *time.Time `json:"last_health_at"`
	LastLatencyMS    *int       `json:"last_latency_ms"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedBy        *string    `json:"created_by"`
}

func (EgressPoolDTO) adalahDTO() {}

// -----------------------------------------------------------------------------
// Domain Gateway Policies
// -----------------------------------------------------------------------------

// RuleProviderDTO adalah relasi upstream provider yang terikat pada aturan routing.
type RuleProviderDTO struct {
	ProviderID string `json:"provider_id"`
	Position   int    `json:"position"`
	Weight     *int   `json:"weight"`
}

// RoutingRuleDTO adalah aturan pemilihan provider dan strategi failover.
type RoutingRuleDTO struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Description       *string           `json:"description"`
	Priority          int               `json:"priority"`
	MatchModelID      *string           `json:"match_model_id"`
	MatchAPIKeyID     *string           `json:"match_api_key_id"`
	MatchCapabilities []string          `json:"match_capabilities"`
	Strategy          string            `json:"strategy"`
	MaxAttempts       int               `json:"max_attempts"`
	BackoffMS         int               `json:"backoff_ms"`
	FailureThreshold  int               `json:"failure_threshold"`
	OpenDurationMS    int               `json:"open_duration_ms"`
	HalfOpenProbes    int               `json:"half_open_probes"`
	Enabled           bool              `json:"enabled"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	CreatedBy         *string           `json:"created_by"`
	Providers         []RuleProviderDTO `json:"providers"`
}

func (RoutingRuleDTO) adalahDTO() {}

// RateLimitDTO adalah konfigurasi pembatasan kuota dan laju inferensi.
type RateLimitDTO struct {
	ID                  string    `json:"id"`
	Scope               string    `json:"scope"`
	ScopeID             string    `json:"scope_id"`
	RequestsPerSecond   *int      `json:"requests_per_second"`
	RequestsPerMinute   *int      `json:"requests_per_minute"`
	TokensPerMinute     *int      `json:"tokens_per_minute"`
	DailyRequestLimit   *int64    `json:"daily_request_limit"`
	MonthlyRequestLimit *int64    `json:"monthly_request_limit"`
	DailyTokenLimit     *int64    `json:"daily_token_limit"`
	MonthlyTokenLimit   *int64    `json:"monthly_token_limit"`
	Enabled             bool      `json:"enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (RateLimitDTO) adalahDTO() {}

// BudgetDTO adalah alokasi batas biaya moneter dengan proteksi threshold peringatan.
type BudgetDTO struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Scope             string     `json:"scope"`
	ScopeID           string     `json:"scope_id"`
	Period            string     `json:"period"`
	LimitUSD          string     `json:"limit_usd"`
	MaxSpendUSD       string     `json:"max_spend_usd"`
	SpentUSD          string     `json:"spent_usd"`
	PeriodStart       time.Time  `json:"period_start"`
	PeriodEnd         *time.Time `json:"period_end"`
	ActionOnExceed    string     `json:"action_on_exceed"`
	Action            string     `json:"action"`
	AlertThresholdPct int        `json:"alert_threshold_pct"`
	AlertThreshold    int        `json:"alert_threshold"`
	AlertedAt         *time.Time `json:"alerted_at"`
	Enabled           bool       `json:"enabled"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (BudgetDTO) adalahDTO() {}

// BanDTO adalah catatan pemblokiran subjek (IP, API Key, atau User).
type BanDTO struct {
	ID          string     `json:"id"`
	SubjectKind string     `json:"subject_kind"`
	Subject     string     `json:"subject"`
	Reason      string     `json:"reason"`
	ExpiresAt   *time.Time `json:"expires_at"`
	LiftedAt    *time.Time `json:"lifted_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (BanDTO) adalahDTO() {}

// ContentFilterDTO adalah aturan penyaringan konten prompt maupun respons.
type ContentFilterDTO struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	Kind                    string    `json:"kind"`
	Priority                int       `json:"priority"`
	AppliesTo               string    `json:"applies_to"`
	Action                  string    `json:"action"`
	Pattern                 string    `json:"pattern"`
	PatternType             string    `json:"pattern_type"`
	CaseSensitive           bool      `json:"case_sensitive"`
	MaxEvalMS               int       `json:"max_eval_ms"`
	ModelID                 string    `json:"model_id"`
	ProviderID              string    `json:"provider_id"`
	MaxRequestBytes         *int64    `json:"max_request_bytes"`
	ModerationIntegrationID string    `json:"moderation_integration_id"`
	ModerationCategories    []string  `json:"moderation_categories"`
	ModerationThreshold     *float64  `json:"moderation_threshold"`
	Enabled                 bool      `json:"enabled"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (ContentFilterDTO) adalahDTO() {}

// CircuitBreakerDTO memuat status sirkuit breaker individual dari Redis.
type CircuitBreakerDTO struct {
	Key           string `json:"key,omitempty"`
	ProviderID    string `json:"provider_id"`
	Model         string `json:"model"`
	State         string `json:"state"`
	FailureCount  int64  `json:"failure_count"`
	LastFailureAt string `json:"last_failure_at,omitempty"`
	NextProbeAt   string `json:"next_probe_at,omitempty"`
}

func (CircuitBreakerDTO) adalahDTO() {}

// -----------------------------------------------------------------------------
// Domain Access & Identity
// -----------------------------------------------------------------------------

// KeyDTO adalah informasi kredensial API key klien publik.
type KeyDTO struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Prefix              string     `json:"prefix"`
	Last4               string     `json:"last4"`
	MaskedKey           string     `json:"masked_key"`
	OwnerUserID         string     `json:"owner_user_id"`
	Status              string     `json:"status"`
	Enabled             bool       `json:"enabled"`
	Scopes              []string   `json:"scopes"`
	RateLimitRPS        *int       `json:"rate_limit_rps"`
	RateLimitRPM        *int       `json:"rate_limit_rpm"`
	RateLimitTPM        *int       `json:"rate_limit_tpm"`
	DailyRequestLimit   *int64     `json:"daily_request_limit"`
	MonthlyRequestLimit *int64     `json:"monthly_request_limit"`
	DailyTokenLimit     *int64     `json:"daily_token_limit"`
	MonthlyTokenLimit   *int64     `json:"monthly_token_limit"`
	IPAllowlist         []string   `json:"ip_allowlist"`
	AllowedModels       []string   `json:"allowed_models"`
	AllowedProviders    []string   `json:"allowed_providers"`
	ExpiresAt           *time.Time `json:"expires_at"`
	LastUsedAt          *time.Time `json:"last_used_at"`
	RevokedAt           *time.Time `json:"revoked_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (KeyDTO) adalahDTO() {}

// KeyDetailResponse adalah respons inspeksi detail satu API key.
type KeyDetailResponse struct {
	Key    KeyDTO `json:"key"`
	Masked string `json:"masked"`
}

func (KeyDetailResponse) adalahDTO() {}

// KeyCreatedResponse adalah respons pembuatan atau rotasi API key.
// Catatan Keamanan: Nilai rahasia API key mentah HANYA dikembalikan satu kali melalui field raw_key.
type KeyCreatedResponse struct {
	Key    KeyDTO `json:"key"`
	Masked string `json:"masked"`
	RawKey string `json:"raw_key"`
}

func (KeyCreatedResponse) adalahDTO() {}

// UserDTO adalah akun pengguna administratif konsol.
type UserDTO struct {
	ID                  string     `json:"id"`
	Email               string     `json:"email"`
	DisplayName         string     `json:"display_name"`
	Status              string     `json:"status"`
	FailedLoginAttempts int        `json:"failed_login_attempts"`
	LockedUntil         *time.Time `json:"locked_until,omitempty"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP         string     `json:"last_login_ip,omitempty"`
	MustChangePassword  bool       `json:"must_change_password"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	Roles               []string   `json:"roles,omitempty"`
}

func (UserDTO) adalahDTO() {}

// UserDetailResponse adalah detail akun admin beserta daftar peran dan izin efektifnya.
type UserDetailResponse struct {
	User        UserDTO   `json:"user"`
	Roles       []RoleDTO `json:"roles"`
	Permissions []string  `json:"permissions"`
}

func (UserDetailResponse) adalahDTO() {}

// RoleDTO adalah definisi peran pada kontrol akses berbasis peran (RBAC).
type RoleDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IsSystem    bool      `json:"is_system"`
	Rank        int       `json:"rank"`
	CreatedAt   time.Time `json:"created_at"`
}

func (RoleDTO) adalahDTO() {}

// RoleDetailDTO adalah peran administratif beserta daftar izin spesifiknya.
type RoleDetailDTO struct {
	RoleDTO
	Permissions []PermissionDTO `json:"permissions"`
}

func (RoleDetailDTO) adalahDTO() {}

// PermissionDTO adalah entri katalog hak akses administratif.
type PermissionDTO struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (PermissionDTO) adalahDTO() {}

// SessionDTO adalah informasi sesi login admin aktif.
type SessionDTO struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	IP         string     `json:"ip,omitempty"`
	UserAgent  string     `json:"user_agent,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func (SessionDTO) adalahDTO() {}

// -----------------------------------------------------------------------------
// Domain Automation & Webhooks
// -----------------------------------------------------------------------------

// WebhookDTO adalah endpoint penerima notifikasi webhook peristiwa Route-X.
// Catatan Keamanan: secret_ciphertext dan encryption_key_id DILARANG KERAS keluar di sini.
// Hanya masked_hint yang aman ditampilkan kepada pengguna dengan izin settings:read.
type WebhookDTO struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	Events              []string   `json:"events"`
	MaskedHint          string     `json:"masked_hint"`
	Enabled             bool       `json:"enabled"`
	MaxRetries          int        `json:"max_retries"`
	TimeoutMS           int        `json:"timeout_ms"`
	LastDeliveryAt      *time.Time `json:"last_delivery_at"`
	LastDeliveryStatus  *string    `json:"last_delivery_status"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	CreatedBy           *string    `json:"created_by"`
}

func (WebhookDTO) adalahDTO() {}

// DeliveryDTO adalah status pengiriman rekaman webhook individual.
// Catatan Keamanan: payload mentah tidak disertakan di sini demi privasi data inferensi.
type DeliveryDTO struct {
	ID                 int64      `json:"id"`
	WebhookID          string     `json:"webhook_id"`
	Event              string     `json:"event"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	Status             string     `json:"status"`
	AttemptCount       int        `json:"attempt_count"`
	NextAttemptAt      time.Time  `json:"next_attempt_at"`
	LockedAt           *time.Time `json:"locked_at,omitempty"`
	LockedBy           *string    `json:"locked_by,omitempty"`
	ResponseStatusCode *int       `json:"response_status_code"`
	ErrorMessage       *string    `json:"error_message"`
	CreatedAt          time.Time  `json:"created_at"`
	DeliveredAt        *time.Time `json:"delivered_at"`
}

func (DeliveryDTO) adalahDTO() {}

// -----------------------------------------------------------------------------
// Domain System, Settings, & Diagnostics
// -----------------------------------------------------------------------------

// SettingDTO adalah setelan konfigurasi runtime sistem yang tersimpan di database.
type SettingDTO struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at"`
	UpdatedBy   string          `json:"updated_by,omitempty"`
}

func (SettingDTO) adalahDTO() {}

// XrayProtocolDTO merepresentasikan metadata spesifik dari suatu protokol tunnel Xray yang didukung.
type XrayProtocolDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	Transport   string `json:"transport"`
	Security    string `json:"security"`
	Port        int    `json:"port"`
	PathOrSNI   string `json:"path_or_sni"`
	ShareLink   string `json:"share_link"`
	EgressURL   string `json:"egress_url"`
	Description string `json:"description"`
}

func (XrayProtocolDTO) adalahDTO() {}

// DomainConfigDTO merepresentasikan konfigurasi nama domain publik dan status HTTPS otomatis.
type DomainConfigDTO struct {
	Domain            string            `json:"domain"`
	Mode              string            `json:"mode"`
	Status            string            `json:"status"`
	PublicURL         string            `json:"public_url"`
	BaseURL           string            `json:"base_url"`
	ServerIP          string            `json:"server_ip"`
	ResolvedIPs       []string          `json:"resolved_ips,omitempty"`
	DNSMatched        bool              `json:"dns_matched"`
	LastChecked       *time.Time        `json:"last_checked,omitempty"`
	Message           string            `json:"message,omitempty"`
	XrayEnabled       bool              `json:"xray_enabled"`
	XrayUUID          string            `json:"xray_uuid,omitempty"`
	XrayVlessWS       string            `json:"xray_vless_ws,omitempty"`
	XrayVlessGRPC     string            `json:"xray_vless_grpc,omitempty"`
	XrayVlessXHTTP    string            `json:"xray_vless_xhttp,omitempty"`
	XrayVlessReality  string            `json:"xray_vless_reality,omitempty"`
	XrayTrojanWS      string            `json:"xray_trojan_ws,omitempty"`
	XrayTrojanGRPC    string            `json:"xray_trojan_grpc,omitempty"`
	XrayVmessWS       string            `json:"xray_vmess_ws,omitempty"`
	XrayVmessGRPC     string            `json:"xray_vmess_grpc,omitempty"`
	XrayShadowsocksWS string            `json:"xray_shadowsocks_ws,omitempty"`
	XrayShadowsocks   string            `json:"xray_shadowsocks,omitempty"`
	XrayProtocols     []XrayProtocolDTO `json:"xray_protocols,omitempty"`
}

func (DomainConfigDTO) adalahDTO() {}

// EgressProbeResponseDTO adalah hasil pengujian konektivitas live ke proxy keluar.
type EgressProbeResponseDTO struct {
	Status     string    `json:"status"`
	LatencyMS  *int      `json:"latency_ms,omitempty"`
	ExitIP     string    `json:"exit_ip,omitempty"`
	Country    string    `json:"country,omitempty"`
	Datacenter string    `json:"datacenter,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
	Message    string    `json:"message"`
	Error      string    `json:"error,omitempty"`
}

func (EgressProbeResponseDTO) adalahDTO() {}

// UpdateDomainRequestDTO adalah masukan untuk mengubah konfigurasi nama domain sistem.
type UpdateDomainRequestDTO struct {
	Domain string `json:"domain"`
	Mode   string `json:"mode"`
}

// JobDTO adalah status konfigurasi background scheduler internal.
type JobDTO struct {
	Name           string     `json:"name"`
	Schedule       string     `json:"schedule"`
	Interval       string     `json:"interval,omitempty"`
	Status         string     `json:"status"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastStatus     string     `json:"last_status,omitempty"`
	LastDurationMS int64      `json:"last_duration_ms"`
	LastError      string     `json:"last_error,omitempty"`
}

func (JobDTO) adalahDTO() {}

// AuditEntryDTO adalah catatan jejak audit aktivitas administratif.
type AuditEntryDTO struct {
	ID           int64           `json:"id"`
	OccurredAt   time.Time       `json:"occurred_at"`
	ActorUserID  string          `json:"actor_user_id,omitempty"`
	ActorEmail   string          `json:"actor_email,omitempty"`
	ActorRole    string          `json:"actor_role,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id,omitempty"`
	IP           string          `json:"ip,omitempty"`
	UserAgent    string          `json:"user_agent,omitempty"`
	RequestID    string          `json:"request_id,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
}

func (AuditEntryDTO) adalahDTO() {}

// DiagnosticsMemoryDTO merinci metrik alokasi memori runtime Go.
type DiagnosticsMemoryDTO struct {
	AllocBytes      uint64 `json:"alloc_bytes"`
	TotalAllocBytes uint64 `json:"total_alloc_bytes"`
	SysBytes        uint64 `json:"sys_bytes"`
	HeapAllocBytes  uint64 `json:"heap_alloc_bytes"`
	HeapInuseBytes  uint64 `json:"heap_inuse_bytes"`
	NumGC           uint32 `json:"num_gc"`
}

// DiagnosticsDTO menyajikan profil diagnostik komprehensif server Route-X.
type DiagnosticsDTO struct {
	Version       string               `json:"version,omitempty"`
	Commit        string               `json:"commit,omitempty"`
	BuiltAt       string               `json:"built_at,omitempty"`
	UptimeSeconds int64                `json:"uptime_seconds"`
	GoVersion     string               `json:"go_version"`
	NumGoroutine  int                  `json:"num_goroutine"`
	NumCPU        int                  `json:"num_cpu"`
	Memory        DiagnosticsMemoryDTO `json:"memory"`
	DBPool        *DBPoolStatsDTO      `json:"db_pool,omitempty"`
}

func (DiagnosticsDTO) adalahDTO() {}

// SystemOverviewDTO menyajikan ringkasan metrik live runtime untuk kartu System Overview di Dashboard.
type SystemOverviewDTO struct {
	PID               int     `json:"pid"`
	OSArch            string  `json:"os_arch"`
	UptimeSeconds     int64   `json:"uptime_seconds"`
	GoVersion         string  `json:"go_version"`
	InFlightRequests  int64   `json:"in_flight_requests"`
	ProcessRSSBytes   uint64  `json:"process_rss_bytes"`
	HostRAMTotalBytes uint64  `json:"host_ram_total_bytes"`
	HostRAMUsedBytes  uint64  `json:"host_ram_used_bytes"`
	ContainerRAMBytes uint64  `json:"container_ram_bytes"`
	GoHeapBytes       uint64  `json:"go_heap_bytes"`
	GoSysBytes        uint64  `json:"go_sys_bytes"`
	StackInuseBytes   uint64  `json:"stack_inuse_bytes"`
	NumGC             uint32  `json:"num_gc"`
	NetTotalBytes     uint64  `json:"net_total_bytes"`
	NetRecvBytes      uint64  `json:"net_recv_bytes"`
	NetSentBytes      uint64  `json:"net_sent_bytes"`
	NetRateMBSec      float64 `json:"net_rate_mb_s"`
	NetRecvRateMBSec  float64 `json:"net_recv_rate_mb_s"`
	NetSentRateMBSec  float64 `json:"net_sent_rate_mb_s"`
	EgressTotalRoutes int     `json:"egress_total_routes"`
	EgressXrayCount   int     `json:"egress_xray_count"`
	EgressHTTPCount   int     `json:"egress_http_count"`
	EgressActiveMode  string  `json:"egress_active_mode"`
	ContainerCPUCap   float64 `json:"container_cpu_cap"`
	ProxyCPUPct       float64 `json:"proxy_cpu_pct"`
	HostCPUPct        float64 `json:"host_cpu_pct"`
	NumGoroutine      int     `json:"num_goroutine"`
}

func (SystemOverviewDTO) adalahDTO() {}

// -----------------------------------------------------------------------------
// Fungsi Pemetaan (Mappers)
// -----------------------------------------------------------------------------

func toDistributionDTO(d traffic.Distribution) DistributionDTO {
	return DistributionDTO{
		Count: d.Count,
		SumMS: d.SumMS,
		MaxMS: d.MaxMS,
		P50:   d.P50,
		P90:   d.P90,
		P95:   d.P95,
		P99:   d.P99,
	}
}

func toTrafficStatsDTO(s *traffic.Stats) TrafficStatsDTO {
	if s == nil {
		return TrafficStatsDTO{}
	}
	avail, _ := s.Availability()
	errRate, _ := s.ErrorRate()
	return TrafficStatsDTO{
		TotalRequests:     s.Requests,
		SuccessRequests:   s.Success,
		ErrorRequests:     s.Errors,
		Timeouts:          s.Timeouts,
		Retries:           s.Retries,
		Failovers:         s.Failovers,
		TotalTokens:       s.TotalTokens,
		PromptTokens:      s.InputTokens,
		CompletionTokens:  s.OutputTokens,
		CachedInputTokens: s.CachedInputTokens,
		ReasoningTokens:   s.ReasoningTokens,
		TotalCostUSD:      s.CostUSD.Decimal(),
		CostUSD:           s.CostUSD.Decimal(),
		AvgLatencyMS:      s.Latency.AvgMS(),
		P95LatencyMS:      s.Latency.P95,
		ErrorRate:         errRate * 100.0,
		Availability:      avail * 100.0,
		From:              s.From,
		To:                s.To,
		Source:            string(s.Source),
		Latency:           toDistributionDTO(s.Latency),
		TTFT:              toDistributionDTO(s.TTFT),
	}
}

func toTrafficPointDTO(p traffic.Point) TrafficPointDTO {
	return TrafficPointDTO{
		Timestamp:    p.Bucket.UTC().Format(time.RFC3339),
		Bucket:       p.Bucket,
		Requests:     p.Requests,
		Success:      p.Success,
		Errors:       p.Errors,
		TotalTokens:  p.TotalTokens,
		Tokens:       p.TotalTokens,
		CostUSD:      p.CostUSD.Decimal(),
		Latency:      toDistributionDTO(p.Latency),
		P50LatencyMS: p.Latency.P50,
		P95LatencyMS: p.Latency.P95,
	}
}

func toTrafficSliceDTO(s traffic.Slice, totalRequests int64) TrafficSliceDTO {
	pct := 0.0
	if totalRequests > 0 {
		pct = (float64(s.Requests) / float64(totalRequests)) * 100.0
	}
	return TrafficSliceDTO{
		ID:          s.ID,
		Name:        s.Name,
		Requests:    s.Requests,
		Success:     s.Success,
		Errors:      s.Errors,
		TotalTokens: s.TotalTokens,
		CostUSD:     s.CostUSD.Decimal(),
		Percentage:  pct,
		Latency:     toDistributionDTO(s.Latency),
	}
}

func toTrafficRowDTO(r traffic.Row) TrafficRowDTO {
	return TrafficRowDTO{
		ID:               r.ID,
		CreatedAt:        r.CreatedAt,
		RequestID:        r.RequestID,
		APIKeyID:         r.APIKeyID,
		APIKeyName:       r.APIKeyName,
		Method:           r.Method,
		Endpoint:         r.Endpoint,
		RequestedModel:   r.RequestedModel,
		ModelName:        r.ModelName,
		ProviderName:     r.ProviderName,
		UpstreamModel:    r.UpstreamModel,
		StatusCode:       r.StatusCode,
		ErrorType:        r.ErrorType,
		ErrorCode:        r.ErrorCode,
		Stream:           r.Stream,
		IsStream:         r.Stream,
		InputTokens:      r.InputTokens,
		PromptTokens:     r.InputTokens,
		OutputTokens:     r.OutputTokens,
		CompletionTokens: r.OutputTokens,
		TotalTokens:      r.TotalTokens,
		CostUSD:          upstream.USD(r.CostUSD).Decimal(),
		LatencyMS:        r.LatencyMS,
		DurationMS:       r.LatencyMS,
		TTFTMS:           r.TTFTMS,
		RetryCount:       r.RetryCount,
		FailoverCount:    r.FailoverCount,
	}
}

func toTrafficDetailDTO(d *traffic.Detail) TrafficDetailDTO {
	if d == nil {
		return TrafficDetailDTO{}
	}
	decision := d.RoutingDecision
	if len(decision) == 0 {
		decision = json.RawMessage("{}")
	}
	return TrafficDetailDTO{
		TrafficRowDTO:     toTrafficRowDTO(d.Row),
		UserID:            d.UserID,
		ModelID:           d.ModelID,
		ProviderID:        d.ProviderID,
		ErrorMessage:      d.ErrorMessage,
		CachedInputTokens: d.CachedInputTokens,
		ReasoningTokens:   d.ReasoningTokens,
		UpstreamLatencyMS: d.UpstreamLatencyMS,
		RoutingStrategy:   d.RoutingStrategy,
		RoutingDecision:   decision,
		ClientIP:          d.ClientIP,
		UserAgent:         d.UserAgent,
	}
}

func toTrafficEventDTO(e traffic.Event) TrafficEventDTO {
	detail := e.Detail
	if len(detail) == 0 {
		detail = json.RawMessage("{}")
	}
	return TrafficEventDTO{
		Seq:           e.Seq,
		Kind:          e.Kind,
		ProviderID:    e.ProviderID,
		ProviderName:  e.ProviderName,
		UpstreamModel: e.UpstreamModel,
		StatusCode:    e.StatusCode,
		LatencyMS:     e.LatencyMS,
		ErrorKind:     e.ErrorKind,
		ErrorMessage:  e.ErrorMessage,
		Detail:        detail,
		CreatedAt:     e.CreatedAt,
	}
}

func toTrafficPayloadDTO(p *traffic.Payload) TrafficPayloadDTO {
	if p == nil {
		return TrafficPayloadDTO{
			RequestHeaders:  json.RawMessage("{}"),
			RequestBody:     json.RawMessage("{}"),
			ResponseHeaders: json.RawMessage("{}"),
			ResponseBody:    json.RawMessage("{}"),
		}
	}
	reqH := p.RequestHeaders
	if len(reqH) == 0 {
		reqH = json.RawMessage("{}")
	}
	reqB := p.RequestBody
	if len(reqB) == 0 {
		reqB = json.RawMessage("{}")
	}
	resH := p.ResponseHeaders
	if len(resH) == 0 {
		resH = json.RawMessage("{}")
	}
	resB := p.ResponseBody
	if len(resB) == 0 {
		resB = json.RawMessage("{}")
	}
	return TrafficPayloadDTO{
		RequestHeaders:  reqH,
		RequestBody:     reqB,
		ResponseHeaders: resH,
		ResponseBody:    resB,
		Truncated:       p.Truncated,
		SizeBytes:       p.SizeBytes,
	}
}

func toProviderDTO(p *upstream.Provider) ProviderDTO {
	if p == nil {
		return ProviderDTO{}
	}
	meta := p.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage("{}")
	}
	return ProviderDTO{
		ID:                  p.ID,
		Name:                p.Name,
		DisplayName:         p.DisplayName,
		Kind:                p.Kind,
		BaseURL:             p.BaseURL,
		Enabled:             p.Enabled,
		Priority:            p.Priority,
		Weight:              p.Weight,
		TimeoutMS:           p.TimeoutMS,
		MaxRetries:          p.MaxRetries,
		RateLimitRPM:        p.RateLimitRPM,
		RateLimitTPM:        p.RateLimitTPM,
		MaxConcurrent:       p.MaxConcurrent,
		EgressPoolID:        p.EgressPoolID,
		IsBYOK:              p.IsBYOK,
		OwnerUserID:         p.OwnerUserID,
		LastHealthStatus:    p.LastHealthStatus,
		LastHealthAt:        p.LastHealthAt,
		LastLatencyMS:       p.LastLatencyMS,
		ConsecutiveFailures: p.ConsecutiveFailures,
		Metadata:            meta,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
		CreatedBy:           p.CreatedBy,
	}
}

func toHealthCheckDTO(h *upstream.HealthCheck) HealthCheckDTO {
	if h == nil {
		return HealthCheckDTO{}
	}
	return HealthCheckDTO{
		ID:           h.ID,
		ProviderID:   h.ProviderID,
		Status:       h.Status,
		LatencyMS:    h.LatencyMS,
		StatusCode:   h.StatusCode,
		ErrorKind:    h.ErrorKind,
		ErrorMessage: h.ErrorMessage,
		CheckedAt:    h.CheckedAt,
		CreatedAt:    h.CheckedAt,
	}
}

func toCredentialMetaDTO(c *upstream.CredentialMeta) CredentialMetaDTO {
	if c == nil {
		return CredentialMetaDTO{}
	}
	return CredentialMetaDTO{
		ID:           c.ID,
		ProviderID:   c.ProviderID,
		Label:        c.Label,
		MaskedHint:   c.MaskedHint,
		Enabled:      c.Enabled,
		LastUsedAt:   c.LastUsedAt,
		ExpiresAt:    c.ExpiresAt,
		AuthFailures: c.AuthFailures,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		CreatedBy:    c.CreatedBy,
	}
}

func toModelDTO(m *upstream.Model) ModelDTO {
	if m == nil {
		return ModelDTO{}
	}
	caps := m.Capabilities
	if caps == nil {
		caps = make([]string, 0)
	}
	meta := m.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage("{}")
	}
	return ModelDTO{
		ID:              m.ID,
		ModelID:         m.ModelID,
		DisplayName:     m.DisplayName,
		Family:          m.Family,
		ContextWindow:   m.ContextWindow,
		MaxOutputTokens: m.MaxOutputTokens,
		Capabilities:    caps,
		Enabled:         m.Enabled,
		RoutingPriority: m.RoutingPriority,
		RoutingStrategy: m.RoutingStrategy,
		DeprecatedAt:    m.DeprecatedAt,
		Metadata:        meta,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
		Providers:       make([]ModelProviderSummaryDTO, 0),
	}
}

func toModelAliasDTO(a *upstream.Alias) ModelAliasDTO {
	if a == nil {
		return ModelAliasDTO{}
	}
	return ModelAliasDTO{
		ID:        a.Alias,
		ModelID:   a.ModelID,
		Alias:     a.Alias,
		CreatedAt: a.CreatedAt,
	}
}

func toModelMappingDTO(pm *upstream.ProviderModel) ModelMappingDTO {
	if pm == nil {
		return ModelMappingDTO{}
	}
	return ModelMappingDTO{
		ID:                pm.ID,
		ModelID:           pm.ModelID,
		ProviderID:        pm.ProviderID,
		UpstreamModelName: pm.UpstreamModelName,
		Priority:          pm.Priority,
		Weight:            pm.Weight,
		MaxContextWindow:  pm.MaxContextWindow,
		SupportsStreaming: pm.SupportsStreaming,
		SupportsTools:     pm.SupportsTools,
		Enabled:           pm.Enabled,
		CreatedAt:         pm.CreatedAt,
		UpdatedAt:         pm.UpdatedAt,
	}
}

func toPriceDTO(p *upstream.Price) PriceDTO {
	if p == nil {
		return PriceDTO{}
	}
	var cachedStr *string
	if p.CachedInput != nil {
		s := p.CachedInput.Decimal()
		cachedStr = &s
	}
	var reasoningStr *string
	if p.Reasoning != nil {
		s := p.Reasoning.Decimal()
		reasoningStr = &s
	}
	return PriceDTO{
		ID:                  p.ID,
		ProviderModelID:     p.ProviderModelID,
		InputPer1MUSD:       p.Input.Decimal(),
		OutputPer1MUSD:      p.Output.Decimal(),
		CachedInputPer1MUSD: cachedStr,
		ReasoningPer1MUSD:   reasoningStr,
		Currency:            p.Currency,
		EffectiveFrom:       p.EffectiveFrom,
		EffectiveTo:         p.EffectiveTo,
		Source:              p.Source,
		CreatedAt:           p.CreatedAt,
		CreatedBy:           p.CreatedBy,
	}
}

func toEgressPoolDTO(e *upstream.EgressPool) EgressPoolDTO {
	if e == nil {
		return EgressPoolDTO{}
	}
	return EgressPoolDTO{
		ID:               e.ID,
		Name:             e.Name,
		Kind:             e.Kind,
		MaskedHint:       e.MaskedHint,
		Enabled:          e.Enabled,
		Weight:           e.Weight,
		Region:           e.Region,
		LastHealthStatus: e.LastHealthStatus,
		LastHealthAt:     e.LastHealthAt,
		LastLatencyMS:    e.LastLatencyMS,
		CreatedAt:        e.CreatedAt,
		UpdatedAt:        e.UpdatedAt,
		CreatedBy:        e.CreatedBy,
	}
}

func toRoutingRuleDTO(r *upstream.RoutingRule) RoutingRuleDTO {
	if r == nil {
		return RoutingRuleDTO{}
	}
	provs := make([]RuleProviderDTO, 0, len(r.Providers))
	for _, p := range r.Providers {
		provs = append(provs, RuleProviderDTO{
			ProviderID: p.ProviderID,
			Position:   p.Position,
			Weight:     p.Weight,
		})
	}
	caps := r.MatchCapabilities
	if caps == nil {
		caps = make([]string, 0)
	}
	return RoutingRuleDTO{
		ID:                r.ID,
		Name:              r.Name,
		Description:       r.Description,
		Priority:          r.Priority,
		MatchModelID:      r.MatchModelID,
		MatchAPIKeyID:     r.MatchAPIKeyID,
		MatchCapabilities: caps,
		Strategy:          r.Strategy,
		MaxAttempts:       r.MaxAttempts,
		BackoffMS:         r.BackoffMS,
		FailureThreshold:  r.FailureThreshold,
		OpenDurationMS:    r.OpenDurationMS,
		HalfOpenProbes:    r.HalfOpenProbes,
		Enabled:           r.Enabled,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
		CreatedBy:         r.CreatedBy,
		Providers:         provs,
	}
}

func toRateLimitDTO(l *policy.RateLimit) RateLimitDTO {
	if l == nil {
		return RateLimitDTO{}
	}
	return RateLimitDTO{
		ID:                  l.ID,
		Scope:               l.Scope,
		ScopeID:             l.ScopeID,
		RequestsPerSecond:   l.RequestsPerSecond,
		RequestsPerMinute:   l.RequestsPerMinute,
		TokensPerMinute:     l.TokensPerMinute,
		DailyRequestLimit:   l.DailyRequestLimit,
		MonthlyRequestLimit: l.MonthlyRequestLimit,
		DailyTokenLimit:     l.DailyTokenLimit,
		MonthlyTokenLimit:   l.MonthlyTokenLimit,
		Enabled:             l.Enabled,
		CreatedAt:           l.CreatedAt,
		UpdatedAt:           l.UpdatedAt,
	}
}

func toBudgetDTO(b *policy.Budget) BudgetDTO {
	if b == nil {
		return BudgetDTO{}
	}
	return BudgetDTO{
		ID:                b.ID,
		Name:              b.Name,
		Scope:             b.Scope,
		ScopeID:           b.ScopeID,
		Period:            b.Period,
		LimitUSD:          b.LimitUSD.Decimal(),
		MaxSpendUSD:       b.LimitUSD.Decimal(),
		SpentUSD:          b.SpentUSD.Decimal(),
		PeriodStart:       b.PeriodStart,
		PeriodEnd:         b.PeriodEnd,
		ActionOnExceed:    b.ActionOnExceed,
		Action:            b.ActionOnExceed,
		AlertThresholdPct: b.AlertThresholdPct,
		AlertThreshold:    b.AlertThresholdPct,
		AlertedAt:         b.AlertedAt,
		Enabled:           b.Enabled,
		CreatedAt:         b.CreatedAt,
		UpdatedAt:         b.UpdatedAt,
	}
}

func toBanDTO(b *policy.Ban) BanDTO {
	if b == nil {
		return BanDTO{}
	}
	return BanDTO{
		ID:          b.ID,
		SubjectKind: b.SubjectKind,
		Subject:     b.Subject,
		Reason:      b.Reason,
		ExpiresAt:   b.ExpiresAt,
		LiftedAt:    b.LiftedAt,
		CreatedAt:   b.CreatedAt,
	}
}

func toContentFilterDTO(f *policy.ContentFilter) ContentFilterDTO {
	if f == nil {
		return ContentFilterDTO{}
	}
	cats := f.ModerationCategories
	if cats == nil {
		cats = make([]string, 0)
	}
	return ContentFilterDTO{
		ID:                      f.ID,
		Name:                    f.Name,
		Description:             f.Description,
		Kind:                    f.Kind,
		Priority:                f.Priority,
		AppliesTo:               f.AppliesTo,
		Action:                  f.Action,
		Pattern:                 f.Pattern,
		PatternType:             f.PatternType,
		CaseSensitive:           f.CaseSensitive,
		MaxEvalMS:               f.MaxEvalMS,
		ModelID:                 f.ModelID,
		ProviderID:              f.ProviderID,
		MaxRequestBytes:         f.MaxRequestBytes,
		ModerationIntegrationID: f.ModerationIntegrationID,
		ModerationCategories:    cats,
		ModerationThreshold:     f.ModerationThreshold,
		Enabled:                 f.Enabled,
		CreatedAt:               f.CreatedAt,
		UpdatedAt:               f.UpdatedAt,
	}
}

func toKeyDTO(k *keys.Key, allowedModels, allowedProviders []string) KeyDTO {
	if k == nil {
		return KeyDTO{}
	}
	scopes := k.Scopes
	if scopes == nil {
		scopes = make([]string, 0)
	}
	ips := make([]string, 0, len(k.IPAllowlist))
	for _, prefix := range k.IPAllowlist {
		ips = append(ips, prefix.String())
	}
	if allowedModels == nil {
		allowedModels = make([]string, 0)
	}
	if allowedProviders == nil {
		allowedProviders = make([]string, 0)
	}
	return KeyDTO{
		ID:                  k.ID,
		Name:                k.Name,
		Prefix:              k.Prefix,
		Last4:               k.Last4,
		MaskedKey:           k.Masked(),
		OwnerUserID:         k.OwnerUserID,
		Status:              k.Status,
		Enabled:             k.Status == keys.StatusActive,
		Scopes:              scopes,
		RateLimitRPS:        k.RateLimitRPS,
		RateLimitRPM:        k.RateLimitRPM,
		RateLimitTPM:        k.RateLimitTPM,
		DailyRequestLimit:   k.DailyRequestLimit,
		MonthlyRequestLimit: k.MonthlyRequestLimit,
		DailyTokenLimit:     k.DailyTokenLimit,
		MonthlyTokenLimit:   k.MonthlyTokenLimit,
		IPAllowlist:         ips,
		AllowedModels:       allowedModels,
		AllowedProviders:    allowedProviders,
		ExpiresAt:           k.ExpiresAt,
		LastUsedAt:          k.LastUsedAt,
		RevokedAt:           k.RevokedAt,
		CreatedAt:           k.CreatedAt,
		UpdatedAt:           k.UpdatedAt,
	}
}

func toUserDTO(u *identity.User, roles []string) UserDTO {
	if u == nil {
		return UserDTO{}
	}
	if roles == nil {
		roles = make([]string, 0)
	}
	return UserDTO{
		ID:                  u.ID,
		Email:               u.Email,
		DisplayName:         u.DisplayName,
		Status:              string(u.Status),
		FailedLoginAttempts: u.FailedLoginAttempts,
		LockedUntil:         u.LockedUntil,
		LastLoginAt:         u.LastLoginAt,
		LastLoginIP:         u.LastLoginIP,
		MustChangePassword:  u.MustChangePassword,
		CreatedAt:           u.CreatedAt,
		UpdatedAt:           u.UpdatedAt,
		Roles:               roles,
	}
}

func toRoleDTO(r identity.Role) RoleDTO {
	return RoleDTO{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		IsSystem:    r.IsSystem,
		Rank:        r.Rank,
		CreatedAt:   r.CreatedAt,
	}
}

func toRoleDetailDTO(d *identity.RoleDetail) RoleDetailDTO {
	if d == nil {
		return RoleDetailDTO{}
	}
	perms := make([]PermissionDTO, 0, len(d.Permissions))
	for _, p := range d.Permissions {
		perms = append(perms, toPermissionDTO(p))
	}
	return RoleDetailDTO{
		RoleDTO:     toRoleDTO(d.Role),
		Permissions: perms,
	}
}

func toPermissionDTO(p identity.Permission) PermissionDTO {
	return PermissionDTO{
		ID:          p.ID,
		Key:         p.Key,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
	}
}

func toSessionDTO(s identity.Session) SessionDTO {
	return SessionDTO{
		ID:         s.ID,
		UserID:     s.UserID,
		IP:         s.IP,
		UserAgent:  s.UserAgent,
		CreatedAt:  s.CreatedAt,
		LastSeenAt: s.LastSeenAt,
		ExpiresAt:  s.ExpiresAt,
		RevokedAt:  s.RevokedAt,
	}
}

func toWebhookDTO(w *webhooks.Webhook) WebhookDTO {
	if w == nil {
		return WebhookDTO{}
	}
	events := w.Events
	if events == nil {
		events = make([]string, 0)
	}
	return WebhookDTO{
		ID:                  w.ID,
		Name:                w.Name,
		URL:                 w.URL,
		Events:              events,
		MaskedHint:          w.MaskedHint,
		Enabled:             w.Enabled,
		MaxRetries:          w.MaxRetries,
		TimeoutMS:           w.TimeoutMS,
		LastDeliveryAt:      w.LastDeliveryAt,
		LastDeliveryStatus:  w.LastDeliveryStatus,
		ConsecutiveFailures: w.ConsecutiveFailures,
		CreatedAt:           w.CreatedAt,
		UpdatedAt:           w.UpdatedAt,
		CreatedBy:           w.CreatedBy,
	}
}

func toDeliveryDTO(d *webhooks.Delivery) DeliveryDTO {
	if d == nil {
		return DeliveryDTO{}
	}
	return DeliveryDTO{
		ID:                 d.ID,
		WebhookID:          d.WebhookID,
		Event:              d.Event,
		Payload:            d.Payload,
		Status:             d.Status,
		AttemptCount:       d.AttemptCount,
		NextAttemptAt:      d.NextAttemptAt,
		LockedAt:           d.LockedAt,
		LockedBy:           d.LockedBy,
		ResponseStatusCode: d.ResponseStatusCode,
		ErrorMessage:       d.ErrorMessage,
		CreatedAt:          d.CreatedAt,
		DeliveredAt:        d.DeliveredAt,
	}
}

func toSettingDTO(s identity.Setting) SettingDTO {
	val := s.Value
	if len(val) == 0 {
		val = json.RawMessage("null")
	} else if strings.HasPrefix(s.Key, "cli:config:") {
		// DATA-001: konfigurasi integrasi CLI dapat memuat material API key.
		// Ciphertext terenkripsi boleh lolos; field plaintext legacy (api_key,
		// dipakai hanya saat migrasi oleh cliconfig) tidak pernah kembali ke
		// klien mana pun, termasuk peran Viewer lewat GET /system/settings.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(val, &obj); err == nil {
			delete(obj, "api_key")
		}
		if len(obj) == 0 {
			val = json.RawMessage("{}")
		} else {
			val, _ = json.Marshal(obj)
		}
	} else if s.Key == SettingKeyXrayConfig {
		// SEC-002: state ini memuat material autentikasi tunnel. Bahkan
		// ciphertext tidak berguna bagi klien settings:read dan tidak boleh
		// membuka detail format penyimpanan internal.
		val = json.RawMessage(`{"redacted":true}`)
	}
	return SettingDTO{
		Key:         s.Key,
		Value:       val,
		Description: s.Description,
		UpdatedAt:   s.UpdatedAt,
		UpdatedBy:   s.UpdatedBy,
	}
}

func toAuditEntryDTO(e identity.Entry) AuditEntryDTO {
	meta := e.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage("{}")
	}
	return AuditEntryDTO{
		ID:           e.ID,
		OccurredAt:   e.OccurredAt,
		ActorUserID:  e.ActorUserID,
		ActorEmail:   e.ActorEmail,
		ActorRole:    e.ActorRole,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		IP:           e.IP,
		UserAgent:    e.UserAgent,
		RequestID:    e.RequestID,
		Metadata:     meta,
	}
}

// -----------------------------------------------------------------------------
// Response Cache DTOs (Fitur 1)
// -----------------------------------------------------------------------------

// CacheStatsDTO adalah respons statistik response cache.
type CacheStatsDTO struct {
	Enabled      bool  `json:"enabled"`
	TTLSeconds   int64 `json:"ttl_seconds"`
	Hits         int64 `json:"hits"`
	Misses       int64 `json:"misses"`
	TotalEntries int64 `json:"total_entries"`
}

func (CacheStatsDTO) adalahDTO() {}

// CacheFlushResponseDTO adalah respons setelah cache dibersihkan.
type CacheFlushResponseDTO struct {
	Deleted int64  `json:"deleted"`
	Message string `json:"message"`
}

func (CacheFlushResponseDTO) adalahDTO() {}

// CacheSettingsResponseDTO adalah respons pembaruan pengaturan cache.
type CacheSettingsResponseDTO struct {
	Enabled    bool   `json:"enabled"`
	TTLSeconds int64  `json:"ttl_seconds"`
	Message    string `json:"message"`
}

func (CacheSettingsResponseDTO) adalahDTO() {}

// SyncModelsResponseDTO adalah respons hasil sinkronisasi model dari provider (Fitur 2).
type SyncModelsResponseDTO struct {
	Count   int      `json:"count"`
	Models  []string `json:"models"`
	Message string   `json:"message"`
}

func (SyncModelsResponseDTO) adalahDTO() {}

// TestModelResponseDTO adalah respons hasil pengujian konektivitas model ke provider upstream.
type TestModelResponseDTO struct {
	Status    string `json:"status"`
	LatencyMS int    `json:"latency_ms"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (TestModelResponseDTO) adalahDTO() {}

// -----------------------------------------------------------------------------
// CLI Integration DTOs (Fitur 4)
// -----------------------------------------------------------------------------

// CLIToolDTO mendeskripsikan status deteksi dan konfigurasi satu perkakas AI CLI.
type CLIToolDTO struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Category       string            `json:"category"`
	Installed      bool              `json:"installed"`
	Path           string            `json:"path,omitempty"`
	Version        string            `json:"version,omitempty"`
	ConfigPath     string            `json:"config_path,omitempty"`
	SupportedModes []string          `json:"supported_modes"`
	ActiveMode     string            `json:"active_mode"`
	ActiveTarget   string            `json:"active_target"`
	EnvVars        map[string]string `json:"env_vars"`
	EnvVarAPIKey   string            `json:"env_var_api_key,omitempty"`
	ExportSnippet  string            `json:"export_snippet"`
	UpdatedAt      *time.Time        `json:"updated_at,omitempty"`
}

func (CLIToolDTO) adalahDTO() {}

// CLIDetectedResponseDTO memuat daftar seluruh tool CLI yang terdeteksi di sistem host.
type CLIDetectedResponseDTO struct {
	Tools          []CLIToolDTO `json:"tools"`
	TotalDetected  int          `json:"total_detected"`
	TotalAvailable int          `json:"total_available"`
	GatewayURL     string       `json:"gateway_url"`
	DefaultEnvFile string       `json:"default_env_file"`
}

func (CLIDetectedResponseDTO) adalahDTO() {}

// CLIConfigureRequestDTO adalah payload pembaruan konfigurasi perkakas CLI.
type CLIConfigureRequestDTO struct {
	ToolID     string `json:"tool_id"`
	Mode       string `json:"mode"`
	Target     string `json:"target"`
	APIKey     string `json:"api_key,omitempty"`
	GatewayURL string `json:"gateway_url,omitempty"`
}

// CLIConfigureResponseDTO membalas hasil mutasi konfigurasi perkakas CLI.
type CLIConfigureResponseDTO struct {
	Success       bool       `json:"success"`
	Tool          CLIToolDTO `json:"tool"`
	Message       string     `json:"message"`
	ConfigFile    string     `json:"config_file,omitempty"`
	ExportSnippet string     `json:"export_snippet"`
}

func (CLIConfigureResponseDTO) adalahDTO() {}

// CLIExportScriptDTO adalah respons pembacaan skrip bash terpadu ~/.routex/cli-env.sh.
type CLIExportScriptDTO struct {
	Content  string `json:"content"`
	FilePath string `json:"file_path"`
}

func (CLIExportScriptDTO) adalahDTO() {}

// Ensure netip import is used
var _ = netip.Prefix{}
