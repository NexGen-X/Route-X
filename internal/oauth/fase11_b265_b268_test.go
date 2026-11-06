package oauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Pembantu uji B-265 -------------------------------------------------

const kidUjiB265 = "kid-uji-b265"
const clientIDUjiB265 = "client-uji.apps.googleusercontent.com"

func klaimSahB265() map[string]any {
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            clientIDUjiB265,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"email":          "pemilik@contoh.test",
		"email_verified": true,
		"name":           "Pemilik Uji",
	}
}

func dokumenJWKSUji(pub *rsa.PublicKey, kid string) string {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	return fmt.Sprintf(`{"keys":[{"kid":"%s","kty":"RSA","alg":"RS256","use":"sig","n":"%s","e":"%s"}]}`, kid, n, e)
}

func tandaTanganiJWTUji(t *testing.T, priv *rsa.PrivateKey, kid string, klaim map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"alg":"RS256","kid":"%s","typ":"JWT"}`, kid)))
	payloadJSON, err := json.Marshal(klaim)
	if err != nil {
		t.Fatalf("marshal klaim: %v", err)
	}
	data := header + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	ringkasan := sha256.Sum256([]byte(data))
	tanda, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, ringkasan[:])
	if err != nil {
		t.Fatalf("menandatangani JWT uji: %v", err)
	}
	return data + "." + base64.RawURLEncoding.EncodeToString(tanda)
}

// serverTokenDenganIDToken menyajikan endpoint token tiruan yang
// mengembalikan idToken yang diberikan, dan endpoint JWKS tiruan.
func serverTokenDenganIDToken(t *testing.T, idToken string, jwksBody string) (tokenURL, jwksURL string) {
	t.Helper()
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(jwksBody))
	}))
	t.Cleanup(jwksSrv.Close)

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"access_token":  "akses-uji",
			"expires_in":    3600,
			"refresh_token": "1//refresh-uji",
			"token_type":    "Bearer",
		}
		if idToken != "" {
			resp["id_token"] = idToken
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(tokenSrv.Close)
	return tokenSrv.URL, jwksSrv.URL
}

func kunciRSAUji(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("membuat kunci RSA uji: %v", err)
	}
	return priv
}

// TestExchangeIDTokenTerverifikasiB265 mengunci B-265: identitas sesi
// hanya boleh lahir dari id_token yang tanda tangannya terbukti milik
// Google. Token sah bertanda tangan benar DITERIMA; JWT alg:none rakitan
// tangan (serangan yang dibuktikan Putaran Bersih), token bertanda tangan
// kunci lain, kid tak dikenal, dan token kedaluwarsa DITOLAK.
func TestExchangeIDTokenTerverifikasiB265(t *testing.T) {
	privGoogle := kunciRSAUji(t)
	privAsing := kunciRSAUji(t)
	jwksBody := dokumenJWKSUji(&privGoogle.PublicKey, kidUjiB265)

	kasus := []struct {
		nama    string
		idToken string
		jwksURL string // kosong = pakai server JWKS bersama kasus
		sukses  bool
	}{
		{
			nama:    "token sah bertanda tangan kunci JWKS diterima",
			idToken: tandaTanganiJWTUji(t, privGoogle, kidUjiB265, klaimSahB265()),
			sukses:  true,
		},
		{
			nama:    "alg none rakitan tangan ditolak",
			idToken: jwtAlgNoneUji(klaimSahB265()),
			sukses:  false,
		},
		{
			nama:    "tanda tangan kunci lain ditolak",
			idToken: tandaTanganiJWTUji(t, privAsing, kidUjiB265, klaimSahB265()),
			sukses:  false,
		},
		{
			nama:    "kid tidak dikenal ditolak",
			idToken: tandaTanganiJWTUji(t, privGoogle, "kid-tidak-ada", klaimSahB265()),
			sukses:  false,
		},
		{
			nama: "token kedaluwarsa walau tanda tangan sah ditolak",
			idToken: tandaTanganiJWTUji(t, privGoogle, kidUjiB265, func() map[string]any {
				k := klaimSahB265()
				k["exp"] = time.Now().Add(-time.Hour).Unix()
				return k
			}()),
			sukses: false,
		},
		{
			nama: "aud salah walau tanda tangan sah ditolak",
			idToken: tandaTanganiJWTUji(t, privGoogle, kidUjiB265, func() map[string]any {
				k := klaimSahB265()
				k["aud"] = "client-lain.apps.googleusercontent.com"
				return k
			}()),
			sukses: false,
		},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			tokenURL, jwksURL := serverTokenDenganIDToken(t, k.idToken, jwksBody)
			client := NewGoogleOAuthClient(http.DefaultClient).
				WithTokenEndpoint(tokenURL).
				WithJWKSURL(jwksURL)
			res, err := client.ExchangeAuthCode(context.Background(),
				"4/kode-uji", "", clientIDUjiB265, "secret-uji")
			if k.sukses {
				if err != nil {
					t.Fatalf("exchange token sah gagal: %v", err)
				}
				if res.AccountEmail != "pemilik@contoh.test" {
					t.Errorf("AccountEmail = %q, mau pemilik@contoh.test", res.AccountEmail)
				}
			} else if err == nil {
				t.Fatalf("exchange token tidak sah DITERIMA (email=%q) — verifikasi tanda tangan tidak bekerja", res.AccountEmail)
			}
		})
	}
}

// TestExchangeIDTokenJWKSTakTerjangkauDitolakB265: bila JWKS tidak bisa
// diambil, verifikasi gagal-tertutup — token ditolak, bukan diterima
// tanpa pemeriksaan.
func TestExchangeIDTokenJWKSTakTerjangkauDitolakB265(t *testing.T) {
	privGoogle := kunciRSAUji(t)
	idToken := tandaTanganiJWTUji(t, privGoogle, kidUjiB265, klaimSahB265())
	tokenURL, _ := serverTokenDenganIDToken(t, idToken, dokumenJWKSUji(&privGoogle.PublicKey, kidUjiB265))

	// Server JWKS yang langsung ditutup: URL-nya tidak lagi bisa dihubungi.
	jwksMati := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	jwksMatiURL := jwksMati.URL
	jwksMati.Close()

	client := NewGoogleOAuthClient(http.DefaultClient).
		WithTokenEndpoint(tokenURL).
		WithJWKSURL(jwksMatiURL)
	if _, err := client.ExchangeAuthCode(context.Background(),
		"4/kode-uji", "", clientIDUjiB265, "secret-uji"); err == nil {
		t.Fatal("exchange diterima padahal JWKS tidak terjangkau — verifikasi harus gagal-tertutup")
	}
}

func jwtAlgNoneUji(klaim map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadJSON, _ := json.Marshal(klaim)
	return header + "." + base64.RawURLEncoding.EncodeToString(payloadJSON) + "."
}

// --- Uji B-268 ----------------------------------------------------------

// TestLoadCodeAssistErrorDisaringDariTokenB268 mengunci B-268: pesan
// error upstream yang memantulkan access token (termasuk bentuk
// Bearer-nya) tidak boleh membawa token utuh ke error yang dicatat
// pemanggil.
func TestLoadCodeAssistErrorDisaringDariTokenB268(t *testing.T) {
	const tokenUji = "ya29.token-uji-b268-rahasia"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"token Bearer ` + tokenUji + ` ditolak; token=` + tokenUji + `"}}`))
	}))
	defer srv.Close()

	client := NewGoogleOAuthClient(srv.Client()).WithCodeAssistEndpoint(srv.URL)
	_, err := client.LoadCodeAssistProject(context.Background(), tokenUji, "")
	if err == nil {
		t.Fatal("loadCodeAssist seharusnya gagal pada jawaban 403")
	}
	if strings.Contains(err.Error(), tokenUji) {
		t.Fatalf("error memuat access token utuh: %q", err.Error())
	}
	if !strings.Contains(err.Error(), penandaSaring) {
		t.Errorf("error tidak memuat penanda saringan: %q", err.Error())
	}
}

