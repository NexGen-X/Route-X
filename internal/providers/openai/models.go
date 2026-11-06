package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// modelsResponse adalah respons GET /v1/models.
type modelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		OwnedBy string `json:"owned_by"`
		Created int64  `json:"created"`
	} `json:"data"`
}

// Models mengambil daftar model yang tersedia di provider.
func (p *Provider) Models(ctx context.Context) ([]providers.ModelInfo, error) {
	resp, doErr := p.do(ctx, http.MethodGet, pathModels, nil, false)
	if doErr != nil {
		return nil, doErr
	}
	defer resp.Body.Close()

	raw, readErr := readBody(p.name, resp.Body)
	if readErr != nil {
		return nil, readErr
	}

	var wire modelsResponse
	if uerr := json.Unmarshal(raw, &wire); uerr != nil {
		return nil, providers.Newf(providers.ErrKindServer, p.name,
			"daftar model upstream bukan JSON yang dikenali")
	}

	// Daftar kosong bukan kegagalan: provider yang belum mengaktifkan satu pun model
	// memang menjawab begitu, dan pemanggil membedakannya dari galat lewat error nil.
	out := make([]providers.ModelInfo, 0, len(wire.Data))
	for _, m := range wire.Data {
		if m.ID == "" {
			// Entri tanpa id tidak bisa dipakai untuk apa pun.
			continue
		}
		out = append(out, providers.ModelInfo{ID: m.ID, OwnedBy: m.OwnedBy, Created: m.Created})
	}
	return out, nil
}

// HealthCheck memeriksa apakah provider bisa dihubungi dan kredensialnya diterima.
//
// Yang dipanggil GET /v1/models, bukan completion pendek: probe berjalan terjadwal untuk
// setiap provider, dan completion — sependek apa pun — menagih pemilik kredensial setiap
// kali dijalankan. Daftar model tidak menagih dan sudah menjawab pertanyaan yang penting:
// upstream hidup, TLS-nya benar, dan kredensialnya masih diterima.
//
// Sesuai kontrak, kegagalan ADALAH hasilnya: tidak ada error yang dikembalikan.
func (p *Provider) HealthCheck(ctx context.Context) providers.HealthResult {
	ctx, cancel := context.WithTimeout(ctx, p.healthTimeout)
	defer cancel()

	start := time.Now()
	resp, doErr := p.do(ctx, http.MethodGet, pathModels, nil, false)
	if doErr != nil {
		return providers.HealthResult{
			Latency:      time.Since(start),
			StatusCode:   doErr.StatusCode,
			ErrorKind:    doErr.Kind,
			ErrorMessage: doErr.Message,
		}
	}
	defer resp.Body.Close()

	// Body kini DIVALIDASI, bukan dibuang (B-214): status 2xx saja pernah
	// menyatakan "sehat" untuk base URL salah yang kebetulan menjawab 200
	// — halaman HTML proksi, atau JSON yang bukan daftar model. Sehat
	// berarti body terurai sebagai daftar model dialek ini. Katalog
	// KOSONG tetap sehat dengan sengaja: daftar kosong bukan kegagalan
	// (lihat Models) — upstream hidup dan kredensial diterima, provider
	// hanya belum mengaktifkan model, dan itu keadaan yang dibedakan
	// pemanggil lewat daftar modelnya sendiri.
	sah := bodyDaftarModelSah(resp.Body)
	// Sisa body tetap dibaca sebagian supaya koneksi bisa dipakai ulang
	// probe berikutnya alih-alih membuka TLS baru setiap kali.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxHealthDrainBytes))
	if !sah {
		return providers.HealthResult{
			Healthy:      false,
			Latency:      time.Since(start),
			StatusCode:   resp.StatusCode,
			ErrorKind:    providers.ErrKindServer,
			ErrorMessage: "respons probe bukan daftar model yang sah",
		}
	}

	// Status non-2xx tidak sampai ke sini: do sudah mengubahnya menjadi error. Termasuk
	// 404 dari provider compatible yang tidak menyediakan /v1/models — provider seperti itu
	// memang tidak bisa diperiksa dengan cara yang murah, dan melaporkannya "sehat" tanpa
	// bukti apa pun lebih berbahaya daripada menandainya untuk diperhatikan operator.
	return providers.HealthResult{
		Healthy:    true,
		Latency:    time.Since(start),
		StatusCode: resp.StatusCode,
	}
}

// maxHealthBodyBytes membatasi body probe yang diurai saat validasi
// health check: daftar model yang sah jauh di bawah batas ini, dan body
// yang lebih besar hampir pasti bukan daftar model.
const maxHealthBodyBytes = 8 << 20

// bodyDaftarModelSah memeriksa body probe health check: JSON berbentuk
// daftar model dialek OpenAI — objek yang field data-nya sebuah larik
// (boleh kosong) (B-214).
func bodyDaftarModelSah(body io.Reader) bool {
	var bentuk struct {
		Data *[]json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(body, maxHealthBodyBytes)).Decode(&bentuk); err != nil {
		return false
	}
	return bentuk.Data != nil
}
