package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/router"
)

// pipelineValid adalah resep combo sah menurut constraint migrasi 0014: salah satu dari
// 4 strategi UI, attempts 1-20, models 1-8.
var pipelineValid = map[string]any{
	"strategy": "round_robin",
	"attempts": 3,
	"models":   []string{"gpt-4o-mini", "gemini-2.5-flash"},
}

// routingRuleResp adalah bentuk respons create/update routing rule yang diperiksa test.
type routingRuleResp struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	Pipeline     *router.ComboPipelineDTO `json:"pipeline"`
	VirtualAlias *string                  `json:"virtual_alias"`
}

// simpanAlias adalah singkatan untuk membuat aturan combo lengkap dipakai beberapa test.
func simpanAlias(t *testing.T, client *adminClient, nama, alias string) *routingRuleResp {
	t.Helper()
	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":          nama,
		"strategy":      "priority",
		"virtual_alias": alias,
		"pipeline":      pipelineValid,
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat %q gagal: status %d, body: %s", nama, res.StatusCode, string(body))
	}
	var got routingRuleResp
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("urai respons %q: %v", nama, err)
	}
	return &got
}

// ambilRule membaca satu aturan lewat API sebagai bentuk kanonik DTO.
func ambilRule(t *testing.T, client *adminClient, id string) *routingRuleResp {
	t.Helper()
	res, body := client.do(http.MethodGet, "/api/admin/gateway/routing-rules/"+id, nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get rule %s status = %d, body: %s", id, res.StatusCode, string(body))
	}
	var got routingRuleResp
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("urai respons get rule: %v", err)
	}
	return &got
}

// kolomPipelineAlias membaca pipeline jsonb mentah dan virtual_alias langsung dari
// database — bukti nyata bahwa kolom benar-benar terisi, bukan hanya DTO yang dibaca
// kembali lewat jalur yang sama.
func kolomPipelineAlias(t *testing.T, env *testEnv, id string) (pipeline string, alias string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pipelineRaw, aliasRaw *string
	err := env.pool.QueryRow(ctx,
		`select pipeline::text, virtual_alias from routing_rules where id = $1`, id).
		Scan(&pipelineRaw, &aliasRaw)
	if err != nil {
		t.Fatalf("baca kolom pipeline/alias: %v", err)
	}
	if pipelineRaw != nil {
		pipeline = *pipelineRaw
	}
	if aliasRaw != nil {
		alias = *aliasRaw
	}
	return pipeline, alias
}

// TestCreateRoutingRuleDenganPipelineDanAlias memverifikasi jalur baru ujung ke ujung:
// handler mengurai pipeline + alias dari body, menyimpannya ke kolom jsonb/text, dan
// mengembalikannya lewat DTO tanpa field sensitif.
func TestCreateRoutingRuleDenganPipelineDanAlias(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	got := simpanAlias(t, client, "Combo Uji Alias", "murah-cerdas")

	if got.Pipeline == nil || got.Pipeline.Strategy != "round_robin" {
		t.Fatalf("Pipeline DTO = %+v, mau round_robin", got.Pipeline)
	}
	if got.Pipeline.Attempts != 3 || len(got.Pipeline.Models) != 2 {
		t.Errorf("Pipeline DTO = %+v, mau attempts 3 dan 2 model", got.Pipeline)
	}
	if got.VirtualAlias == nil || *got.VirtualAlias != "murah-cerdas" {
		t.Errorf("VirtualAlias = %v, mau murah-cerdas", got.VirtualAlias)
	}

	// Kolom di database benar-benar terisi, bukan hanya DTO.
	pipeline, alias := kolomPipelineAlias(t, env, got.ID)
	if !strings.Contains(pipeline, "round_robin") || !strings.Contains(pipeline, "gemini-2.5-flash") {
		t.Errorf("kolom pipeline = %q, mau berisi resep jsonb", pipeline)
	}
	if alias != "murah-cerdas" {
		t.Errorf("kolom virtual_alias = %q, mau murah-cerdas", alias)
	}

	// Baca kembali lewat Get: kontrak DTO konsisten antara create dan baca.
	ulang := ambilRule(t, client, got.ID)
	if ulang.Pipeline == nil || ulang.Pipeline.Strategy != "round_robin" {
		t.Errorf("Pipeline pada Get = %+v", ulang.Pipeline)
	}
	if ulang.VirtualAlias == nil || *ulang.VirtualAlias != "murah-cerdas" {
		t.Errorf("VirtualAlias pada Get = %v", ulang.VirtualAlias)
	}
}

