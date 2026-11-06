package oauth

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

const (
	// DefaultAntigravityClientID adalah Client ID publik resmi Google Antigravity untuk aplikasi desktop/CLI.
	DefaultAntigravityClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"

	// DefaultRedirectURI adalah port redirect lokal standar untuk fallback.
	DefaultRedirectURI = "http://localhost:4567"

	// GoogleTokenEndpoint adalah URL resmi pertukaran token Google OAuth 2.0.
	GoogleTokenEndpoint = "https://oauth2.googleapis.com/token"
)

// cadanganClientSecretAntigravity adalah client secret bawaan aplikasi
// desktop Google Antigravity, tertanam sebagai CADANGAN OTOMATIS: nilai ini
// hanya dipakai bila env ANTIGRAVITY_CLIENT_SECRET kosong atau tidak
// disetel; env yang terisi non-kosong selalu menang (lihat
// GetDefaultAntigravityClientSecret).
//
// Provenans & sifat: nilai ini adalah kredensial client OAuth yang Google
// distribusikan di dalam aplikasi Antigravity itu sendiri. Menurut desain
// OAuth untuk aplikasi desktop (installed app), client secret kelas ini
// bersifat NON-RAHASIA: ia beredar di setiap salinan aplikasi yang
// terpasang, dan fungsinya identitas aplikasi — bukan pengaman akun.
// Keamanan akun pengguna dijaga oleh login Google dan oleh refresh token
// yang gateway simpan terenkripsi, bukan oleh nilai ini.
//
// Riwayat keputusan: fallback tertanam seperti ini pernah dicabut pada
// audit3 grup 8 (B-036) sehingga env menjadi satu-satunya sumber. Akibatnya
// instalasi yang tidak menyetel env gagal menghubungkan Antigravity
// sebelum sempat login Google (error eksplisit dari lengkapiClientSecret;
// terbukti live pada diagnosis 5 Nov 2026). Arpay membatalkan keputusan
// B-036 secara sadar (keputusan B-156, 5 Nov 2026): nilai ditanam kembali
// TANPA obfuskasi — tertulis apa adanya pada konstanta ini — agar alur
// OAuth bekerja tanpa langkah konfigurasi manual. Bila Google suatu saat
// merotasi client ini, perbarui konstanta ini atau timpa lewat env.
const cadanganClientSecretAntigravity = "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"

// ErrTokenDitolakPermanen menandai penolakan token yang TIDAK akan sembuh
// dengan mencoba lagi: invalid_grant berarti refresh token sudah dicabut,
// kedaluwarsa permanen, atau tidak pernah sah. Pemanggil (worker refresh,
// refresh manual) memakai penanda ini untuk menonaktifkan sesi dan
// menampilkannya ke operator, alih-alih mengulang kegagalan yang sama selamanya
// (audit3 V-EO-17).
var ErrTokenDitolakPermanen = errors.New("token ditolak permanen oleh penyedia OAuth")

// GetDefaultAntigravityClientSecret mengembalikan client secret Antigravity
// yang berlaku dengan precedence: env ANTIGRAVITY_CLIENT_SECRET bila terisi
// non-kosong, selain itu cadangan tertanam cadanganClientSecretAntigravity.
//
// Sejarah singkatnya: nilai ini semula tertanam ter-obfuskasi XOR, lalu
// fallback itu dicabut pada audit3 grup 8 (B-036, V-EO-24) sehingga env
// menjadi satu-satunya sumber dan ketiadaannya menggagalkan alur OAuth
// secara eksplisit. Keputusan B-156 (5 Nov 2026) memulihkan cadangan
// tertanam TANPA obfuskasi — lihat komentar pada konstanta cadangannya
// untuk provenans, sifat non-rahasianya, dan alasan pembatalan B-036.
func GetDefaultAntigravityClientSecret() string {
	if s := strings.TrimSpace(os.Getenv("ANTIGRAVITY_CLIENT_SECRET")); s != "" {
		return s
	}
	return cadanganClientSecretAntigravity
}

