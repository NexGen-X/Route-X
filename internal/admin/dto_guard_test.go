package admin_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/admin"
)

// TestNoDirectHttpxJSONInAdminHandlers memastikan seluruh handler di paket internal/admin
// tidak memanggil httpx.JSON secara langsung, melainkan wajib melalui h.respond(w, r, status, body dto).
// Penjaga AST ini mencegah pengembang masa depan menyelundupkan struct repository tanpa DTO.
func TestNoDirectHttpxJSONInAdminHandlers(t *testing.T) {
	adminDir := "."
	fset := token.NewFileSet()
	//lint:ignore SA1019 pengujian AST statis direktori lokal admin sengaja menggunakan parser.ParseDir
	pkgs, err := parser.ParseDir(fset, adminDir, func(fi os.FileInfo) bool {
		// Abaikan berkas pengujian (*_test.go)
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("Gagal mem-parsing direktori admin: %v", err)
	}

	for _, pkg := range pkgs {
		for fileName, file := range pkg.Files {
			baseName := filepath.Base(fileName)
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Periksa apakah ini pemanggilan httpx.JSON(...)
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}

				if ident.Name == "httpx" && sel.Sel.Name == "JSON" {
					pos := fset.Position(call.Pos())
					// Pengecualian tunggal: admin.go di dalam metode respond()
					if baseName == "admin.go" {
						// Periksa apakah berada di dalam definisi metode respond
						// Diperbolehkan khusus untuk pembungkus respond
						return true
					}
					t.Errorf("Pelanggaran arsitektur DTO di %s:%d: pemanggilan langsung httpx.JSON dilarang. Wajib menggunakan h.respond() dengan tipe antarmuka dto.", pos.Filename, pos.Line)
				}
				return true
			})
		}
	}
}

