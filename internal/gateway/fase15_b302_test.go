package gateway

import (
	"strings"
	"testing"
)

// TestB302FieldNDivalidasiDecoder adalah pengunci B-302: field Extra
// "n" dibaca gateway (EstimateTokens menjadikannya pengali estimasi
// keluaran) tetapi sebelumnya lolos decoder tanpa validasi tipe /
// rentang — persis rasional B-250 yang sudah diterapkan pada
// max_completion_tokens. Nilai mustahil kini ditolak di decoder
// dengan menyebut field-nya; nilai sah tetap diteruskan utuh.
func TestB302FieldNDivalidasiDecoder(t *testing.T) {
	dasar := `"model":"gpt-4o-mini","messages":[{"role":"user","content":"halo"}]`
	for _, mentah := range []string{`-5`, `0`, `"banyak"`, `1.5`} {
		body := []byte(`{` + dasar + `,"n":` + mentah + `}`)
		if _, err := DecodeChatRequest(body); err == nil {
			t.Errorf("n=%s: mau ditolak decoder, lolos", mentah)
		} else if !strings.Contains(err.Error(), `"n"`) {
			t.Errorf("n=%s: pesan tidak menyebut field n: %v", mentah, err)
		}
	}
	req, err := DecodeChatRequest([]byte(`{` + dasar + `,"n":3}`))
	if err != nil {
		t.Fatalf("n=3 sah harus diterima: %v", err)
	}
	if _, ok := req.Extra["n"]; !ok {
		t.Errorf("n=3 sah harus tetap diteruskan di Extra")
	}
}
