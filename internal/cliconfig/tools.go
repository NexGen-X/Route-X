package cliconfig

// SupportedTools mengembalikan daftar seluruh perkakas AI CLI yang didukung Route-X.
// Seluruh perkakas secara default mengarah ke model kanonik riil yang terdaftar di katalog sistem.
//
// Perombakan registri Tahap 1: registri dipangkas hanya ke tool yang
// terbukti dapat diarahkan ke gateway Route-X. Dua belas entri dicabut
// total: agy, cursor-agent, cody, dan plandex (sebelumnya berstatus
// tidak-didukung-gateway), ollama (protokol daemon Ollama tidak
// kompatibel), serta continue, sgpt, fabric, llm, opencommit,
// chatblade, dan k8sgpt (didukung pada rilis sebelumnya, kini keluar
// dari kriteria registri: hanya CLI terkemuka, stabil, dan
// berkomunitas besar yang dipertahankan/ditambah).
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
	}
}
