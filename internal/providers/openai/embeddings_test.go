package openai

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// base64Vector menyusun vektor dalam bentuk yang dipakai encoding_format "base64":
// float32 little-endian yang dikodekan base64.
func base64Vector(t *testing.T, values ...float32) string {
	t.Helper()
	buf := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func TestEmbeddingsFloatFormat(t *testing.T) {
	const body = `{
  "object": "list",
  "model": "text-embedding-3-small",
  "data": [
    {"object": "embedding", "index": 0, "embedding": [0.5, -0.25, 0]},
    {"object": "embedding", "index": 1, "embedding": [1, 2]}
  ],
  "usage": {"prompt_tokens": 9, "total_tokens": 9},
  "field_masa_depan": true
}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	resp, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model:          "text-embedding-3-small",
		Input:          []string{"halo", "dunia"},
		Dimensions:     ptr(3),
		User:           "user-abc",
		EncodingFormat: "float",
	})
	if err != nil {
		t.Fatalf("Embeddings: %v", err)
	}

	got := u.last()
	if got.method != http.MethodPost || got.path != "/v1/embeddings" {
		t.Errorf("permintaan = %s %s", got.method, got.path)
	}
	reqBody := u.lastBody()
	if reqBody["model"] != "text-embedding-3-small" {
		t.Errorf("model = %v", reqBody["model"])
	}
	input, ok := reqBody["input"].([]any)
	if !ok || len(input) != 2 || input[1] != "dunia" {
		t.Errorf("input = %v", reqBody["input"])
	}
	if reqBody["dimensions"] != float64(3) {
		t.Errorf("dimensions = %v", reqBody["dimensions"])
	}
	if reqBody["user"] != "user-abc" {
		t.Errorf("user = %v", reqBody["user"])
	}
	if reqBody["encoding_format"] != "float" {
		t.Errorf("encoding_format = %v", reqBody["encoding_format"])
	}

	if resp.Model != "text-embedding-3-small" {
		t.Errorf("Model = %q", resp.Model)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("jumlah vektor = %d", len(resp.Data))
	}
	if want := []float32{0.5, -0.25, 0}; !equalVector(resp.Data[0].Vector, want) {
		t.Errorf("vektor 0 = %v, mau %v", resp.Data[0].Vector, want)
	}
	if resp.Data[1].Index != 1 {
		t.Errorf("index vektor 1 = %d", resp.Data[1].Index)
	}
	if resp.Usage.InputTokens != 9 || resp.Usage.TotalTokens != 9 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
	// Raw tetap body upstream apa adanya.
	if string(resp.Raw) != body {
		t.Errorf("Raw bukan body upstream apa adanya:\n%s", resp.Raw)
	}
}

func TestEmbeddingsBase64Format(t *testing.T) {
	first := base64Vector(t, 0.5, -0.25, 1.5)
	// Varian tanpa padding: dipakai sebagian implementasi compatible.
	second := strings.TrimRight(base64Vector(t, 2, 4), "=")

	body := `{"model":"m","data":[` +
		`{"index":0,"embedding":"` + first + `"},` +
		`{"index":1,"embedding":"` + second + `"}` +
		`],"usage":{"prompt_tokens":4,"total_tokens":4}}`

	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	resp, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model:          "m",
		Input:          []string{"a", "b"},
		EncodingFormat: "base64",
	})
	if err != nil {
		t.Fatalf("Embeddings: %v", err)
	}
	if u.lastBody()["encoding_format"] != "base64" {
		t.Errorf("encoding_format = %v", u.lastBody()["encoding_format"])
	}
	if want := []float32{0.5, -0.25, 1.5}; !equalVector(resp.Data[0].Vector, want) {
		t.Errorf("vektor 0 = %v, mau %v", resp.Data[0].Vector, want)
	}
	if want := []float32{2, 4}; !equalVector(resp.Data[1].Vector, want) {
		t.Errorf("vektor 1 = %v, mau %v", resp.Data[1].Vector, want)
	}
}

// encoding_format tidak dikirim bila pemanggil tidak memintanya: field itu tidak selalu
// dikenal provider compatible, dan adapter tidak membutuhkannya untuk menguraikan respons.
func TestEmbeddingsOmitsEncodingFormatWhenUnset(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"model":"m","data":[{"index":0,"embedding":[1]}]}`))
	p := testProvider(t, u.url(), nil)

	if _, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{Model: "m", Input: []string{"a"}}); err != nil {
		t.Fatalf("Embeddings: %v", err)
	}
	if _, ada := u.lastBody()["encoding_format"]; ada {
		t.Error("encoding_format terkirim padahal tidak diminta")
	}
}

