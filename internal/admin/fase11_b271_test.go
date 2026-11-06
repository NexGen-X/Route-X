package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// FASE 11 B-271 (PB-26): pada forceResetPassword, kegagalan RevokeAllOfUser
// menjawab 500 revoke_failed TANPA writeAudit — padahal password pengguna
// SUDAH diganti pada langkah sebelumnya. Perubahan kredensial yang benar-
// benar terjadi tidak boleh absen dari jejak audit; jalur gagal itu kini
// menulis audit force_reset_password dengan metadata penanda hasil sebagian
// (pencabutan_sesi_gagal: true) sebelum menjawab 500.
//
// Kegagalan pencabutan dipicu secara deterministik dengan mengarahkan
// sessionsRepo handler ke database yang tidak memuat tabel sessions.

func TestFase11B271ForceResetGagalCabutTetapTeraudit(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	target := env.viewerUser.ID

	// Suntik kegagalan RevokeAllOfUser secara bedah: ganti sessionsRepo
	// handler dengan repo di atas pool ke database yang tidak memuat
	// tabel sessions sama sekali (database postgres bawaan di server
	// yang sama) — query pencabutan pasti gagal, sementara
	// ChangePassword dan penulisan audit (pool asli) tetap sehat.
	// Menghancurkan tabel sessions di schema uji TIDAK bisa dipakai:
	// tabel itu menampung sesi login admin si penguji (permintaan
	// berikutnya akan 401), dan pegangan berskala server lain di atas
	// namanya (indeks partisi requests) ikut rusak.
	env.handlers.sessionsRepo = identity.NewSessions(bukaPoolButa(t))

	res, body := client.do(http.MethodPost,
		"/api/admin/access/users/"+target+"/force-reset-password",
		map[string]any{"temp_password": "GANTI_temp_b271!"}, true)
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500: %s", res.StatusCode, body)
	}
	// Catatan: kode revoke_failed dari httpx.WriteError tidak sampai ke
	// body pada harness ini (envelope generik internal_error) — yang
	// dikunci test ini adalah status 500 + baris auditnya, bukan teks body.

	var meta string
	err := env.pool.QueryRow(ctx,
		`select metadata::text from audit_logs
		  where action = 'force_reset_password' and resource_type = 'user' and resource_id = $1`,
		target).Scan(&meta)
	if err != nil {
		t.Fatalf("audit force_reset_password pada jalur gagal TIDAK ditemukan (B-271): %v", err)
	}
	if !strings.Contains(meta, `"pencabutan_sesi_gagal": true`) {
		t.Fatalf("metadata audit = %s, mau memuat pencabutan_sesi_gagal: true", meta)
	}
	if !strings.Contains(meta, `"sesi_tercabut": 0`) {
		t.Fatalf("metadata audit = %s, mau memuat sesi_tercabut: 0", meta)
	}
}
