// Package gateway memuat perkakas jalur request. Berkas ini adalah codec permukaan API:
// menguraikan body bergaya OpenAI dari klien menjadi bentuk kanonik internal/providers,
// lalu menyimpulkan dua hal yang dibutuhkan mesin routing sebelum satu provider dipilih —
// kemampuan apa yang benar-benar dituntut permintaan, dan berapa besar perkiraan tokennya.
//
// # Arah yang berlawanan dengan adapter openai
//
// internal/providers/openai menyerialkan bentuk kanonik KEMBALI ke wire format. Berkas ini
// arah sebaliknya, dan keduanya wajib sepakat pada satu pembagian: field yang punya tempat
// di struct kanonik diurai ke tempat itu, dan HANYA field yang tidak dikenal masuk ke
// Extra. Salah tempat menghasilkan salah satu dari dua kegagalan, keduanya senyap: field
// yang ikut masuk Extra padahal sudah punya tempat akan dikirim dua kali ke upstream,
// sedangkan field yang tidak masuk keduanya hilang tanpa jejak.
//
// Invarian itu tidak dijaga oleh disiplin melainkan oleh bentuk kodenya: seluruh field
// tingkat atas dibaca ke dalam satu peta, setiap field yang dikenal DIKELUARKAN dari peta
// saat diurai, dan apa yang tersisa di peta itulah Extra. Menambah field kanonik baru
// tanpa mengeluarkannya dari peta menjadi mustahil, karena satu-satunya cara membacanya
// adalah lewat ambil() yang sekaligus menghapusnya.
//
// # Pesan error di berkas ini akan dibaca pengguna API
//
// Semuanya menjadi badan 400. Karena itu pesannya hanya menyebut NAMA field yang
// bermasalah, tidak pernah nilainya: body permintaan memuat data milik penyewa yang
// mengirimnya, dan pesan error ikut ke log, metrik, dan tiket dukungan yang dibaca orang
// lain.
package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// Nama field tingkat atas yang punya tempat di struct kanonik. Dikumpulkan sebagai
// konstanta karena masing-masing dipakai dua kali — sekali untuk diambil, sekali di pesan
// error — dan salah tulis di salah satunya menghasilkan field yang diam-diam pindah ke
// Extra lalu dikirim dua kali.
const (
	fModel             = "model"
	fMessages          = "messages"
	fMaxTokens         = "max_tokens"
	fTemperature       = "temperature"
	fTopP              = "top_p"
	fStop              = "stop"
	fStream            = "stream"
	fTools             = "tools"
	fToolChoice        = "tool_choice"
	fParallelToolCalls = "parallel_tool_calls"
	fResponseFormat    = "response_format"
	fSeed              = "seed"
	fReasoningEffort   = "reasoning_effort"
	fUser              = "user"

	fInput          = "input"
	fDimensions     = "dimensions"
	fEncodingFormat = "encoding_format"
)

