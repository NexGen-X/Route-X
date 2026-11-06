package gateway

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// TestB179SinyalDegradasiCombo: anggota resep yang tidak bisa diresolusi
// wajib menaikkan sinyal degradasi tingkat Error yang menyebut anggotanya
// (B-179), bukan hanya Warn per anggota seperti sebelumnya — resep yang
// menyusut harus terlihat sebagai keadaan yang perlu diperbaiki operator.
// Permintaan sendiri tetap dilayani anggota yang sehat.
func TestB179SinyalDegradasiCombo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-model", Enabled: true}
	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-model": m1,
	}}
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {kandidatRute("prov-t1", "upstream-t1")},
	}}
	rule := aturanPipeline("r-b179", router.StrategyPriority, 3,
		[]string{"tier1-model", "model-ngawur-b179"}, 3)
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())
	factory := &pabrikTiruan{perNama: map[string]providers.Provider{
		"prov-t1": &providerTiruan{nama: "prov-t1", kind: providers.KindOpenAI, chat: chatSukses("halo")},
	}}

	h, err := NewHandlers(HandlersDeps{
		Models:     models,
		Lister:     &pendaftarModel{},
		Candidates: cands,
		Factory:    factory,
		Engine:     engine,
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     logger,
	})
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(bodyComboChat))
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (anggota sehat tetap melayani), body: %s", w.Code, w.Body.String())
	}
	log := buf.String()
	if !strings.Contains(log, "resep combo terdegradasi") {
		t.Errorf("log tidak memuat sinyal degradasi B-179:\n%s", log)
	}
	if !strings.Contains(log, "model-ngawur-b179") {
		t.Errorf("sinyal degradasi tidak menyebut anggota yang dilewati:\n%s", log)
	}
	if !strings.Contains(log, "level=ERROR") {
		t.Errorf("sinyal degradasi bukan tingkat ERROR:\n%s", log)
	}
}
