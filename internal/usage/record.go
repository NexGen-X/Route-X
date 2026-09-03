package usage

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Rentang status yang bisa disimpan kolom requests.status_code, dijaga constraint
// requests_status_range.
//
// Ada di satu tempat karena dipakai dua kali: sekali untuk menjepit nilainya, sekali untuk
// memutuskan apakah penjepitan itu perlu dilaporkan. Kalau angkanya ditulis dua kali, salah
// satu bisa berubah sendiri dan penjepitan berjalan tanpa ada yang tahu.
const (
	statusMin = 100
	statusMax = 599
)

// statusTerpakai menjepit status ke rentang yang bisa disimpan.
//
// Status di luar rentang hanya bisa berarti ada jalur keluar di gateway yang lupa
// melaporkan statusnya. Dijepit menjadi 500, bukan dibiarkan menggagalkan penyisipan:
// satu baris log yang statusnya salah masih jauh lebih berguna daripada tidak ada baris
// sama sekali — dan pemanggil melaporkannya ke log supaya jalur yang lupa itu bisa dicari.
func statusTerpakai(status int) int {
	if status < statusMin || status > statusMax {
		return 500
	}
	return status
}

// tokenUsage mengubah token bentuk kanonik menjadi bentuk penagihan.
//
// Lewat providers.Usage supaya penjepitan dan pemisahan bagian bersarang hanya ada satu
// salinannya, di TokensFor.
func (ev *Event) tokenUsage() upstream.TokenUsage {
	return TokensFor(providers.Usage{
		InputTokens:       ev.InputTokens,
		CachedInputTokens: ev.CachedInputTokens,
		OutputTokens:      ev.OutputTokens,
		ReasoningTokens:   ev.ReasoningTokens,
		TotalTokens:       ev.TotalTokens,
	})
}

// record menerjemahkan Event menjadi baris log request.
//
// Field pointer di traffic.Record berarti "tidak diketahui", jadi string kosong menjadi
// NULL — bukan string kosong. Bedanya nyata di dashboard: provider NULL berarti permintaan
// ditolak sebelum satu pun provider terpilih, sementara provider bernama "" adalah baris
// yang tidak bisa dijelaskan.
func (ev *Event) record(biaya upstream.USD) traffic.Record {
	rec := traffic.Record{
		RequestID: ev.RequestID,

		APIKeyID:   nilKosong(ev.APIKeyID),
		APIKeyName: nilKosong(ev.APIKeyName),
		UserID:     nilKosong(ev.UserID),

		Method:         ev.Method,
		Endpoint:       ev.Endpoint,
		RequestedModel: ev.RequestedModel,

		ModelID:       nilKosong(ev.ModelID),
		ModelName:     nilKosong(ev.ModelName),
		ProviderID:    nilKosong(ev.ProviderID),
		ProviderName:  nilKosong(ev.ProviderName),
		UpstreamModel: nilKosong(ev.UpstreamModel),

		StatusCode:   statusTerpakai(ev.StatusCode),
		ErrorType:    nilKosong(ev.ErrorType),
		ErrorCode:    nilKosong(ev.ErrorCode),
		ErrorMessage: nilKosong(ev.ErrorMessage),

		Stream: ev.Stream,

		InputTokens:       max(ev.InputTokens, 0),
		OutputTokens:      max(ev.OutputTokens, 0),
		CachedInputTokens: max(ev.CachedInputTokens, 0),
		ReasoningTokens:   max(ev.ReasoningTokens, 0),
		TotalTokens:       max(ev.TotalTokens, 0),

		CostUSD: max(biaya.Units(), 0),

		LatencyMS:         milidetik(ev.Latency),
		RetryCount:        max(ev.RetryCount, 0),
		FailoverCount:     max(ev.FailoverCount, 0),
		RoutingStrategy:   nilKosong(ev.RoutingStrategy),
		RoutingDecision:   ev.Routing.json(),
		UserAgent:         nilKosong(ev.UserAgent),
		UpstreamLatencyMS: nilNol(milidetik(ev.UpstreamLatency)),
	}

	// TTFT hanya bermakna kalau potongan konten pertama benar-benar terkirim. Nol dicatat
	// sebagai NULL, bukan nol milidetik: "tidak ada token yang pernah sampai" dan "token
	// pertama sampai seketika" adalah dua hal yang berbeda, dan hanya yang kedua boleh ikut
	// menurunkan p95 TTFT.
	if ev.TTFT > 0 {
		rec.TTFTMS = nilNol(milidetik(ev.TTFT))
	}
	if ev.ClientIP.IsValid() {
		ip := ev.ClientIP
		rec.ClientIP = &ip
	}
	return rec
}

