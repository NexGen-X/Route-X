package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
	"github.com/NexGen-X/Route-X/internal/worker"
)

const schemaPrefix = "test_admin_"
const testPassword = "Kata-Sandi-Uji-Admin-2026"
const testSessionSecret = "rahasia-sesi-uji-yang-panjangnya-lebih-dari-32-karakter"

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	security.BurnVerifyTime()

	code := m.Run()
	if !testing.Short() {
		if left := leftoverSchemas(); len(left) > 0 {
			fmt.Fprintf(os.Stderr, "schema test admin tertinggal: %s\n", strings.Join(left, ", "))
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func leftoverSchemas() []string {
	dsn := ""
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			dsn = v
			break
		}
	}
	if dsn == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		select schema_name
		from information_schema.schemata
		where schema_name like $1
		order by schema_name`, schemaPrefix+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			out = append(out, name)
		}
	}
	return out
}

type testEnv struct {
	pool       *pgxpool.Pool
	authSvc    *auth.Service
	handlers   *Handlers
	adminUser  identity.User
	viewerUser identity.User
	cipher     *security.Cipher
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("melewati test integrasi admin pada -short")
	}

	dsn := ""
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			dsn = v
			break
		}
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL atau DATABASE_URL belum diatur")
	}

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}

	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	schemaName := schemaPrefix + hex.EncodeToString(buf[:])

	q := u.Query()
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rootConn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("koneksi root: %v", err)
	}
	t.Cleanup(func() {
		_ = rootConn.Close(context.Background())
	})

	if _, err := rootConn.Exec(ctx, `create schema `+schemaName); err != nil {
		t.Fatalf("buat schema: %v", err)
	}

	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_, _ = rootConn.Exec(dropCtx, `drop schema `+schemaName+` cascade`)
	})

	logger := slog.New(slog.DiscardHandler)

	dbCfg := &config.Config{
		AppEnv:      config.EnvDevelopment,
		DatabaseURL: security.Secret(u.String()),
		DBMaxConns:  5,
		DBMinConns:  0,
	}
	db, err := database.Connect(ctx, dbCfg, logger)
	if err != nil {
		t.Fatalf("buka database: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatalf("migrasi schema: %v", err)
	}
	pool := db.Pool

	// Seed catalog, roles, dan user admin pertama
	var encKey [32]byte
	copy(encKey[:], "kunci-enkripsi-uji-32-byte-tepat!")
	cipher, err := security.NewCipher(encKey[:])
	if err != nil {
		t.Fatalf("buat cipher: %v", err)
	}

	cfg := &config.Config{
		SessionSecret: security.Secret(testSessionSecret),
		AppEnv:        config.EnvDevelopment,
	}

	if _, err := seed.Run(ctx, pool, cfg, logger); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	authSvc := auth.NewService(pool, cfg, logger)

	// Buat Akun Super Admin
	usersRepo := identity.NewUsers(pool)
	rolesRepo := identity.NewRoles(pool)
	adminUser, err := usersRepo.Create(ctx, identity.NewUser{
		Email:              "superadmin@routex.internal",
		DisplayName:        "Super Admin",
		Password:           security.Secret(testPassword),
		MustChangePassword: false,
	})
	if err != nil {
		t.Fatalf("buat super admin: %v", err)
	}

	superRole, err := rolesRepo.GetByName(ctx, seed.RoleSuperAdmin)
	if err != nil {
		t.Fatalf("ambil role super admin: %v", err)
	}
	if err := rolesRepo.Grant(ctx, adminUser.ID, superRole.ID, adminUser.ID); err != nil {
		t.Fatalf("grant super admin: %v", err)
	}

	// Buat Akun Viewer
	viewerUser, err := usersRepo.Create(ctx, identity.NewUser{
		Email:              "viewer@routex.internal",
		DisplayName:        "Viewer User",
		Password:           security.Secret(testPassword),
		MustChangePassword: false,
	})
	if err != nil {
		t.Fatalf("buat viewer: %v", err)
	}

	viewerRole, err := rolesRepo.GetByName(ctx, seed.RoleViewer)
	if err != nil {
		t.Fatalf("ambil role viewer: %v", err)
	}
	if err := rolesRepo.Grant(ctx, viewerUser.ID, viewerRole.ID, adminUser.ID); err != nil {
		t.Fatalf("grant viewer: %v", err)
	}

	// Inisialisasi Repositories
	providerRepo := upstream.NewProviderRepo(pool)
	credentialRepo, _ := upstream.NewCredentialRepo(pool, cipher)
	modelRepo := upstream.NewModelRepo(pool)
	pricingRepo := upstream.NewPricingRepo(pool)
	routingRepo := upstream.NewRoutingRepo(pool)
	egressRepo, _ := upstream.NewEgressRepo(pool, cipher)
	policyRepo := policy.New(pool)
	keyRepo, _ := keys.New(pool, []byte("pepper-uji-32-byte-harus-cukup!"))
	sessionsRepo := identity.NewSessions(pool)
	settingsRepo := identity.NewSettings(pool)
	auditRepo := identity.NewAudit(pool)
	trafficRepo := traffic.New(pool)
	webhooksRepo := webhooks.NewRepo(pool)
	dispatcher := webhooks.NewDispatcher(webhooksRepo, cipher, nil, cfg.UpstreamSSRFPolicy(), logger)
	sup := worker.NewSupervisor(nil, logger)
	sup.Register(worker.JobFunc{JobName: "health_checker", Fn: func(ctx context.Context) error { return nil }}, 30*time.Second, 0)
	sup.Register(worker.JobFunc{JobName: "usage_rollup", Fn: func(ctx context.Context) error { return nil }}, 5*time.Minute, 0)
	sup.Register(worker.JobFunc{JobName: "retention_cleaner", Fn: func(ctx context.Context) error { return nil }}, 1*time.Hour, 0)

	handlers := NewHandlers(Config{
		Pool:           pool,
		AuthSvc:        authSvc,
		Logger:         logger,
		ProviderRepo:   providerRepo,
		CredentialRepo: credentialRepo,
		ModelRepo:      modelRepo,
		PricingRepo:    pricingRepo,
		RoutingRepo:    routingRepo,
		EgressRepo:     egressRepo,
		PolicyRepo:     policyRepo,
		KeyRepo:        keyRepo,
		UsersRepo:      usersRepo,
		RolesRepo:      rolesRepo,
		SessionsRepo:   sessionsRepo,
		SettingsRepo:   settingsRepo,
		AuditRepo:      auditRepo,
		TrafficRepo:    trafficRepo,
		WebhooksRepo:   webhooksRepo,
		Dispatcher:     dispatcher,
		Supervisor:     sup,
		Cipher:         cipher,
	})

	return &testEnv{
		pool:       pool,
		authSvc:    authSvc,
		handlers:   handlers,
		adminUser:  adminUser,
		viewerUser: viewerUser,
		cipher:     cipher,
	}
}

type adminClient struct {
	t      *testing.T
	server *httptest.Server
	http   *http.Client
	csrf   string
}

func (e *testEnv) newClient(t *testing.T) *adminClient {
	t.Helper()

	// Pasang rute auth dan rute admin bersamaan seperti di main.go
	mux := http.NewServeMux()
	mux.Handle("/api/auth/", http.StripPrefix("/api/auth", auth.NewHandlers(e.authSvc).Routes()))
	mux.Handle("/api/admin/", http.StripPrefix("/api/admin", e.handlers.Routes()))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}

	return &adminClient{
		t:      t,
		server: server,
		http:   &http.Client{Jar: jar},
	}
}

func (c *adminClient) login(email, password string) {
	c.t.Helper()

	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})

	req, err := http.NewRequest(http.MethodPost, c.server.URL+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("buat request login: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("kirim login: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		c.t.Fatalf("login gagal: status %d", res.StatusCode)
	}

	// Ekstrak CSRF token dari cookie
	u, _ := url.Parse(c.server.URL)
	for _, cookie := range c.http.Jar.Cookies(u) {
		if cookie.Name == auth.CSRFCookieName {
			c.csrf = cookie.Value
			break
		}
	}
}

func (c *adminClient) do(method, path string, body any, withCSRF bool) (*http.Response, []byte) {
	c.t.Helper()

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.server.URL+path, reader)
	if err != nil {
		c.t.Fatalf("buat request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if withCSRF && c.csrf != "" {
		req.Header.Set("Origin", c.server.URL)
		req.Header.Set(auth.HeaderCSRFToken, c.csrf)
	}

	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("kirim request: %v", err)
	}
	defer res.Body.Close()

	resBody, _ := io.ReadAll(res.Body)
	return res, resBody
}

func TestAdminAuthenticationRequired(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)

	// Akses tanpa cookie sesi wajib membalas 401 Unauthorized
	res, _ := client.do(http.MethodGet, "/api/admin/system/diagnostics", nil, false)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, mau %d (Unauthorized)", res.StatusCode, http.StatusUnauthorized)
	}
}

func TestAdminViewerRBACRestriction(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)

	// Login sebagai Viewer
	client.login("viewer@routex.internal", testPassword)

	// 1. Viewer diizinkan membaca diagnostics (memiliki PermHealthRead)
	res, _ := client.do(http.MethodGet, "/api/admin/system/diagnostics", nil, false)
	if res.StatusCode != http.StatusOK {
		t.Errorf("viewer GET diagnostics status = %d, mau %d", res.StatusCode, http.StatusOK)
	}

	// 2. Viewer DITOLAK saat mencoba membuat provider (tidak memiliki PermProvidersWrite)
	provPayload := map[string]any{
		"name":         "test-provider",
		"display_name": "Test Provider",
		"kind":         "openai",
		"base_url":     "https://api.openai.com/v1",
	}
	resMut, _ := client.do(http.MethodPost, "/api/admin/upstreams/providers", provPayload, true)
	if resMut.StatusCode != http.StatusForbidden {
		t.Errorf("viewer POST providers status = %d, mau %d (Forbidden)", resMut.StatusCode, http.StatusForbidden)
	}
}

func TestAdminCSRFProtection(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)

	// Login sebagai Super Admin
	client.login("superadmin@routex.internal", testPassword)

	filterPayload := map[string]any{
		"name":         "filter-csrf-test",
		"description":  "Filter untuk uji CSRF",
		"kind":         "blocked_pattern",
		"priority":     100,
		"applies_to":   "request",
		"action":       "block",
		"pattern":      "attack-pattern",
		"pattern_type": "substring",
	}

	// 1. Mutasi TANPA header CSRF ditolak (403 Forbidden)
	resNoCSRF, _ := client.do(http.MethodPost, "/api/admin/gateway/content-filters", filterPayload, false)
	if resNoCSRF.StatusCode != http.StatusForbidden {
		t.Errorf("POST tanpa CSRF status = %d, mau %d", resNoCSRF.StatusCode, http.StatusForbidden)
	}

	// 2. Mutasi DENGAN header CSRF valid berhasil (201 Created)
	resValid, resBody := client.do(http.MethodPost, "/api/admin/gateway/content-filters", filterPayload, true)
	if resValid.StatusCode != http.StatusCreated {
		t.Errorf("POST dengan CSRF valid status = %d, mau %d. Body: %s", resValid.StatusCode, http.StatusCreated, string(resBody))
	}
}

func TestAdminContentFilterCRUDDanKeysetPagination(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)

	client.login("superadmin@routex.internal", testPassword)

	// 1. Create Filter
	fPayload := map[string]any{
		"name":         "filter-crud-1",
		"description":  "Filter uji CRUD",
		"kind":         "blocked_pattern",
		"priority":     50,
		"applies_to":   "request",
		"action":       "block",
		"pattern":      "secret_token_123",
		"pattern_type": "substring",
		"enabled":      true,
	}
	resCreate, bodyCreate := client.do(http.MethodPost, "/api/admin/gateway/content-filters", fPayload, true)
	if resCreate.StatusCode != http.StatusCreated {
		t.Fatalf("create filter status = %d, body: %s", resCreate.StatusCode, string(bodyCreate))
	}

	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(bodyCreate, &created); err != nil {
		t.Fatalf("unmarshal created filter: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("ID filter kosong")
	}

	// 2. Get Filter
	resGet, bodyGet := client.do(http.MethodGet, "/api/admin/gateway/content-filters/"+created.ID, nil, false)
	if resGet.StatusCode != http.StatusOK {
		t.Fatalf("get filter status = %d, body: %s", resGet.StatusCode, string(bodyGet))
	}

	// 3. Update Filter
	updPayload := map[string]any{
		"name":         "filter-crud-updated",
		"description":  "Filter diperbarui",
		"kind":         "blocked_pattern",
		"priority":     60,
		"applies_to":   "request",
		"action":       "block",
		"pattern":      "secret_token_123",
		"pattern_type": "substring",
		"enabled":      true,
	}
	resUpd, _ := client.do(http.MethodPut, "/api/admin/gateway/content-filters/"+created.ID, updPayload, true)
	if resUpd.StatusCode != http.StatusOK {
		t.Fatalf("update filter status = %d", resUpd.StatusCode)
	}

	// 4. Toggle Filter
	resToggle, _ := client.do(http.MethodPost, "/api/admin/gateway/content-filters/"+created.ID+"/toggle", map[string]any{"enabled": false}, true)
	if resToggle.StatusCode != http.StatusOK {
		t.Fatalf("toggle filter status = %d", resToggle.StatusCode)
	}

	// 5. List Filters dengan Paginasi Keyset
	resList, bodyList := client.do(http.MethodGet, "/api/admin/gateway/content-filters?limit=1", nil, false)
	if resList.StatusCode != http.StatusOK {
		t.Fatalf("list filters status = %d, body: %s", resList.StatusCode, string(bodyList))
	}
	var listResp struct {
		Items      []any  `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(bodyList, &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listResp.Items) == 0 {
		t.Errorf("list filters item kosong")
	}

	// 6. Delete Filter
	resDel, _ := client.do(http.MethodDelete, "/api/admin/gateway/content-filters/"+created.ID, nil, true)
	if resDel.StatusCode != http.StatusOK {
		t.Fatalf("delete filter status = %d", resDel.StatusCode)
	}

	// Verifikasi sudah terhapus
	resVerify, _ := client.do(http.MethodGet, "/api/admin/gateway/content-filters/"+created.ID, nil, false)
	if resVerify.StatusCode != http.StatusNotFound {
		t.Errorf("status setelah delete = %d, mau %d (NotFound)", resVerify.StatusCode, http.StatusNotFound)
	}
}