// lengkapiClientSecret menentukan client secret yang dipakai untuk client ID
// bawaan Antigravity bila pemanggil tidak menyediakannya.
//
// Untuk client ID lain, secret kosong dibiarkan apa adanya (client publik
// memang tidak punya secret). Untuk Antigravity, resolusinya diambil dari
// GetDefaultAntigravityClientSecret (env menimpa, cadangan tertanam
// mengisi — B-156), yang dipakai bersama oleh penukaran kode DAN refresh
// sesi sehingga kedua jalur tidak bisa berbeda secret. Cabang error di
// bawah kini tidak terjangkau selama konstanta cadangan terisi; ia
// dipertahankan sebagai penjaga agar bila cadangan itu suatu saat sengaja
// dikosongkan pada sebuah build, kegagalannya tetap eksplisit di sini —
// bukan penolakan buram dari endpoint token Google.
func lengkapiClientSecret(clientID, clientSecret string) (string, error) {
	if clientSecret != "" || clientID != DefaultAntigravityClientID {
		return clientSecret, nil
	}
	if s := GetDefaultAntigravityClientSecret(); s != "" {
		return s, nil
	}
	return "", errors.New("client secret Antigravity belum disetel: pasang env ANTIGRAVITY_CLIENT_SECRET pada proses gateway")
}

// maxTokenResponseBytes membatasi ukuran respons endpoint token OAuth (respons
// token Google berukuran ratusan byte; 1 MiB sudah sangat longgar).
const maxTokenResponseBytes = 1 << 20

// penandaSaring menggantikan kredensial yang tersaring dari pesan error
// upstream (penanda yang sama dengan redactedMark pada adapter providers).
const penandaSaring = "[REDACTED]"

// saringKredensial membuang nilai kredensial persis — dan bentuk
// "Bearer <nilai>"-nya — dari teks pesan asal upstream sebelum teks itu
// menyusun error yang akan dicatat pemanggil (B-268).
//
// Pola yang sama dengan safeMessage adapter providers: upstream/proksi
// dapat memantulkan kredensial yang dikirim di dalam pesan errornya, dan
// error itu berakhir di log gateway. Menyaring nilai persis yang sedang
// dipegang pemanggil menutup pantulan itu tanpa membuang sisa pesan yang
// berguna untuk diagnosis.
func saringKredensial(msg string, nilai ...string) string {
	for _, n := range nilai {
		if n == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, "Bearer "+n, "Bearer "+penandaSaring)
		msg = strings.ReplaceAll(msg, n, penandaSaring)
	}
	return msg
}

// TokenResult memuat hasil pertukaran atau pembaharuan token Google.
type TokenResult struct {
	AccessToken  security.Secret
	ExpiresIn    int
	RefreshToken security.Secret
	AccountEmail string
	AccountName  string
	TokenType    string
	Scopes       []string
}

// GoogleOAuthClient menangani pertukaran kode otorisasi dan pembaharuan token ke Google.
type GoogleOAuthClient struct {
	httpClient         *http.Client
	defaultClient      string
	tokenEndpoint      string
	codeAssistEndpoint string

	// Verifikasi tanda tangan id_token (B-265): endpoint JWKS Google dan
	// cache kunci publiknya. Cache hanya memuat kunci PUBLIK — tidak ada
	// rahasia pada struct ini.
	jwksURL           string
	jwksMu            sync.Mutex
	jwksKunci         map[string]*rsa.PublicKey
	jwksBerlakuSampai time.Time
}

// NewGoogleOAuthClient membuat instance klien Google OAuth.
func NewGoogleOAuthClient(client *http.Client) *GoogleOAuthClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &GoogleOAuthClient{
		httpClient:         client,
		defaultClient:      DefaultAntigravityClientID,
		tokenEndpoint:      GoogleTokenEndpoint,
		codeAssistEndpoint: DefaultCodeAssistEndpoint,
		jwksURL:            googleJWKSURL,
	}
}

