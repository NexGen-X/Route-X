package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/NexGen-X/Route-X/internal/security"
)

// DefaultCodeAssistEndpoint adalah endpoint loadCodeAssist Code Assist pada
// host harian yang sama dengan fetchAvailableModels Antigravity.
//
// loadCodeAssist adalah langkah yang dilakukan klien resmi (agy) sesudah
// login untuk memperoleh project Code Assist akun (cloudaicompanionProject).
// Konteks tier/project pada respons menentukan resolusi sesi; tanpa project,
// pemanggil mempertahankan perilaku generik lamanya (B-157).
const DefaultCodeAssistEndpoint = "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"

// Sidik jari klien berikut adalah bentuk yang terbukti pada pembuktian
// bertahap B-157 (5 Nov 2026): metadata generik membuat akun individual
// terbaca pada konteks klien lama yang tidak didukung dan tidak pernah
// menerima project, sedangkan bentuk lengkap di bawah mengembalikan tier
// akun yang sebenarnya beserta cloudaicompanionProject. Versi ini adalah
// snapshot bukti tangkapan saat itu; perbarui hanya dari bukti baru, bukan
// tebakan.
const (
	codeAssistClientVersion  = "1.23.2"
	codeAssistUserAgent      = "antigravity/" + codeAssistClientVersion
	codeAssistClientMetadata = `{"ideType":"ANTIGRAVITY","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`
)

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

// codeAssistTierSummary adalah ringkasan tier aman-log dari respons
// loadCodeAssist. Respons mentahnya sengaja tidak pernah dicatat: di
// dalamnya ada URI upgrade yang dapat memuat alamat email akun. Yang
// dicatat hanya ID tier, status pengelolaan project, dan ada/tidaknya
// project — cukup untuk membedakan konteks akun tanpa membocorkan isi
// respons.
type codeAssistTierSummary struct {
	CurrentTier   string
	PaidTier      string
	AllowedTiers  []string
	GCPManaged    bool
	ProjectAda    bool
	IneligibleAda bool
}

// parseCodeAssistTierSummary mengekstrak field tier yang aman dicatat dari
// respons loadCodeAssist. Field yang absen tetap kosong/false; JSON rusak
// menghasilkan error agar pemanggil tahu ringkasannya tidak tersedia.
func parseCodeAssistTierSummary(body []byte, project string) (codeAssistTierSummary, error) {
	var resp struct {
		CurrentTier *struct {
			ID string `json:"id"`
		} `json:"currentTier"`
		PaidTier *struct {
			ID string `json:"id"`
		} `json:"paidTier"`
		AllowedTiers []struct {
			ID string `json:"id"`
		} `json:"allowedTiers"`
		IneligibleTiers []struct {
			ID string `json:"id"`
		} `json:"ineligibleTiers"`
		GCPManaged bool `json:"gcpManaged"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return codeAssistTierSummary{}, fmt.Errorf("uraian json tier loadCodeAssist: %w", err)
	}
	summary := codeAssistTierSummary{
		GCPManaged:    resp.GCPManaged,
		ProjectAda:    project != "",
		IneligibleAda: len(resp.IneligibleTiers) > 0,
	}
	if resp.CurrentTier != nil {
		summary.CurrentTier = strings.TrimSpace(resp.CurrentTier.ID)
	}
	if resp.PaidTier != nil {
		summary.PaidTier = strings.TrimSpace(resp.PaidTier.ID)
	}
	for _, tier := range resp.AllowedTiers {
		if id := strings.TrimSpace(tier.ID); id != "" {
			summary.AllowedTiers = append(summary.AllowedTiers, id)
		}
	}
	return summary, nil
}

// LoadCodeAssistProject memanggil loadCodeAssist dengan access token sesi
// dan mengembalikan project Code Assist akun ("" bila upstream tidak
// menawarkannya).
//
// Kegagalan memanggil BUKAN kegagalan alur pemanggil: resolusi project adalah
// peningkatan konteks tier/project sesi, dan ketiadaannya berarti perilaku
// generik lama. Pemanggil mencatat error ini lalu melanjutkan tanpa project
// — tidak ada project yang dikarang di sini.
func (c *GoogleOAuthClient) LoadCodeAssistProject(ctx context.Context, accessToken string, proxyURL security.Secret) (string, error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access token kosong untuk loadCodeAssist")
	}

	// Metadata dan header permintaan mengikuti bentuk klien Antigravity
	// yang terbukti pada Tahap 1 B-157: ideType ANTIGRAVITY, ideVersion,
	// User-Agent versi yang sama, dan header Client-Metadata yang konsisten.
	// Mengubah hanya sebagian bentuk ini terbukti tidak cukup — identitas
	// generik maupun ideType saja tetap menghasilkan respons tanpa project.
	body, err := json.Marshal(map[string]any{
		"metadata": map[string]string{
			"ideType":    "ANTIGRAVITY",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
			"ideVersion": codeAssistClientVersion,
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
	req.Header.Set("User-Agent", codeAssistUserAgent)
	req.Header.Set("Client-Metadata", codeAssistClientMetadata)

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

	project, err := ParseCodeAssistProject(raw)
	if err != nil {
		return "", err
	}
	if summary, terr := parseCodeAssistTierSummary(raw, project); terr == nil {
		slog.Default().InfoContext(ctx, "respons loadCodeAssist diterima",
			"current_tier", summary.CurrentTier,
			"paid_tier", summary.PaidTier,
			"allowed_tiers", summary.AllowedTiers,
			"gcp_managed", summary.GCPManaged,
			"project_tersedia", summary.ProjectAda,
			"ineligible_tiers_ada", summary.IneligibleAda)
	} else {
		slog.Default().WarnContext(ctx, "gagal mengurai ringkasan tier loadCodeAssist",
			"error", terr, "project_tersedia", project != "")
	}
	return project, nil
}
