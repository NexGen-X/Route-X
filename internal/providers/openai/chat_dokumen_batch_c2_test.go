package openai

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi Batch C2: payload chat menolak bagian dokumen biner secara
// eksplisit sebelum body disusun — bukan meneruskannya sebagai block asing
// dan bukan membuangnya diam-diam.
func TestPayloadMenolakDokumen(t *testing.T) {
	p := &Provider{}
	req := &providers.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []providers.Message{{
			Role: providers.RoleUser,
			Parts: []providers.ContentPart{
				{Type: providers.PartTypeText, Text: "ringkas"},
				{Type: providers.PartTypeDocument, Document: &providers.Document{
					MediaType: "application/pdf", Data: "JVBERi0xLjQ=",
				}},
			},
		}},
	}
	_, err := p.chatPayload(req, false)
	if err == nil {
		t.Fatal("dokumen tidak ditolak oleh chatPayload")
	}
	if err.Kind != providers.ErrKindInvalidRequest {
		t.Errorf("jenis error = %q, mau InvalidRequest", err.Kind)
	}
}
