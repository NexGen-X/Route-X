package openai

import (
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// ulang adalah reader tak berujung yang mengisi buffer dengan satu byte yang
// sama. Dipakai untuk menguji batas body tanpa mengalokasikan 32MB di awal —
// io.ReadAll berhenti sendiri di batas LimitReader.
type ulang struct{ b byte }

func (u ulang) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = u.b
	}
	return len(p), nil
}

// chatPayload wajib menolak model kosong dengan InvalidRequest, sebelum ada
// byte yang terkirim ke upstream.
func TestChatPayloadMenolakModelKosong(t *testing.T) {
	p, err := New(Config{
		Name:       "uji",
		Kind:       providers.KindOpenAI,
		BaseURL:    "https://api.openai.com",
		Credential: security.Secret(testKey),
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	for _, model := range []string{"", "   "} {
		req := &providers.ChatRequest{
			Model:    model,
			Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
		}
		if _, perr := p.chatPayload(req, false); perr == nil {
			t.Errorf("model %q: chatPayload tidak error", model)
		} else if perr.Kind != providers.ErrKindInvalidRequest {
			t.Errorf("model %q: kind = %s, mau %s", model, perr.Kind, providers.ErrKindInvalidRequest)
		}
		if _, perr := p.chatPayload(req, true); perr == nil || perr.Kind != providers.ErrKindInvalidRequest {
			t.Errorf("model %q (streaming): kind = %v, mau invalid_request", model, perr)
		}
	}
}

// New wajib memvalidasi base URL lewat penjaga SSRF: tanpa ini, konfigurasi
// dari basis data bisa mengarahkan gateway ke alamat internal.
func TestNewMenolakBaseURLInternal(t *testing.T) {
	_, err := New(Config{
		Name:       "uji",
		Kind:       providers.KindOpenAI,
		BaseURL:    "http://169.254.169.254/",
		Credential: security.Secret(testKey),
		// Kebijakan nol = paling ketat: hanya https, tanpa alamat internal.
	})
	if err == nil {
		t.Fatal("New() menerima base URL metadata internal tanpa error")
	}
	// Skema non-https juga ditolak oleh kebijakan bawaan.
	_, err = New(Config{
		Name:       "uji",
		Kind:       providers.KindOpenAI,
		BaseURL:    "http://api.openai.com",
		Credential: security.Secret(testKey),
	})
	if err == nil {
		t.Fatal("New() menerima base URL http tanpa error")
	}
}

// readBody wajib memakai LimitReader(maxResponseBytes+1) dan mengembalikan
// error eksplisit begitu body melewati batas — bukan memotong diam-diam.
func TestReadBodyMenolakMelebihiBatas(t *testing.T) {
	if _, err := readBody("uji", ulang{b: 'x'}); err == nil {
		t.Fatal("readBody tidak error untuk body melebihi batas")
	} else if !strings.Contains(err.Message, "melebihi batas") {
		t.Errorf("pesan error = %q, mau menyebut batas", err.Message)
	}
}
