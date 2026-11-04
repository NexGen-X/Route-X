package cliconfig

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRegistriTahap1KejujuranKatalog membuktikan keputusan kelompok B:
// entri tool yang tidak eksis/mati/tak teridentifikasi DICABUT
// (B-127 gpt-cli, B-132 codecompanion, B-133 mentat), dan tool yang
// secara arsitektur tidak dapat diarahkan ke gateway tetap terdaftar
// dengan alasan eksplisit (B-130 cursor-agent, B-131 cody, B-134
// plandex, B-135 agy, B-136 ollama) — sisanya didukung penuh.
func TestRegistriTahap1KejujuranKatalog(t *testing.T) {
	tools := SupportedTools()
	if len(tools) != 17 {
		t.Fatalf("jumlah tool registri = %d, mau 17 (20 semula - 3 dicabut)", len(tools))
	}
	for _, dicabut := range []string{"gpt-cli", "codecompanion", "mentat"} {
		if _, ok := cariToolDef(dicabut); ok {
			t.Errorf("tool %s seharusnya sudah dicabut dari registri", dicabut)
		}
	}
	tidakDidukung := map[string]bool{
		"agy": true, "ollama": true, "cursor-agent": true, "cody": true, "plandex": true,
	}
	for _, tool := range tools {
		if tidakDidukung[tool.ID] {
			if tool.UnsupportedReason == "" {
				t.Errorf("tool %s wajib membawa UnsupportedReason", tool.ID)
			}
		} else if tool.UnsupportedReason != "" {
			t.Errorf("tool %s seharusnya didukung gateway, alasan terisi: %q", tool.ID, tool.UnsupportedReason)
		}
	}
}

// TestConfigureDitolakToolTidakDidukungGateway membuktikan Configure
// untuk kelompok B ditolak DENGAN alasan, sebelum efek samping apa
// pun: tidak ada setelan tersimpan dan tidak ada key.env tertulis.
func TestConfigureDitolakToolTidakDidukungGateway(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()
	store := newMockSettingsStore()
	mgr := NewManager(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, id := range []string{"agy", "ollama", "cursor-agent", "cody", "plandex"} {
		_, err := mgr.Configure(ctx, ConfigureParams{
			ToolID: id, Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-uji-123",
		}, "http://localhost:8080/v1")
		if err == nil {
			t.Errorf("Configure %s seharusnya ditolak", id)
			continue
		}
		if !strings.Contains(err.Error(), "tidak dapat dikonfigurasi ke gateway") {
			t.Errorf("pesan penolakan %s tidak jujur: %v", id, err)
		}
		if _, errGet := store.Get(ctx, "cli:config:"+id); errGet == nil {
			t.Errorf("Configure %s yang ditolak tetap menyimpan setelan", id)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".routex", "key.env")); !os.IsNotExist(err) {
		t.Errorf("key.env tidak boleh ditulis oleh Configure yang ditolak")
	}

	// Pindaian menampilkan statusnya apa adanya: tidak didukung,
	// membawa alasan, dan tanpa ekspor env karangan.
	statuses, err := mgr.Scan(ctx, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, st := range statuses {
		if st.ID == "ollama" || st.ID == "agy" {
			if st.GatewaySupported {
				t.Errorf("Scan %s: GatewaySupported harus false", st.ID)
			}
			if st.UnsupportedReason == "" {
				t.Errorf("Scan %s: UnsupportedReason kosong", st.ID)
			}
			if len(st.EnvVars) != 0 || st.ExportSnippet != "" {
				t.Errorf("Scan %s: tidak boleh ada ekspor env, dapat %v / %q", st.ID, st.EnvVars, st.ExportSnippet)
			}
		}
		if st.ID == "claude" && !st.GatewaySupported {
			t.Errorf("Scan claude: GatewaySupported harus true")
		}
	}
}

// TestConfigureShellGPTBaseURL membuktikan B-122: sgpt membaca
// API_BASE_URL (dengan /v1), bukan OPENAI_BASE_URL warisan.
func TestConfigureShellGPTBaseURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "sgpt", Mode: ModeModelOnly, Target: "intern-s2",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure sgpt: %v", err)
	}
	if res.EnvVars["API_BASE_URL"] != "http://localhost:8080/v1" {
		t.Errorf("EnvVars API_BASE_URL = %q", res.EnvVars["API_BASE_URL"])
	}
	if _, ada := res.EnvVars["OPENAI_BASE_URL"]; ada {
		t.Errorf("OPENAI_BASE_URL warisan tidak boleh lagi diekspor untuk sgpt")
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "shell_gpt", ".sgptrc"))
	if err != nil {
		t.Fatalf(".sgptrc tidak ditulis: %v", err)
	}
	if !strings.Contains(string(data), "API_BASE_URL=http://localhost:8080/v1") {
		t.Errorf(".sgptrc tidak memuat API_BASE_URL gateway:\n%s", data)
	}
	if strings.Contains(string(data), "OPENAI_BASE_URL") {
		t.Errorf(".sgptrc masih memuat nama warisan:\n%s", data)
	}
}

