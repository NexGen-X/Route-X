package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/seed"
)

// --- B-089: metadata lifecycle discovery --------------------------------

// TestLifecycleDiscoveryInternAI membuktikan bentuk payload discovery
// Intern-AI (dari temuan live B-089) tertangkap parser OpenAI-compatible:
// status/shutdown_date/replacement_model tidak lagi terbuang.
func TestLifecycleDiscoveryInternAI(t *testing.T) {
	payload := `{"data":[
		{"id":"intern-s2"},
		{"id":"Atria-Dawn-Preview","status":"deprecated","shutdown_date":"2026-11-15","replacement_model":"intern-s2"}
	]}`
	var resp openAIModelDiscoveryResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("jumlah model = %d, mau 2", len(resp.Data))
	}
	aktif := resp.Data[0]
	if aktif.Status != "" || aktif.ShutdownDate != "" || aktif.ReplacementModel != "" {
		t.Errorf("model aktif ikut membawa lifecycle: %+v", aktif)
	}
	lama := resp.Data[1]
	lc := &modelLifecycle{Status: lama.Status, ShutdownDate: lama.ShutdownDate, ReplacementModel: lama.ReplacementModel}
	if !lc.ada() {
		t.Fatal("lifecycle model lama tidak tertangkap")
	}
	if !lc.usang() {
		t.Errorf("lifecycle deprecated + shutdown_date harus terbaca usang: %+v", lc)
	}
}

// TestMetadataSyncModelLifecycle: metadata lifecycle tergabung ke metadata
// model TANPA menghapus kunci milik operator, dan bertahan saat dibaca
// ulang dari representasi JSON-nya (bentuk yang disimpan ke models.metadata).
func TestMetadataSyncModelLifecycle(t *testing.T) {
	existing := []byte(`{"catatan_operator":"jangan hapus"}`)
	lc := &modelLifecycle{Status: "deprecated", ShutdownDate: "2026-11-15", ReplacementModel: "intern-s2"}
	out := metadataSyncModel(existing, lc, "")
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("metadata hasil bukan JSON sah: %v", err)
	}
	if obj["catatan_operator"] != "jangan hapus" {
		t.Errorf("kunci operator hilang: %v", obj)
	}
	if obj["lifecycle_status"] != "deprecated" || obj["lifecycle_shutdown_date"] != "2026-11-15" ||
		obj["lifecycle_replacement_model"] != "intern-s2" || obj["lifecycle_source"] != "upstream_discovery" {
		t.Errorf("kunci lifecycle tidak lengkap: %v", obj)
	}
	// Tanpa lifecycle, metadata lama tidak berubah isinya.
	out2 := metadataSyncModel(existing, nil, "")
	var obj2 map[string]any
	_ = json.Unmarshal(out2, &obj2)
	if len(obj2) != 1 {
		t.Errorf("tanpa lifecycle metadata bertambah kunci: %v", obj2)
	}
	// Penanda asal kapabilitas untuk model baru (B-116 butir 9).
	out3 := metadataSyncModel(nil, nil, "default_unverified")
	var obj3 map[string]any
	_ = json.Unmarshal(out3, &obj3)
	if obj3["capabilities_source"] != "default_unverified" {
		t.Errorf("penanda capabilities_source hilang: %v", obj3)
	}
}

// TestModelLifecycleUsang: hanya status akhir-masa / tanggal shutdown yang
// menandai usang; status aktif dan lifecycle kosong tidak.
func TestModelLifecycleUsang(t *testing.T) {
	if (&modelLifecycle{Status: "active"}).usang() {
		t.Error("status active tidak boleh usang")
	}
	if (&modelLifecycle{}).usang() {
		t.Error("lifecycle kosong tidak boleh usang")
	}
	var nilLC *modelLifecycle
	if nilLC.usang() || nilLC.ada() {
		t.Error("lifecycle nil harus aman")
	}
	if !(&modelLifecycle{ShutdownDate: "2026-11-15"}).usang() {
		t.Error("shutdown_date saja harus menandai usang (terjadwal mati)")
	}
}

// TestKapabilitasGoogleB116: kapabilitas diturunkan HANYA dari metode yang
// dinyatakan API Google (B-116 butir 9) — model embeddings hasil sync
// dengan begitu lolos /v1/embeddings tanpa edit manual.
func TestKapabilitasGoogleB116(t *testing.T) {
	caps := kapabilitasGoogle([]string{"embedContent"})
	if len(caps) != 1 || caps[0] != "embeddings" {
		t.Errorf("embedContent → %v, mau [embeddings]", caps)
	}
	caps = kapabilitasGoogle([]string{"generateContent", "countTokens"})
	if len(caps) != 1 || caps[0] != "text" {
		t.Errorf("generateContent → %v, mau [text] (countTokens tidak dipetakan)", caps)
	}
	if caps := kapabilitasGoogle(nil); len(caps) != 0 {
		t.Errorf("tanpa bukti harus kosong, dapat %v", caps)
	}
}

