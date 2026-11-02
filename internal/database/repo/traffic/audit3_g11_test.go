package traffic

import (
	"testing"
	"time"
)

// TestRencanakanMeratakanJendelaSumberRollup memastikan jendela untuk sumber
// rollup diratakan ke batas bucket (audit3 V-DT-04b): tanpa perataan, syarat
// bucket >= from membuang bucket pertama secara diam-diam sementara bidang
// From pada ringkasan tetap mengklaim jendela aslinya.
func TestRencanakanMeratakanJendelaSumberRollup(t *testing.T) {
	from := time.Date(2026, 9, 3, 12, 30, 15, 0, time.UTC)
	to := time.Date(2026, 9, 10, 6, 15, 0, 0, time.UTC)

	p, err := rencanakan("uji", Query{From: from, To: to, Source: SourceDaily, Granularity: GranularityDay})
	if err != nil {
		t.Fatalf("rencanakan harian: %v", err)
	}
	if mau := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC); !p.from.Equal(mau) {
		t.Errorf("from harian = %v, mau %v (awal hari)", p.from, mau)
	}
	if mau := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC); !p.to.Equal(mau) {
		t.Errorf("to harian = %v, mau %v (awal hari sesudahnya)", p.to, mau)
	}

	toJam := time.Date(2026, 9, 4, 6, 15, 0, 0, time.UTC)
	p, err = rencanakan("uji", Query{From: from, To: toJam, Source: SourceHourly, Granularity: GranularityHour})
	if err != nil {
		t.Fatalf("rencanakan per jam: %v", err)
	}
	if mau := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC); !p.from.Equal(mau) {
		t.Errorf("from per jam = %v, mau %v (awal jam)", p.from, mau)
	}
	if mau := time.Date(2026, 9, 4, 7, 0, 0, 0, time.UTC); !p.to.Equal(mau) {
		t.Errorf("to per jam = %v, mau %v (awal jam sesudahnya)", p.to, mau)
	}

	// Jendela yang sudah rata tidak berubah.
	p, err = rencanakan("uji", Query{
		From: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		Source: SourceDaily, Granularity: GranularityDay,
	})
	if err != nil {
		t.Fatalf("rencanakan harian rata: %v", err)
	}
	if !p.from.Equal(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)) || !p.to.Equal(time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("jendela rata berubah menjadi [%v..%v]", p.from, p.to)
	}

	// Sumber mentah TIDAK diratakan: baris requests bertanda waktu persis.
	p, err = rencanakan("uji", Query{From: from, To: to, Source: SourceRequests, Granularity: GranularityDay})
	if err != nil {
		t.Fatalf("rencanakan mentah: %v", err)
	}
	if !p.from.Equal(from) || !p.to.Equal(to) {
		t.Errorf("jendela mentah berubah menjadi [%v..%v]", p.from, p.to)
	}
}

// TestLatencyByProviderModelMengabaikanStreamTerputus mereproduksi
// audit3 V-DT-24: baris status 200 dengan error_type terisi (stream terputus
// di tengah) bukan keberhasilan. Bila ikut dihitung, sampel "sukses" 1 ms
// dari aliran yang putus menyeret p95 dan memenuhi ambang sampel minimum
// dengan data yang salah.
func TestLatencyByProviderModelMengabaikanStreamTerputus(t *testing.T) {
	ctx, pool, r := testEnv(t)

	var providerID, modelID, pmID string
	if err := pool.QueryRow(ctx, `
		insert into providers (name, display_name, kind, base_url)
		values ('smoke', 'Smoke', 'openai_compatible', 'http://127.0.0.1:8123') returning id::text`).
		Scan(&providerID); err != nil {
		t.Fatalf("menyisipkan provider: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		insert into models (model_id, display_name) values ('gpt-5', 'GPT-5') returning id::text`).
		Scan(&modelID); err != nil {
		t.Fatalf("menyisipkan model: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		insert into provider_models (model_id, provider_id, upstream_model_name)
		values ($1, $2, 'gpt-5-2026') returning id::text`, modelID, providerID).Scan(&pmID); err != nil {
		t.Fatalf("menyisipkan pemetaan: %v", err)
	}

	baru := time.Now().Add(-5 * time.Minute)
	ttftOK := 50
	sisipRequest(t, ctx, pool, sisip{
		prefix: "ok", jumlah: 20, at: baru,
		providerID: providerID, providerName: "smoke", modelID: modelID, modelName: "gpt-5",
		status: 200, stream: true, total: 10, latencyMS: 30000, ttftMS: &ttftOK,
	})
	// Stream terputus: status 200 tetapi error_type terisi, ttft 1 ms.
	ttftPutus := 1
	sisipRequest(t, ctx, pool, sisip{
		prefix: "putus", jumlah: 15, at: baru,
		providerID: providerID, providerName: "smoke", modelID: modelID, modelName: "gpt-5",
		status: 200, errorType: "stream_aborted", stream: true, total: 10, latencyMS: 1, ttftMS: &ttftPutus,
	})

	// Hanya 20 sampel yang sah: ambang 30 tidak boleh terpenuhi oleh baris error.
	got, err := r.LatencyByProviderModel(ctx, time.Hour, 0.95, 30)
	if err != nil {
		t.Fatalf("LatencyByProviderModel: %v", err)
	}
	if _, ok := got[pmID]; ok {
		t.Error("pemetaan muncul dengan ambang 30 sampel padahal hanya 20 sampel sukses — baris error ikut terhitung")
	}

	got, err = r.LatencyByProviderModel(ctx, time.Hour, 0.95, 10)
	if err != nil {
		t.Fatalf("LatencyByProviderModel: %v", err)
	}
	if d, ok := got[pmID]; !ok || d != 50*time.Millisecond {
		t.Errorf("latensi pemetaan = %v (%v), mau 50ms dari sampel sukses saja", d, ok)
	}
}