// DecodeChatRequest mengurai body POST /v1/chat/completions menjadi bentuk kanonik.
//
// Field yang belum dikenal gateway TIDAK ditolak dan tidak dibuang: ia diteruskan apa
// adanya lewat ChatRequest.Extra. Itu yang membuat parameter baru dari provider bisa
// dipakai pada hari provider merilisnya, tanpa menunggu gateway diperbarui.
func DecodeChatRequest(body []byte) (*providers.ChatRequest, error) {
	fields, err := bidangTingkatAtas(body)
	if err != nil {
		return nil, err
	}

	out := &providers.ChatRequest{}

	if raw, ok := ambil(fields, fModel); ok {
		if err := uraikanKe(raw, &out.Model, fModel); err != nil {
			return nil, err
		}
	}
	if out.Model == "" {
		return nil, fmt.Errorf("field %s wajib diisi", kutip(fModel))
	}

	raw, ok := ambil(fields, fMessages)
	if !ok {
		return nil, fmt.Errorf("field %s wajib diisi", kutip(fMessages))
	}
	if out.Messages, err = uraikanPesan(raw); err != nil {
		return nil, err
	}

	if raw, ok := ambil(fields, fMaxTokens); ok {
		if err := uraikanKe(raw, &out.MaxTokens, fMaxTokens); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fTemperature); ok {
		if err := uraikanKe(raw, &out.Temperature, fTemperature); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fTopP); ok {
		if err := uraikanKe(raw, &out.TopP, fTopP); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fStop); ok {
		if out.Stop, err = uraikanStop(raw); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fStream); ok {
		if err := uraikanKe(raw, &out.Stream, fStream); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fTools); ok {
		if out.Tools, err = uraikanTools(raw); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fToolChoice); ok {
		out.ToolChoice = bytes.Clone(raw)
	}
	if raw, ok := ambil(fields, fParallelToolCalls); ok {
		if err := uraikanKe(raw, &out.ParallelToolCalls, fParallelToolCalls); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fResponseFormat); ok {
		out.ResponseFormat = bytes.Clone(raw)
	}
	if raw, ok := ambil(fields, fSeed); ok {
		if err := uraikanKe(raw, &out.Seed, fSeed); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fReasoningEffort); ok {
		if err := uraikanKe(raw, &out.ReasoningEffort, fReasoningEffort); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fUser); ok {
		if err := uraikanKe(raw, &out.User, fUser); err != nil {
			return nil, err
		}
	}

	// Sisa peta ADALAH Extra. Field yang sudah punya tempat di atas tidak mungkin ada di
	// sini karena ambil() menghapusnya, dan adapter openai menolak Extra menimpa field
	// yang sudah diisi gateway — dua penjaga untuk satu invarian, karena akibat
	// bocornya (satu field dikirim dua kali) tidak menghasilkan error di mana pun.
	//
	// Keputusan sengaja untuk beberapa field yang sering ditanyakan:
	//
	//   - "n" DITERUSKAN, bukan ditolak. Respons upstream diteruskan apa adanya ke klien,
	//     jadi seluruh choice sampai; dan usage yang menjadi dasar biaya juga datang dari
	//     upstream, jadi tagihannya tetap benar. Yang perlu penyesuaian hanya perkiraan
	//     token untuk routing, dan EstimateTokens membacanya dari sini.
	//   - "stream_options" DITERUSKAN. Untuk kind openai, gateway mengisinya sendiri
	//     (include_usage wajib supaya biaya request streaming bisa dihitung) dan nilai
	//     dari klien kalah. Untuk kind openai_compatible dan custom, gateway sengaja
	//     tidak mengisinya, jadi inilah satu-satunya jalan bagi pemanggil yang tahu
	//     upstream-nya mendukung. Alasan lengkapnya di openai.wantsStreamUsage.
	//   - "max_completion_tokens" DITERUSKAN dan sengaja TIDAK dipetakan ke MaxTokens:
	//     model penalaran OpenAI menolak "max_tokens", jadi memetakannya berarti mengubah
	//     permintaan yang sah menjadi 400 dari upstream. Nilainya tetap dibaca
	//     EstimateTokens sebagai batas atas keluaran.
	if len(fields) > 0 {
		out.Extra = fields
	}
	return out, nil
}

// DecodeEmbeddingsRequest mengurai body POST /v1/embeddings menjadi bentuk kanonik.
func DecodeEmbeddingsRequest(body []byte) (*providers.EmbeddingsRequest, error) {
	fields, err := bidangTingkatAtas(body)
	if err != nil {
		return nil, err
	}

	out := &providers.EmbeddingsRequest{}

	if raw, ok := ambil(fields, fModel); ok {
		if err := uraikanKe(raw, &out.Model, fModel); err != nil {
			return nil, err
		}
	}
	if out.Model == "" {
		return nil, fmt.Errorf("field %s wajib diisi", kutip(fModel))
	}

	raw, ok := ambil(fields, fInput)
	if !ok {
		return nil, fmt.Errorf("field %s wajib diisi", kutip(fInput))
	}
	if out.Input, err = uraikanMasukanEmbedding(raw); err != nil {
		return nil, err
	}

	if raw, ok := ambil(fields, fDimensions); ok {
		if err := uraikanKe(raw, &out.Dimensions, fDimensions); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fUser); ok {
		if err := uraikanKe(raw, &out.User, fUser); err != nil {
			return nil, err
		}
	}
	if raw, ok := ambil(fields, fEncodingFormat); ok {
		if err := uraikanKe(raw, &out.EncodingFormat, fEncodingFormat); err != nil {
			return nil, err
		}
	}

	if len(fields) > 0 {
		out.Extra = fields
	}
	return out, nil
}

// --- Pembacaan field tingkat atas --------------------------------------------