// timeline menyusun baris request_events dari jejak percobaan dan catatan kebijakan.
//
// Seq berjalan satu untuk seluruh timeline, bukan per jenis: inspector menampilkannya
// sebagai satu urutan kejadian, dan dua penomoran yang berjalan sendiri-sendiri tidak bisa
// digabung lagi menjadi urutan yang benar.
func (ev *Event) timeline() []traffic.Event {
	if len(ev.Attempts) == 0 && len(ev.Notes) == 0 {
		return nil
	}
	out := make([]traffic.Event, 0, len(ev.Attempts)+len(ev.Notes))
	seq := 0

	for _, a := range ev.Attempts {
		seq++
		e := traffic.Event{
			Seq:           seq,
			ProviderID:    nilKosong(a.ProviderID),
			ProviderName:  nilKosong(a.ProviderName),
			UpstreamModel: nilKosong(a.UpstreamModel),
			StatusCode:    nilNol(a.StatusCode),
			Detail:        detailPercobaan(a),
		}
		switch {
		case a.SkipReason != "":
			// Satu-satunya alasan pelewatan yang bisa dihasilkan mesin eksekusi saat ini
			// adalah pemutus arus yang terbuka. Kalau nanti ada alasan kedua, percabangan
			// ini WAJIB mendapat kasus baru — kalau tidak, pelewatan itu akan tercatat
			// sebagai pemutus arus yang tidak pernah terbuka.
			e.Kind = traffic.EventCircuitOpen
			e.ErrorMessage = nilKosong(a.SkipReason)
		case a.ErrorKind != "":
			e.Kind = traffic.EventAttemptFailed
			e.ErrorKind = nilKosong(a.ErrorKind)
			e.ErrorMessage = nilKosong(a.ErrorMessage)
			e.LatencyMS = nilNol(milidetik(a.Duration))
		default:
			e.Kind = traffic.EventAttemptSucceeded
			e.LatencyMS = nilNol(milidetik(a.Duration))
		}
		out = append(out, e)
	}

	for _, n := range ev.Notes {
		if n.Kind == "" {
			continue
		}
		seq++
		out = append(out, traffic.Event{
			Seq:          seq,
			Kind:         n.Kind,
			ErrorMessage: nilKosong(n.Message),
			Detail:       json.RawMessage("{}"),
		})
	}
	return out
}

