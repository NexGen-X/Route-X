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

const defaultDenoBaseURL = "https://api.deno.com/v1"

// DenoProxyScriptTemplate adalah skrip Deno streaming proxy yang di-deploy ke Deno Deploy.
const DenoProxyScriptTemplate = `Deno.serve(async (req) => {
  if (req.method === "OPTIONS") {
    return new Response(null, {
      headers: {
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Headers": "*",
        "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
      },
    });
  }

  const url = new URL(req.url);
  let targetHost = "api.openai.com";
  const customTarget = req.headers.get("x-target-host");
  if (customTarget) {
    targetHost = customTarget;
  }
  url.hostname = targetHost;
  url.protocol = "https:";
  url.port = "443";

  const newHeaders = new Headers(req.headers);
  newHeaders.delete("host");
  newHeaders.delete("x-target-host");

  const proxyReq = new Request(url.toString(), {
    method: req.method,
    headers: newHeaders,
    body: req.body,
    redirect: "follow",
  });

  try {
    return await fetch(proxyReq);
  } catch (err) {
    return new Response(JSON.stringify({ error: String(err) }), {
      status: 502,
      headers: { "content-type": "application/json" },
    });
  }
});`

// DenoClient menangani komunikasi ke REST API resmi Deno Deploy.
type DenoClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewDenoClient menginisialisasi klien Deno baru.
func NewDenoClient(baseURL string, timeout time.Duration) *DenoClient {
	if baseURL == "" {
		baseURL = defaultDenoBaseURL
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &DenoClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// DenoProvisionParams parameter input untuk mendaftarkan project dan deployment di Deno Deploy.
type DenoProvisionParams struct {
	AccessToken string `json:"access_token"`
	ProjectName string `json:"project_name"`
}

// DenoProvisionResult hasil deployment di Deno Deploy.
type DenoProvisionResult struct {
	ProjectID    string `json:"project_id"`
	ProjectName  string `json:"project_name"`
	RelayURL     string `json:"relay_url"`
	DeploymentID string `json:"deployment_id"`
}

type denoCreateProjectReq struct {
	Name string `json:"name"`
}

type denoProjectResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

type denoDeploymentReq struct {
	EntryPointURL string                       `json:"entryPointUrl"`
	Assets        map[string]denoAssetFileInfo `json:"assets"`
	EnvVars       map[string]string            `json:"envVars"`
}

type denoAssetFileInfo struct {
	Kind     string `json:"kind"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type denoDeploymentResp struct {
	ID      string   `json:"id"`
	Domains []string `json:"domains"`
	Code    string   `json:"code,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// ProvisionRelay membuat project di Deno Deploy (jika belum ada) dan mendeploy script streaming proxy.
func (c *DenoClient) ProvisionRelay(ctx context.Context, p DenoProvisionParams) (*DenoProvisionResult, error) {
	token := strings.TrimSpace(p.AccessToken)
	if token == "" {
		return nil, errors.New("deno access_token tidak boleh kosong")
	}

	projectName := strings.TrimSpace(p.ProjectName)
	if projectName == "" {
		projectName = fmt.Sprintf("routex-relay-%x", time.Now().UnixNano()%1000000)
	}

	// 1. Buat atau periksa project di Deno
	projectID, err := c.ensureProject(ctx, token, projectName)
	if err != nil {
		return nil, fmt.Errorf("gagal menyiapkan project Deno: %w", err)
	}

	// 2. Deploy script proxy ke project
	depResult, err := c.deployScript(ctx, token, projectID, projectName)
	if err != nil {
		return nil, fmt.Errorf("gagal mendeploy script proxy ke Deno: %w", err)
	}

	return depResult, nil
}

func (c *DenoClient) ensureProject(ctx context.Context, token, projectName string) (string, error) {
	apiURL := fmt.Sprintf("%s/projects", c.baseURL)
	bodyBytes, _ := json.Marshal(denoCreateProjectReq{Name: projectName})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Route-X-Provisioner/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var pResp denoProjectResp
		if err := json.Unmarshal(respBytes, &pResp); err == nil && pResp.ID != "" {
			return pResp.ID, nil
		}
	}

	// Jika 409 Conflict (Project already exists), kita bisa gunakan projectName langsung sebagai project ID
	if resp.StatusCode == http.StatusConflict {
		return projectName, nil
	}

	var errResp denoProjectResp
	_ = json.Unmarshal(respBytes, &errResp)
	errMsg := errResp.Error
	if errMsg == "" {
		errMsg = errResp.Code
	}
	if errMsg == "" {
		errMsg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBytes))
	}
	return "", fmt.Errorf("deno API error: %s", errMsg)
}

func (c *DenoClient) deployScript(ctx context.Context, token, projectID, projectName string) (*DenoProvisionResult, error) {
	apiURL := fmt.Sprintf("%s/projects/%s/deployments", c.baseURL, projectID)

	reqPayload := denoDeploymentReq{
		EntryPointURL: "main.ts",
		Assets: map[string]denoAssetFileInfo{
			"main.ts": {
				Kind:     "file",
				Content:  DenoProxyScriptTemplate,
				Encoding: "utf-8",
			},
		},
		EnvVars: map[string]string{},
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Route-X-Provisioner/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errResp denoDeploymentResp
		_ = json.Unmarshal(respBytes, &errResp)
		errMsg := errResp.Error
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBytes))
		}
		return nil, fmt.Errorf("deno deployment error: %s", errMsg)
	}

	var depResp denoDeploymentResp
	_ = json.Unmarshal(respBytes, &depResp)

	relayURL := fmt.Sprintf("https://%s.deno.dev", projectName)
	if len(depResp.Domains) > 0 {
		relayURL = "https://" + depResp.Domains[0]
	}

	return &DenoProvisionResult{
		ProjectID:    projectID,
		ProjectName:  projectName,
		RelayURL:     relayURL,
		DeploymentID: depResp.ID,
	}, nil
}