// bidangTingkatAtas membaca body sebagai objek JSON tanpa menafsirkan isinya.
//
// json.RawMessage per field, bukan struct dengan tag: struct akan MEMBUANG setiap field
// yang tidak dideklarasikan, dan justru field itulah yang harus diteruskan ke upstream.
func bidangTingkatAtas(body []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("body permintaan kosong")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		// Pesan dari encoding/json memuat potongan body, jadi tidak diteruskan.
		return nil, errors.New("body permintaan bukan objek JSON yang sah")
	}
	if fields == nil {
		// Body "null": JSON yang sah, objek yang bukan.
		return nil, errors.New("body permintaan bukan objek JSON yang sah")
	}
	return fields, nil
}

// ambil mengeluarkan satu field dari kumpulan field tingkat atas.
//
// Mengeluarkan, bukan sekadar membaca: apa yang tersisa di peta setelah seluruh field
// kanonik diambil ADALAH Extra, jadi penghapusannya bagian dari pembacaan, bukan
// pembersihan sesudahnya.
//
// Nilai null dilaporkan sebagai tidak ada sekaligus dihapus. Field kanonik tidak punya
// cara menyatakan "null" — MaxTokens nil dan max_tokens:null adalah hal yang sama bagi
// upstream — dan meneruskan null lewat Extra untuk field yang punya tempat justru
// mengirimnya dua kali.
func ambil(fields map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	raw, ok := fields[key]
	delete(fields, key)
	if !ok || adalahNull(raw) {
		return nil, false
	}
	return raw, true
}

// adalahNull melaporkan apakah nilai mentah ini kosong atau literal null.
func adalahNull(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || bytes.Equal(t, []byte("null"))
}

// uraikanKe mengurai satu field ke tujuannya dengan pesan gagal yang menyebut field-nya.
func uraikanKe(raw json.RawMessage, dst any, nama string) error {
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("field %s bentuknya tidak sesuai", kutip(nama))
	}
	return nil
}

// uraikanStop menerima kedua bentuk yang dipakai dialek OpenAI: satu string atau array.
func uraikanStop(raw json.RawMessage) ([]string, error) {
	var satu string
	if json.Unmarshal(raw, &satu) == nil {
		return []string{satu}, nil
	}
	var banyak []string
	if json.Unmarshal(raw, &banyak) == nil {
		return banyak, nil
	}
	return nil, fmt.Errorf("field %s harus berupa string atau array string", kutip(fStop))
}

// uraikanTools mengurai daftar tool dan menolak yang tidak bisa dipakai model.
//
// type kosong sengaja dibiarkan: adapter openai mengisinya dengan "function", dan
// mengisinya di dua tempat berarti dua tempat yang harus diubah ketika dialeknya menambah
// jenis tool baru.
func uraikanTools(raw json.RawMessage) ([]providers.Tool, error) {
	var tools []providers.Tool
	if err := json.Unmarshal(raw, &tools); err != nil {
		return nil, fmt.Errorf("field %s bentuknya tidak sesuai", kutip(fTools))
	}
	for i := range tools {
		if tools[i].Function.Name == "" {
			return nil, fmt.Errorf("field %s wajib diisi", kutip(fmt.Sprintf("%s[%d].function.name", fTools, i)))
		}
	}
	return tools, nil
}

// uraikanMasukanEmbedding menerima satu string atau array string.
//
// Bentuk ketiga yang dikenal dialek OpenAI — array token yang sudah dikodekan — DITOLAK,
// bukan diteruskan. Bentuk kanonik memuat teks, dan sebagian provider tujuan (Gemini)
// hanya menerima teks; token hanya bermakna terhadap tokenizer model yang menghasilkannya,
// sehingga meneruskannya ke provider lain menghasilkan vektor untuk teks yang berbeda —
// kegagalan yang tidak memunculkan error apa pun, hanya hasil yang salah.
func uraikanMasukanEmbedding(raw json.RawMessage) ([]string, error) {
	var satu string
	if json.Unmarshal(raw, &satu) == nil {
		if satu == "" {
			return nil, fmt.Errorf("field %s wajib diisi", kutip(fInput))
		}
		return []string{satu}, nil
	}

	var banyak []string
	if err := json.Unmarshal(raw, &banyak); err != nil {
		return nil, fmt.Errorf("field %s harus berupa string atau array string; array token belum didukung", kutip(fInput))
	}
	if len(banyak) == 0 {
		return nil, fmt.Errorf("field %s wajib diisi", kutip(fInput))
	}
	for i, s := range banyak {
		if s == "" {
			return nil, fmt.Errorf("field %s wajib diisi", kutip(fmt.Sprintf("%s[%d]", fInput, i)))
		}
	}
	return banyak, nil
}

