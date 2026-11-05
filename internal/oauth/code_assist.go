package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/NexGen-X/Route-X/internal/security"
)

// DefaultCodeAssistEndpoint adalah endpoint loadCodeAssist Code Assist pada
// host harian yang sama dengan fetchAvailableModels Antigravity.
//
// loadCodeAssist adalah langkah yang dilakukan klien resmi (agy) sesudah
// login untuk memperoleh project Code Assist akun (cloudaicompanionProject).
// Tanpa project itu, daftar model yang dijawab upstream bersifat generik:
// model tergating-project (rollout/preview per project) tidak ditawarkan
// (B-157).
const DefaultCodeAssistEndpoint = "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"

// WithCodeAssistEndpoint mengeset custom endpoint loadCodeAssist (berguna
// untuk pengujian unit dengan mock server).
func (c *GoogleOAuthClient) WithCodeAssistEndpoint(endpoint string) *GoogleOAuthClient {
	c.codeAssistEndpoint = endpoint
	return c
}

// ParseCodeAssistProject mengekstrak project Code Assist dari body respons
// loadCodeAssist.
//
// Bentuk cloudaicompanionProject yang dikenal ada dua: string project ID
// langsung, dan objek yang membawa field "id". Keduanya diterima. Field yang
// absen/null/bentuk lain yang tidak dikenal menghasilkan string kosong TANPA
// error — artinya upstream tidak menawarkan project untuk akun ini, dan
// pemanggil jatuh ke perilaku generik (tanpa project), bukan gagal.
func ParseCodeAssistProject(body []byte) (string, error) {
	var resp struct {
		CloudaicompanionProject json.RawMessage `json:"cloudaicompanionProject"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("uraian json respons loadCodeAssist: %w", err)
	}
	raw := bytes.TrimSpace(resp.CloudaicompanionProject)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var sebagaiString string
	if err := json.Unmarshal(raw, &sebagaiString); err == nil {
		return strings.TrimSpace(sebagaiString), nil
	}
	var sebagaiObjek struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &sebagaiObjek); err == nil {
		return strings.TrimSpace(sebagaiObjek.ID), nil
	}
	return "", nil
}

// LoadCodeAssistProject memanggil loadCodeAssist dengan access token sesi
// dan mengembalikan project Code Assist akun ("" bila upstream tidak
// menawarkannya).
//
// Kegagalan memanggil BUKAN kegagalan alur pemanggil: resolusi project adalah
// peningkatan (model tergating-project menjadi terlihat), dan ketiadaannya
// berarti perilaku generik lama. Pemanggil mencatat error ini lalu
// melanjutkan tanpa project — tidak ada project yang dikarang di sini.
func (c *GoogleOAuthClient) LoadCodeAssistProject(ctx context.Context, accessToken string, proxyURL security.Secret) (string, error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access token kosong untuk loadCodeAssist")
	}

	// Metadata permintaan mengikuti bentuk yang dipakai klien Code Assist:
	// identitas IDE/plugin dinyatakan generik, bukan mengklaim sebagai
	// aplikasi tertentu.
	body, err := json.Marshal(map[string]any{
		"metadata": map[string]string{
			"ideType":    "IDE_UNSPECIFIED",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	})
	if err != nil {
		return "", fmt.Errorf("menyusun request loadCodeAssist: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.codeAssistEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("membuat request loadCodeAssist: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client, err := c.httpClientForProxy(proxyURL)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("menghubungi server code assist google: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes))
	if err != nil {
		return "", fmt.Errorf("membaca respons loadCodeAssist: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Pesan error upstream diambil dari field error-nya saja; body
		// mentah tidak diteruskan karena tidak ada jaminan isinya bebas
		// dari pantulan kredensial.
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &errBody)
		msg := strings.TrimSpace(errBody.Error.Message)
		if msg == "" {
			msg = fmt.Sprintf("http status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("server code assist menolak loadCodeAssist: %s", msg)
	}

	return ParseCodeAssistProject(raw)
}