// TestCreateRoutingRuleAliasYatimDitolak: alias tanpa resep pipeline adalah keadaan tak
// bermakna (tidak ada model lain untuk di-failover-kan), wajib ditolak 400 sebelum
// menyimpan — bukan dibiarkan menjadi aturan yang menangkap klien tanpa pernah melayani.
func TestCreateRoutingRuleAliasYatimDitolak(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":          "Alias Yatim",
		"strategy":      "priority",
		"virtual_alias": "yatim-alias",
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 (alias tanpa pipeline), body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "virtual_alias") {
		t.Errorf("pesan 400 tidak menjelaskan penyebabnya: %s", string(body))
	}

	// Aturan TIDAK tersimpan.
	resList, bodyList := client.do(http.MethodGet, "/api/admin/gateway/routing-rules", nil, false)
	if resList.StatusCode != http.StatusOK {
		t.Fatalf("list rules status = %d", resList.StatusCode)
	}
	if strings.Contains(string(bodyList), "Alias Yatim") {
		t.Error("aturan alias yatim tersimpan di database; seharusnya ditolak")
	}
}

// TestCreateRoutingRuleAliasGandaRollback: dua aturan memakai alias yang sama. Yang kedua
// wajib gagal, DAN gagalnya itu membatalkan seluruh transaksi — aturan kedua tidak boleh
// ada meski insert baris aturan dilakukan lebih dulu daripada penolakan unik index.
//
// Ini bukti perbaikan bug K3: dulu alias didaftarkan terpisah lewat api.models.addAlias
// yang kegagalannya hanya console.warn, sehingga rule dibuat tapi virtual endpoint 404.
func TestCreateRoutingRuleAliasGandaRollback(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	simpanAlias(t, client, "Combo Pertama", "sudah-diambil")

	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":          "Combo Kedua Ganda",
		"strategy":      "priority",
		"virtual_alias": "sudah-diambil",
		"pipeline":      pipelineValid,
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 untuk alias ganda, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "sudah-diambil") {
		t.Errorf("pesan tidak menyebut alias yang bentrok: %s", string(body))
	}

	// Rule kedua tidak tersimpan — transaksi di-rollback penuh.
	resList, bodyList := client.do(http.MethodGet, "/api/admin/gateway/routing-rules", nil, false)
	if resList.StatusCode != http.StatusOK {
		t.Fatalf("list rules status = %d", resList.StatusCode)
	}
	if strings.Contains(string(bodyList), "Combo Kedua Ganda") {
		t.Error("aturan dengan alias ganda tersimpan; transaksi seharusnya di-rollback")
	}
}

// TestCreateRoutingRulePipelineInvalidDitolak: pipeline cacat ditolak 400 dengan pesan
// yang menyebut fieldnya, sebelum aturan dibuat — bukan setelahnya sebagai error
// constraint generik dari database.
func TestCreateRoutingRulePipelineInvalidDitolak(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	for _, tc := range []struct {
		nama string
		body map[string]any
	}{
		{
			"strategi asing",
			map[string]any{"name": "Pipeline Asing", "pipeline": map[string]any{
				"strategy": "tidak_ada", "attempts": 2, "models": []string{"a", "b"}}},
		},
		{
			"attempts di luar jangkauan",
			map[string]any{"name": "Pipeline Attempts", "pipeline": map[string]any{
				"strategy": "priority", "attempts": 99, "models": []string{"a", "b"}}},
		},
		{
			"models kosong",
			map[string]any{"name": "Pipeline Kosong", "pipeline": map[string]any{
				"strategy": "priority", "attempts": 3, "models": []string{}}},
		},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", tc.body, true)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400, body: %s", res.StatusCode, string(body))
			}
			if !strings.Contains(string(body), "pipeline") {
				t.Errorf("pesan tidak menyebut pipeline: %s", string(body))
			}
		})
	}
}