// TestConfigureFabricLengkap membuktikan B-123: .env fabric memuat
// OPENAI_API_BASE_URL + DEFAULT_VENDOR + kunci (bukan OPENAI_BASE_URL
// yang diabaikan tool hingga kunci bocor ke api.openai.com),
// config.yaml memuat disableResponsesAPI, dan pola minimal tersedia.
func TestConfigureFabricLengkap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "fabric", Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-fabric-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure fabric: %v", err)
	}
	if res.EnvVars["OPENAI_API_BASE_URL"] != "http://localhost:8080/v1" || res.EnvVars["DEFAULT_VENDOR"] != "OpenAI" {
		t.Errorf("EnvVars fabric salah: %v", res.EnvVars)
	}
	envPath := filepath.Join(home, ".config", "fabric", ".env")
	fi, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf(".env fabric tidak ditulis: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("izin .env fabric = %o, mau 600 (memuat kunci)", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(envPath)
	for _, mau := range []string{
		"OPENAI_API_KEY=kunci-fabric-123",
		"OPENAI_API_BASE_URL=http://localhost:8080/v1",
		"DEFAULT_VENDOR=OpenAI",
		"DEFAULT_MODEL=intern-s2",
	} {
		if !strings.Contains(string(data), mau) {
			t.Errorf(".env fabric tidak memuat %q:\n%s", mau, data)
		}
	}
	if strings.Contains(string(data), "\nOPENAI_BASE_URL=") || strings.HasPrefix(string(data), "OPENAI_BASE_URL=") {
		t.Errorf(".env fabric masih memuat OPENAI_BASE_URL warisan:\n%s", data)
	}
	cfg, err := os.ReadFile(filepath.Join(home, ".config", "fabric", "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml fabric tidak ditulis: %v", err)
	}
	if !strings.Contains(string(cfg), "disableResponsesAPI: true") {
		t.Errorf("config.yaml fabric tidak memuat disableResponsesAPI: true:\n%s", cfg)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "fabric", "patterns", "routex", "system.md")); err != nil {
		t.Errorf("pola minimal fabric tidak dibuat: %v", err)
	}
}

// TestConfigureLLMBerkasModel membuktikan B-124: llm mengenal model
// gateway lewat extra-openai-models.yaml (entri model lain
// dipertahankan) dan default_model.txt; tidak ada ekspor env rekaan.
func TestConfigureLLMBerkasModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	llmDir := filepath.Join(home, ".config", "io.datasette.llm")
	if err := os.MkdirAll(llmDir, 0755); err != nil {
		t.Fatal(err)
	}
	lama := "- model_id: model-lain\n  model_name: model-lain\n  api_base: https://contoh.invalid/v1\n  api_key_name: openai\n"
	if err := os.WriteFile(filepath.Join(llmDir, "extra-openai-models.yaml"), []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "llm", Mode: ModeModelOnly, Target: "intern-s2",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure llm: %v", err)
	}
	if len(res.EnvVars) != 0 {
		t.Errorf("llm tidak boleh mengekspor env apa pun, dapat %v", res.EnvVars)
	}
	data, err := os.ReadFile(filepath.Join(llmDir, "extra-openai-models.yaml"))
	if err != nil {
		t.Fatalf("extra-openai-models.yaml tidak ditulis: %v", err)
	}
	for _, mau := range []string{"model_id: intern-s2", "api_base: http://localhost:8080/v1", "api_key_name: openai", "model_id: model-lain"} {
		if !strings.Contains(string(data), mau) {
			t.Errorf("extra-openai-models.yaml tidak memuat %q:\n%s", mau, data)
		}
	}
	def, err := os.ReadFile(filepath.Join(llmDir, "default_model.txt"))
	if err != nil || strings.TrimSpace(string(def)) != "intern-s2" {
		t.Errorf("default_model.txt = %q, err %v", def, err)
	}
}