func TestAdminSystemEndpoints(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)

	client.login("superadmin@routex.internal", testPassword)

	// 1. Diagnostics
	resDiag, bodyDiag := client.do(http.MethodGet, "/api/admin/system/diagnostics", nil, false)
	if resDiag.StatusCode != http.StatusOK {
		t.Fatalf("diagnostics status = %d", resDiag.StatusCode)
	}
	var diagResp map[string]any
	if err := json.Unmarshal(bodyDiag, &diagResp); err != nil {
		t.Fatalf("unmarshal diagnostics: %v", err)
	}
	if _, ok := diagResp["uptime_seconds"]; !ok {
		t.Errorf("uptime_seconds tidak ditemukan di respons diagnostics")
	}

	// 2. Jobs
	resJobs, bodyJobs := client.do(http.MethodGet, "/api/admin/system/jobs", nil, false)
	if resJobs.StatusCode != http.StatusOK {
		t.Fatalf("jobs status = %d", resJobs.StatusCode)
	}
	var jobsResp struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(bodyJobs, &jobsResp); err != nil {
		t.Fatalf("unmarshal jobs: %v", err)
	}
	if len(jobsResp.Items) == 0 {
		t.Errorf("daftar background jobs kosong")
	}

	// Uji pemicuan job valid: harus berhasil 200 OK
	resTrig, _ := client.do(http.MethodPost, "/api/admin/system/jobs/health_checker/run", nil, true)
	if resTrig.StatusCode != http.StatusOK {
		t.Errorf("pemicuan job valid status = %d, diharapkan 200", resTrig.StatusCode)
	}

	// Uji pemicuan job yang tidak ada: harus ditolak 404 Not Found
	resInvalid, _ := client.do(http.MethodPost, "/api/admin/system/jobs/job_fiktif_tidak_ada/run", nil, true)
	if resInvalid.StatusCode != http.StatusNotFound {
		t.Errorf("pemicuan job tidak dikenal status = %d, diharapkan 404", resInvalid.StatusCode)
	}

	// 3. Permissions Catalog
	resPerms, bodyPerms := client.do(http.MethodGet, "/api/admin/access/permissions", nil, false)
	if resPerms.StatusCode != http.StatusOK {
		t.Fatalf("permissions status = %d", resPerms.StatusCode)
	}
	var permsResp struct {
		Items []struct {
			Key string `json:"Key"`
		} `json:"items"`
	}
	if err := json.Unmarshal(bodyPerms, &permsResp); err != nil {
		t.Fatalf("unmarshal permissions: %v", err)
	}
	if len(permsResp.Items) < 20 {
		t.Errorf("jumlah izin katalog = %d, mau >= 20", len(permsResp.Items))
	}
}

