package cliconfig

// SupportedTools mengembalikan daftar seluruh perkakas AI CLI yang didukung Route-X.
// Seluruh perkakas secara default mengarah ke model kanonik riil yang terdaftar di katalog sistem.
func SupportedTools() []ToolDef {
	return []ToolDef{
		{
			ID:            "agy",
			Name:          "Antigravity CLI",
			Category:      "Coding Agent",
			Description:   "Google DeepMind Advanced Agentic Coding CLI untuk eksekusi tugas otonom dan pair-programming.",
			BinaryNames:   []string{"agy", "antigravity"},
			ConfigPaths:   []string{".gemini/antigravity-cli/config.json", ".config/agy/config.json"},
			VersionArg:    "--version",
			EnvVarBaseURL: "OPENAI_API_BASE",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "ANTIGRAVITY_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
			// B-135: agy wajib masuk dengan akun Google (Code Assist) dan
			// tidak membaca endpoint/kunci OpenAI apa pun. Tool-nya sah
			// dan kedudukannya setara tool lain (B-081) — yang dicabut
			// hanya klaim Configure-nya.
			UnsupportedReason: "Antigravity CLI (agy) wajib masuk dengan akun Google dan tidak membaca endpoint atau kunci OpenAI apa pun, sehingga tidak dapat diarahkan ke gateway Route-X.",
		},
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
			ID:            "ollama",
			Name:          "Ollama CLI",
			Category:      "Local LLM",
			Description:   "CLI runner model open-source lokal (Llama, DeepSeek, Qwen, Mistral).",
			BinaryNames:   []string{"ollama"},
			ConfigPaths:   []string{".ollama"},
			VersionArg:    "--version",
			EnvVarBaseURL: "OLLAMA_HOST",
			EnvVarAPIKey:  "OLLAMA_API_KEY",
			EnvVarModel:   "OLLAMA_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
			// B-136: CLI ollama hanya berbicara protokol Ollama
			// (/api/chat) ke daemon Ollama; gateway Route-X berbicara
			// protokol OpenAI. Tidak ada nilai Configure yang dapat
			// menjembatani keduanya.
			UnsupportedReason: "Ollama CLI hanya berbicara protokol Ollama ke daemon Ollama, sedangkan gateway Route-X berbicara protokol OpenAI — keduanya tidak kompatibel, sehingga Ollama tidak dapat diarahkan ke gateway.",
		},
		{
			ID:          "sgpt",
			Name:        "Shell GPT",
			Category:    "Terminal Assistant",
			Description: "Perkakas CLI bertenaga AI untuk generasi perintah shell, analisis log, dan obrolan terminal.",
			BinaryNames: []string{"sgpt"},
			ConfigPaths: []string{".config/shell_gpt/.sgptrc"},
			VersionArg:  "--version",
			// B-145: sgpt --version gugur saat import tanpa
			// OPENAI_API_KEY di env; deteksi versinya dijalankan
			// dengan kunci tersimpan sgpt bila ada.
			VersionNeedsStoredKey: true,
			// B-122: sgpt 1.5.x membaca base URL dari API_BASE_URL
			// (sgpt/config.py); OPENAI_BASE_URL warisan tidak dibaca
			// tool dan hanya tertolong fallback SDK tanpa sufiks /v1.
			EnvVarBaseURL: "API_BASE_URL",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "DEFAULT_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			ID:          "fabric",
			Name:        "Fabric AI",
			Category:    "Workflow & Prompts",
			Description: "Framework modular untuk mengotomatiskan prompting cerdas dan ekstraksi pola via terminal.",
			BinaryNames: []string{"fabric"},
			ConfigPaths: []string{".config/fabric/.env"},
			VersionArg:  "--version",
			// B-123: vendor OpenAI fabric membaca OPENAI_API_BASE_URL
			// (bawaan hardcoded api.openai.com); OPENAI_BASE_URL warisan
			// diabaikan sehingga kunci gateway terkirim ke OpenAI asli.
			EnvVarBaseURL: "OPENAI_API_BASE_URL",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "DEFAULT_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			ID:          "continue",
			Name:        "Continue CLI",
			Category:    "Coding Agent",
			Description: "Ekstensi dan CLI pendamping coding modular open-source untuk IDE dan shell.",
			// Biner CLI resmi Continue adalah `cn` (diagnostik Langkah 6
			// memakai cn 1.5.47); nama `continue` dipertahankan sebagai
			// cadangan deteksi.
			BinaryNames: []string{"cn", "continue"},
			ConfigPaths: []string{".continue/config.yaml", ".continue/config.json"},
			VersionArg:  "--version",
			// B-101: cn TIDAK membaca variabel khusus apa pun
			// (CONTINUE_API_BASE/API_KEY/MODEL adalah rekaan versi lama
			// yang tidak pernah dibaca tool). Konfigurasi Continue hidup
			// di ~/.continue/config.yaml; satu-satunya nama env yang
			// dikenal provider openai-nya adalah OPENAI_API_KEY, yang
			// menjadi nama kunci env tool ini.
			EnvVarBaseURL: "",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		{
			ID:          "llm",
			Name:        "SimonW LLM",
			Category:    "Terminal Assistant",
			Description: "CLI serbaguna Simon Willison untuk query model LLM, prompt templates, dan embeddings.",
			BinaryNames: []string{"llm"},
			ConfigPaths: []string{".config/io.datasette.llm/extra-openai-models.yaml", ".config/io.datasette.llm/default_model.txt"},
			VersionArg:  "--version",
			// B-124: llm tidak membaca OPENAI_API_BASE/LLM_DEFAULT_MODEL
			// untuk model gateway — model dikenal tool hanya lewat
			// extra-openai-models.yaml dan model bawaan hanya lewat
			// default_model.txt, keduanya ditulis
			// applyToolSpecificConfig. Satu-satunya nama env yang
			// berperan adalah OPENAI_API_KEY (api_key_name: openai).
			EnvVarBaseURL: "",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "",
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
			ID:          "opencommit",
			Name:        "OpenCommit",
			Category:    "Git Automation",
			Description: "Generator otomatis pesan commit konvensional git berbasis inferensi diff kode.",
			BinaryNames: []string{"opencommit", "oco"},
			ConfigPaths: []string{".opencommit"},
			VersionArg:  "--version",
			// B-125: opencommit v3 bermigrasi ke skema tunggal
			// OCO_AI_PROVIDER/OCO_API_KEY/OCO_API_URL; nama warisan
			// OCO_OPENAI_* tidak lagi dibaca dan tool gagal di migrasi
			// internalnya sendiri tanpa OCO_AI_PROVIDER.
			EnvVarBaseURL: "OCO_API_URL",
			EnvVarAPIKey:  "OCO_API_KEY",
			EnvVarModel:   "OCO_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		// B-133: entri "mentat" DICABUT dari registri — biner resmi
		// terakhir Mentat (1.0.19) hanyalah stub depresiasi yang
		// mencetak pemberitahuan lalu keluar (proyek CLI dihentikan);
		// tidak ada tool yang dapat dikonfigurasi.
		{
			ID:            "plandex",
			Name:          "Plandex",
			Category:      "Terminal Engine",
			Description:   "Terminal AI coding engine untuk tugas rekayasa berukuran besar dan multi-tahap.",
			BinaryNames:   []string{"plandex"},
			ConfigPaths:   []string{".plandex/config.json"},
			VersionArg:    "version",
			EnvVarBaseURL: "PLANDEX_OPENAI_API_HOST",
			EnvVarAPIKey:  "PLANDEX_OPENAI_API_KEY",
			EnvVarModel:   "PLANDEX_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
			// B-134: Plandex adalah klien tipis yang wajib terhubung
			// ke server Plandex (Cloud berakun atau self-hosted
			// terpisah); tidak ada konfigurasi env yang mengarahkannya
			// langsung ke gateway.
			UnsupportedReason: "Plandex wajib terhubung ke server Plandex (Cloud berakun atau self-hosted terpisah) dan berhenti di pemilihan autentikasi sebelum panggilan model apa pun, sehingga tidak dapat diarahkan langsung ke gateway Route-X lewat Configure.",
		},
		{
			ID:          "chatblade",
			Name:        "Chatblade",
			Category:    "Terminal Assistant",
			Description: "Perkakas CLI cepat untuk interaksi dengan model bahasa dan ekstraksi output JSON.",
			BinaryNames: []string{"chatblade"},
			ConfigPaths: []string{".chatblade.yaml"},
			// B-145: VersionArg SENGAJA kosong. Pada build chatblade
			// yang terpasang & teruji live di Uji Live Putaran 1,
			// `--version` ditolak argparse ("unrecognized arguments:
			// --version") — definisi lama menjanjikan flag yang tidak
			// ada pada build itu, sehingga versinya selalu kosong
			// lewat jalur kegagalan. Keluaran --help build itu juga
			// tidak memuat nomor versi apa pun, dan tidak ada perintah
			// versi lain yang sah, jadi deteksi versi dicabut:
			// Version tampil kosong secara jujur (omitempty di API),
			// bukan hasil eksekusi yang pasti gagal.
			VersionArg: "",
			// B-126: chatblade membaca base URL dari OPENAI_BASE_URL
			// (SDK) dan model dari OPENAI_API_MODEL; OPENAI_API_BASE
			// dan CHATBLADE_MODEL warisan tidak dibaca tool.
			EnvVarBaseURL: "OPENAI_BASE_URL",
			EnvVarAPIKey:  "OPENAI_API_KEY",
			EnvVarModel:   "OPENAI_API_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		// B-127: entri "gpt-cli" DICABUT dari registri — identitasnya
		// tidak pernah teridentifikasi: profil registri (biner
		// gpt-cli/gpt, berkas gpt.yml) tidak cocok dengan satu pun dari
		// dua paket resmi bernama gpt-cli (npm maupun PyPI), sehingga
		// Configure-nya salah untuk kedua kandidat.
		{
			ID:            "cursor-agent",
			Name:          "Cursor Agent",
			Category:      "Coding Agent",
			Description:   "CLI agent untuk antarmuka pengembang Cursor AI dan otomatisasi remote workflow.",
			BinaryNames:   []string{"cursor-agent", "cursor"},
			ConfigPaths:   []string{".cursor/cli-config.json"},
			VersionArg:    "--version",
			EnvVarBaseURL: "CURSOR_API_BASE",
			EnvVarAPIKey:  "CURSOR_API_KEY",
			EnvVarModel:   "CURSOR_MODEL",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
			// B-130: Cursor Agent terikat cloud Cursor — endpoint-nya
			// proprietari (bukan protokol OpenAI) dan kuncinya adalah
			// kredensial akun Cursor; kunci gateway ditolak cloud
			// Cursor dan endpoint gateway tidak pernah dicapai.
			UnsupportedReason: "Cursor Agent terikat ke cloud Cursor: endpoint-nya memakai protokol proprietari Cursor dan kuncinya adalah kredensial akun Cursor, sehingga tidak dapat diarahkan ke gateway Route-X.",
		},
		{
			ID:          "copilot",
			Name:        "GitHub Copilot CLI",
			Category:    "Terminal Assistant",
			Description: "Asisten CLI resmi GitHub untuk penjelasan perintah shell dan git command helper.",
			BinaryNames: []string{"github-copilot-cli", "copilot"},
			// B-128: CLI generasi kini memakai $COPILOT_HOME (bawaan
			// $HOME/.copilot), bukan hosts.json generasi lama.
			ConfigPaths: []string{".copilot"},
			VersionArg:  "--version",
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
			ID:          "cody",
			Name:        "Sourcegraph Cody",
			Category:    "Coding Agent",
			Description: "CLI asisten kode cerdas Sourcegraph dengan pemahaman konteks grafik kode menyeluruh.",
			BinaryNames: []string{"cody"},
			// B-131: ConfigPaths warisan (.config/cody/config.json)
			// tidak dipakai CLI cody; entri dipertahankan tanpa jalur
			// rekaan. CODY_DEFAULT_MODEL juga rekaan (model hanya
			// lewat flag --model) dan dibuang.
			ConfigPaths:   []string{},
			VersionArg:    "--version",
			EnvVarBaseURL: "SRC_ENDPOINT",
			EnvVarAPIKey:  "SRC_ACCESS_TOKEN",
			EnvVarModel:   "",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "claude-sonnet-5",
			// B-131: SRC_ENDPOINT/SRC_ACCESS_TOKEN adalah kredensial
			// akun Sourcegraph ke instans Sourcegraph dan protokolnya
			// GraphQL Sourcegraph — batas vendor, bukan gateway.
			UnsupportedReason: "Cody terikat ke instans dan akun Sourcegraph (protokol GraphQL Sourcegraph, bukan protokol OpenAI), sehingga tidak dapat diarahkan ke gateway Route-X.",
		},
		{
			ID:          "k8sgpt",
			Name:        "K8sGPT",
			Category:    "DevOps & SRE",
			Description: "Pemindai cluster Kubernetes berbasis AI untuk diagnosis otomatis insiden & error log.",
			BinaryNames: []string{"k8sgpt"},
			// B-129: konfigurasi nyata k8sgpt 0.4.x adalah berkas
			// YAML ini (setara `k8sgpt auth add`); jalur .k8sgpt.yaml
			// adalah jalur versi lama.
			ConfigPaths: []string{".config/k8sgpt/k8sgpt.yaml"},
			VersionArg:  "version",
			// B-129: pola env K8SGPT_OPENAI* tidak ada di biner —
			// tidak ada nama env untuk base URL/kunci/model; semuanya
			// hidup di berkas YAML yang ditulis
			// applyToolSpecificConfig.
			EnvVarBaseURL: "",
			EnvVarAPIKey:  "",
			EnvVarModel:   "",
			DefaultMode:   ModeModelOnly,
			DefaultTarget: "gpt-5",
		},
		// B-132: entri "codecompanion" DICABUT dari registri —
		// CodeCompanion adalah plugin Neovim (codecompanion.nvim);
		// tidak ada CLI bernama itu yang diterbitkan, sehingga
		// seluruh env/ConfigPaths-nya adalah rekaan untuk program
		// yang tidak eksis.
	}
}
