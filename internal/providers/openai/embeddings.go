package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// embeddingsResponse adalah respons endpoint embeddings.
type embeddingsResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index int `json:"index"`
		// Bentuknya bergantung encoding_format: array angka untuk "float", string base64
		// untuk "base64". Dibiarkan mentah dan dikenali dari isinya.
		Embedding json.RawMessage `json:"embedding"`
	} `json:"data"`
	Usage *wireUsage `json:"usage"`
}

// Embeddings menghitung embedding untuk satu atau beberapa masukan.
func (p *Provider) Embeddings(ctx context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	if req == nil || len(req.Input) == 0 {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan embedding tanpa input")
	}

	body := map[string]any{
		"model": req.Model,
		"input": req.Input,
	}
	if req.Dimensions != nil {
		body["dimensions"] = *req.Dimensions
	}
	if req.User != "" {
		body["user"] = req.User
	}
	// encoding_format hanya dikirim bila pemanggil memintanya. Field ini tidak selalu
	// dikenal provider compatible, dan adapter tidak membutuhkannya untuk menguraikan
	// respons — bentuk vektor dikenali dari isi respons, bukan dari apa yang diminta.
	if req.EncodingFormat != "" {
		body["encoding_format"] = req.EncodingFormat
	}

	// Aturannya sama dengan chatPayload: field yang sudah diisi gateway tidak bisa ditimpa,
	// sisanya diteruskan apa adanya. Tanpa ini, parameter embedding yang belum dikenal
	// gateway hilang di perjalanan tanpa satu pun tanda bagi pengirimnya.
	applyExtra(body, req.Extra)

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"body permintaan embedding tidak bisa diserialisasi ke JSON")
	}

	resp, doErr := p.do(ctx, http.MethodPost, pathEmbeddings, payload, false)
	if doErr != nil {
		return nil, doErr
	}
	defer resp.Body.Close()

	raw, readErr := readBody(p.name, resp.Body)
	if readErr != nil {
		return nil, readErr
	}

	var wire embeddingsResponse
	if uerr := json.Unmarshal(raw, &wire); uerr != nil {
		return nil, providers.Newf(providers.ErrKindServer, p.name,
			"respons upstream bukan JSON embeddings yang dikenali")
	}
	if len(wire.Data) == 0 {
		return nil, p.errorFromSuccessBody(raw, resp.StatusCode,
			"respons upstream tidak memuat satu pun vektor embedding")
	}

	out := &providers.EmbeddingsResponse{
		Model: wire.Model,
		Data:  make([]providers.Embedding, 0, len(wire.Data)),
		Raw:   raw,
	}
	if out.Model == "" {
		out.Model = req.Model
	}
	if wire.Usage != nil {
		out.Usage = wire.Usage.toCanonical()
	}

	for i, item := range wire.Data {
		vec, verr := decodeVector(item.Embedding)
		if verr != nil {
			// Pesannya milik kami sendiri, bukan dari upstream, jadi aman diteruskan.
			return nil, providers.Newf(providers.ErrKindServer, p.name,
				"vektor embedding upstream tidak bisa dibaca: %s", verr)
		}
		idx := item.Index
		if idx == 0 && i > 0 {
			// Sebagian provider compatible tidak mengisi index. Urutan array adalah
			// jawaban yang benar, dan tanpa penyesuaian ini seluruh vektor menumpuk di
			// index 0 sehingga pemanggil tidak bisa memasangkannya dengan input.
			idx = i
		}
		out.Data = append(out.Data, providers.Embedding{Index: idx, Vector: vec})
	}
	return out, nil
}

// decodeVector menerima kedua bentuk vektor yang dipakai dialek OpenAI.
//
// Bentuknya dikenali dari isi respons, bukan dari encoding_format yang diminta: provider
// compatible kadang mengabaikan permintaan base64 dan tetap mengirim array float, dan
// menebak dari permintaan berarti seluruh batch gagal diurai padahal datanya lengkap.
func decodeVector(raw json.RawMessage) ([]float32, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("vektor kosong")
	}

	switch trimmed[0] {
	case '[':
		var vec []float32
		if err := json.Unmarshal(trimmed, &vec); err != nil {
			return nil, errors.New("array angka tidak sah")
		}
		return vec, nil
	case '"':
		var encoded string
		if err := json.Unmarshal(trimmed, &encoded); err != nil {
			return nil, errors.New("string base64 tidak sah")
		}
		return decodeBase64Vector(encoded)
	default:
		return nil, errors.New("bentuk vektor tidak dikenali")
	}
}

// base64Variants adalah varian yang dicoba berurutan.
//
// OpenAI mengirim base64 standar berpadding, tetapi implementasi lain memakai varian tanpa
// padding atau varian URL-safe, dan membedakannya dari luar tidak mungkin — mencoba
// berurutan jauh lebih murah daripada menolak vektor yang sebenarnya bisa dibaca.
var base64Variants = []*base64.Encoding{
	base64.StdEncoding,
	base64.RawStdEncoding,
	base64.URLEncoding,
	base64.RawURLEncoding,
}

// decodeBase64Vector menguraikan vektor berkode base64 dari float32 little-endian.
func decodeBase64Vector(encoded string) ([]float32, error) {
	var buf []byte
	var ok bool
	for _, enc := range base64Variants {
		decoded, err := enc.DecodeString(encoded)
		if err == nil {
			buf, ok = decoded, true
			break
		}
	}
	if !ok {
		return nil, errors.New("base64 tidak bisa didekode")
	}
	if len(buf)%4 != 0 {
		return nil, errors.New("panjang base64 bukan kelipatan 4 byte")
	}

	vec := make([]float32, len(buf)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return vec, nil
}
