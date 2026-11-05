package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAntigravitySecretPrecedenceEnvVsCadangan membuktikan precedence
// resolusi secret Antigravity sesudah keputusan B-156 (5 Nov 2026) yang
// membatalkan B-036: env non-kosong selalu menang; env kosong/tidak disetel
// memakai cadangan tertanam (cadanganClientSecretAntigravity) — bukan lagi
// string kosong seperti pada masa B-036, ketika test pendahulu test ini
// (TestAntigravitySecretHanyaDariEnv) mengunci perilaku "tanpa env =
// kosong" dan refresh ditolak dengan error "belum disetel". Pesan kegagalan
// di sini sengaja TIDAK mencetak nilai cadangan (nilai itu tidak boleh
// mengalir ke log test); perbandingan dilakukan terhadap konstantanya.
func TestAntigravitySecretPrecedenceEnvVsCadangan(t *testing.T) {
	t.Setenv("ANTIGRAVITY_CLIENT_SECRET", "")
	got := GetDefaultAntigravityClientSecret()
	if got == "" {
		t.Fatal("tanpa env, GetDefaultAntigravityClientSecret() kosong; ingin cadangan tertanam (B-156)")
	}
	if got != cadanganClientSecretAntigravity {
		t.Fatal("tanpa env, GetDefaultAntigravityClientSecret() tidak sama dengan cadangan tertanam")
	}

	t.Setenv("ANTIGRAVITY_CLIENT_SECRET", "rahasia-dari-env-uji")
	if got := GetDefaultAntigravityClientSecret(); got != "rahasia-dari-env-uji" {
		t.Fatalf("dengan env, GetDefaultAntigravityClientSecret() = %q, ingin nilai env", got)
	}

	// Env berisi spasi saja diperlakukan kosong -> cadangan yang berlaku.
	t.Setenv("ANTIGRAVITY_CLIENT_SECRET", "   ")
	if got := GetDefaultAntigravityClientSecret(); got != cadanganClientSecretAntigravity {
		t.Fatal("env hanya spasi harus dianggap kosong dan memakai cadangan tertanam")
	}
}

// TestExchangeRefreshPrecedenceSecretDiKawat membuktikan precedence yang
// sama pada level protokol untuk KEDUA konsumen resolusi bersama
// (lengkapiClientSecret): penukaran kode otorisasi dan refresh token
// mengirim client_secret dari env bila env terisi, dan client_secret
// cadangan tertanam bila env kosong. Server tiruan menolak secret yang
// tidak cocok dengan harapan kasusnya, jadi keberhasilan panggilan adalah
// buktinya; nilai yang terkirim tidak pernah dicetak.
func TestExchangeRefreshPrecedenceSecretDiKawat(t *testing.T) {
	kasus := []struct {
		nama    string
		env     string
		harapan func() string
	}{
		{"env_kosong_memakai_cadangan", "", func() string { return cadanganClientSecretAntigravity }},
		{"env_terisi_memakai_env", "rahasia-dari-env-uji", func() string { return "rahasia-dari-env-uji" }},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			t.Setenv("ANTIGRAVITY_CLIENT_SECRET", k.env)
			terlihat := map[string]bool{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, `{"error":"bad_form"}`, http.StatusBadRequest)
					return
				}
				grant := r.FormValue("grant_type")
				terlihat[grant] = true
				if r.FormValue("client_secret") != k.harapan() {
					http.Error(w, `{"error":"invalid_client_secret"}`, http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"akses-uji","expires_in":3600,"refresh_token":"1//refresh-uji","token_type":"Bearer"}`))
			}))
			defer srv.Close()

			client := NewGoogleOAuthClient(srv.Client()).WithTokenEndpoint(srv.URL)
			if _, err := client.ExchangeAuthCode(context.Background(), "4/kode-uji", "", "", ""); err != nil {
				t.Fatalf("exchange grant_type=authorization_code gagal: %v", err)
			}
			if _, err := client.RefreshAccessToken(context.Background(), "1//refresh-uji", "", ""); err != nil {
				t.Fatalf("refresh grant_type=refresh_token gagal: %v", err)
			}
			if !terlihat["authorization_code"] || !terlihat["refresh_token"] {
				t.Fatalf("server tiruan tidak menerima kedua grant: %v", terlihat)
			}
		})
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
