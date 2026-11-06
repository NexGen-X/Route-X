package google

import (
	"context"
	"net/http"
	"testing"
)

// B-264 (Google/Gemini): Models() mengikuti nextPageToken sampai habis
// dan mendeduplikasi ID lintas halaman. Sebelumnya token itu diabaikan
// dan hanya halaman pertama yang dibaca.
func TestModelsGeminiMengikutiPageTokenDanDedupB264(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("pageToken") {
		case "":
			_, _ = w.Write([]byte(`{"models":[
				{"name":"models/gemini-a"},
				{"name":"models/gemini-b"}
			],"nextPageToken":"tok-2"}`))
		case "tok-2":
			_, _ = w.Write([]byte(`{"models":[
				{"name":"models/gemini-b"},
				{"name":"models/gemini-c"}
			]}`))
		default:
			http.Error(w, "pageToken tak dikenal", http.StatusBadRequest)
		}
	}
	p, _ := newServer(t, handler)

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("jumlah model = %d, mau 3 (dua halaman, tanpa duplikat): %+v", len(models), models)
	}
	mau := []string{"gemini-a", "gemini-b", "gemini-c"}
	for i, id := range mau {
		if models[i].ID != id {
			t.Errorf("model[%d] = %q, mau %q", i, models[i].ID, id)
		}
	}
}