// --- Pesan -------------------------------------------------------------------

// pesanMasuk adalah satu pesan seperti dikirim klien.
//
// Content dan Arguments dibiarkan mentah karena keduanya punya lebih dari satu bentuk yang
// sah di lapangan, dan bentuk yang salah harus bisa dilaporkan dengan menyebut posisinya,
// bukan menggagalkan pembacaan seluruh array pesan.
//
// Batas yang diketahui: field TAK DIKENAL di dalam satu pesan tidak diteruskan. Bentuk
// kanonik providers.Message tidak punya tempat untuk itu, dan menambahkannya berarti
// setiap adapter harus memutuskan cara menerjemahkannya per pesan. Yang hilang dalam
// praktik adalah field KELUARAN yang ikut terkirim balik ketika klien mengulang riwayat
// (mis. "refusal", "annotations", "audio") — OpenAI sendiri mengabaikannya pada permintaan
// masuk. Field masukan per pesan milik dialek lain, seperti "cache_control" milik
// Anthropic, memang belum bisa dipakai lewat permukaan ini.
type pesanMasuk struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Name       string          `json:"name"`
	ToolCalls  []toolCallMasuk `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
}

// toolCallMasuk adalah satu tool call dalam riwayat yang dikirim ulang klien.
type toolCallMasuk struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
		// Mentah supaya bentuk yang salah bisa dilaporkan dengan tepat. Di dialek OpenAI
		// arguments adalah STRING berisi JSON, bukan objek — mengirim objek adalah
		// kekeliruan yang sering terjadi, dan pesan "field ... harus berupa string JSON"
		// jauh lebih menolong daripada kegagalan pada seluruh array messages.
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// uraikanPesan mengurai array messages menjadi pesan kanonik.
func uraikanPesan(raw json.RawMessage) ([]providers.Message, error) {
	var masuk []pesanMasuk
	if err := json.Unmarshal(raw, &masuk); err != nil {
		return nil, fmt.Errorf("field %s bentuknya tidak sesuai", kutip(fMessages))
	}
	if len(masuk) == 0 {
		return nil, fmt.Errorf("field %s wajib memuat setidaknya satu pesan", kutip(fMessages))
	}

	out := make([]providers.Message, 0, len(masuk))
	for i := range masuk {
		m := &masuk[i]
		jalur := fmt.Sprintf("%s[%d]", fMessages, i)

		if m.Role == "" {
			return nil, fmt.Errorf("field %s wajib diisi", kutip(jalur+".role"))
		}
		// Nilai role yang tidak dikenal diteruskan apa adanya. Daftar role bertambah
		// (RoleDeveloper adalah tambahan yang belum lama), dan menolak yang belum ada di
		// daftar kami berarti gateway ini basi lebih dulu daripada provider di belakangnya.
		pesan := providers.Message{
			Role:       providers.Role(m.Role),
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}

		adaKonten, err := uraikanKonten(&pesan, m.Content, jalur+".content")
		if err != nil {
			return nil, err
		}

		for j := range m.ToolCalls {
			tc := &m.ToolCalls[j]
			jalurTC := fmt.Sprintf("%s.tool_calls[%d]", jalur, j)
			if tc.Function.Name == "" {
				return nil, fmt.Errorf("field %s wajib diisi", kutip(jalurTC+".function.name"))
			}
			var args string
			if !adalahNull(tc.Function.Arguments) {
				if json.Unmarshal(tc.Function.Arguments, &args) != nil {
					return nil, fmt.Errorf("field %s harus berupa string JSON", kutip(jalurTC+".function.arguments"))
				}
			}
			pesan.ToolCalls = append(pesan.ToolCalls, providers.ToolCall{
				// Index diisi dari posisi. Adapter openai memang tidak mengirimkannya balik
				// ke upstream, tetapi bentuk kanonik yang konsisten membuat pembandingan
				// hasil decode dengan hasil parse respons bisa dilakukan apa adanya.
				Index: j,
				ID:    tc.ID,
				Type:  tc.Type,
				Function: providers.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: args,
				},
			})
		}

		// Pesan yang tidak membawa apa pun ditolak: ia tidak punya arti bagi model, dan
		// meneruskannya berarti membiarkan upstream menjawab 400 dengan pesannya sendiri
		// setelah kandidat pertama dihubungi — biaya jaringan untuk kesalahan yang sudah
		// terlihat di sini. "content": "" DITERIMA, karena string kosong yang dikirim
		// sengaja berbeda dari field yang tidak ada, dan pesan asisten yang isinya hanya
		// tool call memang tidak punya konten.
		if !adaKonten && len(pesan.ToolCalls) == 0 {
			return nil, fmt.Errorf("field %s wajib memuat content atau tool_calls", kutip(jalur))
		}
		out = append(out, pesan)
	}
	return out, nil
}

// uraikanKonten mengisi Content atau Parts dari nilai "content" mentah.
//
// Nilai balik pertama melaporkan apakah field-nya benar-benar membawa nilai — dipakai
// pemanggil untuk membedakan konten kosong yang dikirim sengaja dari konten yang tidak ada.
func uraikanKonten(msg *providers.Message, raw json.RawMessage, jalur string) (bool, error) {
	if adalahNull(raw) {
		return false, nil
	}

	var teks string
	if json.Unmarshal(raw, &teks) == nil {
		msg.Content = teks
		return true, nil
	}

	var bagian []json.RawMessage
	if err := json.Unmarshal(raw, &bagian); err != nil {
		return false, fmt.Errorf("field %s harus berupa string atau array bagian konten", kutip(jalur))
	}
	msg.Parts = make([]providers.ContentPart, 0, len(bagian))
	for i, b := range bagian {
		p, err := uraikanBagian(b, fmt.Sprintf("%s[%d]", jalur, i))
		if err != nil {
			return false, err
		}
		msg.Parts = append(msg.Parts, p)
	}
	return true, nil
}

// uraikanBagian mengurai satu bagian konten multimodal.
//
// Jenis bagian yang tidak dikenal DITOLAK, tidak dilewati. Melewatinya berarti prompt yang
// sampai ke model bukan prompt yang dikirim pengguna, dan jawabannya tetap datang dengan
// status 200 — pengguna menerima jawaban atas pertanyaan yang tidak pernah ia ajukan, dan
// tidak ada satu pun tanda di mana pun bahwa sesuatu hilang. Penolakan yang jelas jauh
// lebih murah, meski harganya adalah jenis bagian baru harus ditambahkan di sini dulu.
//
// Nilai type yang bermasalah tidak dikutip di pesan error, hanya jalurnya. Nilai itu
// berasal dari body pengguna, dan pesan ini ikut ke log bersama.
func uraikanBagian(raw json.RawMessage, jalur string) (providers.ContentPart, error) {
	var b struct {
		Type     string              `json:"type"`
		Text     *string             `json:"text"`
		ImageURL *providers.ImageURL `json:"image_url"`
		Audio    *providers.Audio    `json:"input_audio"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return providers.ContentPart{}, fmt.Errorf("field %s bentuknya tidak sesuai", kutip(jalur))
	}

	switch b.Type {
	case providers.PartTypeText:
		if b.Text == nil {
			return providers.ContentPart{}, fmt.Errorf("field %s wajib diisi", kutip(jalur+".text"))
		}
		return providers.ContentPart{Type: b.Type, Text: *b.Text}, nil

	case providers.PartTypeImageURL:
		if b.ImageURL == nil || b.ImageURL.URL == "" {
			return providers.ContentPart{}, fmt.Errorf("field %s wajib diisi", kutip(jalur+".image_url.url"))
		}
		return providers.ContentPart{Type: b.Type, ImageURL: b.ImageURL}, nil

	case providers.PartTypeAudio:
		if b.Audio == nil || b.Audio.Data == "" {
			return providers.ContentPart{}, fmt.Errorf("field %s wajib diisi", kutip(jalur+".input_audio.data"))
		}
		if b.Audio.Format == "" {
			return providers.ContentPart{}, fmt.Errorf("field %s wajib diisi", kutip(jalur+".input_audio.format"))
		}
		return providers.ContentPart{Type: b.Type, Audio: b.Audio}, nil

	case "":
		return providers.ContentPart{}, fmt.Errorf("field %s wajib diisi", kutip(jalur+".type"))

	default:
		return providers.ContentPart{}, fmt.Errorf(
			"field %s memuat jenis bagian konten yang belum didukung gateway ini", kutip(jalur+".type"))
	}
}