// detailPercobaan menyimpan hal-hal yang tidak punya kolom sendiri di request_events.
//
// Nomor percobaan dan state pemutus arus keduanya diperlukan untuk membaca timeline —
// "percobaan kedua pada provider yang sama" dan "percobaan pertama pada provider
// berikutnya" terlihat identik tanpa keduanya — tetapi tidak cukup sering ditanyakan untuk
// pantas mendapat kolom di tabel yang ditulis pada setiap request.
func detailPercobaan(a Attempt) json.RawMessage {
	d := map[string]any{"attempt": a.Nth}
	if a.BreakerState != "" {
		d["breaker"] = a.BreakerState
	}
	b, err := json.Marshal(d)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// json menyusun isi kolom routing_decision.
//
// Mengembalikan nil bila tidak ada apa pun yang bisa dijelaskan, sehingga kolomnya memakai
// nilai default '{}' alih-alih menyimpan objek berisi field kosong.
func (rt Routing) json() json.RawMessage {
	if rt.Strategy == "" && rt.RuleID == "" && len(rt.Candidates) == 0 {
		return nil
	}
	b, err := json.Marshal(rt)
	if err != nil {
		return nil
	}
	return b
}

// catatMetrik mencatat metrik satu permintaan.
//
// Label provider dan model berasal dari registry yang dikurasi operator, jadi jumlahnya
// terbatas. RequestedModel milik Event TIDAK BOLEH masuk ke sini: nilainya datang dari
// klien, dan satu klien yang mengirim nama model acak pada setiap permintaan akan
// menghasilkan satu time series baru setiap kali.
func (r *Recorder) catatMetrik(ev Event) {
	m := r.metrics
	if m == nil {
		return
	}

	m.RequestsTotal.WithLabelValues(ev.ProviderName, ev.ModelName,
		observability.StatusLabel(statusTerpakai(ev.StatusCode)), strconv.FormatBool(ev.Stream)).Inc()

	// Durasi, TTFT, dan token hanya dicatat untuk permintaan yang benar-benar sampai ke
	// provider. Permintaan yang ditolak penyaring konten selesai dalam mikrodetik, dan
	// memasukkannya ke histogram yang sama akan menarik seluruh persentil latensi upstream
	// ke bawah — angka yang lalu tidak menggambarkan apa pun.
	if ev.ProviderName != "" {
		m.RequestDuration.WithLabelValues(ev.ProviderName, ev.ModelName).Observe(ev.Latency.Seconds())
		if ev.TTFT > 0 {
			m.TimeToFirstByte.WithLabelValues(ev.ProviderName, ev.ModelName).Observe(ev.TTFT.Seconds())
		}
		// Hanya input dan output, mengikuti label "direction" pada metriknya. Token cache
		// dan token penalaran adalah BAGIAN dari keduanya, jadi menambahkannya sebagai arah
		// tersendiri membuat penjumlahan seluruh arah menghitungnya dua kali.
		if ev.InputTokens > 0 {
			m.TokensTotal.WithLabelValues(ev.ProviderName, ev.ModelName, "input").Add(float64(ev.InputTokens))
		}
		if ev.OutputTokens > 0 {
			m.TokensTotal.WithLabelValues(ev.ProviderName, ev.ModelName, "output").Add(float64(ev.OutputTokens))
		}
	}

	for i, a := range ev.Attempts {
		if a.ErrorKind != "" {
			m.UpstreamFailures.WithLabelValues(a.ProviderName, a.ErrorKind).Inc()
		}
		if i == 0 {
			continue
		}
		sebelum := ev.Attempts[i-1]
		switch {
		case sebelum.ProviderID != a.ProviderID:
			m.FailoversTotal.WithLabelValues(sebelum.ProviderName, a.ProviderName, alasan(sebelum)).Inc()
		case a.Nth > sebelum.Nth:
			m.RetriesTotal.WithLabelValues(a.ProviderName, alasan(sebelum)).Inc()
		}
	}
}

// alasan mengembalikan sebab satu percobaan berlanjut, sebagai label metrik yang stabil.
//
// Pengenal Inggris yang tetap, bukan SkipReason apa adanya: SkipReason adalah kalimat untuk
// manusia dan bisa diperbaiki kapan saja, sementara label metrik yang berubah memutus
// seluruh grafik yang memakainya.
func alasan(a Attempt) string {
	switch {
	case a.SkipReason != "":
		return "circuit_open"
	case a.ErrorKind != "":
		return a.ErrorKind
	default:
		return "none"
	}
}

// nilKosong mengubah string kosong menjadi nil.
func nilKosong(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nilNol mengubah nol menjadi nil.
func nilNol(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// milidetik membulatkan durasi ke milidetik terdekat, tidak pernah negatif.
//
// Dibulatkan, bukan dipangkas: permintaan 0,6 ms yang tercatat 0 ms membuat seluruh
// permintaan cepat menghilang dari histogram latensi, dan slot pertama histogram menjadi
// tempat sampah yang tidak bisa dibaca.
func milidetik(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	ms := (d + time.Millisecond/2) / time.Millisecond
	return max(int(ms), 0)
}
