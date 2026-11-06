package providers

import (
	"bytes"
	"encoding/json"
)

// SignatureToleran adalah tanda tangan penalaran (thought signature /
// thinking signature) yang unmarshal-nya toleran terhadap bentuk JSON
// non-string (B-263).
//
// Sebagian upstream dan proksi mengirim signature sebagai ANGKA JSON.
// Pada field string biasa, satu signature numerik menggagalkan unmarshal
// SELURUH respons — jawaban yang sebenarnya utuh hilang hanya karena
// metadata kecil ini. Aturannya meniru guard yang dipakai router
// referensi (signature non-string tidak pernah menggagalkan respons):
// string dipakai apa adanya, angka dinormalkan menjadi teks literalnya,
// dan bentuk lain (bool, objek, array, null) diabaikan menjadi kosong.
// Marshal selalu menulis bentuk string kanonik.
type SignatureToleran string

// UnmarshalJSON menormalkan bentuk string dan numerik; bentuk lain
// diabaikan tanpa error.
func (s *SignatureToleran) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*s = ""
		return nil
	}
	switch c := data[0]; {
	case c == '"':
		var v string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*s = SignatureToleran(v)
	case c == '-' || (c >= '0' && c <= '9'):
		// Angka dipertahankan sebagai teks literalnya: signature adalah
		// token buram bagi gateway, dan bentuk teks inilah yang akan
		// dikirim balik ke upstream pada giliran berikutnya.
		*s = SignatureToleran(string(data))
	default:
		*s = ""
	}
	return nil
}

// MarshalJSON menulis signature sebagai string JSON.
func (s SignatureToleran) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}