// --- Kemampuan yang dituntut permintaan --------------------------------------

// RequiredCapabilities menyimpulkan kemampuan yang BENAR-BENAR dibutuhkan permintaan ini.
//
// Disimpulkan dari ISI permintaan, bukan dari kemampuan yang dimiliki modelnya. Hasilnya
// dipakai mesin routing untuk MENYARING kandidat, dan arah kesalahannya tidak simetris:
// menyimpulkan kelebihan berarti provider yang sebenarnya sanggup ikut tersaring dan
// permintaan ditolak 503 padahal ada yang bisa melayaninya, sedangkan menyimpulkan
// kekurangan berarti permintaan sampai ke provider yang menolaknya dengan error miliknya
// sendiri — yang masih bisa dialihkan ke kandidat berikutnya.
//
// Karena itu setiap penyimpulan di sini menuntut BUKTI di dalam body, dan yang meragukan
// dibiarkan lewat:
//
//   - tool_choice "auto" TIDAK menuntut kemampuan tool. Nilainya berarti "pakai kalau ada",
//     dan banyak klien mengirimnya sebagai bawaan pada percakapan tanpa satu pun tool.
//   - Bagian audio TIDAK menuntut apa pun, karena skema kemampuan di migrasi 0003 belum
//     punya nilai untuk audio (hanya text, vision, reasoning, tools, embeddings). Memetakan
//     audio ke vision akan salah di kedua arah: model vision tidak otomatis mendengar, dan
//     model audio tidak otomatis melihat. Akibat yang diterima: permintaan audio bisa
//     dirutekan ke provider yang tidak mendukungnya dan ditolak di sana. Menutupnya butuh
//     migrasi yang menambah nilai kemampuan baru, bukan tambalan di berkas ini.
func RequiredCapabilities(req *providers.ChatRequest) []string {
	// CapText selalu ada: permintaan chat apa pun menghasilkan teks, dan kandidat yang
	// tidak mendukung teks tidak pernah menjadi jawaban yang benar untuk endpoint ini.
	caps := []string{upstream.CapText}
	if req == nil {
		return caps
	}
	if adaGambar(req.Messages) {
		caps = append(caps, upstream.CapVision)
	}
	if len(req.Tools) > 0 || toolChoiceMemaksa(req.ToolChoice) {
		caps = append(caps, upstream.CapTools)
	}
	if req.ReasoningEffort != "" {
		caps = append(caps, upstream.CapReasoning)
	}
	return caps
}

