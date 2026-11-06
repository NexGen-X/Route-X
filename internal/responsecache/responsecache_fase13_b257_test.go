package responsecache

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// B-257: bahan kunci ComputeScopedKey meng-hash request PENUH — termasuk
// nama model mentah yang diketik klien — di samping Scope.ModelID. Dua
// alias berbeda ke model kanonik yang sama menghasilkan kunci berbeda,
// jadi entri tidak pernah terbagi antar-alias (dan entri yatim oleh
// tombstone penggantian nama tetap tersaji di bawah alias lama), persis
// menggagalkan maksud ModelID kanonik.
func TestComputeScopedKeyAliasBerbagiEntriB257(t *testing.T) {
	e := NewEngine(nil, nil)
	req := func(model string) *providers.ChatRequest {
		return &providers.ChatRequest{
			Model:    model,
			Messages: []providers.Message{{Role: "user", Content: "halo"}},
		}
	}
	scope := Scope{ModelID: "550e8400-e29b-41d4-a716-446655440000", OwnerUserID: "u-a", KeyID: "k-1"}

	kAlias1 := e.ComputeScopedKey(req("gpt-4o"), scope)
	kAlias2 := e.ComputeScopedKey(req("gpt-4o-terbaru"), scope)
	if kAlias1 != kAlias2 {
		t.Errorf("dua alias ke model kanonik yang sama menghasilkan kunci berbeda:\n%s\n%s", kAlias1, kAlias2)
	}

	// Model kanonik berbeda tetap kunci berbeda.
	scopeLain := scope
	scopeLain.ModelID = "550e8400-e29b-41d4-a716-446655440001"
	if got := e.ComputeScopedKey(req("gpt-4o"), scopeLain); got == kAlias1 {
		t.Error("model kanonik berbeda berbagi kunci")
	}

	// Scope kosong mempertahankan perilaku lama persis (kompatibilitas
	// ComputeKey): nama mentah tetap membentuk kunci.
	if e.ComputeScopedKey(req("gpt-4o"), Scope{}) == e.ComputeScopedKey(req("gpt-4o-terbaru"), Scope{}) {
		t.Error("scope kosong berubah perilaku: kompatibilitas kunci lama rusak")
	}
	// Dan kunci scoped tidak bertabrakan dengan kunci legacy nama yang
	// kebetulan sama dengan ID kanonik.
	if got := e.ComputeKey(req(scope.ModelID)); got == kAlias1 {
		t.Error("kunci scoped bertabrakan dengan kunci legacy")
	}
}
