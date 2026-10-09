package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Plain code",
			input:    "4/0AfgeXSn12345",
			expected: "4/0AfgeXSn12345",
		},
		{
			name:     "Full redirect URL",
			input:    "http://localhost:4567/?code=4/0AfgeXSn12345&scope=email%20profile",
			expected: "4/0AfgeXSn12345",
		},
		{
			name:     "URL with leading/trailing spaces",
			input:    "  http://localhost:4567/?code=4%2F0AfgeXSn999&state=xyz  ",
			expected: "4/0AfgeXSn999",
		},
		{
			name:     "Arbitrary URL parameter",
			input:    "https://example.com/callback?foo=bar&code=my-secret-auth-code",
			expected: "my-secret-auth-code",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractCode(tc.input)
			if got != tc.expected {
				t.Errorf("ExtractCode(%q) = %q, ingin %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestParseIDTokenClaims(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	claimsJSON, _ := json.Marshal(map[string]any{
		"email": "developer@antigravity.test",
		"name":  "Antigravity Dev",
	})
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	fakeJWT := fmt.Sprintf("%s.%s.fakeSignature", header, payload)

	email, name, err := parseIDTokenClaims(fakeJWT)
	if err != nil {
		t.Fatalf("parseIDTokenClaims gagal: %v", err)
	}
	if email != "developer@antigravity.test" {
		t.Errorf("email = %q, ingin %q", email, "developer@antigravity.test")
	}
	if name != "Antigravity Dev" {
		t.Errorf("name = %q, ingin %q", name, "Antigravity Dev")
	}

	// Sub-pengujian validasi klausa standar Google
	now := time.Now().Unix()

	t.Run("Valid with standard Google claims", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"iss":            "https://accounts.google.com",
			"aud":            "my-client-id.apps.googleusercontent.com",
			"exp":            now + 3600,
			"email":          "user@gmail.com",
			"email_verified": true,
			"name":           "Verified User",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		em, nm, err := parseIDTokenClaims(jwt, "my-client-id.apps.googleusercontent.com")
		if err != nil {
			t.Fatalf("seharusnya valid, tetapi error: %v", err)
		}
		if em != "user@gmail.com" || nm != "Verified User" {
			t.Errorf("email=%q, name=%q", em, nm)
		}
	})

	t.Run("Valid with alternative accounts.google.com issuer", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"iss":            "accounts.google.com",
			"aud":            "client-123",
			"exp":            now + 100,
			"email":          "alt@gmail.com",
			"email_verified": true,
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		em, _, err := parseIDTokenClaims(jwt, "client-123")
		if err != nil {
			t.Fatalf("seharusnya valid dengan issuer accounts.google.com, err: %v", err)
		}
		if em != "alt@gmail.com" {
			t.Errorf("email = %q, ingin alt@gmail.com", em)
		}
	})

	t.Run("Valid aud as array of clients", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"iss":   "https://accounts.google.com",
			"aud":   []string{"client-a", "client-b"},
			"email": "arr@gmail.com",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		_, _, err := parseIDTokenClaims(jwt, "client-b")
		if err != nil {
			t.Fatalf("seharusnya aud dalam array dikenali: %v", err)
		}
	})

	t.Run("Reject invalid issuer", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"iss":   "https://evil-issuer.com",
			"email": "hacker@evil.com",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		_, _, err := parseIDTokenClaims(jwt)
		if err == nil || !strings.Contains(err.Error(), "issuer") {
			t.Fatalf("seharusnya error issuer tidak valid, dapat: %v", err)
		}
	})

	t.Run("Reject mismatched audience", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"aud":   "other-client.apps.googleusercontent.com",
			"email": "user@gmail.com",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		_, _, err := parseIDTokenClaims(jwt, "expected-client.apps.googleusercontent.com")
		if err == nil || !strings.Contains(err.Error(), "aud") {
			t.Fatalf("seharusnya error aud mismatch, dapat: %v", err)
		}
	})

	t.Run("Reject missing audience when expectedClientID given", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"email": "user@gmail.com",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		_, _, err := parseIDTokenClaims(jwt, "expected-client")
		if err == nil || !strings.Contains(err.Error(), "aud") {
			t.Fatalf("seharusnya error aud kosong ketika expectedClientID diberikan, dapat: %v", err)
		}
	})

	t.Run("Reject expired token", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"exp":   now - 70, // Melebihi 60s skew allowance
			"email": "expired@gmail.com",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		_, _, err := parseIDTokenClaims(jwt)
		if err == nil || !strings.Contains(err.Error(), "kadaluarsa") {
			t.Fatalf("seharusnya error token kadaluarsa, dapat: %v", err)
		}
	})

	t.Run("Allow token within 60s clock skew window", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"exp":   now - 30, // Dalam 60s skew allowance (exp >= now - 60s)
			"email": "skew@gmail.com",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		em, _, err := parseIDTokenClaims(jwt)
		if err != nil {
			t.Fatalf("seharusnya diizinkan dalam toleransi skew 60s: %v", err)
		}
		if em != "skew@gmail.com" {
			t.Errorf("email = %q", em)
		}
	})

	t.Run("Reject unverified email bool", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"email":          "unverified@gmail.com",
			"email_verified": false,
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		_, _, err := parseIDTokenClaims(jwt)
		if err == nil || !strings.Contains(err.Error(), "belum diverifikasi") {
			t.Fatalf("seharusnya error email belum diverifikasi, dapat: %v", err)
		}
	})

	t.Run("Reject unverified email string", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"email":          "unverified@gmail.com",
			"email_verified": "false",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		_, _, err := parseIDTokenClaims(jwt)
		if err == nil || !strings.Contains(err.Error(), "belum diverifikasi") {
			t.Fatalf("seharusnya error email string belum diverifikasi, dapat: %v", err)
		}
	})

	t.Run("Accept verified email string", func(t *testing.T) {
		cj, _ := json.Marshal(map[string]any{
			"email":          "verified@gmail.com",
			"email_verified": "true",
		})
		jwt := fmt.Sprintf("%s.%s.sig", header, base64.RawURLEncoding.EncodeToString(cj))
		em, _, err := parseIDTokenClaims(jwt)
		if err != nil {
			t.Fatalf("seharusnya valid dengan email_verified 'true', err: %v", err)
		}
		if em != "verified@gmail.com" {
			t.Errorf("email = %q", em)
		}
	})

	t.Run("Reject malformed JWT format", func(t *testing.T) {
		_, _, err := parseIDTokenClaims("invalid-token-no-dots")
		if err == nil {
			t.Fatal("seharusnya error pada format token tanpa titik")
		}
	})

	t.Run("Empty token returns empty strings without error", func(t *testing.T) {
		em, nm, err := parseIDTokenClaims("")
		if err != nil {
			t.Fatalf("token kosong seharusnya tidak error: %v", err)
		}
		if em != "" || nm != "" {
			t.Errorf("token kosong menghasilkan non-empty email/nama: %q, %q", em, nm)
		}
	})
}

