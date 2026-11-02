package gateway

import (
	"encoding/json"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/router"
)

// TestTeksDariRawJawaban adalah regresi audit3 V-GW-06: entri lama tanpa
// Teks harus tetap bisa disaring pada HIT lewat teks yang diekstrak dari
// Raw-nya — bukan melewati SaringJawaban sepenuhnya.
func TestTeksDariRawJawaban(t *testing.T) {
	kasus := []struct {
		nama string
		raw  string
		mau  string
	}{
		{
			nama: "konten string",
			raw:  `{"choices":[{"message":{"role":"assistant","content":"halo dunia"}}]}`,
			mau:  "halo dunia",
		},
		{
			nama: "konten bagian multimodal",
			raw:  `{"choices":[{"message":{"content":[{"type":"text","text":"satu "},{"type":"image_url","image_url":{"url":"x"}},{"type":"text","text":"dua"}]}}]}`,
			mau:  "satu dua",
		},
		{
			nama: "dua pilihan digabung baris baru",
			raw:  `{"choices":[{"message":{"content":"a"}},{"message":{"content":"b"}}]}`,
			mau:  "a\nb",
		},
		{
			nama: "tanpa teks (tool call saja)",
			raw:  `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"t1"}]}}]}`,
			mau:  "",
		},
		{nama: "json rusak", raw: `{"choices":`, mau: ""},
		{nama: "raw kosong", raw: ``, mau: ""},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			if got := teksDariRawJawaban(json.RawMessage(k.raw)); got != k.mau {
				t.Errorf("teksDariRawJawaban = %q, mau %q", got, k.mau)
			}
		})
	}
}

// TestPemakaianCacheHIT adalah regresi audit3 V-GW-09: pemakaian yang
// dicatat untuk HIT harus sama dengan yang ditagih — entri lama bertoken
// nol mendapat lantai taksiran di kedua sisi, entri ber-usage utuh tidak
// diubah.
func TestPemakaianCacheHIT(t *testing.T) {
	req := &providers.ChatRequest{
		Model:    "gpt-4o",
		Messages: []providers.Message{{Role: "user", Content: "jelaskan fotosintesis"}},
	}

	utuh := &responsecache.Entry{Usage: providers.Usage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18}}
	if got := pemakaianCacheHIT(req, utuh); got != utuh.Usage {
		t.Errorf("entri ber-usage utuh diubah: got %+v, mau %+v", got, utuh.Usage)
	}

	nol := &responsecache.Entry{}
	got := pemakaianCacheHIT(req, nol)
	est := EstimateTokens(req).InputTokens
	if est < 1 {
		t.Fatalf("taksiran prompt tidak positif: %d", est)
	}
	if got.InputTokens != est || got.TotalTokens != est {
		t.Errorf("entri nol: got %+v, mau InputTokens=TotalTokens=%d", got, est)
	}
}

// TestKandidatCacheHIT adalah regresi audit3 V-GW-09: baris requests
// untuk HIT menyebut provider asal jawaban — dari kandidat keputusan
// saat ini bila providernya masih terdaftar, dari identitas entri bila
// sudah tidak, dan nil bila entri warisan tidak membawa ProviderID.
func TestKandidatCacheHIT(t *testing.T) {
	kandidat := &upstream.RouteCandidate{
		ProviderID: "prov-1", ProviderName: "Penyedia Satu",
		ProviderModelID: "pm-1", UpstreamModelName: "gpt-4o-2024",
	}
	pr := &persiapan{decision: router.Decision{Candidates: []*upstream.RouteCandidate{
		{ProviderID: "prov-2", ProviderName: "Penyedia Dua"},
		kandidat,
	}}}

	if got := kandidatCacheHIT(pr, &responsecache.Entry{ProviderID: "prov-1", Model: "gpt-4o-2024"}); got != kandidat {
		t.Errorf("provider masih kandidat: got %+v, mau kandidat keputusan yang sama", got)
	}

	sintetis := kandidatCacheHIT(pr, &responsecache.Entry{ProviderID: "prov-lama", Model: "model-lama"})
	if sintetis == nil || sintetis.ProviderID != "prov-lama" || sintetis.UpstreamModelName != "model-lama" {
		t.Errorf("provider tak lagi kandidat: got %+v, mau kandidat sintetis dari entri", sintetis)
	}

	if got := kandidatCacheHIT(pr, &responsecache.Entry{}); got != nil {
		t.Errorf("entri tanpa ProviderID: got %+v, mau nil", got)
	}
}
