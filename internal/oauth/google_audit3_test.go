package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAntigravitySecretHanyaDariEnv membuktikan fallback tertanam XOR sudah
// dihapus (audit3 V-EO-24): tanpa env, fungsi mengembalikan string kosong dan
// exchange/refresh client Antigravity gagal dengan pesan yang menunjuk env —
// bukan diam-diam memakai rahasia publik dari biner.
func TestAntigravitySecretHanyaDariEnv(t *testing.T) {
	t.Setenv("ANTIGRAVITY_CLIENT_SECRET", "")
	if got := GetDefaultAntigravityClientSecret(); got != "" {
		t.Fatalf("tanpa env, GetDefaultAntigravityClientSecret() = %q, ingin kosong", got)
	}

	t.Setenv("ANTIGRAVITY_CLIENT_SECRET", "rahasia-dari-env-uji")
	if got := GetDefaultAntigravityClientSecret(); got != "rahasia-dari-env-uji" {
		t.Fatalf("dengan env, GetDefaultAntigravityClientSecret() = %q", got)
	}
}

func TestRefreshAntigravityTanpaEnvDitolakJelas(t *testing.T) {
	t.Setenv("ANTIGRAVITY_CLIENT_SECRET", "")
	client := NewGoogleOAuthClient(nil)
	_, err := client.RefreshAccessToken(context.Background(), "1//refresh", "", "")
	if err == nil || !strings.Contains(err.Error(), "ANTIGRAVITY_CLIENT_SECRET") {
		t.Fatalf("refresh tanpa env = %v, ingin error menyebut ANTIGRAVITY_CLIENT_SECRET", err)
	}
}

// TestRefreshTokenDitolakPermanenTerdeteksi membuktikan invalid_grant dari
// penyedia dibungkus sentinel ErrTokenDitolakPermanen (dipakai pemanggil
// untuk menonaktifkan sesi, audit3 V-EO-17), sedangkan kegagalan sementara
// TIDAK dianggap permanen.
func TestRefreshTokenDitolakPermanenTerdeteksi(t *testing.T) {
	t.Setenv("ANTIGRAVITY_CLIENT_SECRET", "rahasia-dari-env-uji")

	kasus := []struct {
		nama     string
		body     string
		permanen bool
	}{
		{"invalid_grant", `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`, true},
		{"server_error", `{"error":"server_error","error_description":"coba lagi"}`, false},
		{"temporarily_unavailable", `{"error":"temporarily_unavailable"}`, false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(k.body))
			}))
			defer srv.Close()

			client := NewGoogleOAuthClient(srv.Client()).WithTokenEndpoint(srv.URL)
			_, err := client.RefreshAccessToken(context.Background(), "1//refresh", "", "")
			if err == nil {
				t.Fatal("refresh ke endpoint gagal tidak menghasilkan error")
			}
			if got := errors.Is(err, ErrTokenDitolakPermanen); got != k.permanen {
				t.Errorf("errors.Is(ErrTokenDitolakPermanen) = %v, ingin %v (err: %v)", got, k.permanen, err)
			}
		})
	}
}
