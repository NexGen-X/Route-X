package provision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCloudflareProvisionAIGateway_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("auth header = %s, want Bearer test-token", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/accounts/test-acc/ai-gateway/gateways" {
			t.Errorf("path = %s, want /accounts/test-acc/ai-gateway/gateways", r.URL.Path)
		}

		var req cfGatewayReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if req.ID != "my-gateway" {
			t.Errorf("req.ID = %s, want my-gateway", req.ID)
		}
		if !req.CollectLogs {
			t.Errorf("req.CollectLogs = false, want true")
		}
		if req.RateLimitingTechnique != "fixed" {
			t.Errorf("req.RateLimitingTechnique = %s, want fixed", req.RateLimitingTechnique)
		}
		if req.RateLimitingInterval != 0 || req.RateLimitingLimit != 0 {
			t.Errorf("rate limit interval/limit = %d/%d, want 0/0", req.RateLimitingInterval, req.RateLimitingLimit)
		}
		if req.CacheTTL != 300 {
			t.Errorf("req.CacheTTL = %d, want 300", req.CacheTTL)
		}
		if req.CacheInvalidateOnUpdate {
			t.Errorf("req.CacheInvalidateOnUpdate = true, want false")
		}

		resp := cfGatewayResp{
			Success: true,
			Result: struct {
				ID          string `json:"id"`
				CollectLogs bool   `json:"collect_logs"`
			}{
				ID:          "my-gateway",
				CollectLogs: true,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewCloudflareClient(server.URL, 5*time.Second)
	res, err := client.ProvisionAIGateway(context.Background(), CloudflareProvisionParams{
		AccountID:   "test-acc",
		APIToken:    "test-token",
		GatewayID:   "my-gateway",
		CollectLogs: true,
		EnableCache: true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.GatewayID != "my-gateway" {
		t.Errorf("gatewayID = %s, want my-gateway", res.GatewayID)
	}
	expectedURL := "https://gateway.ai.cloudflare.com/v1/test-acc/my-gateway"
	if res.GatewayURL != expectedURL {
		t.Errorf("gatewayURL = %s, want %s", res.GatewayURL, expectedURL)
	}
	if res.AlreadyExist {
		t.Errorf("alreadyExist = true, want false")
	}
}

func TestCloudflareProvisionAIGateway_AlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cfGatewayResp{
			Success: false,
			Errors: []struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{
				{Code: 1000, Message: "Gateway with id already exists"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewCloudflareClient(server.URL, 5*time.Second)
	res, err := client.ProvisionAIGateway(context.Background(), CloudflareProvisionParams{
		AccountID: "test-acc",
		APIToken:  "test-token",
		GatewayID: "my-existing-gateway",
	})

	if err != nil {
		t.Fatalf("expected graceful handle of existing gateway, got error: %v", err)
	}
	if !res.AlreadyExist {
		t.Errorf("alreadyExist = false, want true")
	}
}

func TestCloudflareProvisionAIGateway_InvalidParams(t *testing.T) {
	client := NewCloudflareClient("", 5*time.Second)
	_, err := client.ProvisionAIGateway(context.Background(), CloudflareProvisionParams{
		AccountID: "",
		APIToken:  "test",
	})
	if err == nil {
		t.Error("expected error for empty account_id, got nil")
	}

	_, err = client.ProvisionAIGateway(context.Background(), CloudflareProvisionParams{
		AccountID: "test",
		APIToken:  "",
	})
	if err == nil {
		t.Error("expected error for empty api_token, got nil")
	}
}

func TestDenoProvisionRelay_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer deno-token" {
			t.Errorf("auth header = %s, want Bearer deno-token", r.Header.Get("Authorization"))
		}

		if r.URL.Path == "/projects" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(denoProjectResp{
				ID:   "proj-123",
				Name: "test-proj",
			})
			return
		}

		if r.URL.Path == "/projects/proj-123/deployments" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(denoDeploymentResp{
				ID:      "dep-456",
				Domains: []string{"test-proj.deno.dev"},
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewDenoClient(server.URL, 5*time.Second)
	res, err := client.ProvisionRelay(context.Background(), DenoProvisionParams{
		AccessToken: "deno-token",
		ProjectName: "test-proj",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProjectName != "test-proj" {
		t.Errorf("projectName = %s, want test-proj", res.ProjectName)
	}
	if res.RelayURL != "https://test-proj.deno.dev" {
		t.Errorf("relayURL = %s, want https://test-proj.deno.dev", res.RelayURL)
	}
}

func TestDenoProvisionRelay_InvalidToken(t *testing.T) {
	client := NewDenoClient("", 5*time.Second)
	_, err := client.ProvisionRelay(context.Background(), DenoProvisionParams{
		AccessToken: "",
	})
	if err == nil {
		t.Error("expected error for empty access_token, got nil")
	}
}
