package provision

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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
//
// Relay ini WAJIB diautentikasi: tanpa pemeriksaan token, siapa pun yang tahu
// atau menebak URL *.deno.dev dapat meneruskan request ke host arbitrer memakai
// kuota dan reputasi akun Deno operator (open relay — audit3 V-EO-03). Tokennya
// disuntikkan saat provisioning lewat env ROUTEX_RELAY_TOKEN dan dikirim
// gateway sebagai header x-relay-token; kedua header kendali (x-relay-token &
// x-target-host) dibuang sebelum diteruskan ke upstream. Template yang dipasang
// manual tanpa env itu tetap terbuka — teks ini adalah satu-satunya tempat
// fakta tersebut dinyatakan.
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

  const relayToken = Deno.env.get("ROUTEX_RELAY_TOKEN");
  if (relayToken && req.headers.get("x-relay-token") !== relayToken) {
    return new Response(JSON.stringify({ error: "unauthorized" }), {
      status: 401,
      headers: { "content-type": "application/json" },
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
  newHeaders.delete("x-relay-token");

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
	// RelayToken adalah token autentikasi relay yang baru dibuat untuk
	// deployment ini. Pemanggil WAJIB menyimpannya bersama URL relay (sebagai
	// kredensial pool) — tanpa token yang sama, gateway tidak bisa memakai
	// relay dan relay tidak bisa membedakan gateway dari orang asing.
	RelayToken string `json:"relay_token"`
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

// hasilProject adalah keluaran ensureProject: ID project beserta penanda
// apakah project itu BARU dibuat oleh panggilan ini (relevan untuk kompensasi
// yatim — project yang sudah ada sebelumnya bukan milik panggilan ini dan
// tidak boleh dihapus saat langkah berikutnya gagal).
type hasilProject struct {
	id   string
	baru bool
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

	// Token autentikasi relay untuk deployment ini. Dibuat per provisioning
	// dan ditanam sebagai env pada deployment; gateway mengirimnya kembali
	// sebagai header x-relay-token (lihat DenoProxyScriptTemplate).
	relayToken, err := buatTokenRelay()
	if err != nil {
		return nil, fmt.Errorf("gagal membuat token relay: %w", err)
	}

	// 1. Buat atau periksa project di Deno
	proj, err := c.ensureProject(ctx, token, projectName)
	if err != nil {
		return nil, fmt.Errorf("gagal menyiapkan project Deno: %w", err)
	}

	// 2. Deploy script proxy ke project
	depResult, err := c.deployScript(ctx, token, proj.id, projectName, relayToken)
	if err != nil {
		// Kompensasi yatim (audit3 V-EO-06): project yang BARU dibuat oleh
		// panggilan ini tetapi gagal dideploy tidak berguna bagi siapa pun,
		// dan nama otomatisnya berubah tiap percobaan sehingga retry hanya
		// akan menumpuk yatim baru. Hapus best-effort; project yang sudah
		// ada sebelumnya TIDAK disentuh.
		if proj.baru {
			if derr := c.deleteProject(ctx, token, proj.id); derr != nil {
				return nil, fmt.Errorf("gagal mendeploy script proxy ke Deno: %w (project yatim %s juga gagal dihapus: %v)",
					err, proj.id, derr)
			}
			return nil, fmt.Errorf("gagal mendeploy script proxy ke Deno: %w (project %s yang baru dibuat sudah dihapus)", err, proj.id)
		}
		return nil, fmt.Errorf("gagal mendeploy script proxy ke Deno: %w", err)
	}
	depResult.RelayToken = relayToken

	return depResult, nil
}

// buatTokenRelay membuat token autentikasi relay acak (32 byte, heksadesimal).
func buatTokenRelay() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (c *DenoClient) ensureProject(ctx context.Context, token, projectName string) (hasilProject, error) {
	apiURL := fmt.Sprintf("%s/projects", c.baseURL)
	bodyBytes, _ := json.Marshal(denoCreateProjectReq{Name: projectName})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return hasilProject{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Route-X-Provisioner/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return hasilProject{}, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return hasilProject{}, err
	}

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var pResp denoProjectResp
		if err := json.Unmarshal(respBytes, &pResp); err == nil && pResp.ID != "" {
			return hasilProject{id: pResp.ID, baru: true}, nil
		}
	}

	// Jika 409 Conflict (Project already exists), kita bisa gunakan projectName langsung sebagai project ID
	if resp.StatusCode == http.StatusConflict {
		return hasilProject{id: projectName, baru: false}, nil
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
	return hasilProject{}, fmt.Errorf("deno API error: %s", errMsg)
}

// deleteProject menghapus project Deno Deploy. Dipakai hanya sebagai
// kompensasi best-effort untuk project yang baru dibuat provisioning yang
// sama (lihat ProvisionRelay).
func (c *DenoClient) deleteProject(ctx context.Context, token, projectID string) error {
	apiURL := fmt.Sprintf("%s/projects/%s", c.baseURL, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Route-X-Provisioner/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("HTTP %d saat menghapus project", resp.StatusCode)
	}
	return nil
}

func (c *DenoClient) deployScript(ctx context.Context, token, projectID, projectName, relayToken string) (*DenoProvisionResult, error) {
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
		EnvVars: map[string]string{
			"ROUTEX_RELAY_TOKEN": relayToken,
		},
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