// WithTokenEndpoint mengeset custom endpoint (berguna untuk pengujian unit dengan mock server).
func (c *GoogleOAuthClient) WithTokenEndpoint(endpoint string) *GoogleOAuthClient {
	c.tokenEndpoint = endpoint
	return c
}

// ExtractCode membersihkan masukan dari pengguna jika mereka menempelkan seluruh URL redirect.
func ExtractCode(input string) string {
	input = strings.TrimSpace(input)
	if strings.Contains(input, "code=") {
		u, err := url.Parse(input)
		if err == nil {
			if qCode := u.Query().Get("code"); qCode != "" {
				return qCode
			}
		}
		// Fallback manual substring jika parse URL gagal
		parts := strings.Split(input, "code=")
		if len(parts) > 1 {
			codePart := parts[1]
			if idx := strings.Index(codePart, "&"); idx != -1 {
				codePart = codePart[:idx]
			}
			if unescaped, err := url.QueryUnescape(codePart); err == nil {
				return unescaped
			}
			return codePart
		}
	}
	return input
}

type googleTokenJSON struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// httpClientForProxy membuat client HTTP khusus dengan proxy jika proxyURL disetel.
func (c *GoogleOAuthClient) httpClientForProxy(proxyURL security.Secret) (*http.Client, error) {
	if proxyURL.IsZero() {
		return c.httpClient, nil
	}
	u, err := url.Parse(proxyURL.Reveal())
	if err != nil {
		// Detail parse dibuang: *url.Error memuat URL proxy mentah beserta
		// kredensialnya, dan error ini bisa mengalir ke respons/log pemanggil
		// (audit3 V-EO-23).
		return nil, errors.New("URL proxy tidak sah")
	}
	tr := &http.Transport{
		Proxy:               http.ProxyURL(u),
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}, nil
}

// ExchangeAuthCode menukarkan kode otorisasi menjadi access token dan refresh token.
func (c *GoogleOAuthClient) ExchangeAuthCode(ctx context.Context, code, redirectURI, clientID, clientSecret string) (*TokenResult, error) {
	return c.ExchangeAuthCodeWithProxy(ctx, code, redirectURI, clientID, clientSecret, security.Secret(""))
}

// ExchangeAuthCodeWithProxy menukarkan kode otorisasi via proxy keluar jika disetel.
func (c *GoogleOAuthClient) ExchangeAuthCodeWithProxy(
	ctx context.Context,
	code, redirectURI, clientID, clientSecret string,
	proxyURL security.Secret,
) (*TokenResult, error) {
	code = ExtractCode(code)
	if code == "" {
		return nil, errors.New("kode otorisasi tidak boleh kosong")
	}

	if clientID == "" {
		clientID = c.defaultClient
	}
	if redirectURI == "" {
		redirectURI = DefaultRedirectURI
	}

	clientSecret, err := lengkapiClientSecret(clientID, clientSecret)
	if err != nil {
		return nil, err
	}

	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"client_id":    {clientID},
		"redirect_uri": {redirectURI},
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("membuat request token google: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client, err := c.httpClientForProxy(proxyURL)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("menghubungi server oauth google: %w", err)
	}
	defer resp.Body.Close()

	// AUDIT FIX: baca respons dengan batas ukuran agar endpoint/proxy yang bermasalah
	// tidak bisa menguras memori (menyamai pola io.LimitReader di internal/providers).
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("membaca respons token google: %w", err)
	}

	var res googleTokenJSON
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("uraian json respons token google (%d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode >= 400 || res.Error != "" {
		errMsg := res.Error
		if res.ErrorDesc != "" {
			errMsg += ": " + res.ErrorDesc
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("http status %d", resp.StatusCode)
		}
		// Pesan upstream disaring dari kredensial yang sedang dikirim
		// (kode otorisasi & client secret) sebelum menyusun error (B-268).
		errMsg = saringKredensial(errMsg, clientSecret, code)
		if res.Error == "invalid_grant" {
			return nil, fmt.Errorf("%w: server oauth google menolak kode: %s", ErrTokenDitolakPermanen, errMsg)
		}
		return nil, fmt.Errorf("server oauth google menolak kode: %s", errMsg)
	}

	// Identitas akun hanya boleh berasal dari id_token yang tanda
	// tangannya terbukti milik Google (B-265). id_token absen berarti
	// tidak ada identitas terverifikasi sama sekali: email dibiarkan
	// kosong dan pemanggil (handler exchange) menolak membuat sesi
	// tanpa email terverifikasi — bukan mengarang label cadangan.
	var email, name string
	if res.IDToken != "" {
		email, name, err = c.verifikasiIDToken(ctx, client, res.IDToken, clientID)
		if err != nil {
			return nil, fmt.Errorf("validasi id_token google: %w", err)
		}
	}
	var scopes []string
	if res.Scope != "" {
		scopes = strings.Fields(res.Scope)
	}

	return &TokenResult{
		AccessToken:  security.Secret(res.AccessToken),
		ExpiresIn:    res.ExpiresIn,
		RefreshToken: security.Secret(res.RefreshToken),
		AccountEmail: email,
		AccountName:  name,
		TokenType:    res.TokenType,
		Scopes:       scopes,
	}, nil
}