// Bentuk vektor dikenali dari isi respons, bukan dari yang diminta: provider compatible
// kadang mengabaikan permintaan base64 dan tetap mengirim array float.
func TestEmbeddingsAcceptsFloatArrayDespiteBase64Request(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"model":"m","data":[{"index":0,"embedding":[3.5]}]}`))
	p := testProvider(t, u.url(), nil)

	resp, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model: "m", Input: []string{"a"}, EncodingFormat: "base64",
	})
	if err != nil {
		t.Fatalf("Embeddings: %v", err)
	}
	if want := []float32{3.5}; !equalVector(resp.Data[0].Vector, want) {
		t.Errorf("vektor = %v, mau %v", resp.Data[0].Vector, want)
	}
}

// Sebagian provider compatible tidak mengisi index. Tanpa penyesuaian, seluruh vektor
// menumpuk di index 0 dan pemanggil tidak bisa memasangkannya dengan input.
func TestEmbeddingsFillsMissingIndexFromOrder(t *testing.T) {
	u := newUpstream(t, jsonHandler(200,
		`{"model":"m","data":[{"embedding":[1]},{"embedding":[2]},{"embedding":[3]}]}`))
	p := testProvider(t, u.url(), nil)

	resp, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model: "m", Input: []string{"a", "b", "c"},
	})
	if err != nil {
		t.Fatalf("Embeddings: %v", err)
	}
	for i, item := range resp.Data {
		if item.Index != i {
			t.Errorf("Data[%d].Index = %d", i, item.Index)
		}
	}
}

func TestEmbeddingsRejectsUnreadableVector(t *testing.T) {
	tests := []struct{ name, body string }{
		{"base64 rusak", `{"model":"m","data":[{"index":0,"embedding":"@@bukan base64@@"}]}`},
		{"panjang bukan kelipatan 4", `{"model":"m","data":[{"index":0,"embedding":"YWJj"}]}`},
		{"bentuk tidak dikenali", `{"model":"m","data":[{"index":0,"embedding":{"nilai":1}}]}`},
		{"array bukan angka", `{"model":"m","data":[{"index":0,"embedding":["a","b"]}]}`},
		{"vektor kosong", `{"model":"m","data":[{"index":0}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(200, tc.body))
			p := testProvider(t, u.url(), nil)

			_, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{Model: "m", Input: []string{"a"}})
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error bukan *providers.Error: %v", err)
			}
			if e.Kind != providers.ErrKindServer {
				t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindServer)
			}
		})
	}
}

func TestEmbeddingsRejectsEmptyInput(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"model":"m","data":[{"index":0,"embedding":[1]}]}`))
	p := testProvider(t, u.url(), nil)
	ctx := context.Background()

	for _, req := range []*providers.EmbeddingsRequest{nil, {Model: "m"}} {
		if _, err := p.Embeddings(ctx, req); err == nil {
			t.Errorf("permintaan %+v diterima", req)
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.reqs) != 0 {
		t.Errorf("%d permintaan tetap dikirim ke upstream", len(u.reqs))
	}
}

// Respons 200 tanpa satu pun vektor tetap kegagalan, dan galat berstatus sukses harus
// terklasifikasi seperti galat biasa.
func TestEmbeddingsRejectsResponseWithoutData(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantKind providers.ErrorKind
	}{
		{"data kosong", `{"model":"m","data":[]}`, providers.ErrKindServer},
		{"galat berstatus 200", `{"error":{"message":"model tidak ada","code":"model_not_found"}}`, providers.ErrKindModelNotFound},
		{"bukan JSON", `bukan json sama sekali`, providers.ErrKindServer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(200, tc.body))
			p := testProvider(t, u.url(), nil)

			_, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{Model: "m", Input: []string{"a"}})
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error bukan *providers.Error: %v", err)
			}
			if e.Kind != tc.wantKind {
				t.Errorf("Kind = %s, mau %s", e.Kind, tc.wantKind)
			}
		})
	}
}

// equalVector membandingkan dua vektor float32 apa adanya; nilai di test dipilih agar bisa
// diwakili tepat oleh float32 sehingga tidak butuh toleransi.
func equalVector(got, want []float32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
