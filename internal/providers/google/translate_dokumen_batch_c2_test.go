package google

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi Batch C2: bagian dokumen biner yang tiba di adapter Google
// ditolak eksplisit — tidak dibuang diam-diam oleh cabang default.
func TestDokumenDitolakEksplisit(t *testing.T) {
	p := &Provider{}
	m := &providers.Message{
		Role: providers.RoleUser,
		Parts: []providers.ContentPart{
			{Type: providers.PartTypeText, Text: "ringkas"},
			{Type: providers.PartTypeDocument, Document: &providers.Document{
				MediaType: "application/pdf", Data: "JVBERi0xLjQ=",
			}},
		},
	}
	_, err := p.contentParts(m)
	if err == nil {
		t.Fatal("dokumen tidak ditolak — ia akan hilang diam-diam seperti V-GW-22")
	}
	if perr := providers.AsError(err); perr == nil || perr.Kind != providers.ErrKindInvalidRequest {
		t.Errorf("error = %v, mau *providers.Error berjenis InvalidRequest", err)
	}
}
