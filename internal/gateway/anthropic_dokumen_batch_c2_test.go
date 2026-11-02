package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi Batch C2: dukungan block document di permukaan Anthropic.
//
// PDF base64 hanya boleh tiba di upstream Anthropic; ke upstream lain ia
// ditolak 400 SEBELUM jaringan dengan kode unsupported_document_upstream.
// Dokumen teks diinjeksi sebagai teks dan boleh dilayani upstream mana pun.

const bodyMessagesPDF = `{"model":"gpt-4o-mini","max_tokens":128,"messages":[{"role":"user","content":[
	{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0xLjQ="}},
	{"type":"text","text":"ringkas dokumen ini"}
]}]}`

const bodyMessagesPDFStream = `{"model":"gpt-4o-mini","max_tokens":128,"stream":true,"messages":[{"role":"user","content":[
	{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0xLjQ="}},
	{"type":"text","text":"ringkas dokumen ini"}
]}]}`

const bodyMessagesDokumenTeks = `{"model":"gpt-4o-mini","max_tokens":128,"messages":[{"role":"user","content":[
	{"type":"text","text":"pertanyaan tentang lampiran:"},
	{"type":"document","source":{"type":"text","media_type":"text/plain","data":"ISI-LAMPIRAN-TEKS"}}
]}]}`

func TestDokumenPDFDitolakTanpaKandidatAnthropic(t *testing.T) {
	// Kandidat bawaan susun() berkind OpenAI: bukan Anthropic.
	prov := providerSukses()
	s := susun(t, prov, nil)

	for nama, body := range map[string]string{
		"non-streaming": bodyMessagesPDF,
		"streaming":     bodyMessagesPDFStream,
	} {
		w := s.panggil(t, http.MethodPost, "/v1/messages", body, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, mau 400 (body: %s)", nama, w.Code, w.Body.String())
		}
		var env struct {
			Type  string `json:"type"`
			Error struct {
				Type string `json:"type"`
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s: body bukan JSON: %v", nama, err)
		}
		if env.Type != "error" || env.Error.Type != "invalid_request_error" {
			t.Errorf("%s: envelope = %s, mau error invalid_request_error Anthropic", nama, w.Body.String())
		}
		if env.Error.Code != "unsupported_document_upstream" {
			t.Errorf("%s: error.code = %q, mau unsupported_document_upstream", nama, env.Error.Code)
		}
	}

	// Penolakan terjadi sebelum jaringan: pabrik adapter tidak pernah
	// dipanggil dan provider tidak menerima satu pun permintaan.
	if urutan := s.factory.urutan(); len(urutan) != 0 {
		t.Errorf("pabrik adapter dipanggil %v — penolakan harus mendahului eksekusi", urutan)
	}
	prov.mu.Lock()
	diterima := len(prov.diterima)
	prov.mu.Unlock()
	if diterima != 0 {
		t.Errorf("provider menerima %d permintaan, mau 0", diterima)
	}
}

func TestDokumenPDFDilayaniKandidatAnthropic(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindAnthropic,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID: "msg_uji",
				Choices: []providers.Choice{{
					Message:      providers.Message{Role: providers.RoleAssistant, Content: "ringkasan"},
					FinishReason: providers.FinishStop,
				}},
				Usage: providers.Usage{InputTokens: 10, OutputTokens: 3, TotalTokens: 13},
			}, nil
		},
	}
	s := susun(t, prov, func(su *susunan, _ *HandlersDeps) {
		su.cands.cands[0].Kind = providers.KindAnthropic
	})

	w := s.panggil(t, http.MethodPost, "/v1/messages", bodyMessagesPDF, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}

	// Dokumen tiba di upstream Anthropic utuh, pada posisi block-nya.
	diterima := prov.terakhir(t)
	if len(diterima.Messages) != 1 || len(diterima.Messages[0].Parts) != 2 {
		t.Fatalf("pesan diterima = %+v, mau 1 pesan 2 bagian", diterima.Messages)
	}
	dok := diterima.Messages[0].Parts[0]
	if dok.Type != providers.PartTypeDocument || dok.Document == nil ||
		dok.Document.MediaType != "application/pdf" || dok.Document.Data != "JVBERi0xLjQ=" {
		t.Errorf("bagian[0] = %+v, mau dokumen PDF utuh", dok)
	}
	if diterima.Messages[0].Parts[1].Text != "ringkas dokumen ini" {
		t.Errorf("bagian[1] = %+v, mau teks pendamping sesudah dokumen", diterima.Messages[0].Parts[1])
	}
}

func TestDokumenTeksDiinjeksiUntukUpstreamNonAnthropic(t *testing.T) {
	prov := providerSukses()
	s := susun(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/v1/messages", bodyMessagesDokumenTeks, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	diterima := prov.terakhir(t)
	teks := diterima.Messages[0].Text()
	if !strings.Contains(teks, "ISI-LAMPIRAN-TEKS") || !strings.Contains(teks, "pertanyaan tentang lampiran:") {
		t.Errorf("teks diterima upstream = %q, mau memuat pertanyaan dan isi lampiran teks", teks)
	}
	if strings.Index(teks, "pertanyaan tentang lampiran:") > strings.Index(teks, "ISI-LAMPIRAN-TEKS") {
		t.Errorf("urutan teks berubah: %q", teks)
	}
}

func TestEstimateTokensMenghitungDokumen(t *testing.T) {
	dasar := &providers.ChatRequest{Model: "m", Messages: []providers.Message{
		{Role: providers.RoleUser, Content: "halo"},
	}}
	berdokumen := &providers.ChatRequest{Model: "m", Messages: []providers.Message{
		{Role: providers.RoleUser, Parts: []providers.ContentPart{
			{Type: providers.PartTypeText, Text: "halo"},
			{Type: providers.PartTypeDocument, Document: &providers.Document{
				MediaType: "application/pdf",
				Data:      strings.Repeat("QUJD", 4000), // ±12.000 byte mentah
			}},
		}},
	}}
	if EstimateTokens(berdokumen).InputTokens <= EstimateTokens(dasar).InputTokens {
		t.Errorf("taksiran berdokumen (%d) tidak melebihi taksiran teks saja (%d) — byte dokumen tidak terhitung",
			EstimateTokens(berdokumen).InputTokens, EstimateTokens(dasar).InputTokens)
	}
}