// RefreshAccessToken memperbarui access token Google menggunakan refresh token yang ada.
func (c *GoogleOAuthClient) RefreshAccessToken(ctx context.Context, refreshToken, clientID, clientSecret string) (*TokenResult, error) {
	return c.RefreshAccessTokenWithProxy(ctx, refreshToken, clientID, clientSecret, security.Secret(""))
}

// RefreshAccessTokenWithProxy memperbarui access token Google via proxy keluar jika disetel.
func (c *GoogleOAuthClient) RefreshAccessTokenWithProxy(
	ctx context.Context,
	refreshToken, clientID, clientSecret string,
	proxyURL security.Secret,
) (*TokenResult, error) {
	if refreshToken == "" {
		return nil, errors.New("refresh token tidak boleh kosong")
	}
	if clientID == "" {
		clientID = c.defaultClient
	}

	clientSecret, err := lengkapiClientSecret(clientID, clientSecret)
	if err != nil {
		return nil, err
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("membuat request refresh token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client, err := c.httpClientForProxy(proxyURL)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("menghubungi server oauth google: %w", err)
	}
	defer resp.Body.Close()

	// AUDIT FIX: batas ukuran respons, sama seperti pembacaan token di atas.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("membaca respons refresh token: %w", err)
	}

	var res googleTokenJSON
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("uraian json respons refresh token (%d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode >= 400 || res.Error != "" {
		errMsg := res.Error
		if res.ErrorDesc != "" {
			errMsg += ": " + res.ErrorDesc
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("http status %d", resp.StatusCode)
		}
		// Pesan upstream disaring dari kredensial yang sedang dikirim
		// (refresh token & client secret) sebelum menyusun error (B-268).
		errMsg = saringKredensial(errMsg, clientSecret, refreshToken)
		if res.Error == "invalid_grant" {
			// Refresh token dicabut/kedaluwarsa permanen: mencoba lagi pada
			// putaran berikutnya tidak akan mengubah hasil (audit3 V-EO-17).
			return nil, fmt.Errorf("%w: gagal me-refresh token google: %s", ErrTokenDitolakPermanen, errMsg)
		}
		return nil, fmt.Errorf("gagal me-refresh token google: %s", errMsg)
	}

	var scopes []string
	if res.Scope != "" {
		scopes = strings.Fields(res.Scope)
	}

	return &TokenResult{
		AccessToken:  security.Secret(res.AccessToken),
		ExpiresIn:    res.ExpiresIn,
		RefreshToken: security.Secret(res.RefreshToken), // Biasanya kosong pada refresh biasa
		TokenType:    res.TokenType,
		Scopes:       scopes,
	}, nil
}