// adaGambar melaporkan apakah ada satu pun bagian gambar di seluruh pesan.
func adaGambar(msgs []providers.Message) bool {
	for i := range msgs {
		for _, p := range msgs[i].Parts {
			if p.Type == providers.PartTypeImageURL {
				return true
			}
		}
	}
	return false
}

// toolChoiceMemaksa melaporkan apakah tool_choice menuntut model memanggil tool.
//
// Diperiksa terpisah dari daftar tools karena keduanya bisa tidak sejalan: tool_choice yang
// memaksa tanpa tools adalah permintaan yang akan ditolak upstream, dan menuntut kemampuan
// tool untuk permintaan seperti itu tidak merugikan siapa pun — sebaliknya, tools yang
// dikirim bersama tool_choice "none" tetap menuntut kemampuan tool, karena definisinya tetap
// ikut terkirim dan model yang tidak mengenal field itu menolak seluruh permintaan.
func toolChoiceMemaksa(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return false
	}
	if t[0] == '{' {
		// Bentuk objek selalu menunjuk satu tool tertentu.
		return true
	}
	var s string
	if json.Unmarshal(t, &s) == nil {
		return s != "none" && s != "auto"
	}
	return false
}

// --- Perkiraan token ---------------------------------------------------------

// Angka-angka perkiraan token. Semuanya perkiraan, dan komentar di EstimateTokens
// menjelaskan apa akibat kesalahannya ke mana.
const (
	// bytePerToken adalah rasio kasar byte UTF-8 per token.
	//
	// BYTE, bukan karakter. Untuk teks Latin keduanya sama, tetapi untuk tulisan non-Latin
	// jumlah byte jauh lebih dekat ke jumlah token BPE daripada jumlah karakter: satu
	// karakter CJK memakan tiga byte dan biasanya menjadi satu sampai dua token, sehingga
	// byte/4 menghasilkan angka yang seukuran, sedangkan karakter/4 menghasilkan seperempat
	// dari yang sebenarnya. Gateway ini melayani permintaan berbahasa apa pun, jadi yang
	// dipilih adalah rasio yang salahnya paling merata, bukan yang paling tepat untuk
	// bahasa Inggris.
	bytePerToken = 4

	// overheadPesan adalah biaya pembungkus satu pesan (penanda peran dan pemisah).
	// Empat token per pesan adalah angka yang dipakai OpenAI sendiri saat menjelaskan cara
	// menghitung token percakapan. Kecil per pesan, tetapi percakapan panjang berisi
	// ratusan pesan pendek akan salah hitung besar tanpa ini.
	overheadPesan = 4

	// overheadPermintaan adalah biaya pembungkus seluruh percakapan.
	overheadPermintaan = 3

	// tokenGambarDetailRendah adalah biaya satu gambar pada detail "low": angka tetap,
	// tidak bergantung ukuran gambar.
	tokenGambarDetailRendah = 85

	// tokenGambarDetailTinggi adalah biaya satu gambar 1024x1024 pada detail "high":
	// 85 dasar ditambah 170 per petak 512x512, empat petak. Gambar yang lebih besar lebih
	// mahal, tetapi ukurannya tidak bisa dibaca dari body — untuk data URI base64 ia harus
	// didekode dulu, dan untuk URL biasa ia harus diunduh lebih dulu, keduanya di jalur
	// request yang sedang menunggu.
	tokenGambarDetailTinggi = 765

	// tokenAudioMinimum adalah LANTAI, bukan pengukuran.
	//
	// Biaya audio ditentukan durasinya, dan durasi tidak ada di wire format — yang ada
	// hanya base64. Menghitung base64 itu sebagai teks akan menghasilkan ratusan ribu token
	// untuk satu berkas pendek dan menyaring habis semua kandidat, jadi yang dipakai angka
	// tetap yang kecil. Akibat yang diterima: permintaan audio bisa dinilai terlalu kecil
	// dan lolos ke jendela konteks yang tidak cukup, lalu ditolak upstream dengan errornya
	// sendiri. Fase 9 mencatat usage nyata, dan di sanalah rasio yang bisa dipertanggung-
	// jawabkan bisa diturunkan.
	tokenAudioMinimum = 100

	// perkiraanOutputBawaan dipakai ketika permintaan tidak menyebut batas keluaran.
	//
	// Nol akan salah dengan cara yang paling mahal: StrategyLowestCost hanya membandingkan
	// harga input, sehingga provider dengan input murah dan output mahal selalu menang untuk
	// setiap permintaan tanpa max_tokens — yaitu mayoritas permintaan. Jendela konteks
	// penuh juga salah ke arah lain: setiap permintaan tampak semahal mungkin dan urutannya
	// terbalik seluruhnya. Angka sedang menjaga kedua harga tetap ikut dihitung.
	perkiraanOutputBawaan = 512

	// maksSalinanOutput membatasi pengali dari "n". 128 adalah batas yang dipakai dialek
	// OpenAI sendiri; tanpa batas, satu angka besar dari klien membuat perkiraan biaya
	// meledak dan seluruh urutan lowest_cost ditentukan oleh field yang tidak diperiksa.
	maksSalinanOutput = 128
)

