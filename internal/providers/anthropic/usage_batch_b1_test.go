package anthropic

import (
	"encoding/json"
	"testing"
)

// Test regresi sisa B-011 di sisi adaptor: objek cache_creation pada
// usage Anthropic (ephemeral_1h/ephemeral_5m) diurai ke bentuk kanonik,
// pada jalur non-streaming maupun streaming.

// TestToCanonicalUsageRincianCacheCreation: respons non-streaming
// berincian TTL menghasilkan pecahan kanonik yang tepat, dan total tetap
// sama seperti sebelum rincian ada.
func TestToCanonicalUsageRincianCacheCreation(t *testing.T) {
	var u upstreamUsage
	body := `{
		"input_tokens": 100,
		"output_tokens": 50,
		"cache_read_input_tokens": 200,
		"cache_creation_input_tokens": 1000,
		"cache_creation": {
			"ephemeral_1h_input_tokens": 400,
			"ephemeral_5m_input_tokens": 600
		}
	}`
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatalf("unmarshal usage: %v", err)
	}

	got := toCanonicalUsage(u)
	if got.CacheCreationInputTokens != 1000 {
		t.Errorf("CacheCreationInputTokens = %d, mau 1000", got.CacheCreationInputTokens)
	}
	if got.CacheCreation1hInputTokens != 400 {
		t.Errorf("CacheCreation1hInputTokens = %d, mau 400", got.CacheCreation1hInputTokens)
	}
	if got.CacheCreation5mInputTokens != 600 {
		t.Errorf("CacheCreation5mInputTokens = %d, mau 600", got.CacheCreation5mInputTokens)
	}
	if got.InputTokens != 1300 || got.CachedInputTokens != 200 || got.TotalTokens != 1350 {
		t.Errorf("kanonik = %+v, mau input 1300 cached 200 total 1350", got)
	}
}

// TestToCanonicalUsageTanpaRincianCacheCreation: tanpa objek
// cache_creation pecahan tetap nol — penagihan lalu menerapkan tarif
// 5 menit ke seluruh creation, persis perilaku lama.
func TestToCanonicalUsageTanpaRincianCacheCreation(t *testing.T) {
	got := toCanonicalUsage(upstreamUsage{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 1000,
	})
	if got.CacheCreation1hInputTokens != 0 || got.CacheCreation5mInputTokens != 0 {
		t.Errorf("pecahan = %d/%d, mau 0/0",
			got.CacheCreation1hInputTokens, got.CacheCreation5mInputTokens)
	}
	if got.CacheCreationInputTokens != 1000 {
		t.Errorf("CacheCreationInputTokens = %d, mau 1000", got.CacheCreationInputTokens)
	}
}

// TestStreamMergeUsageRincianCacheCreation: pada streaming, rincian
// datang di message_start dan message_delta hanya membawa output —
// penggabungan tidak boleh mengosongkan rincian yang sudah terkumpul.
func TestStreamMergeUsageRincianCacheCreation(t *testing.T) {
	s := &stream{}
	s.mergeUsage(&upstreamUsage{
		InputTokens:              100,
		CacheCreationInputTokens: 1000,
		CacheCreation:            &rincianCacheCreation{Ephemeral1hInputTokens: 400, Ephemeral5mInputTokens: 600},
	})
	s.mergeUsage(&upstreamUsage{OutputTokens: 50})

	got := toCanonicalUsage(s.usage)
	if got.CacheCreation1hInputTokens != 400 || got.CacheCreation5mInputTokens != 600 {
		t.Errorf("pecahan hasil gabungan = %d/%d, mau 400/600",
			got.CacheCreation1hInputTokens, got.CacheCreation5mInputTokens)
	}
	if got.CacheCreationInputTokens != 1000 || got.OutputTokens != 50 {
		t.Errorf("gabungan = creation %d output %d, mau 1000/50",
			got.CacheCreationInputTokens, got.OutputTokens)
	}
}