func TestExchangeAuthCodeMock(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	mockClaims, _ := json.Marshal(map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            DefaultAntigravityClientID,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"email":          "test@gmail.com",
		"email_verified": true,
		"name":           "Test User",
	})
	payload := base64.RawURLEncoding.EncodeToString(mockClaims)
	fakeJWT := header + "." + payload + ".sig"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if r.FormValue("grant_type") == "authorization_code" {
			if r.FormValue("code") != "4/test-code" {
				http.Error(w, `{"error":"invalid_grant"}`, 400)
				return
			}
			if r.FormValue("client_secret") != GetDefaultAntigravityClientSecret() {
				http.Error(w, `{"error":"invalid_client_secret"}`, 400)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "ya29.sample-token",
				"expires_in":    3600,
				"refresh_token": "1//sample-refresh",
				"token_type":    "Bearer",
				"id_token":      fakeJWT,
			})
			return
		}

		if r.FormValue("grant_type") == "refresh_token" {
			if r.FormValue("refresh_token") != "1//sample-refresh" {
				http.Error(w, `{"error":"invalid_grant"}`, 400)
				return
			}
			if r.FormValue("client_secret") != GetDefaultAntigravityClientSecret() {
				http.Error(w, `{"error":"invalid_client_secret"}`, 400)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "ya29.refreshed-token",
				"expires_in":   3600,
				"token_type":   "Bearer",
			})
			return
		}

		http.Error(w, `{"error":"unsupported_grant_type"}`, 400)
	}))
	defer mockServer.Close()

	client := NewGoogleOAuthClient(mockServer.Client()).WithTokenEndpoint(mockServer.URL)

	// Test 1: Exchange Auth Code
	res, err := client.ExchangeAuthCode(context.Background(), "http://localhost:4567/?code=4/test-code", "", "", "")
	if err != nil {
		t.Fatalf("ExchangeAuthCode error: %v", err)
	}
	if res.AccessToken.Reveal() != "ya29.sample-token" {
		t.Errorf("access_token = %q, mau ya29.sample-token", res.AccessToken.Reveal())
	}
	if res.RefreshToken.Reveal() != "1//sample-refresh" {
		t.Errorf("refresh_token = %q, mau 1//sample-refresh", res.RefreshToken.Reveal())
	}
	if res.AccountEmail != "test@gmail.com" {
		t.Errorf("email = %q, mau test@gmail.com", res.AccountEmail)
	}

	// Test 2: Refresh Access Token
	refRes, err := client.RefreshAccessToken(context.Background(), "1//sample-refresh", "", "")
	if err != nil {
		t.Fatalf("RefreshAccessToken error: %v", err)
	}
	if refRes.AccessToken.Reveal() != "ya29.refreshed-token" {
		t.Errorf("access_token = %q, mau ya29.refreshed-token", refRes.AccessToken.Reveal())
	}
}
