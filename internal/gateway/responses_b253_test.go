package gateway

import (
	"net/http"
	"strings"
	"testing"
)

// B-253(a): kesalahan decode body Responses tidak boleh membocorkan jeroan
// Go ke klien — nama struct (bentukPermintaanResponses), frasa stdlib
// "cannot unmarshal", dan detail sintaks mentah adalah informasi
// implementasi. Yang sah diketahui klien: nama field dalam ejaan
// JSON-nya beserta fakta tipenya tak cocok, atau bahwa body-nya bukan
// JSON yang sah.

func TestB253PesanDecodeResponsesTidakBocor(t *testing.T) {
	kasus := []struct {
		nama       string
		body       string
		mengandung string
	}{
		{"tipe max_output_tokens", `{"model":"m","input":"halo","max_output_tokens":1.5}`, "max_output_tokens"},
		{"tipe temperature", `{"model":"m","input":"halo","temperature":"panas"}`, "temperature"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := uraiPermintaanResponses([]byte(k.body))
			if err == nil {
				t.Fatal("body bertipe salah diterima")
			}
			pesan := err.Error()
			if !strings.Contains(pesan, k.mengandung) {
				t.Errorf("pesan = %q, mau menyebut field %s", pesan, k.mengandung)
			}
			for _, bocor := range []string{"bentukPermintaanResponses", "cannot unmarshal", "Go struct field", "encoding/json"} {
				if strings.Contains(pesan, bocor) {
					t.Errorf("pesan membocorkan %q: %q", bocor, pesan)
				}
			}
		})
	}

	// Kesalahan sintaks: pesan umum tanpa detail stdlib.
	_, err := uraiPermintaanResponses([]byte(`{"model":`))
	if err == nil {
		t.Fatal("body JSON terpotong diterima")
	}
	if !strings.Contains(err.Error(), "body bukan JSON yang sah") {
		t.Errorf("pesan sintaks = %q, mau pesan umum body bukan JSON yang sah", err.Error())
	}
	for _, bocor := range []string{"unexpected end", "cannot unmarshal", "bentukPermintaanResponses"} {
		if strings.Contains(err.Error(), bocor) {
			t.Errorf("pesan sintaks membocorkan %q: %q", bocor, err.Error())
		}
	}

	// Kontrol: body sah tetap diterima.
	if _, err := uraiPermintaanResponses([]byte(`{"model":"m","input":"halo"}`)); err != nil {
		t.Errorf("body sah ditolak: %v", err)
	}
}

// B-253(b) — GUGUR-BUKTI di main ead325b, dikunci di sini sebagai regresi:
// sangkaan "arguments objek pada tool_calls chat menghasilkan pesan
// generik" tidak terbukti; decoder chat sudah menghasilkan pesan presisi
// yang menyebut jalur field-nya. Test tingkat handler ini memastikan
// pesan itu yang sampai ke klien, supaya perbaikan B-253(a) di permukaan
// Responses tidak pernah "menyelaraskan" chat ke pesan yang lebih buruk.
func TestB253ArgumentsObjekChatPesanPresisi(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	body := `{"model":"gpt-4o-mini","messages":[{"role":"assistant","content":null,` +
		`"tool_calls":[{"id":"call_1","type":"function","function":{"name":"cek_cuaca","arguments":{"kota":"makassar"}}}]}]}`
	w := s.panggil(t, http.MethodPost, "/chat/completions", body, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 (body: %s)", w.Code, w.Body.String())
	}
	ingin := `field "messages[0].tool_calls[0].function.arguments" harus berupa string JSON`
	if pesan := galat(t, w).Message; !strings.Contains(pesan, ingin) {
		t.Errorf("pesan = %q, mau memuat %q", pesan, ingin)
	}
}
