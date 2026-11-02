package usage

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
)

// Grup 12 audit3 (B-021 / V-GW-46): RecordRejection mengubah penolakan
// middleware menjadi baris requests yang minimal dan jujur.

func TestRecordRejectionGrup12(t *testing.T) {
	rec, store, _, _ := pencatatUji(t)

	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("User-Agent", "klien-uji/1.0")
	rec.RecordRejection(context.Background(), apikey.RejectionInfo{
		Request:      req,
		StatusCode:   401,
		ErrorType:    "auth",
		ErrorCode:    "invalid_api_key",
		ErrorMessage: apikey.MessageInvalidKey,
	})

	req429 := httptest.NewRequest("POST", "/v1/messages", nil)
	rec.RecordRejection(context.Background(), apikey.RejectionInfo{
		Request:      req429,
		StatusCode:   429,
		ErrorType:    "rate_limit",
		ErrorCode:    "rate_limit_exceeded",
		ErrorMessage: apikey.MessageRateLimited,
		APIKeyID:     "key-1",
		APIKeyName:   "kunci uji",
	})

	tutup(t, rec)

	baris := store.semua()
	if len(baris) != 2 {
		t.Fatalf("baris tertulis = %d, mau 2", len(baris))
	}
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	// Urutan penulisan TIDAK dijamin (beberapa penulis bersamaan menguras
	// antrean), jadi baris dicari menurut statusnya, bukan posisinya.
	cari := func(status int) *traffic.Record {
		for i := range baris {
			if baris[i].StatusCode == status {
				return &baris[i]
			}
		}
		return nil
	}

	pertama := cari(401)
	if pertama == nil {
		t.Fatal("baris 401 tidak tertulis")
	}
	if pertama.StatusCode != 401 || deref(pertama.ErrorType) != "auth" || deref(pertama.ErrorCode) != "invalid_api_key" {
		t.Errorf("baris 401 = status %d, tipe %q, kode %q", pertama.StatusCode, deref(pertama.ErrorType), deref(pertama.ErrorCode))
	}
	if pertama.Endpoint != "/v1/chat/completions" || pertama.Method != "POST" {
		t.Errorf("baris 401 = %s %s", pertama.Method, pertama.Endpoint)
	}
	if deref(pertama.APIKeyID) != "" {
		t.Errorf("baris 401 APIKeyID = %q, mau kosong", deref(pertama.APIKeyID))
	}
	if deref(pertama.UserAgent) != "klien-uji/1.0" {
		t.Errorf("baris 401 UserAgent = %q", deref(pertama.UserAgent))
	}
	// Penolakan tidak pernah mencapai upstream: model & token harus kosong.
	if deref(pertama.ModelName) != "" || pertama.TotalTokens != 0 {
		t.Errorf("baris 401 membawa model/token karangan: model %q, token %d", deref(pertama.ModelName), pertama.TotalTokens)
	}

	kedua := cari(429)
	if kedua == nil {
		t.Fatal("baris 429 tidak tertulis")
	}
	if deref(kedua.ErrorType) != "rate_limit" || deref(kedua.APIKeyID) != "key-1" || deref(kedua.APIKeyName) != "kunci uji" {
		t.Errorf("baris 429 = tipe %q, key %q/%q", deref(kedua.ErrorType), deref(kedua.APIKeyID), deref(kedua.APIKeyName))
	}
	if kedua.Endpoint != "/v1/messages" {
		t.Errorf("baris 429 endpoint = %q, mau /v1/messages", kedua.Endpoint)
	}
}

func TestRecordRejectionNilAmanGrup12(t *testing.T) {
	var rec *Recorder
	// Recorder nil dan Request nil tidak boleh panik: pencatatan tidak
	// boleh menggagalkan penolakan itu sendiri.
	rec.RecordRejection(context.Background(), apikey.RejectionInfo{StatusCode: 401})

	r2, store, _, _ := pencatatUji(t)
	r2.RecordRejection(context.Background(), apikey.RejectionInfo{StatusCode: 401})
	tutup(t, r2)
	if n := len(store.semua()); n != 0 {
		t.Errorf("baris tertulis = %d, mau 0 untuk Request nil", n)
	}
}