// EstimateTokens memperkirakan pemakaian token permintaan ini.
//
// Ini PERKIRAAN, dan sengaja tidak berpura-pura lain. Tokenizer yang tepat berbeda per
// keluarga model; menanamnya berarti satu tabel yang harus diperbarui setiap kali provider
// merilis model baru, dan tabel yang basi memberi angka yang salah dengan percaya diri.
//
// Nilainya dipakai dua tempat, dengan arah kesalahan yang berlawanan: StrategyLowestCost
// memakainya untuk menimbang harga input terhadap harga output, dan penyaring jendela
// konteks memakainya untuk membuang kandidat yang tidak cukup besar. Perkiraan yang terlalu
// besar membuang kandidat yang sebenarnya sanggup; yang terlalu kecil mengirim permintaan
// ke jendela yang tidak cukup dan ditolak upstream.
func EstimateTokens(req *providers.ChatRequest) router.TokenEstimate {
	if req == nil {
		return router.TokenEstimate{}
	}

	masuk := overheadPermintaan
	for i := range req.Messages {
		masuk += perkiraanTokenPesan(&req.Messages[i])
	}

	// Definisi tool benar-benar ikut terkirim ke model dan benar-benar ditagih. Mengabaikannya
	// membuat setiap permintaan agen — yang skema tool-nya sering lebih panjang daripada
	// percakapannya — dinilai jauh lebih kecil daripada kenyataan.
	if len(req.Tools) > 0 {
		if b, err := json.Marshal(req.Tools); err == nil {
			masuk += bagiKeAtas(len(b), bytePerToken)
		}
	}
	// Skema response_format juga ikut terkirim.
	masuk += bagiKeAtas(len(req.ResponseFormat), bytePerToken)

	keluar := perkiraanOutputBawaan
	switch {
	case req.MaxTokens != nil && *req.MaxTokens > 0:
		// Batas dari klien adalah satu-satunya angka yang benar-benar diketahui.
		keluar = *req.MaxTokens
	default:
		// Model penalaran OpenAI memakai nama lain untuk batas yang sama, dan codec
		// meneruskannya lewat Extra alih-alih memetakannya (lihat DecodeChatRequest).
		if n, ok := angkaExtra(req.Extra, "max_completion_tokens"); ok && n > 0 {
			keluar = n
		}
	}
	if n, ok := angkaExtra(req.Extra, "n"); ok && n > 1 {
		if n > maksSalinanOutput {
			n = maksSalinanOutput
		}
		keluar *= n
	}

	return router.TokenEstimate{InputTokens: masuk, OutputTokens: keluar}
}