// TestBatch4ValidationsAndConsistency memvalidasi secara ketat kepatuhan perbaikan Batch 4:
// penolakan masukan tidak sah (400 bukan 500), atomisitas transaksi, kelengkapan pembaruan field,
// dan pencegahan kebocoran error constraint internal.
func TestBatch4ValidationsAndConsistency(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)

	client.login("superadmin@routex.internal", testPassword)

	// 4.1 Validasi IP Allowlist
	t.Run("4.1 IP allowlist tidak sah harus ditolak dengan HTTP 400", func(t *testing.T) {
		res, body := client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
			"name":         "Kunci IP Rusak",
			"ip_allowlist": []string{"203.0.113.0/33"},
		}, true)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, mau 400 Bad Request", res.StatusCode)
		}
		if !strings.Contains(string(body), "invalid_ip_allowlist") {
			t.Errorf("respons tidak memuat kode invalid_ip_allowlist: %s", string(body))
		}
	})

	// 4.2 Validasi ExpiresAt
	t.Run("4.2 expires_at tidak sah harus ditolak dengan HTTP 400", func(t *testing.T) {
		// Pada API Key
		resKey, bodyKey := client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
			"name":       "Kunci Expired Rusak",
			"expires_at": "tanggal-palsu",
		}, true)
		if resKey.StatusCode != http.StatusBadRequest {
			t.Fatalf("API key status = %d, mau 400", resKey.StatusCode)
		}
		if !strings.Contains(string(bodyKey), "invalid_expires_at") {
			t.Errorf("respons tidak memuat invalid_expires_at: %s", string(bodyKey))
		}

		// Pada Ban
		resBan, bodyBan := client.do(http.MethodPost, "/api/admin/gateway/bans", map[string]any{
			"subject_kind": "ip",
			"subject":      "10.0.0.1",
			"reason":       "uji ban kadaluarsa",
			"expires_at":   "bukan-rfc3339",
		}, true)
		if resBan.StatusCode != http.StatusBadRequest {
			t.Fatalf("Ban status = %d, mau 400", resBan.StatusCode)
		}
		if !strings.Contains(string(bodyBan), "invalid_expires_at") {
			t.Errorf("respons tidak memuat invalid_expires_at: %s", string(bodyBan))
		}
	})

	// 4.3 Atomisitas Transaksi API Key dan Pembatasan Model
	t.Run("4.3 pembuatan api key beserta allowed models atomik dalam satu transaksi", func(t *testing.T) {
		invalidModelID := "00000000-0000-0000-0000-000000000999"
		resKey, _ := client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
			"name":      "Kunci Model Gagal",
			"model_ids": []string{invalidModelID},
		}, true)
		if resKey.StatusCode != http.StatusBadRequest {
			t.Fatalf("API key status = %d, mau 400 Bad Request", resKey.StatusCode)
		}

		// Pastikan key tidak terbuat di database (rollback penuh)
		resList, bodyList := client.do(http.MethodGet, "/api/admin/access/api-keys", nil, false)
		if resList.StatusCode != http.StatusOK {
			t.Fatalf("list key status = %d", resList.StatusCode)
		}
		if strings.Contains(string(bodyList), "Kunci Model Gagal") {
			t.Errorf("API key dengan model tidak sah bocor ke database (transaksi tidak di-rollback)")
		}
	})

	// 4.4 Atomisitas Transaksi Routing Rule dan Provider
	t.Run("4.4 pembuatan routing rule beserta provider atomik dalam satu transaksi", func(t *testing.T) {
		invalidProvID := "00000000-0000-0000-0000-000000000888"
		resRule, _ := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
			"name":         "Aturan Gagal Provider",
			"strategy":     "priority",
			"provider_ids": []string{invalidProvID},
			"weights":      map[string]int{invalidProvID: 100},
		}, true)
		if resRule.StatusCode != http.StatusBadRequest {
			t.Fatalf("routing rule status = %d, mau 400 Bad Request", resRule.StatusCode)
		}

		// Pastikan rule tidak terbuat di database (rollback penuh)
		resList, bodyList := client.do(http.MethodGet, "/api/admin/gateway/routing-rules", nil, false)
		if resList.StatusCode != http.StatusOK {
			t.Fatalf("list routing rules status = %d", resList.StatusCode)
		}
		if strings.Contains(string(bodyList), "Aturan Gagal Provider") {
			t.Errorf("routing rule dengan provider tidak sah bocor ke database (transaksi tidak di-rollback)")
		}
	})

	// 4.5 Pembaruan Content Filter, Provider, dan Model
	t.Run("4.5 pembaruan filter, provider, dan model mempertahankan field terkait", func(t *testing.T) {
		// Filter Konten: blocked_pattern
		resCF, bodyCF := client.do(http.MethodPost, "/api/admin/gateway/content-filters", map[string]any{
			"name":         "Filter Uji Batch 4 Pattern",
			"description":  "Filter awal",
			"kind":         "blocked_pattern",
			"priority":     10,
			"applies_to":   "request",
			"action":       "block",
			"pattern":      "kunci_rahasia",
			"pattern_type": "substring",
		}, true)
		if resCF.StatusCode != http.StatusCreated {
			t.Fatalf("buat filter pattern gagal: %d, body: %s", resCF.StatusCode, string(bodyCF))
		}
		var createdCF struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(bodyCF, &createdCF)

		// Update Filter: ubah pattern_type dari substring ke regex
		resUpdateCF, _ := client.do(http.MethodPut, "/api/admin/gateway/content-filters/"+createdCF.ID, map[string]any{
			"pattern_type": "regex",
		}, true)
		if resUpdateCF.StatusCode != http.StatusOK {
			t.Fatalf("update filter pattern status = %d, mau 200", resUpdateCF.StatusCode)
		}

		resGetCF, bodyGetCF := client.do(http.MethodGet, "/api/admin/gateway/content-filters/"+createdCF.ID, nil, false)
		if resGetCF.StatusCode != http.StatusOK {
			t.Fatalf("get filter pattern status = %d", resGetCF.StatusCode)
		}
		var gotCF struct {
			PatternType *string `json:"pattern_type"`
		}
		_ = json.Unmarshal(bodyGetCF, &gotCF)
		if gotCF.PatternType == nil || *gotCF.PatternType != "regex" {
			t.Errorf("pattern_type filter = %v, mau 'regex'", gotCF.PatternType)
		}

		// Filter Konten: request_size
		maxBytes := int64(4096)
		resCFSize, bodyCFSize := client.do(http.MethodPost, "/api/admin/gateway/content-filters", map[string]any{
			"name":              "Filter Uji Batch 4 Size",
			"kind":              "request_size",
			"priority":          20,
			"max_request_bytes": maxBytes,
		}, true)
		if resCFSize.StatusCode != http.StatusCreated {
			t.Fatalf("buat filter size gagal: %d, body: %s", resCFSize.StatusCode, string(bodyCFSize))
		}
		var createdCFSize struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(bodyCFSize, &createdCFSize)

		// Update Filter Size: kecilkan max_request_bytes ke 2048
		newMaxBytes := int64(2048)
		resUpdateCFSize, _ := client.do(http.MethodPut, "/api/admin/gateway/content-filters/"+createdCFSize.ID, map[string]any{
			"max_request_bytes": newMaxBytes,
		}, true)
		if resUpdateCFSize.StatusCode != http.StatusOK {
			t.Fatalf("update filter size status = %d, mau 200", resUpdateCFSize.StatusCode)
		}

		resGetCFSize, bodyGetCFSize := client.do(http.MethodGet, "/api/admin/gateway/content-filters/"+createdCFSize.ID, nil, false)
		if resGetCFSize.StatusCode != http.StatusOK {
			t.Fatalf("get filter size status = %d", resGetCFSize.StatusCode)
		}
		var gotCFSize struct {
			MaxRequestBytes *int64 `json:"max_request_bytes"`
		}
		_ = json.Unmarshal(bodyGetCFSize, &gotCFSize)
		if gotCFSize.MaxRequestBytes == nil || *gotCFSize.MaxRequestBytes != 2048 {
			t.Errorf("max_request_bytes = %v, mau 2048", gotCFSize.MaxRequestBytes)
		}

		// Update Provider: ubah name dan kind
		resProv, bodyProv := client.do(http.MethodPost, "/api/admin/upstreams/providers", map[string]any{
			"name":     "prov-batch4-lama",
			"kind":     "openai",
			"base_url": "https://api.openai.com/v1",
		}, true)
		if resProv.StatusCode != http.StatusCreated {
			t.Fatalf("buat provider status = %d, body: %s", resProv.StatusCode, string(bodyProv))
		}
		var createdProv struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(bodyProv, &createdProv)

		resUpdateProv, _ := client.do(http.MethodPut, "/api/admin/upstreams/providers/"+createdProv.ID, map[string]any{
			"name": "prov-batch4-baru",
			"kind": "anthropic",
		}, true)
		if resUpdateProv.StatusCode != http.StatusOK {
			t.Fatalf("update provider status = %d, mau 200", resUpdateProv.StatusCode)
		}

		resGetProv, bodyGetProv := client.do(http.MethodGet, "/api/admin/upstreams/providers/"+createdProv.ID, nil, false)
		if resGetProv.StatusCode != http.StatusOK {
			t.Fatalf("get provider status = %d", resGetProv.StatusCode)
		}
		var gotProv struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(bodyGetProv, &gotProv)
		if gotProv.Name != "prov-batch4-baru" {
			t.Errorf("name provider = %q, mau 'prov-batch4-baru'", gotProv.Name)
		}
		if gotProv.Kind != "anthropic" {
			t.Errorf("kind provider = %q, mau 'anthropic'", gotProv.Kind)
		}

		// Update Model: ubah model_id, routing_priority, routing_strategy
		resModel, bodyModel := client.do(http.MethodPost, "/api/admin/upstreams/models", map[string]any{
			"model_id":     "gpt-batch4-lama",
			"display_name": "Model Batch 4",
		}, true)
		if resModel.StatusCode != http.StatusCreated {
			t.Fatalf("buat model status = %d, body: %s", resModel.StatusCode, string(bodyModel))
		}
		var createdModel struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(bodyModel, &createdModel)

		prio := 42
		strat := "lowest_cost"
		resUpdateModel, _ := client.do(http.MethodPut, "/api/admin/upstreams/models/"+createdModel.ID, map[string]any{
			"model_id":         "gpt-batch4-baru",
			"routing_priority": prio,
			"routing_strategy": strat,
		}, true)
		if resUpdateModel.StatusCode != http.StatusOK {
			t.Fatalf("update model status = %d, mau 200", resUpdateModel.StatusCode)
		}

		resGetModel, bodyGetModel := client.do(http.MethodGet, "/api/admin/upstreams/models/"+createdModel.ID, nil, false)
		if resGetModel.StatusCode != http.StatusOK {
			t.Fatalf("get model status = %d", resGetModel.StatusCode)
		}
		var gotDetail struct {
			Model struct {
				ModelID         string  `json:"model_id"`
				RoutingPriority int     `json:"routing_priority"`
				RoutingStrategy *string `json:"routing_strategy"`
			} `json:"model"`
		}
		_ = json.Unmarshal(bodyGetModel, &gotDetail)
		gotModel := gotDetail.Model
		if gotModel.ModelID != "gpt-batch4-baru" {
			t.Errorf("model_id = %q, mau 'gpt-batch4-baru'", gotModel.ModelID)
		}
		if gotModel.RoutingPriority != 42 {
			t.Errorf("routing_priority = %d, mau 42", gotModel.RoutingPriority)
		}
		if gotModel.RoutingStrategy == nil || *gotModel.RoutingStrategy != "lowest_cost" {
			t.Errorf("routing_strategy = %v, mau 'lowest_cost'", gotModel.RoutingStrategy)
		}
	})

	// 4.6 & 4.7 Penanganan ErrConstraint (400 bukan 500, tanpa kebocoran %v, dan format raw_key)
	t.Run("4.6 dan 4.7 penanganan batasan mengembalikan 400 tanpa kebocoran %v", func(t *testing.T) {
		// Pembuatan user dengan password lemah (<8 karakter) harus 400 Bad Request, BUKAN 500
		resUser, bodyUser := client.do(http.MethodPost, "/api/admin/access/users", map[string]any{
			"email":    "user-lemah@routex.test",
			"password": "pendek",
			"name":     "User Pendek",
		}, true)
		if resUser.StatusCode != http.StatusBadRequest {
			t.Fatalf("status user password pendek = %d, diharapkan 400 (bukan 500)", resUser.StatusCode)
		}
		if !strings.Contains(string(bodyUser), "constraint_violation") {
			t.Errorf("respons tidak memuat error code constraint_violation: %s", string(bodyUser))
		}
		// 4.7 Verifikasi tidak ada nama operasi internal Go / SQL leaked
		if strings.Contains(string(bodyUser), "membuat pengguna:") {
			t.Errorf("respons membocorkan nama operasi/constraint internal: %s", string(bodyUser))
		}

		// Kueri traffic dengan kolom sort fiktif harus 400 Bad Request, BUKAN 500
		resTraffic, bodyTraffic := client.do(http.MethodGet, "/api/admin/requests?sort=kolom_fiktif_123", nil, false)
		if resTraffic.StatusCode != http.StatusBadRequest {
			t.Fatalf("status traffic sort invalid = %d, diharapkan 400 (bukan 500)", resTraffic.StatusCode)
		}
		if !strings.Contains(string(bodyTraffic), "constraint_violation") {
			t.Errorf("respons traffic tidak memuat constraint_violation: %s", string(bodyTraffic))
		}

		// Rotasi API Key harus mengembalikan raw_key
		resKey, bodyKey := client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
			"name": "Kunci Rotasi Uji",
		}, true)
		if resKey.StatusCode != http.StatusCreated {
			t.Fatalf("buat key status = %d, body: %s", resKey.StatusCode, string(bodyKey))
		}
		var createdKey struct {
			Key struct {
				ID string `json:"id"`
			} `json:"key"`
			RawKey string `json:"raw_key"`
		}
		_ = json.Unmarshal(bodyKey, &createdKey)
		if createdKey.RawKey == "" {
			t.Errorf("raw_key tidak boleh kosong pada pembuatan")
		}

		resRotate, bodyRotate := client.do(http.MethodPost, "/api/admin/access/api-keys/"+createdKey.Key.ID+"/rotate", nil, true)
		if resRotate.StatusCode != http.StatusOK {
			t.Fatalf("rotasi key status = %d, body: %s", resRotate.StatusCode, string(bodyRotate))
		}
		var rotatedResp map[string]any
		_ = json.Unmarshal(bodyRotate, &rotatedResp)
		if _, ok := rotatedResp["raw_key"]; !ok {
			t.Errorf("respons rotasi wajib memuat raw_key")
		}
		if _, ok := rotatedResp["token"]; ok {
			t.Errorf("respons rotasi tidak boleh memuat token ganda")
		}
	})
}

