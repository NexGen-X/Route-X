package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestBanKedaluarsaBlokirBanBaruB587 mengunci skenario dinamis persis entri
// B-587 lewat handler admin sungguhan (201/409/200):
//
//   - POST ban IP 203.0.113.77 expires +3 dtk → 201;
//   - GET daftar bans = 1 item; POST duplikat selagi aktif → 409;
//   - tidur 4 dtk → GET daftar bans = 0 item;
//   - POST ban ulang subjek sama → HARUS 201 (di base: 409
//     {"code":"conflict","message":"pemblokiran sudah ada atau melanggar
//     keunikan"});
//   - POST lift ban baru → 200; POST ban ulang → 201.
func TestBanKedaluarsaBlokirBanBaruB587(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	const jalur = "/api/admin/gateway/bans"
	buat := func(expires string) (*http.Response, []byte) {
		return c.do(http.MethodPost, jalur, map[string]any{
			"subject_kind": "ip",
			"subject":      "203.0.113.77",
			"reason":       "uji B-587",
			"expires_at":   expires,
		}, true)
	}
	daftar := func() []BanDTO {
		t.Helper()
		res, body := c.do(http.MethodGet, jalur, nil, false)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET daftar bans: status %d, body %s", res.StatusCode, body)
		}
		var env struct {
			Items []BanDTO `json:"items"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("parse daftar bans: %v (body %s)", err, body)
		}
		return env.Items
	}

	// Ban pertama, kedaluwarsa +3 detik.
	res, body := buat(time.Now().Add(3 * time.Second).UTC().Format(time.RFC3339))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat ban pertama: status %d, body %s", res.StatusCode, body)
	}
	var pertama BanDTO
	if err := json.Unmarshal(body, &pertama); err != nil {
		t.Fatalf("parse ban pertama: %v", err)
	}

	if items := daftar(); len(items) != 1 {
		t.Fatalf("daftar bans = %d item, mau 1", len(items))
	}

	// Duplikat selagi aktif tetap 409.
	res, body = buat("")
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("buat duplikat aktif: status %d, mau 409 (body %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "conflict") {
		t.Errorf("respons duplikat aktif tidak memuat conflict: %s", body)
	}

	time.Sleep(4 * time.Second)

	// Baris kedaluwarsa tak terlihat lagi di API.
	if items := daftar(); len(items) != 0 {
		t.Fatalf("daftar bans sesudah kedaluwarsa = %d item, mau 0", len(items))
	}

	// Inti B-587: ban ulang subjek yang sama harus 201, bukan 409.
	res, body = buat("")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat ban ulang sesudah kedaluwarsa: status %d, mau 201 (body %s)", res.StatusCode, body)
	}
	var kedua BanDTO
	if err := json.Unmarshal(body, &kedua); err != nil {
		t.Fatalf("parse ban kedua: %v", err)
	}
	if kedua.ID == pertama.ID {
		t.Fatalf("ban ulang memakai ID lama %s", pertama.ID)
	}

	// Lift ban baru → 200, lalu buat ulang → 201.
	res, body = c.do(http.MethodPost, jalur+"/"+kedua.ID+"/lift", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("lift ban baru: status %d, body %s", res.StatusCode, body)
	}
	res, body = buat("")
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat ban sesudah lift: status %d, mau 201 (body %s)", res.StatusCode, body)
	}
}