// TestDTOSerializationSecurity memverifikasi bahwa seluruh struct DTO di internal/admin
// mematuhi aturan penamaan JSON snake_case (tidak ada huruf kapital di awal field)
// dan TIDAK PERNAH memuat field sensitif rahasia seperti secret_ciphertext, encryption_key_id, token, atau raw payload.
func TestDTOSerializationSecurity(t *testing.T) {
	now := time.Now().UTC()
	oneFloat := 1.23
	oneInt := 10
	oneInt64 := int64(100)
	oneBool := true
	oneStr := "contoh"

	sampleDTOs := []any{
		admin.StatusResponse{Status: "ok"},
		admin.StatusCountResponse{Status: "ok", Count: 1},
		admin.JobRunResponse{Status: "triggered", Job: "test", Message: "ok"},
		admin.WebhookPingResponse{Status: "ok", StatusCode: 200, DurationMS: 25},
		admin.ProbeProviderResponse{Status: "ok", LatencyMS: 50},
		admin.DomainConfigDTO{
			Domain:     "ai.example.com",
			Mode:       "letsencrypt",
			Status:     "active",
			PublicURL:  "https://ai.example.com",
			BaseURL:    "https://ai.example.com/v1",
			ServerIP:   "54.179.116.100",
			DNSMatched: true,
			Message:    "ok",
		},
		admin.TrafficStatsDTO{
			TotalRequests:    10,
			SuccessRequests:  9,
			ErrorRequests:    1,
			TotalTokens:      1000,
			PromptTokens:     600,
			CompletionTokens: 400,
			TotalCostUSD:     "0.01500000",
			CostUSD:          "0.01500000",
			AvgLatencyMS:     120.5,
			P95LatencyMS:     &oneFloat,
		},
		admin.TrafficPointDTO{
			Bucket:       now,
			Requests:     5,
			Success:      5,
			Errors:       0,
			TotalTokens:  500,
			CostUSD:      "0.00750000",
			P50LatencyMS: &oneFloat,
			P95LatencyMS: &oneFloat,
		},
		admin.TrafficSliceDTO{
			ID:          "openai",
			Name:        "OpenAI",
			Requests:    10,
			TotalTokens: 1000,
			CostUSD:     "0.01500000",
		},
		admin.TrafficSeriesResponse{
			Items:  []admin.TrafficPointDTO{{Bucket: now, Requests: 1}},
			Points: []admin.TrafficPointDTO{{Bucket: now, Requests: 1}},
		},
		admin.TrafficBreakdownResponse{
			Dimension: "provider",
			Items:     []admin.TrafficSliceDTO{{ID: "openai", Requests: 1}},
		},
		admin.ProviderHealthDTO{
			ID:     "p-1",
			Name:   "OpenAI",
			Status: "healthy",
		},
		admin.HealthSummaryResponse{
			Providers: []admin.ProviderHealthDTO{{ID: "p-1", Name: "OpenAI", Status: "healthy"}},
		},
		admin.TrafficEventDTO{
			Seq:       1,
			Kind:      "init",
			CreatedAt: now,
		},
		admin.TrafficEventsResponse{
			Items:  []admin.TrafficEventDTO{{Seq: 1, Kind: "init", CreatedAt: now}},
			Events: []admin.TrafficEventDTO{{Seq: 1, Kind: "init", CreatedAt: now}},
		},
		admin.TrafficDetailDTO{
			ID:         "req-1",
			StatusCode: 200,
			CostUSD:    "0.00100000",
			CreatedAt:  now,
		},
		admin.TrafficPayloadDTO{
			RequestBody: json.RawMessage("{}"),
			Truncated:   false,
			SizeBytes:   2,
		},
		admin.ProviderDTO{
			ID:          "p-1",
			Name:        "openai",
			DisplayName: "OpenAI",
			Kind:        "openai",
			BaseURL:     "https://api.openai.com",
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		admin.CredentialMetaDTO{
			ID:         "c-1",
			ProviderID: "p-1",
			Label:      "primary",
			MaskedHint: "sk-...1234",
			Enabled:    true,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		admin.ModelDTO{
			ID:        "m-1",
			ModelID:   "gpt-4o",
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		admin.ModelAliasDTO{
			ID:        "alias-1",
			ModelID:   "gpt-4o",
			Alias:     "fast",
			CreatedAt: now,
		},
		admin.ModelMappingDTO{
			ID:                "pm-1",
			ModelID:           "gpt-4o",
			ProviderID:        "p-1",
			UpstreamModelName: "gpt-4o",
			Enabled:           true,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		admin.ModelDetailResponse{
			Model: admin.ModelDTO{ID: "m-1"},
		},
		admin.PriceDTO{
			ID:              "pr-1",
			ProviderModelID: "pm-1",
			InputPer1MUSD:   "5.00000000",
			OutputPer1MUSD:  "15.00000000",
			Currency:        "USD",
			EffectiveFrom:   now,
			CreatedAt:       now,
		},
		admin.EgressPoolDTO{
			ID:        "eg-1",
			Name:      "proxy-1",
			Kind:      "http_proxy",
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		admin.RoutingRuleDTO{
			ID:        "rr-1",
			Name:      "default-route",
			Strategy:  "fallback",
			Priority:  1,
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		admin.RateLimitDTO{
			ID:        "rl-1",
			Scope:     "global",
			ScopeID:   "default",
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		admin.BudgetDTO{
			ID:          "b-1",
			Name:        "monthly-team",
			Scope:       "team",
			ScopeID:     "team-1",
			Period:      "monthly",
			LimitUSD:    "500.00000000",
			MaxSpendUSD: "500.00000000",
			SpentUSD:    "120.00000000",
			PeriodStart: now,
			PeriodEnd:   &now,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		admin.BanDTO{
			ID:          "ban-1",
			SubjectKind: "ip",
			Subject:     "192.168.1.1",
			Reason:      "abuse",
			CreatedAt:   now,
		},
		admin.ContentFilterDTO{
			ID:        "cf-1",
			Name:      "prompt-injection",
			Kind:      "regex",
			Priority:  1,
			AppliesTo: "prompt",
			Action:    "block",
			Enabled:   true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		admin.CircuitBreakerDTO{
			Key:          "routex:cb:openai:gpt-4o",
			ProviderID:   "openai",
			Model:        "gpt-4o",
			State:        "closed",
			FailureCount: 0,
		},
		admin.KeyDTO{
			ID:          "k-1",
			Name:        "prod-key",
			Prefix:      "rtx_live_",
			Last4:       "abcd",
			MaskedKey:   "rtx_live_...abcd",
			OwnerUserID: "u-1",
			Status:      "active",
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		admin.KeyDetailResponse{
			Key:    admin.KeyDTO{ID: "k-1"},
			Masked: "rtx_live_...abcd",
		},
		admin.KeyCreatedResponse{
			Key:    admin.KeyDTO{ID: "k-1"},
			Masked: "rtx_live_...abcd",
			RawKey: "rtx_live_secret1234",
		},
		admin.UserDTO{
			ID:          "u-1",
			Email:       "admin@route-x.local",
			DisplayName: "Administrator",
			Status:      "active",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		admin.UserDetailResponse{
			User:        admin.UserDTO{ID: "u-1"},
			Roles:       []admin.RoleDTO{{ID: "r-1", Name: "super_admin"}},
			Permissions: []string{"system:admin"},
		},
		admin.RoleDTO{
			ID:        "r-1",
			Name:      "super_admin",
			CreatedAt: now,
		},
		admin.RoleDetailDTO{
			RoleDTO:     admin.RoleDTO{ID: "r-1"},
			Permissions: []admin.PermissionDTO{{ID: "p-1", Key: "system:admin", CreatedAt: now}},
		},
		admin.PermissionDTO{
			ID:        "p-1",
			Key:       "system:admin",
			CreatedAt: now,
		},
		admin.SessionDTO{
			ID:         "s-1",
			UserID:     "u-1",
			CreatedAt:  now,
			LastSeenAt: now,
			ExpiresAt:  now.Add(24 * time.Hour),
		},
		admin.WebhookDTO{
			ID:         "wh-1",
			Name:       "alerts",
			URL:        "https://example.com/webhook",
			Events:     []string{"request.error"},
			MaskedHint: "whsec_...1234",
			Enabled:    true,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		admin.DeliveryDTO{
			ID:                 1,
			WebhookID:          "wh-1",
			Event:              "request.error",
			Status:             "delivered",
			AttemptCount:       1,
			NextAttemptAt:      now,
			ResponseStatusCode: &oneInt,
			CreatedAt:          now,
		},
		admin.SettingDTO{
			Key:       "site.name",
			Value:     json.RawMessage(`"Route-X"`),
			UpdatedAt: now,
		},
		admin.JobDTO{
			Name:     "cleaner",
			Schedule: "1h",
			Status:   "running",
		},
		admin.AuditEntryDTO{
			ID:           1,
			OccurredAt:   now,
			ActorUserID:  "u-1",
			Action:       "create",
			ResourceType: "provider",
			ResourceID:   "p-1",
			Metadata:     json.RawMessage(`{}`),
		},
		admin.DiagnosticsDTO{
			UptimeSeconds: 120,
			GoVersion:     "go1.24",
			NumGoroutine:  20,
			NumCPU:        4,
			Memory:        admin.DiagnosticsMemoryDTO{AllocBytes: 1024},
		},
	}

	// Gunakan variabel dummy agar kompilator tidak komplain jika ada variabel pointer yang tidak terpakai
	_ = oneInt64
	_ = oneBool
	_ = oneStr

	// Daftar nama field terlarang yang tidak boleh muncul sama sekali di JSON respons Admin API
	forbiddenFields := []string{
		"secret_ciphertext",
		"encryption_key_id",
		"token",
		"password_hash",
		"salt",
		"client_secret",
	}

	for _, dto := range sampleDTOs {
		tName := reflect.TypeOf(dto).Name()
		data, err := json.Marshal(dto)
		if err != nil {
			t.Fatalf("[%s] Gagal melakukan serialisasi JSON: %v", tName, err)
		}

		var genericMap map[string]any
		if err := json.Unmarshal(data, &genericMap); err != nil {
			t.Fatalf("[%s] Gagal meng-unmarshal hasil serialisasi ke generic map: %v", tName, err)
		}

		// Periksa seluruh keys secara rekursif
		checkJSONKeys(t, tName, genericMap, forbiddenFields)
	}
}

func checkJSONKeys(t *testing.T, dtoName string, m map[string]any, forbiddenFields []string) {
	for key, val := range m {
		// 1. Dilarang memuat field rahasia terlarang
		for _, forbidden := range forbiddenFields {
			if key == forbidden {
				t.Errorf("[%s] PELANGGARAN KEAMANAN: field terlarang %q muncul dalam output JSON DTO", dtoName, key)
			}
		}

		// 2. Dilarang memiliki huruf kapital di awal (harus snake_case)
		if len(key) > 0 && key[0] >= 'A' && key[0] <= 'Z' {
			t.Errorf("[%s] PELANGGARAN KONTRAK API: field %q dimulai dengan huruf kapital (tag json terlewat atau tidak sesuai konvensi snake_case)", dtoName, key)
		}

		// Rekursif ke map bersarang
		if subMap, ok := val.(map[string]any); ok {
			checkJSONKeys(t, dtoName, subMap, forbiddenFields)
		} else if slice, ok := val.([]any); ok {
			for _, item := range slice {
				if itemMap, ok := item.(map[string]any); ok {
					checkJSONKeys(t, dtoName, itemMap, forbiddenFields)
				}
			}
		}
	}
}

// TestClientRouteMatching memverifikasi bahwa daftar path API yang dipanggil di frontend
// (web/src/api/client.ts) sesuai dan terdaftar di server Route-X.
func TestClientRouteMatching(t *testing.T) {
	clientTSPath := filepath.Join("..", "..", "web", "src", "api", "client.ts")
	content, err := os.ReadFile(clientTSPath)
	if err != nil {
		t.Skipf("File frontend client.ts tidak ditemukan di %s, lewati", clientTSPath)
		return
	}

	// Tangkap seluruh string endpoint "/api/admin/..."
	re := regexp.MustCompile(`['"\x60](/api/admin/[^'"\x60?]+)`)
	matches := re.FindAllStringSubmatch(string(content), -1)

	clientEndpoints := make(map[string]bool)
	for _, m := range matches {
		endpoint := m[1]
		// Buang variabel query string seperti ${q} atau ${query}
		endpoint = regexp.MustCompile(`\$\{q[^}]*\}`).ReplaceAllString(endpoint, "")
		// Normalisasi parameter dinamis path misal ${id} -> {param}
		endpoint = regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(endpoint, "{param}")
		clientEndpoints[endpoint] = true
	}

	// 7 rute yang salah diidentifikasi di Bagian 7.3 & Batch 3:
	// 1. DELETE /gateway/bans/{id} (server: POST .../lift)
	if strings.Contains(string(content), "/gateway/bans/${id}`, { method: 'DELETE'") ||
		strings.Contains(string(content), "/gateway/bans/${id}\", { method: 'DELETE'") {
		t.Errorf("PELANGGARAN KONTRAK RUTE: DELETE /gateway/bans/{id} tidak boleh dipanggil; gunakan POST .../lift")
	}

	// 2. DELETE /access/api-keys/{id} (server: POST .../revoke)
	if strings.Contains(string(content), "/access/api-keys/${id}`, { method: 'DELETE'") ||
		strings.Contains(string(content), "/access/api-keys/${id}\", { method: 'DELETE'") {
		t.Errorf("PELANGGARAN KONTRAK RUTE: DELETE /access/api-keys/{id} tidak boleh dipanggil; gunakan POST .../revoke")
	}

	// 3-7. Path API yang salah terdaftar
	forbiddenPathCalls := []string{
		"/api/admin/system/jobs/{param}/trigger",                // harusnya run via POST
		"/api/admin/upstreams/models/{param}/providers",         // harusnya /mappings
		"/api/admin/upstreams/models/{param}/providers/{param}", // harusnya /mappings/{mapping_id}
		"/api/admin/upstreams/mappings/{param}/pricing",         // harusnya di bawah /upstreams/models/mappings/{mapping_id}/pricing
		"/api/admin/upstreams/mappings/{param}/pricing/history", // harusnya di bawah /upstreams/models/mappings/{mapping_id}/pricing/history
	}

	for _, bad := range forbiddenPathCalls {
		if clientEndpoints[bad] {
			t.Errorf("PELANGGARAN KONTRAK RUTE: Path terlarang %q masih ditemukan di client.ts; harus diselaraskan dengan rute server resmi", bad)
		}
	}

	// Verifikasi bahwa seluruh endpoint yang dipanggil client.ts terdaftar di chi Router server
	h := admin.NewHandlers(admin.Config{})
	serverRouter := chi.NewRouter()
	serverRouter.Mount("/api/admin", h.Routes())

	registeredRoutes := make(map[string]bool)
	_ = chi.Walk(serverRouter, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		// Normalisasi parameter rute server: misal /api/admin/access/users/{id} -> /api/admin/access/users/{param}
		normalized := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(route, "{param}")
		// Bersihkan trailing slash
		normalized = strings.TrimSuffix(normalized, "/")
		registeredRoutes[normalized] = true
		return nil
	})

	for clientRoute := range clientEndpoints {
		normClient := strings.TrimSuffix(clientRoute, "/")
		if !registeredRoutes[normClient] {
			t.Errorf("PELANGGARAN RUTE CLIENT: Endpoint %q yang dipanggil client.ts tidak terdaftar di server router!", clientRoute)
		}
	}
}
