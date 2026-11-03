package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

// --- B-102: peran pengguna sungguhan ---------------------------------------
//
// Bukti live Langkah 7 yang diregresikan di sini: sebelum perbaikan,
// CreateUserRequest tidak punya field peran, pengguna hasil createUser
// membawa roles:["Admin"] + permissions:["*"], dan sesinya membaca
// GET /api/admin/access/users dengan 200. Sesudah perbaikan: peran adalah
// kolom is_admin, bawaan pembuatan adalah NON-admin, admin hanya lahir dari
// role:"admin" eksplisit, dan Principal.Can memeriksa izin sungguhan
// sehingga RequirePermission menolak non-admin 403 di rute admin.

type userDTOUji struct {
	ID      string   `json:"id"`
	Email   string   `json:"email"`
	IsAdmin bool     `json:"is_admin"`
	Roles   []string `json:"roles"`
}

func buatPenggunaUji(t *testing.T, c *adminClient, email string, extra map[string]any) (*http.Response, userDTOUji, []byte) {
	t.Helper()
	body := map[string]any{
		"email":    email,
		"name":     "Pengguna Uji B-102",
		"password": testPassword,
	}
	for k, v := range extra {
		body[k] = v
	}
	res, raw := c.do(http.MethodPost, "/api/admin/access/users", body, true)
	var dto userDTOUji
	if res.StatusCode == http.StatusCreated {
		if err := json.Unmarshal(raw, &dto); err != nil {
			t.Fatalf("urai respons buat pengguna: %v (body: %s)", err, raw)
		}
	}
	return res, dto, raw
}

// 1. Tanpa field role: hasilnya NON-admin dan sesinya ditolak 403 di rute
// admin — persis kebalikan bukti live B-102.
func TestB102BuatTanpaPeranMenjadiNonAdmin(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)

	res, dto, raw := buatPenggunaUji(t, admin, "bawaan@routex.internal", nil)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat pengguna status = %d, mau 201 (body: %s)", res.StatusCode, raw)
	}
	if dto.IsAdmin {
		t.Errorf("is_admin = true padahal role tidak disebut; bawaan harus non-admin")
	}
	if len(dto.Roles) != 1 || dto.Roles[0] != "User" {
		t.Errorf("roles = %v, mau [User]", dto.Roles)
	}

	// Detail dari sisi admin: permissions kosong, bukan ["*"].
	res, raw = admin.do(http.MethodGet, "/api/admin/access/users/"+dto.ID, nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("detail pengguna status = %d, mau 200", res.StatusCode)
	}
	var detail UserDetailResponse
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("urai detail: %v", err)
	}
	if len(detail.Permissions) != 0 {
		t.Errorf("permissions detail = %v, mau kosong untuk non-admin", detail.Permissions)
	}

	// Sesi pengguna baru: login sah, tetapi rute admin menolak 403.
	pengguna := env.newClient(t)
	pengguna.login("bawaan@routex.internal", testPassword)
	res, _ = pengguna.do(http.MethodGet, "/api/admin/access/users", nil, false)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin GET /access/users status = %d, mau 403", res.StatusCode)
	}
	res, _ = pengguna.do(http.MethodGet, "/api/admin/system/diagnostics", nil, false)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin GET /system/diagnostics status = %d, mau 403", res.StatusCode)
	}
}

// 2. role:"admin" eksplisit: tetap admin penuh, rute admin menjawab 200.
func TestB102BuatDenganPeranAdminEksplisit(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)

	res, dto, raw := buatPenggunaUji(t, admin, "eksplisit@routex.internal", map[string]any{"role": "admin"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat admin status = %d, mau 201 (body: %s)", res.StatusCode, raw)
	}
	if !dto.IsAdmin || len(dto.Roles) != 1 || dto.Roles[0] != "Admin" {
		t.Errorf("dto = %+v, mau is_admin=true roles=[Admin]", dto)
	}

	pengguna := env.newClient(t)
	pengguna.login("eksplisit@routex.internal", testPassword)
	res, _ = pengguna.do(http.MethodGet, "/api/admin/access/users", nil, false)
	if res.StatusCode != http.StatusOK {
		t.Errorf("admin eksplisit GET /access/users status = %d, mau 200", res.StatusCode)
	}
}

// 3. Nilai peran di luar taksonomi ditolak 400 di pembuatan maupun pembaruan.
func TestB102PeranTidakSahDitolak(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)

	res, _, raw := buatPenggunaUji(t, admin, "aneh@routex.internal", map[string]any{"role": "superuser"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("buat role aneh status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}

	res, raw = admin.do(http.MethodPut, "/api/admin/access/users/"+env.viewerUser.ID,
		map[string]any{"role": "pemilik"}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("update role aneh status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}
}

// 4. Pembaruan peran bekerja dua arah dan berlaku pada sesi yang SEDANG
// berjalan (principal dimuat ulang dari baris pengguna pada tiap request).
func TestB102UbahPeranDuaArah(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)

	_, dto, _ := buatPenggunaUji(t, admin, "naikturun@routex.internal", nil)

	pengguna := env.newClient(t)
	pengguna.login("naikturun@routex.internal", testPassword)
	if res, _ := pengguna.do(http.MethodGet, "/api/admin/access/users", nil, false); res.StatusCode != http.StatusForbidden {
		t.Fatalf("sebelum naik peran: status = %d, mau 403", res.StatusCode)
	}

	// Naik: user -> admin.
	res, raw := admin.do(http.MethodPut, "/api/admin/access/users/"+dto.ID, map[string]any{"role": "admin"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("naikkan peran status = %d, mau 200 (body: %s)", res.StatusCode, raw)
	}
	var naik userDTOUji
	if err := json.Unmarshal(raw, &naik); err != nil || !naik.IsAdmin {
		t.Errorf("sesudah naik: dto = %+v err = %v, mau is_admin=true", naik, err)
	}
	if res, _ := pengguna.do(http.MethodGet, "/api/admin/access/users", nil, false); res.StatusCode != http.StatusOK {
		t.Errorf("sesudah naik peran, sesi yang sama: status = %d, mau 200", res.StatusCode)
	}

	// Turun: admin -> user.
	res, raw = admin.do(http.MethodPut, "/api/admin/access/users/"+dto.ID, map[string]any{"role": "user"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("turunkan peran status = %d, mau 200 (body: %s)", res.StatusCode, raw)
	}
	var turun userDTOUji
	if err := json.Unmarshal(raw, &turun); err != nil || turun.IsAdmin {
		t.Errorf("sesudah turun: dto = %+v err = %v, mau is_admin=false", turun, err)
	}
	if res, _ := pengguna.do(http.MethodGet, "/api/admin/access/users", nil, false); res.StatusCode != http.StatusForbidden {
		t.Errorf("sesudah turun peran, sesi yang sama: status = %d, mau 403", res.StatusCode)
	}
}

// 5. Menurunkan peran akun sendiri ditolak — admin terakhir tidak bisa
// dikunci keluar lewat jalurnya sendiri.
func TestB102TurunkanPeranSendiriDitolak(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)

	res, raw := admin.do(http.MethodPut, "/api/admin/access/users/"+env.adminUser.ID,
		map[string]any{"role": "user"}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("turunkan peran sendiri status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}

	// Dan admin itu tetap admin.
	if res, _ := admin.do(http.MethodGet, "/api/admin/access/users", nil, false); res.StatusCode != http.StatusOK {
		t.Errorf("admin sesudah penolakan: status = %d, mau 200", res.StatusCode)
	}
}
