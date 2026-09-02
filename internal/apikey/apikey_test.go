package apikey

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/security"
)

// testRawKey berbentuk API key yang sah: prefix live plus 43 karakter base62. Bentuk
// yang benar dipakai di seluruh test supaya tidak ada jalur yang lolos hanya karena
// nilai ujinya jelas-jelas bukan key.
const testRawKey = "sk_live_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"

// containsRawKey melaporkan apakah sebuah teks memuat nilai key mentah. Dipakai test
// yang memastikan kredensial tidak pernah mendarat di log atau respons.
func containsRawKey(s string) bool { return strings.Contains(s, testRawKey) }

// Bentuk testRawKey dijaga: kalau aturan format di paket security berubah, test lain di
// berkas ini akan menguji hal yang salah tanpa ada yang gagal.
func TestTestRawKeyIsWellFormed(t *testing.T) {
	prefix, err := security.ParseAPIKey(testRawKey)
	if err != nil {
		t.Fatalf("security.ParseAPIKey(testRawKey): %v", err)
	}
	if prefix != security.KeyPrefixLive {
		t.Errorf("prefix = %q, mau %q", prefix, security.KeyPrefixLive)
	}
}

func TestCredential(t *testing.T) {
	const other = "sk_live_ZYXWVUTSRQPONMLKJIHGFEDCBA9876543210zyxwvut"

	tests := []struct {
		name    string
		headers [][2]string
		target  string
		want    string
		wantErr error
	}{
		{
			name:    "bearer biasa",
			headers: [][2]string{{"Authorization", "Bearer " + testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "bearer huruf kecil",
			headers: [][2]string{{"Authorization", "bearer " + testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "bearer huruf besar",
			headers: [][2]string{{"Authorization", "BEARER " + testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "bearer huruf campur",
			headers: [][2]string{{"Authorization", "BeArEr " + testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "spasi berlebih dipangkas",
			headers: [][2]string{{"Authorization", "   Bearer     " + testRawKey + "   "}},
			want:    testRawKey,
		},
		{
			name:    "pemisah tab",
			headers: [][2]string{{"Authorization", "Bearer\t" + testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "x-api-key",
			headers: [][2]string{{"X-Api-Key", testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "x-api-key huruf kecil dikanonikalisasi",
			headers: [][2]string{{"x-api-key", testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "x-api-key dengan spasi di pinggir",
			headers: [][2]string{{"X-Api-Key", "  " + testRawKey + " "}},
			want:    testRawKey,
		},
		{
			name: "kedua header bernilai sama",
			headers: [][2]string{
				{"Authorization", "Bearer " + testRawKey},
				{"X-Api-Key", testRawKey},
			},
			want: testRawKey,
		},
		{
			name: "kedua header berbeda ditolak",
			headers: [][2]string{
				{"Authorization", "Bearer " + testRawKey},
				{"X-Api-Key", other},
			},
			wantErr: ErrConflictingCredential,
		},
		{
			name: "authorization ganda berbeda ditolak",
			headers: [][2]string{
				{"Authorization", "Bearer " + testRawKey},
				{"Authorization", "Bearer " + other},
			},
			wantErr: ErrConflictingCredential,
		},
		{
			name: "authorization ganda bernilai sama diterima",
			headers: [][2]string{
				{"Authorization", "Bearer " + testRawKey},
				{"Authorization", "Bearer " + testRawKey},
			},
			want: testRawKey,
		},
		{
			name:    "tanpa header",
			wantErr: ErrNoCredential,
		},
		{
			name:    "kredensial di query string diabaikan",
			target:  "/v1/chat/completions?api_key=" + testRawKey + "&key=" + other,
			wantErr: ErrNoCredential,
		},
		{
			name:    "query string diabaikan walau header ada",
			target:  "/v1/chat/completions?api_key=" + other,
			headers: [][2]string{{"Authorization", "Bearer " + testRawKey}},
			want:    testRawKey,
		},
		{
			name:    "authorization kosong dianggap tidak ada",
			headers: [][2]string{{"Authorization", "   "}},
			wantErr: ErrNoCredential,
		},
		{
			name:    "x-api-key kosong dianggap tidak ada",
			headers: [][2]string{{"X-Api-Key", ""}},
			wantErr: ErrNoCredential,
		},
		{
			name: "x-api-key kosong tidak menghalangi bearer",
			headers: [][2]string{
				{"X-Api-Key", ""},
				{"Authorization", "Bearer " + testRawKey},
			},
			want: testRawKey,
		},
		{
			name:    "bearer tanpa nilai",
			headers: [][2]string{{"Authorization", "Bearer"}},
			wantErr: ErrMalformedCredential,
		},
		{
			name:    "bearer dengan nilai kosong",
			headers: [][2]string{{"Authorization", "Bearer    "}},
			wantErr: ErrMalformedCredential,
		},
		{
			name:    "skema selain bearer",
			headers: [][2]string{{"Authorization", "Basic dXNlcjpwYXNz"}},
			wantErr: ErrMalformedCredential,
		},
		{
			name:    "tanpa skema",
			headers: [][2]string{{"Authorization", testRawKey}},
			wantErr: ErrMalformedCredential,
		},
		{
			name:    "nilai bearer memuat spasi",
			headers: [][2]string{{"Authorization", "Bearer " + testRawKey + " tambahan"}},
			wantErr: ErrMalformedCredential,
		},
		{
			name:    "x-api-key memuat spasi",
			headers: [][2]string{{"X-Api-Key", testRawKey + " tambahan"}},
			wantErr: ErrMalformedCredential,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := tc.target
			if target == "" {
				target = "/v1/chat/completions"
			}
			r := httptest.NewRequest("POST", target, nil)
			for _, h := range tc.headers {
				r.Header.Add(h[0], h[1])
			}

			got, err := Credential(r)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, mau %v", err, tc.wantErr)
				}
				if got != "" {
					t.Errorf("kredensial = %q, mau kosong saat gagal", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("error tak terduga: %v", err)
			}
			if got != tc.want {
				t.Errorf("kredensial = %q, mau %q", got, tc.want)
			}
		})
	}
}

// Pesan error dari Credential tidak boleh memuat nilai kredensial: pesan itu bisa
// mendarat di log walau tidak pernah sampai ke klien.
func TestCredentialErrorsDoNotLeakValue(t *testing.T) {
	cases := []string{
		"Basic " + testRawKey,
		testRawKey,
		"Bearer " + testRawKey + " tambahan",
	}
	for _, value := range cases {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("Authorization", value)

		_, err := Credential(r)
		if err == nil {
			t.Fatalf("%q: mau error", value)
		}
		if containsRawKey(err.Error()) {
			t.Errorf("%q: pesan error memuat nilai kredensial: %s", value, err.Error())
		}
	}
}

func TestCredentialNilRequest(t *testing.T) {
	if _, err := Credential(nil); !errors.Is(err, ErrNoCredential) {
		t.Errorf("error = %v, mau %v", err, ErrNoCredential)
	}
}

func TestPrincipal(t *testing.T) {
	key := &keys.Key{
		ID:          "8c6f0a0e-0f2a-4a1c-9f0e-2d0b8a7c1f11",
		Name:        "kunci uji",
		Prefix:      security.KeyPrefixLive,
		Last4:       "EFG",
		OwnerUserID: "1f2e3d4c-5b6a-4798-8899-aabbccddeeff",
		Status:      keys.StatusActive,
		Scopes:      []string{keys.ScopeInference, keys.ScopeModelsRead},
	}
	p := &Principal{Key: key}

	if p.ID() != key.ID {
		t.Errorf("ID() = %q", p.ID())
	}
	if p.OwnerUserID() != key.OwnerUserID {
		t.Errorf("OwnerUserID() = %q", p.OwnerUserID())
	}
	if want := security.MaskedFromParts(key.Prefix, key.Last4); p.Masked() != want {
		t.Errorf("Masked() = %q, mau %q", p.Masked(), want)
	}
	if !p.Can(keys.ScopeInference) {
		t.Error("Can(inference) = false")
	}
	if p.Can(keys.ScopeAdminWrite) {
		t.Error("Can(admin:write) = true")
	}
	if p.Can("") {
		t.Error("Can(\"\") = true, cakupan kosong harus selalu ditolak")
	}

	scopes := p.Scopes()
	if len(scopes) != 2 {
		t.Fatalf("Scopes() = %v", scopes)
	}
	// Salinan, bukan slice aslinya.
	scopes[0] = "diubah"
	if key.Scopes[0] != keys.ScopeInference {
		t.Error("Scopes() mengembalikan slice yang bisa mengubah principal")
	}
}

// Seluruh method Principal harus aman pada penerima nil dan pada Key nil, supaya handler
// tidak perlu bercabang sebelum menulis log.
func TestPrincipalNilSafe(t *testing.T) {
	var nilPrincipal *Principal
	empty := &Principal{}

	for name, p := range map[string]*Principal{"nil": nilPrincipal, "tanpa key": empty} {
		if p.ID() != "" || p.OwnerUserID() != "" || p.Masked() != "" {
			t.Errorf("%s: method pengenal tidak mengembalikan kosong", name)
		}
		if p.Can(keys.ScopeInference) {
			t.Errorf("%s: Can() = true", name)
		}
		if got := p.Scopes(); got == nil || len(got) != 0 {
			t.Errorf("%s: Scopes() = %v, mau slice kosong non-nil", name, got)
		}
	}
}

func TestPrincipalContext(t *testing.T) {
	ctx := context.Background()

	if _, ok := PrincipalFrom(ctx); ok {
		t.Error("PrincipalFrom pada context kosong = true")
	}

	// Principal tanpa Key tidak dianggap sah: handler yang menerimanya akan mengira
	// request sudah terautentikasi padahal tidak ada key di belakangnya.
	if _, ok := PrincipalFrom(WithPrincipal(ctx, &Principal{})); ok {
		t.Error("PrincipalFrom pada principal tanpa key = true")
	}
	if _, ok := PrincipalFrom(WithPrincipal(ctx, nil)); ok {
		t.Error("PrincipalFrom pada principal nil = true")
	}

	want := &Principal{Key: &keys.Key{ID: "abc"}}
	got, ok := PrincipalFrom(WithPrincipal(ctx, want))
	if !ok || got != want {
		t.Errorf("PrincipalFrom = (%v, %v)", got, ok)
	}
}