// TestBYOKIsolationAndNestedCredentialSecurity menguji kepatuhan isolasi BYOK (5.1)
// dan pencegahan manipulasi kredensial pada sumber daya bersarang /providers/{id}/credentials/{cred_id}.
func TestBYOKIsolationAndNestedCredentialSecurity(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	usersRepo := identity.NewUsers(env.pool)
	rolesRepo := identity.NewRoles(env.pool)
	providerRepo := upstream.NewProviderRepo(env.pool)
	credentialRepo, _ := upstream.NewCredentialRepo(env.pool, env.cipher)

	// Ambil role Operator (memiliki izin providers:write & credentials:write)
	opRole, err := rolesRepo.GetByName(ctx, seed.RoleOperator)
	if err != nil {
		t.Fatalf("ambil role operator: %v", err)
	}

	userA, err := usersRepo.Create(ctx, identity.NewUser{
		Email: "tenant-a@example.test", DisplayName: "Tenant A", Password: security.Secret(testPassword),
	})
	if err != nil {
		t.Fatalf("buat user A: %v", err)
	}
	_ = rolesRepo.Grant(ctx, userA.ID, opRole.ID, env.adminUser.ID)

	userB, err := usersRepo.Create(ctx, identity.NewUser{
		Email: "tenant-b@example.test", DisplayName: "Tenant B", Password: security.Secret(testPassword),
	})
	if err != nil {
		t.Fatalf("buat user B: %v", err)
	}
	_ = rolesRepo.Grant(ctx, userB.ID, opRole.ID, env.adminUser.ID)

	// Buat provider publik, provider BYOK A, dan provider BYOK B
	provShared, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "shared-provider-test", DisplayName: "Shared Provider", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provShared: %v", err)
	}

	ownerA := userA.ID
	provA, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "byok-a-test", DisplayName: "BYOK A", Kind: "openai", BaseURL: "https://api.openai.com",
		IsBYOK: true, OwnerUserID: &ownerA,
	})
	if err != nil {
		t.Fatalf("buat provA: %v", err)
	}

	ownerB := userB.ID
	provB, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "byok-b-test", DisplayName: "BYOK B", Kind: "openai", BaseURL: "https://api.openai.com",
		IsBYOK: true, OwnerUserID: &ownerB,
	})
	if err != nil {
		t.Fatalf("buat provB: %v", err)
	}

	// Buat credential di bawah provA
	credA, err := credentialRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: provA.ID, Label: "key-a", Secret: security.Secret("sk-secret-key-a"),
	})
	if err != nil {
		t.Fatalf("buat credA: %v", err)
	}

	// Login sebagai User B (Operator)
	clientB := env.newClient(t)
	clientB.login("tenant-b@example.test", testPassword)

	// User B mencoba mengakses provA -> HARUS 404 (bukan 200, bukan bocor)
	res, _ := clientB.do(http.MethodGet, "/api/admin/upstreams/providers/"+provA.ID, nil, false)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User B mengakses provA GET = %d, diharapkan 404", res.StatusCode)
	}

	// User B mencoba mengarahkan BaseURL provA ke host miliknya -> HARUS 404
	res, _ = clientB.do(http.MethodPut, "/api/admin/upstreams/providers/"+provA.ID, map[string]any{
		"base_url": "https://attacker.test",
	}, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User B memodifikasi BaseURL provA PUT = %d, diharapkan 404", res.StatusCode)
	}

	// User B mencoba menghapus provA -> HARUS 404
	res, _ = clientB.do(http.MethodDelete, "/api/admin/upstreams/providers/"+provA.ID, nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User B menghapus provA DELETE = %d, diharapkan 404", res.StatusCode)
	}

	// User B mencoba toggle provA -> HARUS 404
	res, _ = clientB.do(http.MethodPost, "/api/admin/upstreams/providers/"+provA.ID+"/toggle", map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User B toggle provA POST = %d, diharapkan 404", res.StatusCode)
	}

	// User B mencoba melihat kredensial provA -> HARUS 404
	res, _ = clientB.do(http.MethodGet, "/api/admin/upstreams/providers/"+provA.ID+"/credentials", nil, false)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User B mendaftar kredensial provA GET = %d, diharapkan 404", res.StatusCode)
	}

	// Eksploitasi Rute Bersarang: User B mencoba menghapus credA lewat provB miliknya -> HARUS 404
	res, _ = clientB.do(http.MethodDelete, "/api/admin/upstreams/providers/"+provB.ID+"/credentials/"+credA.ID, nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User B menghapus credA lewat provB = %d, diharapkan 404", res.StatusCode)
	}

	// Eksploitasi Rute Bersarang: User B mencoba toggle credA lewat provB miliknya -> HARUS 404
	res, _ = clientB.do(http.MethodPost, "/api/admin/upstreams/providers/"+provB.ID+"/credentials/"+credA.ID+"/toggle", map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("User B toggle credA lewat provB = %d, diharapkan 404", res.StatusCode)
	}

	// List providers oleh User B: provShared dan provB harus muncul, provA TIDAK BOLEH muncul
	resList, bodyList := clientB.do(http.MethodGet, "/api/admin/upstreams/providers", nil, false)
	if resList.StatusCode != http.StatusOK {
		t.Fatalf("User B list providers = %d", resList.StatusCode)
	}
	var listResp struct {
		Items []ProviderDTO `json:"items"`
	}
	_ = json.Unmarshal(bodyList, &listResp)
	for _, item := range listResp.Items {
		if item.ID == provA.ID {
			t.Fatalf("provA milik Tenant A bocor di list providers Tenant B!")
		}
	}
	_ = provShared
}
