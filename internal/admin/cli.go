package admin

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/cliconfig"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

// cliRoutes mendaftarkan seluruh endpoint manajemen dan integrasi perkakas CLI AI.
func (h *Handlers) cliRoutes(r chi.Router) {
	// Menampilkan daftar CLI yang terdeteksi di sistem dan status modenya
	r.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/detected", h.listDetectedCLIs)

	// Menerapkan konfigurasi 3-mode ke perkakas CLI tertentu
	r.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/configure", h.configureCLI)

	// Mengunduh / melihat skrip shell gabungan ~/.routex/cli-env.sh
	r.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/env-export", h.getCLIExportScript)
}

// listDetectedCLIs memindai binary CLI di sistem dan mengembalikan status terpasang beserta mode aktifnya.
func (h *Handlers) listDetectedCLIs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.cliManager == nil {
		httpx.BadRequest(w, r, "cli_manager_unavailable", "Manajer integrasi CLI belum diinisialisasi pada server")
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	gwURL := fmt.Sprintf("%s://%s/v1", scheme, r.Host)

	tools, err := h.cliManager.Scan(ctx, gwURL)
	if err != nil {
		h.logger.Error("gagal memindai CLI di host", "error", err)
		httpx.InternalError(w, r)
		return
	}

	dtoTools := make([]CLIToolDTO, len(tools))
	detectedCount := 0
	for i, t := range tools {
		if t.Installed {
			detectedCount++
		}
		var updPtr *time.Time
		if !t.UpdatedAt.IsZero() {
			updPtr = &t.UpdatedAt
		}

		dtoTools[i] = CLIToolDTO{
			ID:             t.ID,
			Name:           t.Name,
			Description:    t.Description,
			Category:       t.Category,
			Installed:      t.Installed,
			Path:           t.Path,
			Version:        t.Version,
			ConfigPath:     t.ConfigPath,
			SupportedModes: t.SupportedModes,
			ActiveMode:     t.ActiveMode,
			ActiveTarget:   t.ActiveTarget,
			EnvVars:        t.EnvVars,
			ExportSnippet:  t.ExportSnippet,
			UpdatedAt:      updPtr,
		}
	}

	homeDir, _ := os.UserHomeDir()
	envFilePath := filepath.Join(homeDir, ".routex", "cli-env.sh")

	res := CLIDetectedResponseDTO{
		Tools:          dtoTools,
		TotalDetected:  detectedCount,
		TotalAvailable: len(dtoTools),
		GatewayURL:     gwURL,
		DefaultEnvFile: envFilePath,
	}

	_ = h.respond(w, r, http.StatusOK, res)
}

// configureCLI memproses permintaan konfigurasi 3-Mode untuk perkakas CLI tertentu.
func (h *Handlers) configureCLI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.cliManager == nil {
		httpx.BadRequest(w, r, "cli_manager_unavailable", "Manajer integrasi CLI belum diinisialisasi")
		return
	}

	var req CLIConfigureRequestDTO
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_payload", "Format JSON konfigurasi CLI tidak valid: "+err.Error())
		return
	}

	if req.ToolID == "" {
		httpx.BadRequest(w, r, "missing_field", "tool_id wajib diisi")
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	gwURL := req.GatewayURL
	if gwURL == "" {
		gwURL = fmt.Sprintf("%s://%s/v1", scheme, r.Host)
	}

	params := cliconfig.ConfigureParams{
		ToolID:     req.ToolID,
		Mode:       req.Mode,
		Target:     req.Target,
		APIKey:     req.APIKey,
		GatewayURL: gwURL,
	}

	status, err := h.cliManager.Configure(ctx, params, gwURL)
	if err != nil {
		h.logger.Warn("gagal mengonfigurasi CLI", "tool", req.ToolID, "error", err)
		httpx.BadRequest(w, r, "configure_failed", err.Error())
		return
	}

	h.writeAudit(ctx, r, "configure", "cli_tool", req.ToolID, map[string]any{
		"mode":   req.Mode,
		"target": req.Target,
	})

	var updPtr *time.Time
	if !status.UpdatedAt.IsZero() {
		updPtr = &status.UpdatedAt
	}

	res := CLIConfigureResponseDTO{
		Success: true,
		Tool: CLIToolDTO{
			ID:             status.ID,
			Name:           status.Name,
			Description:    status.Description,
			Category:       status.Category,
			Installed:      status.Installed,
			Path:           status.Path,
			Version:        status.Version,
			ConfigPath:     status.ConfigPath,
			SupportedModes: status.SupportedModes,
			ActiveMode:     status.ActiveMode,
			ActiveTarget:   status.ActiveTarget,
			EnvVars:        status.EnvVars,
			ExportSnippet:  status.ExportSnippet,
			UpdatedAt:      updPtr,
		},
		Message:       fmt.Sprintf("Konfigurasi mode '%s' untuk %s berhasil diterapkan ke sistem.", status.ActiveMode, status.Name),
		ConfigFile:    status.ConfigPath,
		ExportSnippet: status.ExportSnippet,
	}

	_ = h.respond(w, r, http.StatusOK, res)
}

// getCLIExportScript menyajikan konten lengkap berkas shell script ~/.routex/cli-env.sh.
func (h *Handlers) getCLIExportScript(w http.ResponseWriter, r *http.Request) {
	if h.cliManager == nil {
		httpx.BadRequest(w, r, "cli_manager_unavailable", "Manajer integrasi CLI belum diinisialisasi")
		return
	}

	homeDir, _ := os.UserHomeDir()
	content, err := h.cliManager.GetExportScript(homeDir)
	if err != nil {
		h.logger.Error("gagal membaca skrip ekspor CLI", "error", err)
		httpx.InternalError(w, r)
		return
	}

	res := CLIExportScriptDTO{
		Content:  content,
		FilePath: filepath.Join(homeDir, ".routex", "cli-env.sh"),
	}

	_ = h.respond(w, r, http.StatusOK, res)
}
