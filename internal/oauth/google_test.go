package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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

	email, name := parseIDTokenClaims(fakeJWT)
	if email != "developer@antigravity.test" {
		t.Errorf("email = %q, ingin %q", email, "developer@antigravity.test")
	}
	if name != "Antigravity Dev" {
		t.Errorf("name = %q, ingin %q", name, "Antigravity Dev")
	}
}

func TestExchangeAuthCodeMock(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"test@gmail.com","name":"Test User"}`))
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