// perkiraanTokenPesan memperkirakan token satu pesan.
func perkiraanTokenPesan(m *providers.Message) int {
	total := overheadPesan + bagiKeAtas(len(m.Role)+len(m.Name)+len(m.ToolCallID), bytePerToken)

	if !m.IsMultimodal() {
		total += bagiKeAtas(len(m.Content), bytePerToken)
	} else {
		for _, p := range m.Parts {
			switch p.Type {
			case providers.PartTypeText:
				total += bagiKeAtas(len(p.Text), bytePerToken)
			case providers.PartTypeImageURL:
				// Panjang p.ImageURL.URL sengaja TIDAK dihitung. Untuk data URI base64 ia
				// bisa mencapai megabyte, dan menghitungnya sebagai teks menghasilkan
				// ratusan ribu token untuk satu gambar — cukup untuk menyaring habis setiap
				// kandidat pada permintaan yang sebenarnya biasa saja.
				total += tokenGambar(p.ImageURL)
			case providers.PartTypeAudio:
				total += tokenAudioMinimum
			}
		}
	}

	// Tool call di riwayat ikut terkirim ulang dan ikut ditagih.
	for _, tc := range m.ToolCalls {
		total += overheadPesan + bagiKeAtas(len(tc.Function.Name)+len(tc.Function.Arguments), bytePerToken)
	}
	return total
}

// tokenGambar memperkirakan biaya satu gambar dari detail yang diminta.
//
// Detail kosong berarti "auto", dan auto memilih berdasarkan ukuran gambar yang tidak bisa
// kami lihat tanpa mengunduh atau mendekodenya. Yang dipakai angka detail tinggi: keliru ke
// atas di sini berarti kandidat berjendela sempit tersaring, sedangkan keliru ke bawah
// berarti permintaan multimodal dikirim ke jendela yang hampir pasti tidak cukup.
func tokenGambar(img *providers.ImageURL) int {
	if img != nil && img.Detail == "low" {
		return tokenGambarDetailRendah
	}
	return tokenGambarDetailTinggi
}

// angkaExtra membaca satu field Extra sebagai bilangan bulat.
func angkaExtra(extra map[string]json.RawMessage, key string) (int, bool) {
	raw, ok := extra[key]
	if !ok {
		return 0, false
	}
	var f float64
	if json.Unmarshal(raw, &f) != nil {
		return 0, false
	}
	if f < 0 || f > math.MaxInt32 {
		return 0, false
	}
	return int(f), true
}

// bagiKeAtas membagi dengan pembulatan ke atas. Pembulatan ke atas, bukan ke bawah: teks
// sepanjang satu byte tetap satu token, dan pembulatan ke bawah membuat ratusan pesan
// pendek menghilang seluruhnya dari perkiraan.
func bagiKeAtas(n, pembagi int) int {
	if n <= 0 {
		return 0
	}
	return (n + pembagi - 1) / pembagi
}