// TestUpdateRoutingRulePertahankanAlias: permintaan sebagian yang TIDAK menyebut
// alias/pipeline wajib membiarkan keduanya utuh. Tanpa ini, mengubah provider saja
// menghapus alias virtual endpoint secara diam-diam.
func TestUpdateRoutingRulePertahankanAlias(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	got := simpanAlias(t, client, "Combo Dipertahankan", "tetap-ada")

	res, body := client.do(http.MethodPut,
		"/api/admin/gateway/routing-rules/"+got.ID,
		map[string]any{"strategy": "round_robin"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, body: %s", res.StatusCode, string(body))
	}

	ulang := ambilRule(t, client, got.ID)
	if ulang.VirtualAlias == nil || *ulang.VirtualAlias != "tetap-ada" {
		t.Errorf("VirtualAlias setelah update sebagian = %v, alias harusnya utuh", ulang.VirtualAlias)
	}
	if ulang.Pipeline == nil || ulang.Pipeline.Strategy != "round_robin" {
		t.Errorf("Pipeline setelah update sebagian = %+v, resep harusnya utuh", ulang.Pipeline)
	}
}

// TestUpdateRoutingRuleGantiAlias: alias bisa diganti, dan alias lama dibebaskan sehingga
// aturan lain boleh memakainya. Ini perbaikan bug K3: dulu alias lama tidak pernah
// dihapus saat edit, sehingga alias terkunci di aturan yang sudah tak memakainya.
func TestUpdateRoutingRuleGantiAlias(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	got := simpanAlias(t, client, "Combo Ganti Alias", "alias-lama")

	res, body := client.do(http.MethodPut,
		"/api/admin/gateway/routing-rules/"+got.ID,
		map[string]any{"virtual_alias": "alias-baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update alias status = %d, body: %s", res.StatusCode, string(body))
	}

	ulang := ambilRule(t, client, got.ID)
	if ulang.VirtualAlias == nil || *ulang.VirtualAlias != "alias-baru" {
		t.Errorf("VirtualAlias = %v, mau alias-baru", ulang.VirtualAlias)
	}
	if ulang.Pipeline == nil {
		t.Error("Pipeline hilang saat hanya alias yang diubah")
	}

	// Alias lama bebas: aturan lain bisa memakainya sekarang.
	kedua := simpanAlias(t, client, "Combo Rebut Alias Lama", "alias-lama")
	if kedua.VirtualAlias == nil || *kedua.VirtualAlias != "alias-lama" {
		t.Errorf("alias lama seharusnya sudah bebas: %v", kedua.VirtualAlias)
	}
}

// TestUpdateRoutingRuleHapusAliasPipeline: menghapus resep pipeline harus disertai hapus
// alias (alias yatim ditolak), dan penghapusan tersebut harus tersimpan permanen.
func TestUpdateRoutingRuleHapusAliasPipeline(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	got := simpanAlias(t, client, "Combo Dibersihkan", "akan-dihapus")

	res, body := client.do(http.MethodPut,
		"/api/admin/gateway/routing-rules/"+got.ID,
		map[string]any{"pipeline": nil, "virtual_alias": ""}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("hapus pipeline+alias status = %d, body: %s", res.StatusCode, string(body))
	}

	ulang := ambilRule(t, client, got.ID)
	if ulang.Pipeline != nil {
		t.Errorf("Pipeline = %+v, mau null (jalur lama)", ulang.Pipeline)
	}
	if ulang.VirtualAlias != nil {
		t.Errorf("VirtualAlias = %v, mau null", ulang.VirtualAlias)
	}

	// Kolom database benar-benar NULL.
	pipeline, alias := kolomPipelineAlias(t, env, got.ID)
	if pipeline != "" {
		t.Errorf("kolom pipeline = %q, mau NULL", pipeline)
	}
	if alias != "" {
		t.Errorf("kolom virtual_alias = %q, mau NULL", alias)
	}
}

// TestUpdateRoutingRuleAliasGandaDitolak: memasang alias yang sudah diduduki aturan lain
// wajib ditolak dengan pesan yang menyebut aturan pemiliknya — bukan error 500 dari
// pelanggaran unik index yang tidak terbaca operator.
func TestUpdateRoutingRuleAliasGandaDitolak(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	simpanAlias(t, client, "Combo Pemilik Alias", "milik-saya")
	got := simpanAlias(t, client, "Combo Penerima Alias", "alias-bebas")

	res, body := client.do(http.MethodPut,
		"/api/admin/gateway/routing-rules/"+got.ID,
		map[string]any{"virtual_alias": "milik-saya"}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 untuk alias ganda, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "milik-saya") || !strings.Contains(string(body), "Combo Pemilik Alias") {
		t.Errorf("pesan tidak menjelaskan pemilik alias: %s", string(body))
	}

	// Alias aturan kedua tidak berubah.
	ulang := ambilRule(t, client, got.ID)
	if ulang.VirtualAlias == nil || *ulang.VirtualAlias != "alias-bebas" {
		t.Errorf("VirtualAlias = %v, seharusnya tidak berubah karena update ditolak", ulang.VirtualAlias)
	}
}

// TestRoutingRuleDTOTidakMembocorkanFieldSensitif memeriksa respons create/get routing
// rule tidak memuat field rahasia (pepper, hash, ciphertext) dan memuat field routing
// publik (pipeline, virtual_alias) — keduanya konfigurasi routing, bukan kredensial.
//
// Pemeriksaan dilakukan terhadap KUNCI JSON, bukan substring bebas: match_api_key_id
// adalah referensi FK publik (aturan mana yang cocok) dan tidak boleh dituduh sebagai
// kebocoran hanya karena mengandung kata "api_key".
func TestRoutingRuleDTOTidakMembocorkanFieldSensitif(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	got := simpanAlias(t, client, "Combo Audit DTO", "audit-dto")

	for _, jalur := range []string{
		"/api/admin/gateway/routing-rules/" + got.ID,
		"/api/admin/gateway/routing-rules",
	} {
		res, body := client.do(http.MethodGet, jalur, nil, false)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", jalur, res.StatusCode)
		}

		kunci := map[string]bool{}
		kumpulkanKunciJSON(t, body, kunci)

		for _, terlarang := range []string{
			"pepper", "password_hash", "secret", "salt",
			"ciphertext", "encryption_key_id", "private_key", "session_token",
		} {
			if kunci[terlarang] {
				t.Errorf("GET %s memuat field sensitif %q: %s", jalur, terlarang, string(body))
			}
		}
		// Field routing publik ada di respons (bukti kontrak baru ekspos dengan benar).
		for _, diharapkan := range []string{"pipeline", "virtual_alias", "providers"} {
			if !kunci[diharapkan] {
				t.Errorf("GET %s tidak memuat field %q", jalur, diharapkan)
			}
		}
	}
}

