package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Test pengunci Fase 6 kampanye tuntas-logika, wilayah auth/admin:
//
// B-193: penjaga BYOK authorizeProviderAccess tidak pernah membandingkan
// pemilik — principal terautentikasi mana pun lolos, dan daftar provider
// tidak disaring per pemilik (filter repo AccessibleByUserID mati).
// B-194: updateUser bermutasi langkah demi langkah (nama ter-commit
// sebelum peran divalidasi) dan audit ditulis sesudah baca-ulang — body
// campuran sah+tak sah meninggalkan perubahan parsial tanpa audit.
// B-195: deleteSetting melewati penjaga kunci terlarang milik putSetting.

// --- B-193 -------------------------------------------------------------------

func TestB193PenjagaBYOKMembandingkanPemilik(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	providerRepo := upstream.NewProviderRepo(env.pool)

	bersama, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b193-bersama", DisplayName: "Bersama", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider bersama: %v", err)
	}
	byokViewer, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b193-byok-viewer", DisplayName: "BYOK Viewer", Kind: "openai", BaseURL: "https://api.openai.com",
		IsBYOK: true, OwnerUserID: &env.viewerUser.ID,
	})
	if err != nil {
		t.Fatalf("buat BYOK viewer: %v", err)
	}
	byokAdmin, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b193-byok-admin", DisplayName: "BYOK Admin", Kind: "openai", BaseURL: "https://api.openai.com",
		IsBYOK: true, OwnerUserID: &env.adminUser.ID,
	})
	if err != nil {
		t.Fatalf("buat BYOK admin: %v", err)
	}

	ctxViewer := auth.WithPrincipal(ctx, &auth.Principal{User: env.viewerUser})
	ctxAdmin := auth.WithPrincipal(ctx, &auth.Principal{User: env.adminUser})
	h := env.handlers

	// Pemilik membuka BYOK miliknya.
	if _, err := h.authorizeProviderAccess(ctxViewer, byokViewer.ID); err != nil {
		t.Errorf("pemilik membuka BYOK sendiri: err = %v, mau nil", err)
	}
	// Bukan pemilik: harus ErrNotFound (keberadaan tidak bocor).
	if _, err := h.authorizeProviderAccess(ctxViewer, byokAdmin.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("non-pemilik membuka BYOK orang lain: err = %v, mau ErrNotFound", err)
	}
	// Admin penuh membuka BYOK siapa pun.
	if _, err := h.authorizeProviderAccess(ctxAdmin, byokViewer.ID); err != nil {
		t.Errorf("admin membuka BYOK viewer: err = %v, mau nil", err)
	}
	// Provider bersama terbuka untuk principal mana pun.
	if _, err := h.authorizeProviderAccess(ctxViewer, bersama.ID); err != nil {
		t.Errorf("provider bersama untuk viewer: err = %v, mau nil", err)
	}
	// Tanpa principal: BYOK tertutup.
	if _, err := h.authorizeProviderAccess(ctx, byokViewer.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("tanpa principal membuka BYOK: err = %v, mau ErrNotFound", err)
	}
}

func TestB193DaftarProviderMenyaringBYOKOrangLain(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	providerRepo := upstream.NewProviderRepo(env.pool)

	buat := func(nama string, byok bool, pemilik *string) *upstream.Provider {
		t.Helper()
		p, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: "openai", BaseURL: "https://api.openai.com",
			IsBYOK: byok, OwnerUserID: pemilik,
		})
		if err != nil {
			t.Fatalf("buat %s: %v", nama, err)
		}
		return p
	}
	bersama := buat("b193-list-bersama", false, nil)
	byokViewer := buat("b193-list-viewer", true, &env.viewerUser.ID)
	byokAdmin := buat("b193-list-admin", true, &env.adminUser.ID)

	daftar := func(p *auth.Principal) map[string]bool {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/admin/providers?search=b193-list", nil)
		if p != nil {
			req = req.WithContext(auth.WithPrincipal(req.Context(), p))
		}
		rec := httptest.NewRecorder()
		env.handlers.listProviders(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("listProviders status = %d (body: %s)", rec.Code, rec.Body.String())
		}
		var envResp struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envResp); err != nil {
			t.Fatalf("urai daftar provider: %v", err)
		}
		ada := map[string]bool{}
		for _, it := range envResp.Items {
			ada[it.ID] = true
		}
		return ada
	}

	punyaViewer := daftar(&auth.Principal{User: env.viewerUser})
	if !punyaViewer[bersama.ID] || !punyaViewer[byokViewer.ID] {
		t.Errorf("daftar viewer kehilangan provider yang sah ia lihat: %v", punyaViewer)
	}
	if punyaViewer[byokAdmin.ID] {
		t.Errorf("daftar viewer memuat BYOK milik admin — isolasi daftar tidak berjalan")
	}

	punyaAdmin := daftar(&auth.Principal{User: env.adminUser})
	if !punyaAdmin[bersama.ID] || !punyaAdmin[byokViewer.ID] || !punyaAdmin[byokAdmin.ID] {
		t.Errorf("daftar admin harus memuat ketiganya: %v", punyaAdmin)
	}
}

// --- B-194 -------------------------------------------------------------------

func jumlahAuditUpdateUser(t *testing.T, env *testEnv, userID string) int {
	t.Helper()
	var n int
	err := env.pool.QueryRow(context.Background(),
		`select count(*) from audit_logs where resource_type = 'user' and resource_id = $1 and action = 'update'`,
		userID).Scan(&n)
	if err != nil {
		t.Fatalf("hitung audit update user: %v", err)
	}
	return n
}

