package gateway

import (
	"strings"
	"testing"
)

// TestB329NullBerartiTidakAda adalah pengunci B-329: validasi susulan
// DecodeChatRequest membaca field mentah TANPA perlakuan null yang
// ambil() terapkan di mana-mana — "n":null dan
// "max_completion_tokens":null ditolak 400 walau di seluruh codec
// null berarti "tidak ada". Dan "n" yang lolos tak dibatasi: nilai
// raksasa diteruskan ke upstream (penolakannya tercatat sebagai
// kegagalan provider) padahal EstimateTokens sendiri mengklem
// pengali ini ke 128.
func TestB329NullBerartiTidakAda(t *testing.T) {
	kasus := []struct {
		nama     string
		body     string
		mauGagal bool
	}{
		{"n null diterima", `{"model":"m","messages":[{"role":"user","content":"halo"}],"n":null}`, false},
		{"max_completion_tokens null diterima", `{"model":"m","messages":[{"role":"user","content":"halo"}],"max_completion_tokens":null}`, false},
		{"n wajar diterima (kontrol)", `{"model":"m","messages":[{"role":"user","content":"halo"}],"n":2}`, false},
		{"n raksasa ditolak", `{"model":"m","messages":[{"role":"user","content":"halo"}],"n":100000}`, true},
		{"n tepat di batas diterima (kontrol)", `{"model":"m","messages":[{"role":"user","content":"halo"}],"n":128}`, false},
		{"n negatif tetap ditolak (kontrol B-302)", `{"model":"m","messages":[{"role":"user","content":"halo"}],"n":-1}`, true},
		{"max_completion_tokens nol tetap ditolak (kontrol B-250)", `{"model":"m","messages":[{"role":"user","content":"halo"}],"max_completion_tokens":0}`, true},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := DecodeChatRequest([]byte(k.body))
			if k.mauGagal && err == nil {
				t.Errorf("DecodeChatRequest(%s) sukses, mau ditolak", k.body)
			}
			if !k.mauGagal && err != nil {
				t.Errorf("DecodeChatRequest(%s) = %v, mau sukses", k.body, err)
			}
		})
	}
}

