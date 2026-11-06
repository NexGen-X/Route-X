package gateway

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/billing"
)

// TestBatasRencanaDiBawahTTLTahanB189 mengunci relasi lintas paket B-189:
// batas total rencana eksekusi WAJIB lebih pendek dari umur tahanan
// reservasi anggaran bawaan. Bila seseorang menaikkan batasTotalRencana
// melewati billing.HoldTTLBawaan tanpa menaikkan TTL tahanan, tahanan
// gugur di tengah permintaan yang sah dan pagu anggaran kembali bisa
// terlampaui oleh permintaan paralel yang lambat.
func TestBatasRencanaDiBawahTTLTahanB189(t *testing.T) {
	if batasTotalRencana >= billing.HoldTTLBawaan {
		t.Errorf("batasTotalRencana = %v, billing.HoldTTLBawaan = %v; "+
			"tahanan anggaran harus hidup lebih lama dari permintaan terlama yang sah (B-189)",
			batasTotalRencana, billing.HoldTTLBawaan)
	}
}