func namaPengguna(t *testing.T, c *adminClient, userID string) string {
	t.Helper()
	res, raw := c.do(http.MethodGet, "/api/admin/access/users/"+userID, nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ambil pengguna status = %d (body: %s)", res.StatusCode, raw)
	}
	var dto struct {
		User struct {
			DisplayName string `json:"display_name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &dto); err != nil {
		t.Fatalf("urai pengguna: %v", err)
	}
	return dto.User.DisplayName
}

func TestB194UpdateCampuranTakSahTidakMeninggalkanPerubahanParsial(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)

	_, dto, raw := buatPenggunaUji(t, admin, "b194-target@routex.internal", nil)
	if dto.ID == "" {
		t.Fatalf("buat pengguna uji gagal: %s", raw)
	}
	namaAwal := namaPengguna(t, admin, dto.ID)

	// Body campuran: nama sah + peran tak sah. Pra-fix nama ter-commit
	// lebih dulu, handler menjawab 400, dan tidak ada audit.
	res, raw := admin.do(http.MethodPut, "/api/admin/access/users/"+dto.ID,
		map[string]any{"name": "Nama Berubah", "role": "bogus"}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("update role bogus status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}
	if got := namaPengguna(t, admin, dto.ID); got != namaAwal {
		t.Errorf("nama ikut berubah menjadi %q padahal body ditolak — perubahan parsial terjadi", got)
	}
	if n := jumlahAuditUpdateUser(t, env, dto.ID); n != 0 {
		t.Errorf("audit update = %d untuk update yang ditolak, mau 0", n)
	}

	// Status tak sah juga divalidasi di muka (pra-fix: diteruskan ke
	// constraint DB sebagai 500 sesudah nama ter-commit).
	res, raw = admin.do(http.MethodPut, "/api/admin/access/users/"+dto.ID,
		map[string]any{"name": "Nama Berubah", "status": "bogus"}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("update status bogus status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}
	if got := namaPengguna(t, admin, dto.ID); got != namaAwal {
		t.Errorf("nama ikut berubah menjadi %q padahal status ditolak", got)
	}

	// Kontrol: body campuran yang SAH berlaku penuh dan teraudit sekali.
	res, raw = admin.do(http.MethodPut, "/api/admin/access/users/"+dto.ID,
		map[string]any{"name": "Nama Sah Baru", "role": "admin"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update sah status = %d, mau 200 (body: %s)", res.StatusCode, raw)
	}
	if got := namaPengguna(t, admin, dto.ID); got != "Nama Sah Baru" {
		t.Errorf("nama sesudah update sah = %q, mau %q", got, "Nama Sah Baru")
	}
	if n := jumlahAuditUpdateUser(t, env, dto.ID); n != 1 {
		t.Errorf("audit update = %d sesudah satu update sah, mau 1", n)
	}
}

func TestB194TurunPeranSendiriDitolakSebelumNamaBerubah(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)
	namaAwal := namaPengguna(t, admin, env.adminUser.ID)

	res, raw := admin.do(http.MethodPut, "/api/admin/access/users/"+env.adminUser.ID,
		map[string]any{"name": "Nama Admin Berubah", "role": "user"}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("turun peran sendiri status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}
	if !json.Valid(raw) || !mengandungKode(raw, "self_demote_forbidden") {
		t.Errorf("kode error = %s, mau self_demote_forbidden", raw)
	}
	if got := namaPengguna(t, admin, env.adminUser.ID); got != namaAwal {
		t.Errorf("nama admin ikut berubah menjadi %q padahal penurunan peran ditolak di muka", got)
	}
}

func mengandungKode(raw []byte, kode string) bool {
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return false
	}
	return e.Error.Code == kode
}

// --- B-195 -------------------------------------------------------------------

func TestB195DeleteSettingMenjagaKunciTerlarang(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)
	ctx := context.Background()

	// Tanam baris cli:config:* langsung lewat repo (jalur khususnya
	// sendiri), lalu coba hapus lewat endpoint generik.
	kunci := "cli:config:b195-uji"
	if _, err := env.handlers.settingsRepo.Put(ctx, identity.SettingWrite{
		Key:   kunci,
		Value: json.RawMessage(`{"token":"GANTI_token_uji"}`),
	}); err != nil {
		t.Fatalf("tanam setting: %v", err)
	}

	res, raw := admin.do(http.MethodDelete, "/api/admin/system/settings/"+kunci, nil, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("DELETE kunci terlarang status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}
	if !mengandungKode(raw, "reserved_setting") {
		t.Errorf("kode error = %s, mau reserved_setting (sama seperti PUT)", raw)
	}
	if _, err := env.handlers.settingsRepo.Get(ctx, kunci); err != nil {
		t.Errorf("baris %s hilang padahal DELETE ditolak: %v", kunci, err)
	}

	// Kontrol: PUT kunci terlarang tetap ditolak dengan kode yang sama.
	res, _ = admin.do(http.MethodPut, "/api/admin/system/settings/"+kunci,
		map[string]any{"value": json.RawMessage(`{}`)}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT kunci terlarang status = %d, mau 400", res.StatusCode)
	}

	// Kontrol: kunci biasa tetap bisa ditulis dan dihapus.
	kunciBiasa := "b195.kunci.biasa"
	res, raw = admin.do(http.MethodPut, "/api/admin/system/settings/"+kunciBiasa,
		map[string]any{"value": json.RawMessage(`"halo"`)}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT kunci biasa status = %d (body: %s)", res.StatusCode, raw)
	}
	res, raw = admin.do(http.MethodDelete, "/api/admin/system/settings/"+kunciBiasa, nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE kunci biasa status = %d, mau 200 (body: %s)", res.StatusCode, raw)
	}
}
