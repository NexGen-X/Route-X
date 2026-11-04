package billing

import (
	"context"
	"slices"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
)

// namaEventUji membaca daftar event yang sudah diantrekan tiruan webhook.
func namaEventUji(a *antreanWebhookUji) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.events...)
}

// TestAnnounceAmbangTerlompatTetapDiumumkan membuktikan B-107: satu
// permintaan bisa membawa spent dari nol langsung melewati batas (biaya
// per request lebih besar dari seluruh plafon), sehingga jendela ambang
// [pct, 100%) tidak pernah teramati oleh satu pun pemeriksaan —
// ShouldAlertThreshold mensyaratkan spent belum terlampaui. Versi lama
// kehilangan peringatan ambangnya permanen (alerted_at tetap NULL,
// anggaran warn tampak bisu). Kini pelampauan yang melompati ambang tetap
// mengumumkan ambangnya, tepat satu kali, bersamaan dengan pengumuman
// terlampauinya.
func TestAnnounceAmbangTerlompatTetapDiumumkan(t *testing.T) {
	warn := anggaran("warn-lompat", policy.ScopeGlobal, "", "0.00001", "0.000075", func(b *policy.Budget) {
		b.ActionOnExceed = policy.ActionWarn
		b.AlertThresholdPct = 50
	})
	if warn.ShouldAlertThreshold() {
		t.Fatal("prasyarat skenario rusak: ShouldAlertThreshold harus false bila spent sudah melewati batas")
	}
	if !warn.ShouldAlertExceeded() {
		t.Fatal("prasyarat skenario rusak: ShouldAlertExceeded harus true")
	}

	src := &sumber{budgets: []*policy.Budget{warn}, tandaiOK: true}
	queue := &antreanWebhookUji{}
	e := NewEnforcer(src, nil, WithWebhookEnqueuer(queue))

	e.Announce(context.Background(), []*policy.Budget{warn})

	tanda := src.penandaan()
	if !slices.Contains(tanda, "warn-lompat") {
		t.Errorf("alerted_at tidak ditandai untuk ambang yang terlompat; penandaan = %v", tanda)
	}
	if !slices.Contains(tanda, "warn-lompat:exceeded") {
		t.Errorf("exceeded_alerted_at tidak ditandai; penandaan = %v", tanda)
	}
	events := namaEventUji(queue)
	if !slices.Contains(events, "budget.threshold") {
		t.Errorf("event budget.threshold tidak diterbitkan; events = %v", events)
	}
	if !slices.Contains(events, "budget.exceeded") {
		t.Errorf("event budget.exceeded tidak diterbitkan; events = %v", events)
	}
}

// TestAnnounceAmbangMatiTidakDiumumkanSaatTerlampaui menjaga batas
// desainnya: alert_threshold_pct 0 berarti peringatan ambang MATI, jadi
// pelampauan tanpa ambang aktif hanya mengumumkan budget.exceeded.
func TestAnnounceAmbangMatiTidakDiumumkanSaatTerlampaui(t *testing.T) {
	b := anggaran("tanpa-ambang", policy.ScopeGlobal, "", "10", "25", func(b *policy.Budget) {
		b.AlertThresholdPct = 0
	})
	src := &sumber{budgets: []*policy.Budget{b}, tandaiOK: true}
	queue := &antreanWebhookUji{}
	e := NewEnforcer(src, nil, WithWebhookEnqueuer(queue))

	e.Announce(context.Background(), []*policy.Budget{b})

	tanda := src.penandaan()
	if slices.Contains(tanda, "tanpa-ambang") {
		t.Errorf("ambang mati (pct 0) tidak boleh ditandai; penandaan = %v", tanda)
	}
	if !slices.Contains(tanda, "tanpa-ambang:exceeded") {
		t.Errorf("penandaan terlampaui hilang; penandaan = %v", tanda)
	}
	if slices.Contains(namaEventUji(queue), "budget.threshold") {
		t.Errorf("event budget.threshold terbit untuk ambang mati; events = %v", namaEventUji(queue))
	}
}