// --- B-090: katalog harga acuan global -----------------------------------

func TestKatalogHargaAcuanGlobalB090(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodGet, "/api/admin/upstreams/models/pricing/reference-catalog", nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status katalog acuan = %d, mau 200: %s", res.StatusCode, body)
	}
	var dto ReferencePriceCatalogDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("decode katalog: %v", err)
	}
	if len(dto.Items) != len(seed.ReferencePrices()) {
		t.Errorf("jumlah entri = %d, mau %d (seluruh katalog seed)", len(dto.Items), len(seed.ReferencePrices()))
	}
	if dto.Note == "" {
		t.Error("note cakupan katalog kosong — endpoint wajib menyatakan sumbernya apa adanya")
	}
	for _, it := range dto.Items {
		if !it.Available || it.Source == "" || it.FetchedOn == "" {
			t.Errorf("entri acuan tidak lengkap/asli: %+v", it)
		}
	}
}

// --- B-116 butir 1: egress description + masked_hint ----------------------

func TestEgressDescriptionDanMaskedHintB116(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/upstreams/egress-pools", map[string]any{
		"name":        "pool-uji-b116",
		"description": "Pool uji tahap 5",
		"kind":        "http",
		"proxy_url":   "http://pengguna:GANTI_rahasia@proxy.contoh.test:18080",
	}, true)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("buat egress = %d: %s", res.StatusCode, body)
	}
	var dibuat EgressPoolDTO
	if err := json.Unmarshal(body, &dibuat); err != nil {
		t.Fatalf("decode egress: %v", err)
	}
	if dibuat.Description != "Pool uji tahap 5" {
		t.Errorf("description buat = %q, mau tersimpan apa adanya", dibuat.Description)
	}
	if strings.Contains(dibuat.MaskedHint, "pengguna") || strings.Contains(dibuat.MaskedHint, "GANTI_rahasia") {
		t.Errorf("masked_hint membocorkan userinfo: %q", dibuat.MaskedHint)
	}
	if dibuat.MaskedHint != "http://****@proxy.contoh.test:18080" {
		t.Errorf("masked_hint = %q, mau http://****@proxy.contoh.test:18080", dibuat.MaskedHint)
	}

	// Ubah description lewat PUT; bidang lain tidak ikut berubah.
	res, body = client.do(http.MethodPut, "/api/admin/upstreams/egress-pools/"+dibuat.ID, map[string]any{
		"description": "Deskripsi hasil ubah",
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ubah egress = %d: %s", res.StatusCode, body)
	}
	var diubah EgressPoolDTO
	if err := json.Unmarshal(body, &diubah); err != nil {
		t.Fatalf("decode egress ubah: %v", err)
	}
	if diubah.Description != "Deskripsi hasil ubah" {
		t.Errorf("description ubah = %q", diubah.Description)
	}
	if diubah.Name != "pool-uji-b116" {
		t.Errorf("nama ikut berubah padahal tidak dikirim: %q", diubah.Name)
	}
}

// --- B-116 butir 8: status domain dari sumber installer -------------------

func TestDomainDariPublicURLB116(t *testing.T) {
	kasus := []struct {
		publicURL string
		mau       string
	}{
		{"https://founder.id-tech.cloud", "founder.id-tech.cloud"},
		{"https://founder.id-tech.cloud:8443/v1", "founder.id-tech.cloud"},
		{"http://13.212.164.108:8080", ""}, // host IP bukan domain
		{"http://localhost:8080", ""},      // localhost bukan domain publik
		{"", ""},
		{"bukan-url", ""},
	}
	for _, k := range kasus {
		h := &Handlers{publicURL: k.publicURL}
		if got := h.domainDariPublicURL(); got != k.mau {
			t.Errorf("publicURL %q → %q, mau %q", k.publicURL, got, k.mau)
		}
	}
}

func TestStatusDomainSumberInstallerB116(t *testing.T) {
	env := setupTestEnv(t)
	env.handlers.publicURL = "https://founder.id-tech.cloud"
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodGet, "/api/admin/system/domain/", nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status domain = %d: %s", res.StatusCode, body)
	}
	var dto DomainConfigDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("decode domain: %v", err)
	}
	if dto.Domain != "founder.id-tech.cloud" || dto.Status != "active" || dto.Mode != "installer" {
		t.Errorf("status domain installer salah: %+v", dto)
	}
	if !strings.Contains(dto.Message, "installer") {
		t.Errorf("pesan wajib menyebut sumber installer: %q", dto.Message)
	}
}
