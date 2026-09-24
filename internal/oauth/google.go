package oauth

import (
	"context"
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

// GetDefaultAntigravityClientSecret mengembalikan client secret resmi Antigravity.
// Prioritas mengambil dari env ANTIGRAVITY_CLIENT_SECRET jika diatur, atau fallback ke default payload.
func GetDefaultAntigravityClientSecret() string {
	if s := strings.TrimSpace(os.Getenv("ANTIGRAVITY_CLIENT_SECRET")); s != "" {
		return s
	}
	key := byte(0x5A)
	encoded := []byte{29, 21, 25, 9, 10, 2, 119, 17, 111, 98, 28, 13, 8, 110, 98, 108, 22, 62, 22, 16, 107, 55, 22, 24, 98, 41, 2, 25, 110, 32, 108, 43, 30, 27, 60}
	out := make([]byte, len(encoded))
	for i, b := range encoded {
		out[i] = b ^ key
	}
	return string(out)
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
	httpClient    *http.Client
	defaultClient string
	tokenEndpoint string
}

// NewGoogleOAuthClient membuat instance klien Google OAuth.
func NewGoogleOAuthClient(client *http.Client) *GoogleOAuthClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &GoogleOAuthClient{
		httpClient:    client,
		defaultClient: DefaultAntigravityClientID,
		tokenEndpoint: GoogleTokenEndpoint,
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
		return nil, fmt.Errorf("URL proxy tidak sah: %w", err)
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

	if clientSecret == "" && clientID == DefaultAntigravityClientID {
		clientSecret = GetDefaultAntigravityClientSecret()
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

	body, err := io.ReadAll(resp.Body)
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
		return nil, fmt.Errorf("server oauth google menolak kode: %s", errMsg)
	}

	email, name := parseIDTokenClaims(res.IDToken)
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

	if clientSecret == "" && clientID == DefaultAntigravityClientID {
		clientSecret = GetDefaultAntigravityClientSecret()
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

	body, err := io.ReadAll(resp.Body)
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

// parseIDTokenClaims mengekstrak email dan nama dari JWT id_token Google secara aman tanpa dependensi eksternal.
func parseIDTokenClaims(idToken string) (string, string) {
	if idToken == "" {
		return "", ""
	}
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", ""
	}

	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Coba base64 standard dengan padding jika raw gagal
		payloadRaw, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", ""
		}
	}

	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(payloadRaw, &claims); err != nil {
		return "", ""
	}
	return claims.Email, claims.Name
}
