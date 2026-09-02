package seed

import (
	"context"
	"fmt"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Katalog model bawaan.
//
// Yang ditanam di sini hanya fakta yang stabil: pengenal model, nama tampilan,
// kemampuan, dan ukuran jendela konteks. HARGA TIDAK ditanam bersama katalog, dan itu
// keputusan yang disengaja:
//
//  1. Secara struktural harga menempel pada pasangan model-provider (`model_pricing` →
//     `provider_models`), sedangkan provider dibuat operator beserta kredensialnya
//     sendiri. Menanam harga karena itu menuntut menanam provider palsu lebih dulu.
//  2. Harga berubah, dan harga yang salah pada produk pelacak biaya menghasilkan
//     laporan yang salah tanpa satu pun peringatan. Angka yang tidak bisa
//     dipertanggungjawabkan lebih buruk daripada kolom yang jelas-jelas kosong.
//
// Sebagai gantinya, `ReferencePrices` di bawah menyediakan harga acuan berikut asalnya,
// yang ditawarkan API admin sebagai nilai awal saat operator memetakan model ke
// provider. Operator tetap yang memutuskan dan mengonfirmasi.

// Model adalah satu entri katalog.
type CatalogModel struct {
	ModelID         string
	DisplayName     string
	Family          string
	ContextWindow   int
	MaxOutputTokens int
	Capabilities    []string
	Aliases         []string
}

// catalogModels adalah model yang dikenali gateway sejak awal.
//
// Daftar ini bukan pembatas: operator bisa menambah model apa pun dari dashboard.
// Isinya sekadar titik mulai supaya instalasi baru tidak menghadapi registry kosong.
//
// Catatan soal ContextWindow dan MaxOutputTokens: ini nilai awal yang perlu
// dikonfirmasi operator, tidak semuanya terverifikasi dari dokumentasi resmi. Keduanya
// sengaja diperlakukan berbeda dari harga — angka jendela konteks yang salah
// menghasilkan penolakan dari upstream, yaitu kegagalan yang terlihat dan langsung bisa
// diperbaiki, sementara harga yang salah menghasilkan laporan biaya yang salah tanpa
// satu pun tanda. Karena itu harga menuntut asal yang tercatat (lihat ReferencePrice)
// sedangkan angka di bawah cukup ditandai sebagai perkiraan awal.
var catalogModels = []CatalogModel{
	// --- Anthropic ---
	{
		ModelID: "claude-opus-5", DisplayName: "Claude Opus 5", Family: "claude",
		ContextWindow: 1_000_000, MaxOutputTokens: 64_000,
		Capabilities: []string{upstream.CapText, upstream.CapVision, upstream.CapReasoning, upstream.CapTools},
		Aliases:      []string{"claude-opus-latest"},
	},
	{
		ModelID: "claude-sonnet-5", DisplayName: "Claude Sonnet 5", Family: "claude",
		ContextWindow: 1_000_000, MaxOutputTokens: 64_000,
		Capabilities: []string{upstream.CapText, upstream.CapVision, upstream.CapReasoning, upstream.CapTools},
		Aliases:      []string{"claude-sonnet-latest"},
	},
	{
		ModelID: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5", Family: "claude",
		ContextWindow: 200_000, MaxOutputTokens: 64_000,
		Capabilities: []string{upstream.CapText, upstream.CapVision, upstream.CapTools},
		Aliases:      []string{"claude-haiku-latest"},
	},

	// --- OpenAI ---
	{
		ModelID: "gpt-5", DisplayName: "GPT-5", Family: "gpt",
		ContextWindow: 400_000, MaxOutputTokens: 128_000,
		Capabilities: []string{upstream.CapText, upstream.CapVision, upstream.CapReasoning, upstream.CapTools},
	},
	{
		ModelID: "text-embedding-3-large", DisplayName: "Text Embedding 3 Large", Family: "embedding",
		ContextWindow: 8_191,
		Capabilities:  []string{upstream.CapEmbeddings},
	},

	// --- Google ---
	{
		ModelID: "gemini-2-5-pro", DisplayName: "Gemini 2.5 Pro", Family: "gemini",
		ContextWindow: 1_048_576, MaxOutputTokens: 65_536,
		Capabilities: []string{upstream.CapText, upstream.CapVision, upstream.CapReasoning, upstream.CapTools},
	},
}

// ReferencePrice adalah harga acuan untuk satu model di satu jenis provider, beserta
// asal angkanya.
//
// Nilai USD memakai satuan 10⁻⁸ dolar (lihat upstream.USD). Field Source dan FetchedOn
// wajib terisi: harga tanpa asal tidak boleh ditawarkan sebagai default, karena operator
// tidak punya cara memeriksanya.
type ReferencePrice struct {
	ProviderKind string
	// UpstreamModelName adalah nama model di sisi provider, yang bisa berbeda dari
	// pengenal kanonik di katalog.
	UpstreamModelName string
	Input             upstream.USD
	Output            upstream.USD
	// CachedInput nil berarti provider tidak menagih cache read terpisah.
	CachedInput *upstream.USD
	Source      string
	FetchedOn   string
}

// referencePrices memuat harga yang bisa dipertanggungjawabkan asalnya.
//
// Hanya Anthropic yang terisi karena hanya tabel harganya yang berhasil saya baca
// langsung dari halaman resmi. Halaman harga OpenAI menolak pengambilan otomatis (HTTP
// 403), dan sumber pihak ketiga saling bertentangan — pada model Claude Sonnet 5 saja,
// dua agregator menyebut $3/$15 sementara halaman resmi menyatakan $2/$10 dan
// menjelaskan bahwa kenaikan ke $3/$15 yang dijadwalkan 1 September 2026 dibatalkan.
// Persis kesalahan yang akan diam-diam merusak laporan biaya kalau ditanam.
//
// Model tanpa entri di sini akan tampil "harga belum diatur" di dashboard, dan biayanya
// dilaporkan nol — terlihat jelas, bukan salah diam-diam.
var referencePrices = []ReferencePrice{
	{
		ProviderKind: upstream.KindAnthropic, UpstreamModelName: "claude-opus-5",
		Input: upstream.MustParseUSD("5"), Output: upstream.MustParseUSD("25"),
		CachedInput: usd("0.50"),
		Source:      "platform.claude.com/docs/en/about-claude/pricing", FetchedOn: "2026-09-02",
	},
	{
		ProviderKind: upstream.KindAnthropic, UpstreamModelName: "claude-sonnet-5",
		Input: upstream.MustParseUSD("2"), Output: upstream.MustParseUSD("10"),
		CachedInput: usd("0.20"),
		Source:      "platform.claude.com/docs/en/about-claude/pricing", FetchedOn: "2026-09-02",
	},
	{
		ProviderKind: upstream.KindAnthropic, UpstreamModelName: "claude-haiku-4-5",
		Input: upstream.MustParseUSD("1"), Output: upstream.MustParseUSD("5"),
		CachedInput: usd("0.10"),
		Source:      "platform.claude.com/docs/en/about-claude/pricing", FetchedOn: "2026-09-02",
	},
}

func usd(s string) *upstream.USD {
	v := upstream.MustParseUSD(s)
	return &v
}

// ReferencePriceFor mencari harga acuan untuk satu pasangan jenis provider dan nama
// model upstream. Kembalian nil berarti tidak ada acuan yang bisa dipertanggungjawabkan,
// dan API admin harus meminta operator mengisinya sendiri.
func ReferencePriceFor(providerKind, upstreamModelName string) *ReferencePrice {
	for i := range referencePrices {
		if referencePrices[i].ProviderKind == providerKind &&
			referencePrices[i].UpstreamModelName == upstreamModelName {
			return &referencePrices[i]
		}
	}
	return nil
}

// ReferencePrices mengembalikan seluruh harga acuan, untuk ditampilkan di dashboard.
func ReferencePrices() []ReferencePrice {
	out := make([]ReferencePrice, len(referencePrices))
	copy(out, referencePrices)
	return out
}

// CatalogModels mengembalikan katalog bawaan.
func CatalogModels() []CatalogModel {
	out := make([]CatalogModel, len(catalogModels))
	copy(out, catalogModels)
	return out
}

// CatalogResult melaporkan hasil penanaman katalog.
type CatalogResult struct {
	ModelsCreated  int
	ModelsExisting int
	AliasesCreated int
}

// SeedCatalog menanam katalog model bawaan.
//
// Idempoten dan tidak merusak: model yang sudah ada dibiarkan apa adanya, karena
// operator mungkin sudah menyesuaikan kemampuan atau prioritasnya. Hanya model yang
// belum ada yang dibuat.
func SeedCatalog(ctx context.Context, q repo.Querier) (CatalogResult, error) {
	var res CatalogResult
	models := upstream.NewModelRepo(q)

	for _, cm := range catalogModels {
		existing, err := models.GetByModelID(ctx, cm.ModelID)
		if err == nil {
			res.ModelsExisting++
			_ = existing
			continue
		}
		if !isNotFound(err) {
			return res, fmt.Errorf("memeriksa model %q: %w", cm.ModelID, err)
		}

		created, err := models.Create(ctx, upstream.CreateModelParams{
			ModelID:         cm.ModelID,
			DisplayName:     cm.DisplayName,
			Family:          strPtr(cm.Family),
			ContextWindow:   intPtr(cm.ContextWindow),
			MaxOutputTokens: intPtr(cm.MaxOutputTokens),
			Capabilities:    cm.Capabilities,
		})
		if err != nil {
			return res, fmt.Errorf("membuat model %q: %w", cm.ModelID, err)
		}
		res.ModelsCreated++

		for _, alias := range cm.Aliases {
			if _, err := models.AddAlias(ctx, created.ID, alias, nil); err != nil {
				// Alias yang sudah dipakai model lain bukan alasan menggagalkan seluruh
				// penanaman katalog; model utamanya sudah ada dan bisa dipakai.
				if isConflict(err) {
					continue
				}
				return res, fmt.Errorf("menambah alias %q: %w", alias, err)
			}
			res.AliasesCreated++
		}
	}
	return res, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}
