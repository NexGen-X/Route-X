package apikey

import (
	"context"
	"testing"
	"time"
)

// TestB353RefundScopesAtMenurunkanEmberAsal membuktikan pasangan
// berjangkar di level cakupan (jalur cakupan model gateway memakai
// jalan yang sama): RefundScopesAt dengan waktu kenaikan menurunkan
// ember asal; RefundScopes tanpa jangkar sesudah batas jendela —
// perilaku lama — meninggalkan ember asal tetap terisi (itulah
// bentuk cacatnya, didokumentasikan di sini sebagai kontrol).
func TestB353RefundScopesAtMenurunkanEmberAsal(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	scopes := []ScopeLimits{{Scope: ScopeAPIKey, ID: testKeyID, Limits: Limits{RPM: 100}}}

	saatNaik := env.now
	if d := env.limiter.AllowScopes(ctx, scopes); !d.Allowed {
		t.Fatalf("AllowScopes ditolak: %+v", d)
	}
	kunci := kunciJendela(t, env, "rpm-")
	if len(kunci) == 0 {
		t.Fatal("kunci rpm tidak ditemukan")
	}

	env.advance(61 * time.Second)
	// Kontrol perilaku lama: refund tanpa jangkar tidak menyentuh
	// ember asal.
	env.limiter.RefundScopes(ctx, scopes)
	if v, _ := env.mr.Get(kunci[0]); v != "1" {
		t.Fatalf("kontrol: counter ember asal = %q sesudah refund tanpa jangkar, mau \"1\" (cacat lama)", v)
	}
	// Refund berjangkar menurunkan ember asal walau jam sudah pindah.
	env.limiter.RefundScopesAt(ctx, scopes, saatNaik)
	if v, _ := env.mr.Get(kunci[0]); v != "0" {
		t.Errorf("counter ember asal = %q sesudah RefundScopesAt, mau \"0\"", v)
	}
}
