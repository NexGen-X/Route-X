package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestAdminUnlockUser membuktikan endpoint POST
// /api/admin/access/users/{id}/unlock (audit3 B-076): akun terkunci
// (status locked + locked_until + penghitung gagal terisi) kembali aktif
// dan bersih, tercatat di audit log, idempoten untuk akun yang tidak
// terkunci, dan 404 untuk pengguna yang tidak ada.
func TestAdminUnlockUser(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	ctx := context.Background()
	usersRepo := identity.NewUsers(env.pool)

	// Pengguna sasaran yang dikunci seperti oleh mekanisme brute force:
	// status locked, lima kegagalan, dan locked_until di masa depan.
	sasaran, err := usersRepo.Create(ctx, identity.NewUser{
		Email:              "terkunci@routex.internal",
		DisplayName:        "Pengguna Terkunci",
		Password:           security.Secret(testPassword),
		MustChangePassword: false,
	})
	if err != nil {
		t.Fatalf("buat pengguna sasaran: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `UPDATE users
		SET status = 'locked', failed_login_attempts = 5, locked_until = $2
		WHERE id = $1`, sasaran.ID, time.Now().Add(15*time.Minute)); err != nil {
		t.Fatalf("mengunci pengguna sasaran: %v", err)
	}

	res, body := client.do(http.MethodPost, "/api/admin/access/users/"+sasaran.ID+"/unlock", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unlock status = %d, mau 200. body: %s", res.StatusCode, body)
	}
	var dto struct {
		Status              string `json:"status"`
		FailedLoginAttempts int    `json:"failed_login_attempts"`
		LockedUntil         any    `json:"locked_until"`
	}
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("urai respons unlock: %v (body: %s)", err, body)
	}
	if dto.Status != "active" {
		t.Errorf("status respons = %q, mau active", dto.Status)
	}
	if dto.FailedLoginAttempts != 0 {
		t.Errorf("failed_login_attempts respons = %d, mau 0", dto.FailedLoginAttempts)
	}
	if dto.LockedUntil != nil {
		t.Errorf("locked_until respons = %v, mau kosong/null", dto.LockedUntil)
	}

	// Keadaan persistennya ikut bersih (bukan hanya DTO).
	sesudah, err := usersRepo.GetByID(ctx, sasaran.ID)
	if err != nil {
		t.Fatalf("baca ulang pengguna: %v", err)
	}
	if sesudah.Status != identity.UserStatusActive || sesudah.FailedLoginAttempts != 0 || sesudah.LockedUntil != nil {
		t.Errorf("keadaan persisten belum terbuka: status=%s gagal=%d locked_until=%v",
			sesudah.Status, sesudah.FailedLoginAttempts, sesudah.LockedUntil)
	}

	// Tercatat di audit log seperti mutasi user lain.
	var jumlahAudit int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs
		WHERE action = 'unlock' AND resource_type = 'user' AND resource_id = $1`, sasaran.ID).Scan(&jumlahAudit); err != nil {
		t.Fatalf("membaca audit log: %v", err)
	}
	if jumlahAudit != 1 {
		t.Errorf("baris audit unlock = %d, mau 1", jumlahAudit)
	}

	// Idempoten: pengguna yang sama (kini aktif, tidak terkunci) dibuka
	// kuncinya lagi → tetap 200 active, bukan error.
	res, body = client.do(http.MethodPost, "/api/admin/access/users/"+sasaran.ID+"/unlock", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unlock ulang status = %d, mau 200. body: %s", res.StatusCode, body)
	}

	// Pengguna yang tidak ada → 404.
	res, body = client.do(http.MethodPost, "/api/admin/access/users/00000000-0000-0000-0000-000000000000/unlock", nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("unlock pengguna tak ada status = %d, mau 404. body: %s", res.StatusCode, body)
	}
}
