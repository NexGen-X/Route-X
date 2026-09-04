// Package admin menyediakan endpoint REST API administratif untuk mengelola dashboard,
// upstream, aturan gateway, kebijakan pembatasan, manajemen akses, otomasi webhooks,
// dan observabilitas sistem Route-X.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
	"github.com/NexGen-X/Route-X/internal/worker"
)

// maxAdminBodyBytes membatasi ukuran request body untuk rute admin (1 MB).
const maxAdminBodyBytes = 1 << 20

// Handlers adalah pengontrol utama REST API admin yang mengorkestrasi seluruh repository.
type Handlers struct {
	pool    *pgxpool.Pool
	redis   *redis.Client
	authSvc *auth.Service
	csrf    *auth.CSRF
	logger  *slog.Logger

	// Repositories
	providerRepo   *upstream.ProviderRepo
	credentialRepo *upstream.CredentialRepo
	modelRepo      *upstream.ModelRepo
	pricingRepo    *upstream.PricingRepo
	routingRepo    *upstream.RoutingRepo
	egressRepo     *upstream.EgressRepo
	policyRepo     *policy.Repo
	keyRepo        *keys.Repo
	usersRepo      *identity.Users
	rolesRepo      *identity.Roles
	sessionsRepo   *identity.Sessions
	settingsRepo   *identity.Settings
	auditRepo      *identity.Audit
	trafficRepo    *traffic.Repo
	webhooksRepo   *webhooks.Repo
	dispatcher     *webhooks.Dispatcher
	factory        *gateway.Factory
	breaker        *gateway.Breaker
	supervisor     *worker.Supervisor
	cipher         *security.Cipher
}

// Config membawa seluruh dependensi yang dibutuhkan oleh Admin Handlers.
type Config struct {
	Pool           *pgxpool.Pool
	Redis          *redis.Client
	AuthSvc        *auth.Service
	Logger         *slog.Logger
	ProviderRepo   *upstream.ProviderRepo
	CredentialRepo *upstream.CredentialRepo
	ModelRepo      *upstream.ModelRepo
	PricingRepo    *upstream.PricingRepo
	RoutingRepo    *upstream.RoutingRepo
	EgressRepo     *upstream.EgressRepo
	PolicyRepo     *policy.Repo
	KeyRepo        *keys.Repo
	UsersRepo      *identity.Users
	RolesRepo      *identity.Roles
	SessionsRepo   *identity.Sessions
	SettingsRepo   *identity.Settings
	AuditRepo      *identity.Audit
	TrafficRepo    *traffic.Repo
	WebhooksRepo   *webhooks.Repo
	Dispatcher     *webhooks.Dispatcher
	Factory        *gateway.Factory
	Breaker        *gateway.Breaker
	Supervisor     *worker.Supervisor
	Cipher         *security.Cipher
}

// NewHandlers menginisialisasi controller REST API admin.
func NewHandlers(cfg Config) *Handlers {
	var l *slog.Logger
	if cfg.Logger != nil {
		l = cfg.Logger
	} else {
		l = slog.Default()
	}

	var csrf *auth.CSRF
	if cfg.AuthSvc != nil {
		csrf = cfg.AuthSvc.CSRF()
	}

	return &Handlers{
		pool:           cfg.Pool,
		redis:          cfg.Redis,
		authSvc:        cfg.AuthSvc,
		csrf:           csrf,
		logger:         l,
		providerRepo:   cfg.ProviderRepo,
		credentialRepo: cfg.CredentialRepo,
		modelRepo:      cfg.ModelRepo,
		pricingRepo:    cfg.PricingRepo,
		routingRepo:    cfg.RoutingRepo,
		egressRepo:     cfg.EgressRepo,
		policyRepo:     cfg.PolicyRepo,
		keyRepo:        cfg.KeyRepo,
		usersRepo:      cfg.UsersRepo,
		rolesRepo:      cfg.RolesRepo,
		sessionsRepo:   cfg.SessionsRepo,
		settingsRepo:   cfg.SettingsRepo,
		auditRepo:      cfg.AuditRepo,
		trafficRepo:    cfg.TrafficRepo,
		webhooksRepo:   cfg.WebhooksRepo,
		dispatcher:     cfg.Dispatcher,
		factory:        cfg.Factory,
		breaker:        cfg.Breaker,
		supervisor:     cfg.Supervisor,
		cipher:         cfg.Cipher,
	}
}

