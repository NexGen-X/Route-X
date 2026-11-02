package openai

// Test regresi audit3 grup 3 untuk adapter streaming OpenAI (B-016):
// V-GW-13 (EOF telanjang bukan sukses) dan V-GW-12 (objek error di dalam
// aliran adalah kegagalan, bukan chunk).

import (
	"errors"
	"io"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// EOF tanpa [DONE] dan tanpa chunk finish_reason, SESUDAH sebagian konten
// terkirim: aliran terpotong, terklasifikasi stream_aborted dengan
// StreamedBytes terisi supaya tidak dialihkan.
func TestStreamEOFTelanjangSetelahKontenAdalahTerpotong(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkHa, chunkLo))
	p := testProvider(t, u.url(), func(c *Config) { c.Kind = providers.KindOpenAI })
	s := openStream(t, p, chatRequest())

	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv 1: %v", err)
	}
	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv 2: %v", err)
	}
	_, err := s.Recv()
	if errors.Is(err, io.EOF) {
		t.Fatal("EOF telanjang diterima sebagai selesai bersih — bug V-GW-13")
	}
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindStreamAborted {
		t.Errorf("Kind = %q, mau stream_aborted", pe.Kind)
	}
	if pe.StreamedBytes == 0 {
		t.Error("StreamedBytes = 0, mau > 0 — konten sudah terkirim, failover harus tertahan")
	}
}

// EOF telanjang sebelum satu pun chunk: kegagalan server yang masih boleh
// dialihkan (StreamedBytes nol).
func TestStreamEOFTelanjangTanpaKontenAdalahKegagalanServer(t *testing.T) {
	u := newUpstream(t, sseHandler())
	p := testProvider(t, u.url(), func(c *Config) { c.Kind = providers.KindOpenAI })
	s := openStream(t, p, chatRequest())

	_, err := s.Recv()
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, mau *providers.Error (bukan EOF bersih)", err)
	}
	if pe.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %q, mau server", pe.Kind)
	}
	if pe.StreamedBytes != 0 {
		t.Errorf("StreamedBytes = %d, mau 0", pe.StreamedBytes)
	}
}

// Objek error sebagai peristiwa pertama aliran (pola overloaded_error
// Anthropic/OpenAI) harus muncul sebagai kegagalan terklasifikasi dari
// Recv, bukan diteruskan sebagai chunk kosong ke klien.
func TestStreamObjekErrorPertamaMenjadiKegagalan(t *testing.T) {
	u := newUpstream(t, sseHandler(
		`{"error":{"message":"Overloaded","type":"overloaded_error","code":"overloaded"}}`,
	))
	p := testProvider(t, u.url(), func(c *Config) { c.Kind = providers.KindOpenAI })
	s := openStream(t, p, chatRequest())

	_, err := s.Recv()
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindOverloaded {
		t.Errorf("Kind = %q, mau overloaded — klasifikasi ini yang membuat failover V-GW-12 mungkin", pe.Kind)
	}
	if pe.StreamedBytes != 0 {
		t.Errorf("StreamedBytes = %d, mau 0", pe.StreamedBytes)
	}
	// Pesan mentah upstream tidak boleh bocor apa adanya ke Message.
	if pe.Message == "Overloaded" {
		t.Error("Message memuat pesan mentah upstream tanpa sanitasi")
	}
}

// Objek error di TENGAH aliran: konten yang sudah terkirim menahan failover
// lewat StreamedBytes, klasifikasinya tetap terbawa.
func TestStreamObjekErrorDiTengahAliran(t *testing.T) {
	u := newUpstream(t, sseHandler(
		chunkHa,
		`{"error":{"message":"rate limited","type":"rate_limit_error","code":"rate_limit"}}`,
	))
	p := testProvider(t, u.url(), func(c *Config) { c.Kind = providers.KindOpenAI })
	s := openStream(t, p, chatRequest())

	if _, err := s.Recv(); err != nil {
		t.Fatalf("Recv 1: %v", err)
	}
	_, err := s.Recv()
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %q, mau rate_limit", pe.Kind)
	}
	if pe.StreamedBytes == 0 {
		t.Error("StreamedBytes = 0 — kegagalan tengah aliran akan keliru dialihkan")
	}
	if pe.Failoverable() {
		t.Error("Failoverable() = true padahal sebagian jawaban sudah terkirim")
	}
}
