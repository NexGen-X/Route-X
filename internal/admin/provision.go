package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/provision"
	"github.com/NexGen-X/Route-X/internal/security"
)

type provisionCloudflareReq struct {
	AccountID   string  `json:"account_id"`
	APIToken    string  `json:"api_token"`
	GatewayID   string  `json:"gateway_id"`
	Name        string  `json:"name"`
	CollectLogs bool    `json:"collect_logs"`
	EnableCache bool    `json:"enable_cache"`
	Region      *string `json:"region"`
}

type provisionDenoReq struct {
	AccessToken string  `json:"access_token"`
	ProjectName string  `json:"project_name"`
	Name        string  `json:"name"`
	Region      *string `json:"region"`
}

type provisionResultDTO struct {
	Pool         EgressPoolDTO `json:"pool"`
	ProviderType string        `json:"provider_type"`
	TargetURL    string        `json:"target_url"`
	AlreadyExist bool          `json:"already_exist,omitempty"`
}

func (provisionResultDTO) adalahDTO() {}

func (h *Handlers) provisionCloudflareAIGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req provisionCloudflareReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	accountID := strings.TrimSpace(req.AccountID)
	if accountID == "" {
		httpx.BadRequest(w, r, "missing_account_id", "cloudflare account_id wajib diisi")
		return
	}
	apiToken := strings.TrimSpace(req.APIToken)
	if apiToken == "" {
		httpx.BadRequest(w, r, "missing_api_token", "cloudflare api_token wajib diisi")
		return
	}

	gatewayID := strings.TrimSpace(req.GatewayID)
	if gatewayID == "" {
		gatewayID = "routex-ai-gateway"
	}

	client := provision.NewCloudflareClient("", 20*time.Second)
	res, err := client.ProvisionAIGateway(ctx, provision.CloudflareProvisionParams{
		AccountID:   accountID,
		APIToken:    apiToken,
		GatewayID:   gatewayID,
		CollectLogs: req.CollectLogs,
		EnableCache: req.EnableCache,
	})
	if err != nil {
		httpx.BadRequest(w, r, "cloudflare_provision_failed", fmt.Sprintf("Gagal mendaftarkan AI Gateway di Cloudflare: %v", err))
		return
	}

	poolName := strings.TrimSpace(req.Name)
	if poolName == "" {
		poolName = fmt.Sprintf("☁️ Cloudflare AI Gateway (%s)", res.GatewayID)
	}

	region := "cloudflare-edge"
	if req.Region != nil && strings.TrimSpace(*req.Region) != "" {
		region = strings.TrimSpace(*req.Region)
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	enabled := true
	weight := 1
	ep, err := h.egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name:      poolName,
		Kind:      "https",
		ProxyURL:  security.Secret(res.GatewayURL),
		Enabled:   &enabled,
		Weight:    &weight,
		Region:    &region,
		CreatedBy: actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	h.writeAudit(ctx, r, "provision", "egress_pool_cloudflare", ep.ID, map[string]any{
		"name":        ep.Name,
		"account_id":  res.AccountID,
		"gateway_id":  res.GatewayID,
		"gateway_url": res.GatewayURL,
	})

	_ = h.respond(w, r, http.StatusCreated, provisionResultDTO{
		Pool:         toEgressPoolDTO(ep),
		ProviderType: "cloudflare_ai_gateway",
		TargetURL:    res.GatewayURL,
		AlreadyExist: res.AlreadyExist,
	})
}

func (h *Handlers) provisionDenoRelay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req provisionDenoReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	accessToken := strings.TrimSpace(req.AccessToken)
	if accessToken == "" {
		httpx.BadRequest(w, r, "missing_access_token", "deno access_token wajib diisi")
		return
	}

	projectName := strings.TrimSpace(req.ProjectName)
	if projectName == "" {
		projectName = fmt.Sprintf("routex-relay-%x", time.Now().UnixNano()%1000000)
	}

	client := provision.NewDenoClient("", 30*time.Second)
	res, err := client.ProvisionRelay(ctx, provision.DenoProvisionParams{
		AccessToken: accessToken,
		ProjectName: projectName,
	})
	if err != nil {
		httpx.BadRequest(w, r, "deno_provision_failed", fmt.Sprintf("Gagal mendeploy script relay ke Deno: %v", err))
		return
	}

	poolName := strings.TrimSpace(req.Name)
	if poolName == "" {
		poolName = fmt.Sprintf("🦕 Deno Relay (%s)", res.ProjectName)
	}

	region := "deno-edge"
	if req.Region != nil && strings.TrimSpace(*req.Region) != "" {
		region = strings.TrimSpace(*req.Region)
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	enabled := true
	weight := 1
	ep, err := h.egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name:      poolName,
		Kind:      "https",
		ProxyURL:  security.Secret(res.RelayURL),
		Enabled:   &enabled,
		Weight:    &weight,
		Region:    &region,
		CreatedBy: actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	h.writeAudit(ctx, r, "provision", "egress_pool_deno", ep.ID, map[string]any{
		"name":         ep.Name,
		"project_name": res.ProjectName,
		"relay_url":    res.RelayURL,
	})

	_ = h.respond(w, r, http.StatusCreated, provisionResultDTO{
		Pool:         toEgressPoolDTO(ep),
		ProviderType: "deno_deploy_relay",
		TargetURL:    res.RelayURL,
	})
}