// Redaksi rahasia untuk log dan format string dengan receiver nilai.
func (h Handlers) String() string       { return "admin.Handlers{[REDACTED]}" }
func (h Handlers) GoString() string     { return h.String() }
func (h Handlers) LogValue() slog.Value { return slog.StringValue(h.String()) }

// writeAuditTx mencatat satu aksi audit di dalam transaksi yang sedang berjalan (atau pool).
// Memastikan integritas ACID: bila transaksi dibatalkan (rollback), catatan audit ikut
// dibatalkan agar tidak meninggalkan jejak mutasi yang sebenarnya gagal.
func (h *Handlers) writeAuditTx(ctx context.Context, q repo.Querier, r *http.Request, action, resourceType, resourceID string, metadata any) error {
	var actor identity.Actor
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		role := ""
		if len(p.Roles) > 0 {
			role = p.Roles[0]
		}
		actor = identity.Actor{
			UserID: p.User.ID,
			Email:  p.User.Email,
			Role:   role,
		}
	} else {
		actor = identity.Actor{
			Email: "system@internal",
			Role:  "System",
		}
	}

	ip := r.RemoteAddr
	if host, _, ok := strings.Cut(ip, ":"); ok && host != "" {
		ip = host
	}

	var metaMap map[string]any
	if metadata != nil {
		if m, ok := metadata.(map[string]any); ok {
			metaMap = m
		} else {
			metaBytes, _ := json.Marshal(metadata)
			_ = json.Unmarshal(metaBytes, &metaMap)
		}
	}

	event := identity.Event{
		Actor:        actor,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		IP:           ip,
		UserAgent:    r.UserAgent(),
		RequestID:    observability.RequestIDFrom(ctx),
		Metadata:     metaMap,
	}

	audit := identity.NewAudit(q)
	return audit.Write(ctx, event)
}

func (h *Handlers) writeAudit(ctx context.Context, r *http.Request, action, resourceType, resourceID string, metadata any) {
	if err := h.writeAuditTx(ctx, h.pool, r, action, resourceType, resourceID, metadata); err != nil {
		h.logger.LogAttrs(ctx, slog.LevelError, "gagal menulis audit log",
			slog.String("action", action),
			slog.String("resource_type", resourceType),
			slog.String("resource_id", resourceID),
			slog.String("error", err.Error()),
		)
	}
}

// decodeJSON membaca request body JSON dengan batas ukuran aman maxAdminBodyBytes.
func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	limited := io.LimitReader(r.Body, maxAdminBodyBytes)
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("membaca body JSON: %w", err)
	}
	return nil
}

// mapRepoError memetakan error repository ke respons HTTP standar httpx.
func mapRepoError(w http.ResponseWriter, r *http.Request, err error, resource string) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, repo.ErrNotFound):
		httpx.NotFound(w, r, fmt.Sprintf("%s tidak ditemukan", resource))
	case errors.Is(err, repo.ErrConflict):
		httpx.BadRequest(w, r, "conflict", fmt.Sprintf("%s sudah ada atau melanggar keunikan", resource))
	case errors.Is(err, repo.ErrInvalidReference):
		httpx.BadRequest(w, r, "invalid_reference", fmt.Sprintf("referensi data untuk %s tidak valid", resource))
	case errors.Is(err, repo.ErrConstraint):
		observability.LoggerFrom(r.Context()).LogAttrs(r.Context(), slog.LevelWarn,
			"constraint violation",
			slog.String("resource", resource),
			slog.String("error", err.Error()),
		)
		httpx.BadRequest(w, r, "constraint_violation", fmt.Sprintf("nilai input melanggar batasan untuk %s", resource))
	default:
		observability.LoggerFrom(r.Context()).LogAttrs(r.Context(), slog.LevelError,
			"kegagalan database di admin API",
			slog.String("resource", resource),
			slog.String("error", err.Error()),
		)
		httpx.InternalError(w, r)
	}
}

// respond menuliskan respons JSON yang mewajibkan tipe data mengimplementasikan antarmuka dto.
// Penegakan ini dilakukan oleh compiler Go sehingga tidak ada struct repository atau map inline
// yang dapat lolos ke respons publik tanpa melalui kontrak DTO yang telah diaudit.
func (h *Handlers) respond(w http.ResponseWriter, r *http.Request, status int, body dto) error {
	return httpx.JSON(w, status, body)
}