// kumpulkanKunciJSON mengumpulkan seluruh kunci objek JSON bersarang; array hanya
// dituruni isinya, tipe skalar diabaikan.
func kumpulkanKunciJSON(t *testing.T, body []byte, out map[string]bool) {
	t.Helper()
	var obj any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("urai JSON respons: %v", err)
	}
	turuniKunci(obj, out)
}

func turuniKunci(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			out[k] = true
			turuniKunci(val, out)
		}
	case []any:
		for _, val := range x {
			turuniKunci(val, out)
		}
	}
}

// TestRoutingRuleFindByAliasRepo memverifikasi method repo baru: aturan dimatikan tetap
// diduduki aliasnya — pemeriksaan konflik yang hanya melihat aturan aktif akan salah
// melaporkan alias bebas.
func TestRoutingRuleFindByAliasRepo(t *testing.T) {
	env := setupTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)
	got := simpanAlias(t, client, "Combo Repo Alias", "alias-repo")

	repoAturan := upstream.NewRoutingRepo(env.pool)

	// Alias terikat pada aturan aktif.
	ditemukan, err := repoAturan.FindByAlias(ctx, "alias-repo")
	if err != nil {
		t.Fatalf("FindByAlias: %v", err)
	}
	if ditemukan.ID != got.ID {
		t.Errorf("FindByAlias menemukan %s, mau %s", ditemukan.ID, got.ID)
	}

	// Alias yang tidak dipakai → ErrNotFound.
	if _, err := repoAturan.FindByAlias(ctx, "tidak-ada"); !errIsNotFound(err) {
		t.Errorf("FindByAlias alias bebas = %v, mau ErrNotFound", err)
	}

	// Aturan DIMATIKAN tetap memegang aliasnya.
	if _, err := repoAturan.Update(ctx, got.ID, upstream.UpdateRoutingRuleParams{Enabled: boolPtr(false)}); err != nil {
		t.Fatalf("mematikan aturan: %v", err)
	}
	if ditemukan, err := repoAturan.FindByAlias(ctx, "alias-repo"); err != nil || ditemukan.ID != got.ID {
		t.Errorf("FindByAlias atas aturan mati = %v %v; alias harusnya tetap diduduki", ditemukan, err)
	}

	// Alias string kosong atau whitespace ditolak sebagai argumen.
	if _, err := repoAturan.FindByAlias(ctx, "   "); !errIsInvalidRef(err) {
		t.Errorf("FindByAlias whitespace = %v, mau ErrInvalidReference", err)
	}
}

func errIsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), repo.ErrNotFound.Error())
}

func errIsInvalidRef(err error) bool {
	return err != nil && strings.Contains(err.Error(), repo.ErrInvalidReference.Error())
}

func boolPtr(b bool) *bool { return &b }
