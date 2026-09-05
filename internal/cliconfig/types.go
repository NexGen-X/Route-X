// Package cliconfig mengelola deteksi otomatis perkakas AI CLI di sistem operasi lokal
// serta konfigurasi terpadu 3-Mode (Model Only, Routing, Combo Routing).
package cliconfig

import (
	"time"
)

// Mode konfigurasi CLI yang didukung oleh Route-X.
const (
	// ModeModelOnly mengarahkan CLI langsung ke model target tanpa failover multi-provider.
	ModeModelOnly = "model_only"

	// ModeRouting mengarahkan CLI ke aturan routing dinamis / load balancer Route-X.
	ModeRouting = "routing"

	// ModeCombo mengarahkan CLI ke Smart Tiered Cascade (Tier 1 Cepat/Hemat -> Tier 2 Flagship Fallback).
	ModeCombo = "combo"
)

// ToolDef merepresentasikan definisi dan aturan deteksi satu perkakas AI CLI.
type ToolDef struct {
	ID            string   // Pengidentifikasi unik tool, misalnya "agy", "claude"
	Name          string   // Nama tampilan perkakas
	Category      string   // Kategori tool: "Coding Agent", "Terminal Chat", "Workflow"
	Description   string   // Penjelasan ringkas fungsionalitas tool
	BinaryNames   []string // Nama-nama biner eksekusi untuk pencarian di PATH
	ConfigPaths   []string // Jalur relatif berkas konfigurasi di bawah direktori $HOME
	VersionArg    string   // Argumen baris perintah untuk memeriksa versi (misal "--version")
	EnvVarBaseURL string   // Nama variabel lingkungan untuk base URL API
	EnvVarAPIKey  string   // Nama variabel lingkungan untuk API key
	EnvVarModel   string   // Nama variabel lingkungan untuk model bawaan
	DefaultMode   string   // Mode bawaan saat pertama kali terdeteksi
	DefaultTarget string   // Target model/rule bawaan
}

// ToolStatus adalah hasil pemindaian dan status terkini satu perkakas AI CLI.
type ToolStatus struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Category       string            `json:"category"`
	Installed      bool              `json:"installed"`
	Path           string            `json:"path,omitempty"`
	Version        string            `json:"version,omitempty"`
	ConfigPath     string            `json:"config_path,omitempty"`
	SupportedModes []string          `json:"supported_modes"`
	ActiveMode     string            `json:"active_mode"`
	ActiveTarget   string            `json:"active_target"`
	EnvVars        map[string]string `json:"env_vars"`
	ExportSnippet  string            `json:"export_snippet"`
	UpdatedAt      time.Time         `json:"updated_at,omitempty"`
}

// ConfigureParams memuat parameter mutasi konfigurasi CLI dari admin request.
type ConfigureParams struct {
	ToolID     string `json:"tool_id"`
	Mode       string `json:"mode"`   // "model_only", "routing", "combo"
	Target     string `json:"target"` // nama model, rule, atau combo
	APIKey     string `json:"api_key,omitempty"`
	GatewayURL string `json:"gateway_url,omitempty"`
}