// TestB331PesanMenguapDitolak adalah pengunci B-331: jalur Messages
// menerima daftar pesan yang menguap total saat decode — messages
// hilang/kosong, atau satu-satunya pesan user ber-content ""/[]
// (decodePesanAnthropic mengembalikan (nil, nil) untuk keduanya) —
// dan permintaan nol-percakapan itu diteruskan ke upstream sebagai
// kegagalan provider. Kini decoder menolaknya 400 di pintu: sesudah
// penguraian, nol pesan percakapan kanonik berarti permintaan tidak
// sah. System saja TIDAK menyelamatkan permintaan.
func TestB331PesanMenguapDitolak(t *testing.T) {
	kasus := []struct {
		nama     string
		body     string
		mauGagal bool
	}{
		{"messages hilang", `{"model":"m","max_tokens":10}`, true},
		{"messages kosong", `{"model":"m","max_tokens":10,"messages":[]}`, true},
		{"content string kosong menguap", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":""}]}`, true},
		{"content larik kosong menguap", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":[]}]}`, true},
		{"system saja tetap ditolak", `{"model":"m","max_tokens":10,"system":"kamu asisten","messages":[]}`, true},
		{"satu pesan user sah (kontrol)", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"halo"}]}`, false},
		{"pesan system di daftar + user (kontrol)", `{"model":"m","max_tokens":10,"messages":[{"role":"system","content":"konteks"},{"role":"user","content":"halo"}]}`, false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := DecodeAnthropicMessagesRequest([]byte(k.body))
			if k.mauGagal && err == nil {
				t.Errorf("DecodeAnthropicMessagesRequest(%s) sukses, mau ditolak — nol pesan percakapan lolos", k.body)
			}
			if !k.mauGagal && err != nil {
				t.Errorf("DecodeAnthropicMessagesRequest(%s) = %v, mau sukses", k.body, err)
			}
		})
	}
}

// TestB334PeranAsingDitolak adalah pengunci B-334: peran selain
// user/assistant/system pada messages Anthropic dulu diteruskan apa
// adanya ke bentuk kanonik (dan peran "" menjadi pesan tanpa peran);
// decoder Messages OpenAI menolak peran asing di pintunya, permukaan
// ini tidak. Kini peran asing ditolak 400 dengan menyebut perannya.
func TestB334PeranAsingDitolak(t *testing.T) {
	for _, peran := range []string{"tool", "developer", "function", ""} {
		body := `{"model":"m","max_tokens":10,"messages":[{"role":"` + peran + `","content":"halo"}]}`
		_, err := DecodeAnthropicMessagesRequest([]byte(body))
		if err == nil {
			t.Errorf("peran %q: decode sukses, mau ditolak", peran)
			continue
		}
		if peran != "" && !strings.Contains(err.Error(), peran) {
			t.Errorf("peran %q: error %q tidak menyebut perannya", peran, err)
		}
	}
}

// TestB337SystemRusakDitolak adalah pengunci B-337: teksSystemAnthropic
// menelan SEMUA kesalahan unmarshal — system berbentuk block dengan
// text bukan-string (atau bentuk tak dikenal lain) ter-decode
// "sukses" dengan system prompt hilang senyap, sementara kesalahan
// decode di tempat lain pada permukaan yang sama adalah 400. Kini
// bentuk yang tidak terurai menjadi error decode.
func TestB337SystemRusakDitolak(t *testing.T) {
	kasus := []struct {
		nama      string
		body      string
		mauGagal  bool
		mauSistem string
	}{
		{"text block bukan string ditolak", `{"model":"m","max_tokens":10,"system":[{"type":"text","text":123}],"messages":[{"role":"user","content":"halo"}]}`, true, ""},
		{"system angka ditolak", `{"model":"m","max_tokens":10,"system":123,"messages":[{"role":"user","content":"halo"}]}`, true, ""},
		{"system string sah (kontrol)", `{"model":"m","max_tokens":10,"system":"kamu asisten","messages":[{"role":"user","content":"halo"}]}`, false, "kamu asisten"},
		{"system block sah (kontrol)", `{"model":"m","max_tokens":10,"system":[{"type":"text","text":"konteks"}],"messages":[{"role":"user","content":"halo"}]}`, false, "konteks"},
		{"tanpa system sah (kontrol)", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"halo"}]}`, false, ""},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			req, err := DecodeAnthropicMessagesRequest([]byte(k.body))
			if k.mauGagal {
				if err == nil {
					t.Errorf("decode sukses dengan system hilang, mau ditolak")
				}
				return
			}
			if err != nil {
				t.Fatalf("decode = %v, mau sukses", err)
			}
			if k.mauSistem != "" {
				if len(req.Messages) == 0 || req.Messages[0].Content != k.mauSistem {
					t.Errorf("pesan pertama = %+v, mau system %q", req.Messages, k.mauSistem)
				}
			}
		})
	}
}

// TestB338BudgetThinkingNegatifDitolak adalah pengunci B-338:
// thinking.budget_tokens negatif dulu lolos decode (satu-satunya
// validasi numerik yang hilang di antara temperature/top_p/top_k
// yang semuanya divalidasi) dan diteruskan ke upstream. Negatif
// adalah satu-satunya nilai tak sah di sini: field ini int polos,
// jadi 0 tak terbedakan dari "tidak disetel" dan tetap diterima —
// menolak 0 akan menghukum klien yang mengirim type enabled tanpa
// budget.
func TestB338BudgetThinkingNegatifDitolak(t *testing.T) {
	kasus := []struct {
		nama     string
		body     string
		mauGagal bool
	}{
		{"budget negatif ditolak", `{"model":"m","max_tokens":10,"thinking":{"type":"enabled","budget_tokens":-5},"messages":[{"role":"user","content":"halo"}]}`, true},
		{"budget nol diterima (kontrol)", `{"model":"m","max_tokens":10,"thinking":{"type":"enabled","budget_tokens":0},"messages":[{"role":"user","content":"halo"}]}`, false},
		{"budget positif diterima (kontrol)", `{"model":"m","max_tokens":10,"thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"user","content":"halo"}]}`, false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := DecodeAnthropicMessagesRequest([]byte(k.body))
			if k.mauGagal && err == nil {
				t.Errorf("decode sukses, mau ditolak untuk budget negatif")
			}
			if !k.mauGagal && err != nil {
				t.Errorf("decode = %v, mau sukses", err)
			}
		})
	}
}
