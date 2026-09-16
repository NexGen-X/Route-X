package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultCloudflareBaseURL = "https://api.cloudflare.com/client/v4"

// CloudflareClient menangani komunikasi ke REST API resmi Cloudflare untuk AI Gateway.
type CloudflareClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewCloudflareClient menginisialisasi klien Cloudflare baru.
func NewCloudflareClient(baseURL string, timeout time.Duration) *CloudflareClient {
	if baseURL == "" {
		baseURL = defaultCloudflareBaseURL
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &CloudflareClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// CloudflareProvisionParams parameter input untuk mendaftarkan AI Gateway di Cloudflare.
type CloudflareProvisionParams struct {
	AccountID   string `json:"account_id"`
	APIToken    string `json:"api_token"`
	GatewayID   string `json:"gateway_id"`
	CollectLogs bool   `json:"collect_logs"`
	EnableCache bool   `json:"enable_cache"`
}

// CloudflareProvisionResult hasil dari pendaftaran AI Gateway di Cloudflare.
type CloudflareProvisionResult struct {
	GatewayID    string `json:"gateway_id"`
	AccountID    string `json:"account_id"`
	GatewayURL   string `json:"gateway_url"`
	AlreadyExist bool   `json:"already_exist"`
}

type cfGatewayReq struct {
	ID                    string `json:"id"`
	CollectLogs           bool   `json:"collect_logs"`
	RateLimitingTechnique string `json:"rate_limiting_technique,omitempty"`
}

type cfGatewayResp struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		ID          string `json:"id"`
		CollectLogs bool   `json:"collect_logs"`
	} `json:"result"`
}

// ProvisionAIGateway memanggil API Cloudflare untuk memastikan AI Gateway telah dibuat.
func (c *CloudflareClient) ProvisionAIGateway(ctx context.Context, p CloudflareProvisionParams) (*CloudflareProvisionResult, error) {
	if strings.TrimSpace(p.AccountID) == "" {
		return nil, errors.New("cloudflare account_id tidak boleh kosong")
	}
	if strings.TrimSpace(p.APIToken) == "" {
		return nil, errors.New("cloudflare api_token tidak boleh kosong")
	}
	gatewayID := strings.TrimSpace(p.GatewayID)
	if gatewayID == "" {
		gatewayID = "routex-ai-gateway"
	}

	apiURL := fmt.Sprintf("%s/accounts/%s/ai-gateway/gateways", c.baseURL, p.AccountID)

	reqBody := cfGatewayReq{
		ID:                    gatewayID,
		CollectLogs:           p.CollectLogs,
		RateLimitingTechnique: "fixed",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gagal menyusun request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.APIToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Route-X-Provisioner/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi API Cloudflare: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca respons API Cloudflare: %w", err)
	}

	var cfResp cfGatewayResp
	if err := json.Unmarshal(respBytes, &cfResp); err != nil {
		return nil, fmt.Errorf("respons Cloudflare tidak valid (%d): %s", resp.StatusCode, string(respBytes))
	}

	alreadyExist := false
	if !cfResp.Success {
		// Cek jika error menyatakan gateway sudah ada
		for _, e := range cfResp.Errors {
			if strings.Contains(strings.ToLower(e.Message), "already exists") || e.Code == 1000 {
				alreadyExist = true
				break
			}
		}
		if !alreadyExist {
			msg := "error tidak diketahui dari Cloudflare"
			if len(cfResp.Errors) > 0 {
				msg = cfResp.Errors[0].Message
			}
			return nil, fmt.Errorf("cloudflare API error: %s", msg)
		}
	}

	gatewayURL := fmt.Sprintf("https://gateway.ai.cloudflare.com/v1/%s/%s", p.AccountID, gatewayID)

	return &CloudflareProvisionResult{
		GatewayID:    gatewayID,
		AccountID:    p.AccountID,
		GatewayURL:   gatewayURL,
		AlreadyExist: alreadyExist,
	}, nil
}