// parseIDTokenClaims mengekstrak dan memvalidasi klausa JWT id_token Google secara aman tanpa dependensi eksternal.
//
// Fungsi ini HANYA memvalidasi klaim — ia tidak memeriksa tanda tangan.
// Pemanggil produksi wajib melewati verifikasiIDToken (B-265), yang
// memverifikasi tanda tangan RS256 terhadap JWKS Google terlebih dahulu
// lalu memanggil fungsi ini; memanggil fungsi ini langsung atas token
// dari jaringan berarti mempercayai klaim yang belum terbukti.
// Memvalidasi:
// - Issuer (iss) wajib https://accounts.google.com atau accounts.google.com (jika ada).
// - Audience (aud) wajib cocok dengan expectedClientID (jika expectedClientID diberikan).
// - Expiry (exp) wajib belum kadaluarsa dengan toleransi clock skew 60 detik (jika ada).
// - Email verified (email_verified) wajib bernilai true bila klausa tersebut ada.
func parseIDTokenClaims(idToken string, expectedClientID ...string) (string, string, error) {
	if idToken == "" {
		return "", "", nil
	}
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", "", errors.New("format id_token tidak valid: token bukan JWT")
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Coba base64 standard dengan padding jika raw gagal
		payloadRaw, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", "", fmt.Errorf("dekode payload id_token gagal: %w", err)
		}
	}

	var claims struct {
		Iss           string          `json:"iss"`
		Aud           json.RawMessage `json:"aud"`
		Exp           int64           `json:"exp"`
		Email         string          `json:"email"`
		EmailVerified any             `json:"email_verified"`
		Name          string          `json:"name"`
	}
	if err := json.Unmarshal(payloadRaw, &claims); err != nil {
		return "", "", fmt.Errorf("uraian json klausa id_token gagal: %w", err)
	}

	// 1. Validasi Issuer (iss) jika ada
	if claims.Iss != "" && claims.Iss != "https://accounts.google.com" && claims.Iss != "accounts.google.com" {
		return "", "", fmt.Errorf("issuer id_token tidak valid: %s", claims.Iss)
	}

	// 2. Validasi Audience (aud) jika expectedClientID diberikan
	var targetClient string
	if len(expectedClientID) > 0 {
		targetClient = expectedClientID[0]
	}
	if targetClient != "" {
		if len(claims.Aud) == 0 {
			return "", "", errors.New("klausa aud id_token kosong atau tidak ada")
		}
		var audStr string
		var audArr []string
		matched := false
		if err := json.Unmarshal(claims.Aud, &audStr); err == nil {
			matched = (audStr == targetClient)
		} else if err := json.Unmarshal(claims.Aud, &audArr); err == nil {
			for _, a := range audArr {
				if a == targetClient {
					matched = true
					break
				}
			}
		} else {
			return "", "", errors.New("format klausa aud id_token tidak valid")
		}
		if !matched {
			return "", "", fmt.Errorf("aud id_token tidak cocok dengan expected client ID %q", targetClient)
		}
	}

	// 3. Validasi Expiry (exp) jika ada (toleransi 60s clock skew: exp >= now - 60s)
	if claims.Exp > 0 {
		now := time.Now().Unix()
		if claims.Exp < (now - 60) {
			return "", "", fmt.Errorf("id_token telah kadaluarsa (exp: %d, now: %d)", claims.Exp, now)
		}
	}

	// 4. Validasi Email Verified jika ada klausa tersebut
	if claims.EmailVerified != nil {
		switch v := claims.EmailVerified.(type) {
		case bool:
			if !v {
				return "", "", errors.New("email id_token belum diverifikasi oleh Google")
			}
		case string:
			if strings.ToLower(v) != "true" {
				return "", "", errors.New("email id_token belum diverifikasi oleh Google")
			}
		default:
			return "", "", errors.New("format klausa email_verified tidak valid")
		}
	}

	return claims.Email, claims.Name, nil
}