// TestConfigureOpenCommitSkemaV3 membuktikan B-125: keluaran memakai
// OCO_AI_PROVIDER/OCO_API_KEY/OCO_API_URL, bukan skema OCO_OPENAI_*.
func TestConfigureOpenCommitSkemaV3(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "opencommit", Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-oco-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure opencommit: %v", err)
	}
	if res.EnvVars["OCO_AI_PROVIDER"] != "openai" || res.EnvVars["OCO_API_URL"] != "http://localhost:8080/v1" {
		t.Errorf("EnvVars opencommit salah: %v", res.EnvVars)
	}
	data, err := os.ReadFile(filepath.Join(home, ".opencommit"))
	if err != nil {
		t.Fatalf(".opencommit tidak ditulis: %v", err)
	}
	for _, mau := range []string{"OCO_AI_PROVIDER=openai", "OCO_API_KEY=kunci-oco-123", "OCO_API_URL=http://localhost:8080/v1", "OCO_MODEL=intern-s2"} {
		if !strings.Contains(string(data), mau) {
			t.Errorf(".opencommit tidak memuat %q:\n%s", mau, data)
		}
	}
	if strings.Contains(string(data), "OCO_OPENAI") {
		t.Errorf(".opencommit masih memuat skema warisan:\n%s", data)
	}
}

// TestConfigureChatbladeVariabelBenar membuktikan B-126: chatblade
// membaca OPENAI_BASE_URL + OPENAI_API_MODEL.
func TestConfigureChatbladeVariabelBenar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "chatblade", Mode: ModeModelOnly, Target: "intern-s2",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure chatblade: %v", err)
	}
	if res.EnvVars["OPENAI_BASE_URL"] != "http://localhost:8080/v1" || res.EnvVars["OPENAI_API_MODEL"] != "intern-s2" {
		t.Errorf("EnvVars chatblade salah: %v", res.EnvVars)
	}
	if _, ada := res.EnvVars["CHATBLADE_MODEL"]; ada {
		t.Errorf("CHATBLADE_MODEL rekaan tidak boleh lagi diekspor")
	}
}

// TestConfigureCopilotBYOK membuktikan B-128: mode BYOK resmi
// COPILOT_PROVIDER_* menggantikan GITHUB_COPILOT_* rekaan.
func TestConfigureCopilotBYOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "copilot", Mode: ModeModelOnly, Target: "intern-s2",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure copilot: %v", err)
	}
	mau := map[string]string{
		"COPILOT_PROVIDER_BASE_URL": "http://localhost:8080/v1",
		"COPILOT_PROVIDER_TYPE":     "openai",
		"COPILOT_MODEL":             "intern-s2",
	}
	for k, v := range mau {
		if res.EnvVars[k] != v {
			t.Errorf("EnvVars[%s] = %q, mau %q", k, res.EnvVars[k], v)
		}
	}
	for k := range res.EnvVars {
		if strings.HasPrefix(k, "GITHUB_COPILOT_") {
			t.Errorf("variabel rekaan warisan masih diekspor: %s", k)
		}
	}
	def, _ := cariToolDef("copilot")
	if def.EnvVarAPIKey != "COPILOT_PROVIDER_API_KEY" {
		t.Errorf("EnvVarAPIKey copilot = %q", def.EnvVarAPIKey)
	}
}

