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
