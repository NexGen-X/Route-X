package cliconfig

// SupportedTools mengembalikan daftar seluruh perkakas AI CLI yang didukung Route-X.
// Seluruh perkakas secara default mengarah ke model kanonik riil yang terdaftar di katalog sistem.
//
// Perombakan registri Tahap 1: registri dipangkas hanya ke tool yang
// terbukti dapat diarahkan ke gateway Route-X, lalu diisi tool baru
// yang format konfigurasinya terverifikasi dari dokumentasi resmi
// masing-masing. Registri final TEPAT 11 entri. Dua belas entri
// dicabut total: agy, cursor-agent, cody, dan plandex (sebelumnya
// berstatus tidak-didukung-gateway), ollama (protokol daemon Ollama
// tidak kompatibel), serta continue, sgpt, fabric, llm, opencommit,
// chatblade, dan k8sgpt (didukung pada rilis sebelumnya, kini keluar
// dari kriteria registri: hanya CLI terkemuka, stabil, dan
// berkomunitas besar yang dipertahankan/ditambah).
//
// Tahap 3 (B-153): uji live Tahap 2 membuktikan SATU pengecualian —
// codex mati pada permintaan pertamanya karena gateway belum
// mendukung streaming pada /v1/responses. Codex TETAP terdaftar
// (format konfigurasinya benar dan terbaca tool-nya), tetapi kini
// membawa UnsupportedReason SEMENTARA sehingga Configure-nya ditolak
// dengan jujur sampai dukungan streaming Responses tiba: registri
// kini 11 entri, 10 didukung + codex beralasan sementara.
func SupportedTools() []ToolDef {
	return []ToolDef{
		{
			ID:            "claude",
			Name:          "Claude Code",
			Category:      "Coding Agent",
			Description:   "Anthropic Claude Code CLI untuk analisis basis kode, refactoring, dan penyuntingan berkas.",
			BinaryNames:   []string{"claude", "claude-code"},
			ConfigPaths:   []string{".claude/settings.json", ".claude.json"},
			VersionArg:    "--version",
			EnvVarBaseURL: "ANTHROPIC_BASE_URL",
			EnvVarAPIKey:  "ANTHROPIC_API_KEY",
			EnvVarModel:   "ANTHROPIC_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "claude-sonnet-5",
		},
		{
			ID:            "opencode",
			Name:          "OpenCode CLI",
			Category:      "Coding Agent",
			Description:   "Terminal coding assistant opensource dengan dukungan multi-provider model AI.",
			BinaryNames:   []string{"opencode", "opencode-cli"},
			ConfigPaths:   []string{".config/opencode/opencode.json"},
			VersionArg:    "--version",
			EnvVarBaseURL: "OPENCODE_API_BASE",
			EnvVarAPIKey:  "OPENCODE_API_KEY",
			EnvVarModel:   "OPENCODE_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			ID:            "aider",
			Name:          "Aider",
			Category:      "Coding Agent",
			Description:   "AI pair programming tool di terminal yang langsung menyunting dan commit kode ke git.",
			BinaryNames:   []string{"aider"},
			ConfigPaths:   []string{".aider.conf.yml"},
			VersionArg:    "--version",
			EnvVarBaseURL: "OPENAI_API_BASE",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "AIDER_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			ID:          "goose",
			Name:        "Goose Agent",
			Category:    "Developer Agent",
			Description: "Developer agent open-source dari Block untuk otomatisasi pekerjaan rekayasa perangkat lunak.",
			BinaryNames: []string{"goose"},
			ConfigPaths: []string{".config/goose/config.yaml"},
			// B-100: goose menolak subcommand `version`
			// ("unrecognized subcommand"); yang sah adalah --version.
			VersionArg: "--version",
			// B-099: Goose membaca provider openai lewat OPENAI_HOST
			// (tanpa sufiks /v1) dan OPENAI_API_KEY. Nama lama
			// GOOSE_OPENAI_HOST/GOOSE_API_KEY tidak dibaca Goose sama
			// sekali dan tidak lagi diekspor.
			EnvVarBaseURL: "OPENAI_HOST",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "GOOSE_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			ID:          "copilot",
			Name:        "GitHub Copilot CLI",
			Category:    "Terminal Assistant",
			Description: "Asisten CLI resmi GitHub untuk penjelasan perintah shell dan git command helper.",
			BinaryNames: []string{"github-copilot-cli", "copilot"},
			// B-128: CLI generasi kini memakai $COPILOT_HOME (bawaan
			// $HOME/.copilot), bukan hosts.json generasi lama.
			// B-219: ConfigPaths SENGAJA kosong — Configure copilot
			// murni env (BYOK di bawah) dan produk tidak menulis/
			// mengelola berkas apa pun di $COPILOT_HOME; mendaftarkan
			// ".copilot" di sini membuat Scan/Configure melaporkan
			// DIREKTORI itu sebagai config_file/config_path, padahal
			// cariConfigPath hanya memeriksa keberadaan jalur, bukan
			// berkas konfigurasi yang benar-benar dikelola.
			VersionArg: "--version",
			// B-128: jalur resmi ke gateway adalah mode BYOK
			// COPILOT_PROVIDER_* (dari `copilot help environment`);
			// GITHUB_COPILOT_* warisan adalah rekaan yang tidak dibaca
			// tool ("No authentication information found").
			EnvVarBaseURL: "COPILOT_PROVIDER_BASE_URL",
			EnvVarAPIKey:  "COPILOT_PROVIDER_API_KEY",
			EnvVarModel:   "COPILOT_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			// OpenAI Codex CLI (openai/codex). Format resmi dari docs
			// konfigurasi Codex (developers.openai.com/codex/config-reference):
			// config.toml memuat model_provider + [model_providers.<id>]
			// dengan base_url, env_key, dan wire_api "responses"
			// (satu-satunya wire API yang didukung versi kini; chat
			// completions sudah tidak didukung). Codex memanggil POST
			// {base_url}/responses dan membaca kunci dari variabel
			// lingkungan bernama env_key — produk menulis kunci per
			// tool ini ke key.env di bawah OPENAI_API_KEY (nama env
			// resminya untuk provider kustom di dokumentasi Codex).
			ID:          "codex",
			Name:        "OpenAI Codex CLI",
			Category:    "Coding Agent",
			Description: "CLI agentic coding resmi OpenAI yang berjalan lokal di terminal.",
			BinaryNames: []string{"codex"},
			ConfigPaths: []string{".codex/config.toml"},
			VersionArg:  "--version",
			// Konfigurasi hidup di config.toml; satu-satunya jalur env
			// adalah kunci lewat env_key (OPENAI_API_KEY di key.env).
			EnvVarAPIKey: "OPENAI_API_KEY",
			// B-153 (SEMENTARA, Tahap 3): uji live Tahap 2 membuktikan
			// Configure di bawah ini BENAR dan terbaca codex, tetapi
			// permintaan pertamanya mati 400 streaming_not_supported —
			// codex hanya berbicara Responses API streaming sedangkan
			// /v1/responses gateway baru mendukung non-streaming.
			// Selama itu, Configure DITOLAK dengan alasan ini daripada
			// mengklaim didukung untuk tool yang mati di permintaan
			// pertama. Penulis config.toml di manager.go SENGAJA
			// dipertahankan: begitu streaming Responses tiba, cukup
			// cabut alasan ini untuk menghidupkan codex kembali.
			UnsupportedReason: "Codex CLI hanya berbicara Responses API dengan streaming (stream: true), sedangkan /v1/responses gateway saat ini baru mendukung non-streaming, sehingga Codex belum dapat diarahkan ke gateway sampai dukungan streaming Responses tiba (B-153).",
			DefaultMode:       ModeModelOnly,
			DefaultTarget:     "gpt-5",
		},
		{
			// xAI Grok Build (xai-org/grok-build). Format model kustom
			// resmi dari user guide repo (11-custom-models.md):
			// ~/.grok/config.toml memuat [models] default = "<nama>"
			// dan [model.<nama>] dengan model, base_url, env_key.
			// Binernya `grok`; `grok --version` mencetak
			// "grok X.Y.Z (<hash>) [stable]".
			ID:          "grok-build",
			Name:        "Grok Build",
			Category:    "Coding Agent",
			Description: "CLI coding agent resmi xAI dengan mode TUI dan headless.",
			BinaryNames: []string{"grok"},
			ConfigPaths: []string{".grok/config.toml"},
			VersionArg:  "--version",
			// env_key TOML merujuk variabel yang memuat kunci; nama
			// XAI_API_KEY adalah nama resmi bawaan Grok Build untuk
			// kredensial xAI dan ditulis produk ke key.env.
			EnvVarAPIKey:  "XAI_API_KEY",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			// Qwen Code (QwenLM/qwen-code). Jalur resmi dari dokumentasi
			// (qwenlm.github.io/qwen-code-docs): env OPENAI_BASE_URL/
			// OPENAI_API_KEY/OPENAI_MODEL untuk provider OpenAI-
			// compatible, plus berkas ~/.qwen/settings.json dengan
			// modelProviders.openai dan security.auth.selectedType
			// "openai". Kedua jalur ditulis Configure.
			ID:            "qwen",
			Name:          "Qwen Code",
			Category:      "Coding Agent",
			Description:   "CLI agentic coding open-source dari tim Qwen (Alibaba) untuk terminal.",
			BinaryNames:   []string{"qwen"},
			ConfigPaths:   []string{".qwen/settings.json"},
			VersionArg:    "--version",
			EnvVarBaseURL: "OPENAI_BASE_URL",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "OPENAI_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			// Crush (charmbracelet/crush). Konfigurasi global resmi di
			// ~/.config/crush/crush.json (docs konfigurasi Crush):
			// providers.<id> bertipe "openai-compat" dengan base_url
			// dan api_key, dan models.large/small menunjuk provider
			// itu. Kunci hidup di berkas (izin 0600), bukan env.
			ID:            "crush",
			Name:          "Crush",
			Category:      "Coding Agent",
			Description:   "Coding agent terminal dari Charm dengan dukungan banyak provider model.",
			BinaryNames:   []string{"crush"},
			ConfigPaths:   []string{".config/crush/crush.json"},
			VersionArg:    "--version",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			// Hermes Agent (NousResearch/hermes-agent). Dokumentasi
			// resmi "Configuring Models" (hermes-agent.nousresearch.com):
			// blok model: di ~/.hermes/config.yaml dengan provider
			// "custom", default, base_url, api_mode; kunci untuk
			// provider kustom dibaca dari OPENAI_API_KEY di
			// ~/.hermes/.env. Hermes sengaja mengabaikan env
			// OPENAI_BASE_URL, jadi konfigurasi memang harus berkas.
			ID:          "hermes",
			Name:        "Hermes Agent",
			Category:    "Developer Agent",
			Description: "Agent otonom open-source dari Nous Research dengan perangkat terminal dan memori persisten. Catatan (B-154): jalan headless dapat meninggalkan daemon gateway latar yang keluar saat idle — matikan manual bila tidak dipakai lagi.",
			BinaryNames: []string{"hermes"},
			ConfigPaths: []string{".hermes/config.yaml"},
			VersionArg:  "--version",
			// Kunci ditulis ke ~/.hermes/.env oleh Configure dan juga
			// diekspor lewat key.env dengan nama yang sama.
			EnvVarAPIKey:  "OPENAI_API_KEY",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			// Cline CLI (cline/cline). CLI mandiri v3 dengan perintah
			// resmi `cline auth` (dokumentasi CLI di docs.cline.bot):
			// `cline auth -p openai -k <key> -b <url> -m <model>`
			// mengonfigurasi provider OpenAI-compatible; kredensial
			// disimpan CLI sendiri di ~/.cline/data/settings/.
			// Configure MENJALANKAN perintah resmi itu (bukan menulis
			// berkas internal cline yang spesifik-versi), dan gagal
			// jujur bila biner cline tidak terpasang.
			ID:          "cline",
			Name:        "Cline CLI",
			Category:    "Coding Agent",
			Description: "CLI coding agent open-source dari tim Cline untuk terminal dan CI. Konfigurasinya disimpan dan dikelola store internal CLI Cline sendiri lewat perintah resmi `cline auth` — status konfigurasinya dikelola tool itu, bukan berkas Route-X (B-149). Catatan (B-154): jalan headless dapat meninggalkan daemon hub latar yang perlu dimatikan operator.",
			BinaryNames: []string{"cline"},
			ConfigPaths: []string{".cline/data/settings/providers.json"},
			VersionArg:  "--version",
			// Tidak ada jalur env: kunci diserahkan ke store cline
			// sendiri lewat `cline auth`.
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
	}
}