// TestConfigureK8sGPTMenulisYAML membuktikan B-129: Configure menulis
// berkas YAML yang setara `k8sgpt auth add` + `auth default`
// (ai.providers + ai.defaultprovider), mempertahankan provider lain
// milik pengguna, dan tidak mengekspor env rekaan apa pun.
func TestConfigureK8sGPTMenulisYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	k8sDir := filepath.Join(home, ".config", "k8sgpt")
	if err := os.MkdirAll(k8sDir, 0755); err != nil {
		t.Fatal(err)
	}
	lama := "ai:\n  providers:\n    - name: ollama\n      baseurl: http://localhost:11434\n      model: llama3\nkubeconfig: /home/uji/.kube/config\n"
	if err := os.WriteFile(filepath.Join(k8sDir, "k8sgpt.yaml"), []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "k8sgpt", Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-k8s-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure k8sgpt: %v", err)
	}
	if len(res.EnvVars) != 0 {
		t.Errorf("k8sgpt tidak boleh mengekspor env apa pun, dapat %v", res.EnvVars)
	}
	yamlPath := filepath.Join(k8sDir, "k8sgpt.yaml")
	fi, err := os.Stat(yamlPath)
	if err != nil {
		t.Fatalf("k8sgpt.yaml tidak ditulis: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("izin k8sgpt.yaml = %o, mau 600 (memuat password)", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(yamlPath)
	for _, mau := range []string{
		"defaultprovider: openai",
		"- name: openai",
		"baseurl: http://localhost:8080/v1",
		"model: intern-s2",
		"password: kunci-k8s-123",
		"- name: ollama",                     // provider lain milik pengguna dipertahankan
		"kubeconfig: /home/uji/.kube/config", // seksi lain dipertahankan
	} {
		if !strings.Contains(string(data), mau) {
			t.Errorf("k8sgpt.yaml tidak memuat %q:\n%s", mau, data)
		}
	}
	if strings.Count(string(data), "- name: openai") != 1 {
		t.Errorf("entri openai harus tepat satu:\n%s", data)
	}
}

// TestEkstrakVersi membuktikan B-121: baris placeholder seperti
// "None" tidak pernah dilaporkan sebagai versi; keluaran nyata
// `cn --version` (semver polos) terbaca utuh.
func TestEkstrakVersi(t *testing.T) {
	kasus := []struct{ in, mau string }{
		{"1.5.47\n", "1.5.47"},
		{"None\n1.5.47\n", "1.5.47"},
		{"None\n", ""},
		{"unknown\n", ""},
		{"llm, version 0.37.1\n", "llm, version 0.37.1"},
		{"k8sgpt: 0.4.41 (3644ecf)\n", "k8sgpt: 0.4.41 (3644ecf)"},
		{"ShellGPT 1.5.1\n", "ShellGPT 1.5.1"},
		{"\n\n  \n", ""},
	}
	for _, k := range kasus {
		if got := ekstrakVersi(k.in); got != k.mau {
			t.Errorf("ekstrakVersi(%q) = %q, mau %q", k.in, got, k.mau)
		}
	}
}

// TestKeyEnvMelewatiToolTidakDidukung membuktikan sisi key.env dari
// kejujuran katalog: kunci warisan milik tool yang tidak didukung
// gateway (atau sudah dicabut) tidak lagi disumbangkan — termasuk ke
// nama generik — sementara tool yang didukung tidak berubah.
func TestKeyEnvMelewatiToolTidakDidukung(t *testing.T) {
	lama := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	baru := lama.Add(time.Hour)
	perTool := map[string]kunciTersimpan{
		"claude": {kunci: "kunci-claude", diperbarui: lama},
		"ollama": {kunci: "kunci-ollama", diperbarui: baru}, // terbaru, tetapi tidak didukung
		"mentat": {kunci: "kunci-mentat", diperbarui: baru}, // sudah dicabut dari registri
	}
	konten, err := susunKontenKeyEnv(perTool)
	if err != nil {
		t.Fatalf("susunKontenKeyEnv: %v", err)
	}
	if strings.Contains(konten, "kunci-ollama") || strings.Contains(konten, "kunci-mentat") {
		t.Errorf("kunci tool tidak didukung/dicabut masih disumbangkan:\n%s", konten)
	}
	if !strings.Contains(konten, "export ANTHROPIC_API_KEY='kunci-claude'") {
		t.Errorf("kunci claude harus tetap terisi:\n%s", konten)
	}
	// Nama generik mengikuti tool DIDUKUNG terbaru (claude), bukan
	// tool tidak didukung yang kebetulan lebih baru.
	if !strings.Contains(konten, "export ROUTEX_API_KEY='kunci-claude'") {
		t.Errorf("nama generik harus mengikuti claude:\n%s", konten)
	}
	// Keputusan desain B-117 dinyatakan di header berkas itu sendiri.
	if !strings.Contains(konten, "B-117") {
		t.Errorf("header key.env harus mencatat keputusan B-117:\n%s", konten)
	}
}