// TestErrorEndpointTokenDisaringDariKredensialB268: jalur error exchange
// dan refresh menyaring kredensial yang sedang dikirim (kode otorisasi,
// refresh token, client secret) dari pesan upstream sebelum menyusun
// error.
func TestErrorEndpointTokenDisaringDariKredensialB268(t *testing.T) {
	const kodeUji = "4/kode-uji-b268"
	const refreshUji = "1//refresh-uji-b268"
	const secretUji = "GOCSPX-secret-uji-b268"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		switch r.FormValue("grant_type") {
		case "authorization_code":
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"kode ` + kodeUji + ` dengan secret ` + secretUji + ` ditolak"}`))
		default:
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh ` + refreshUji + ` dengan secret ` + secretUji + ` ditolak"}`))
		}
	}))
	defer srv.Close()

	client := NewGoogleOAuthClient(srv.Client()).WithTokenEndpoint(srv.URL)

	_, err := client.ExchangeAuthCode(context.Background(), kodeUji, "", clientIDUjiB265, secretUji)
	if err == nil {
		t.Fatal("exchange seharusnya gagal")
	}
	if strings.Contains(err.Error(), kodeUji) || strings.Contains(err.Error(), secretUji) {
		t.Fatalf("error exchange memuat kredensial utuh: %q", err.Error())
	}

	_, err = client.RefreshAccessToken(context.Background(), refreshUji, clientIDUjiB265, secretUji)
	if err == nil {
		t.Fatal("refresh seharusnya gagal")
	}
	if strings.Contains(err.Error(), refreshUji) || strings.Contains(err.Error(), secretUji) {
		t.Fatalf("error refresh memuat kredensial utuh: %q", err.Error())
	}
}
