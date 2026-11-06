package oauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// googleJWKSURL adalah endpoint kunci publik penandatanganan Google
// (dokumen JWKS). id_token Google selalu RS256 dan kuncinya dirotasi
// berkala; cache di bawah menghormati Cache-Control dokumen ini.
const googleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"

// WithJWKSURL mengeset endpoint JWKS alternatif (berguna untuk pengujian
// unit dengan mock server).
func (c *GoogleOAuthClient) WithJWKSURL(u string) *GoogleOAuthClient {
	c.jwksURL = u
	return c
}

// verifikasiIDToken memverifikasi TANDA TANGAN id_token Google terhadap
// kunci publik Google (JWKS) lalu memvalidasi klaimnya (B-265).
//
// Sebelum perbaikan ini parseIDTokenClaims hanya memecah JWT dan membaca
// klaimnya — segmen tanda tangan tidak pernah diperiksa, sehingga JWT
// rakitan tangan ber-alg "none" diterima dan email penyerang menjadi
// identitas sesi. Di sini: header wajib alg RS256 dengan kid yang dikenal
// dari JWKS Google, tanda tangan diverifikasi RSASSA-PKCS1-v1_5/SHA-256,
// dan kegagalan mengambil kunci berarti token DITOLAK (gagal-tertutup:
// identitas yang tidak bisa diverifikasi tidak boleh menjadi sesi).
func (c *GoogleOAuthClient) verifikasiIDToken(ctx context.Context, client *http.Client, idToken, expectedClientID string) (string, string, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", "", errors.New("format id_token tidak valid: token bukan JWT bertanda tangan")
	}

	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("dekode header id_token gagal: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return "", "", fmt.Errorf("uraian header id_token gagal: %w", err)
	}
	if header.Alg != "RS256" {
		return "", "", fmt.Errorf("algoritma id_token %q tidak didukung: hanya RS256 Google yang diterima", header.Alg)
	}
	if header.Kid == "" {
		return "", "", errors.New("header id_token tidak memuat kid")
	}

	kunci, err := c.kunciPublikGoogle(ctx, client, header.Kid)
	if err != nil {
		return "", "", err
	}

	tandaTangan, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", fmt.Errorf("dekode tanda tangan id_token gagal: %w", err)
	}
	ringkasan := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(kunci, crypto.SHA256, ringkasan[:], tandaTangan); err != nil {
		return "", "", fmt.Errorf("tanda tangan id_token tidak sah: %w", err)
	}

	// Tanda tangan terbukti milik Google; klaim divalidasi oleh pemeriksa
	// klaim yang sama seperti sebelumnya (iss/aud/exp/email_verified).
	return parseIDTokenClaims(idToken, expectedClientID)
}

// kunciPublikGoogle mengembalikan kunci RSA Google untuk kid yang diminta,
// memakai cache JWKS selama masih berlaku dan menyegarkannya bila kid
// tidak dikenal (rotasi kunci Google) atau cache kedaluwarsa.
func (c *GoogleOAuthClient) kunciPublikGoogle(ctx context.Context, client *http.Client, kid string) (*rsa.PublicKey, error) {
	if k := c.kunciDariCache(kid); k != nil {
		return k, nil
	}
	if err := c.segarkanJWKS(ctx, client); err != nil {
		return nil, err
	}
	if k := c.kunciDariCache(kid); k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("kid %q tidak ditemukan pada JWKS Google", kid)
}

func (c *GoogleOAuthClient) kunciDariCache(kid string) *rsa.PublicKey {
	c.jwksMu.Lock()
	defer c.jwksMu.Unlock()
	if time.Now().After(c.jwksBerlakuSampai) {
		return nil
	}
	return c.jwksKunci[kid]
}

// segarkanJWKS mengambil dokumen JWKS Google dan mengganti isi cache.
// Masa berlaku cache mengikuti max-age Cache-Control dokumen (dijepit
// 1 menit–24 jam; bawaan 1 jam bila header tidak menyatakan).
func (c *GoogleOAuthClient) segarkanJWKS(ctx context.Context, client *http.Client) error {
	u := c.jwksURL
	if u == "" {
		u = googleJWKSURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("membuat request JWKS google: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("menghubungi JWKS google: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS google menjawab status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	if err != nil {
		return fmt.Errorf("membaca JWKS google: %w", err)
	}

	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("uraian JWKS google gagal: %w", err)
	}

	kunci := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kid == "" || k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		eBig := new(big.Int).SetBytes(eBytes)
		if !eBig.IsInt64() || eBig.Int64() < 2 {
			continue
		}
		kunci[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(eBig.Int64()),
		}
	}
	if len(kunci) == 0 {
		return errors.New("JWKS google tidak memuat kunci RSA yang dapat dipakai")
	}

	c.jwksMu.Lock()
	c.jwksKunci = kunci
	c.jwksBerlakuSampai = time.Now().Add(umurCacheJWKS(resp.Header.Get("Cache-Control")))
	c.jwksMu.Unlock()
	return nil
}

// umurCacheJWKS menafsirkan max-age dari Cache-Control; nilai di luar
// rentang waras dijepit supaya cache tidak abadi dan tidak pula kosong.
func umurCacheJWKS(cacheControl string) time.Duration {
	const (
		minUmur = time.Minute
		maxUmur = 24 * time.Hour
		bawaan  = time.Hour
	)
	for _, bagian := range strings.Split(cacheControl, ",") {
		bagian = strings.TrimSpace(bagian)
		if !strings.HasPrefix(bagian, "max-age=") {
			continue
		}
		detik, err := strconv.ParseInt(strings.TrimPrefix(bagian, "max-age="), 10, 64)
		if err != nil || detik <= 0 {
			return bawaan
		}
		umur := time.Duration(detik) * time.Second
		if umur < minUmur {
			return minUmur
		}
		if umur > maxUmur {
			return maxUmur
		}
		return umur
	}
	return bawaan
}
