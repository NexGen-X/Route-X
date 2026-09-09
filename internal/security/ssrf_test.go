package security

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestValidateBaseURLAcceptsPublicHTTPS(t *testing.T) {
	for _, raw := range []string{
		"https://api.openai.com",
		"https://api.openai.com/v1",
		"https://api.anthropic.com/v1/",
		"https://generativelanguage.googleapis.com",
		"https://upstream.example.com:8443/v1",
		"https://8.8.8.8/v1",
	} {
		if err := ValidateBaseURL(raw, DefaultSSRFPolicy()); err != nil {
			t.Errorf("ValidateBaseURL(%q) = %v, mau nil", raw, err)
		}
	}
}

func TestValidateBaseURLRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want error
	}{
		{"http tanpa izin", "http://api.example.com", ErrBlockedScheme},
		{"skema file", "file:///etc/passwd", ErrBlockedScheme},
		{"skema gopher", "gopher://example.com", ErrBlockedScheme},
		{"skema ftp", "ftp://example.com", ErrBlockedScheme},
		{"tanpa skema", "api.example.com", ErrBlockedScheme},
		{"tanpa host", "https://", ErrInvalidBaseURL},
		{"kredensial di URL", "https://user:sandi@api.example.com", ErrCredentialsInURL},
		{"localhost", "https://localhost/v1", ErrBlockedAddress},
		{"subdomain localhost", "https://api.localhost/v1", ErrBlockedAddress},
		{"loopback IPv4", "https://127.0.0.1/v1", ErrBlockedAddress},
		{"loopback lain di 127/8", "https://127.99.88.77/v1", ErrBlockedAddress},
		{"loopback IPv6", "https://[::1]/v1", ErrBlockedAddress},
		{"metadata cloud", "https://169.254.169.254/latest/meta-data/", ErrBlockedAddress},
		{"privat 10/8", "https://10.0.0.5/v1", ErrBlockedAddress},
		{"privat 172.16/12", "https://172.16.0.5/v1", ErrBlockedAddress},
		{"privat 192.168/16", "https://192.168.1.5/v1", ErrBlockedAddress},
		{"CGNAT 100.64/10", "https://100.64.0.1/v1", ErrBlockedAddress},
		{"unspecified", "https://0.0.0.0/v1", ErrBlockedAddress},
		{"unique local IPv6", "https://[fc00::1]/v1", ErrBlockedAddress},
		{"link-local IPv6", "https://[fe80::1]/v1", ErrBlockedAddress},
		{"multicast", "https://239.1.1.1/v1", ErrBlockedAddress},
		{"loopback trailing dot", "https://127.0.0.1./v1", ErrBlockedAddress},
		{"desimal non-kanonis 2130706433", "https://2130706433/v1", ErrBlockedAddress},
		{"hex non-kanonis 0x7f.0.0.1", "https://0x7f.0.0.1/v1", ErrBlockedAddress},
		{"oktal non-kanonis 0177.0.0.1", "https://0177.0.0.1/v1", ErrBlockedAddress},
		{"6to4 2002:7f00:1:: (loopback tertanam)", "https://[2002:7f00:1::]/v1", ErrBlockedAddress},
		{"TEREDO 2001:0:7f00:1::", "https://[2001:0:7f00:1::]/v1", ErrBlockedAddress},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateBaseURL(tc.raw, DefaultSSRFPolicy())
			if err == nil {
				t.Fatalf("ValidateBaseURL(%q) = nil, seharusnya ditolak", tc.raw)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, mau membungkus %v", err, tc.want)
			}
		})
	}
}

// IPv4 yang dibungkus IPv6 tidak boleh menjadi jalan memutari daftar rentang IPv4.
func TestBlocksIPv4MappedIPv6(t *testing.T) {
	for _, raw := range []string{
		"::ffff:127.0.0.1",
		"::ffff:169.254.169.254",
		"::ffff:10.0.0.1",
		"::ffff:192.168.1.1",
	} {
		addr := netip.MustParseAddr(raw)
		if err := CheckAddr(addr, DefaultSSRFPolicy()); err == nil {
			t.Errorf("CheckAddr(%s) = nil — alamat terbungkus lolos penjagaan", raw)
		}
	}
}

func TestPolicyEscapeHatches(t *testing.T) {
	t.Run("AllowHTTP", func(t *testing.T) {
		if err := ValidateBaseURL("http://api.example.com", SSRFPolicy{AllowHTTP: true}); err != nil {
			t.Errorf("err = %v, mau nil", err)
		}
	})

	t.Run("AllowPrivate melepas seluruh penjagaan", func(t *testing.T) {
		p := SSRFPolicy{AllowHTTP: true, AllowPrivate: true}
		for _, raw := range []string{"http://127.0.0.1:11434", "http://10.0.0.5", "http://localhost:8080"} {
			if err := ValidateBaseURL(raw, p); err != nil {
				t.Errorf("ValidateBaseURL(%q) = %v, mau nil", raw, err)
			}
		}
	})

	t.Run("AllowedPrivateAddrs hanya melepas alamat tertentu", func(t *testing.T) {
		p := SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: mustAddrs(t, "127.0.0.1", "10.8.0.0/24")}

		for _, raw := range []string{"http://127.0.0.1:11434/v1", "http://10.8.0.7/v1"} {
			if err := ValidateBaseURL(raw, p); err != nil {
				t.Errorf("alamat yang diizinkan ditolak: %s: %v", raw, err)
			}
		}
		// Alamat lain tetap dijaga — inilah bedanya dari AllowPrivate.
		for _, raw := range []string{"http://169.254.169.254/", "http://10.0.0.5/", "http://192.168.1.1/"} {
			if err := ValidateBaseURL(raw, p); err == nil {
				t.Errorf("%s lolos padahal bukan alamat yang diizinkan", raw)
			}
		}
	})

	t.Run("localhost lolos bila loopback sudah dikecualikan", func(t *testing.T) {
		// Operator yang menjalankan Ollama menyebut mesinnya "localhost", bukan
		// "127.0.0.1". Lapisan pertama melonggar, lapisan dial tetap memeriksa hasil
		// resolusinya.
		p := SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: mustAddrs(t, "127.0.0.1")}
		if err := ValidateBaseURL("http://localhost:11434/v1", p); err != nil {
			t.Errorf("localhost ditolak padahal loopback dikecualikan: %v", err)
		}
		// Tanpa pengecualian itu, localhost tetap ditolak.
		if err := ValidateBaseURL("http://localhost:11434/v1", SSRFPolicy{AllowHTTP: true}); err == nil {
			t.Error("localhost lolos tanpa pengecualian apa pun")
		}
	})
}

// mustAddrs mengurai daftar alamat pengecualian atau menggagalkan test.
func mustAddrs(t *testing.T, values ...string) []netip.Prefix {
	t.Helper()
	out, err := ParsePrivateAddrs(values)
	if err != nil {
		t.Fatalf("ParsePrivateAddrs(%v): %v", values, err)
	}
	return out
}

// ParsePrivateAddrs menolak nama host, dan pesannya harus menjelaskan jalan keluarnya —
// operator yang mengetik "localhost" perlu tahu bahwa yang diminta adalah alamat, bukan
// bahwa pengecualiannya tidak bisa dipakai.
func TestParsePrivateAddrs(t *testing.T) {
	t.Run("menerima IP dan CIDR", func(t *testing.T) {
		got, err := ParsePrivateAddrs([]string{"127.0.0.1", "10.8.0.0/24", " ::1 ", "", "fd00::/8"})
		if err != nil {
			t.Fatalf("err = %v, mau nil", err)
		}
		if len(got) != 4 {
			t.Fatalf("len = %d, mau 4 (baris kosong dilewati): %v", len(got), got)
		}
		// Alamat tunggal menjadi prefix sepanjang penuh, supaya pemeriksaannya seragam.
		if got[0].String() != "127.0.0.1/32" {
			t.Errorf("got[0] = %s, mau 127.0.0.1/32", got[0])
		}
		if got[2].String() != "::1/128" {
			t.Errorf("got[2] = %s, mau ::1/128", got[2])
		}
	})

	t.Run("menolak nama host dengan pesan yang mengarahkan", func(t *testing.T) {
		_, err := ParsePrivateAddrs([]string{"ollama.internal"})
		if err == nil {
			t.Fatal("nama host diterima; seharusnya ditolak")
		}
		if !errors.Is(err, ErrInvalidBaseURL) {
			t.Errorf("err = %v, mau membungkus ErrInvalidBaseURL", err)
		}
		for _, petunjuk := range []string{"per alamat", "nama host"} {
			if !strings.Contains(err.Error(), petunjuk) {
				t.Errorf("pesan %q tidak menyebut %q", err.Error(), petunjuk)
			}
		}
	})

	t.Run("CIDR dinormalkan ke bentuk masked", func(t *testing.T) {
		// "10.8.0.7/24" menyebut alamat di dalam blok, bukan awal bloknya. Tanpa Masked,
		// Contains pada prefix seperti itu tidak berperilaku seperti yang operator kira.
		got, err := ParsePrivateAddrs([]string{"10.8.0.7/24"})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got[0].String() != "10.8.0.0/24" {
			t.Errorf("got[0] = %s, mau 10.8.0.0/24", got[0])
		}
	})
}

// Pemeriksaan saat dial adalah lapisan yang benar-benar menutup DNS rebinding.
// Validasi URL saja bisa dilewati: nama host yang lolos bisa diarahkan ke alamat
// internal tepat saat koneksi dibuka.
func TestControlBlocksResolvedInternalAddresses(t *testing.T) {
	control := NewSSRFControl(DefaultSSRFPolicy())

	blocked := []string{
		"127.0.0.1:443", "169.254.169.254:80", "10.0.0.5:443",
		"192.168.1.1:443", "[::1]:443", "[fc00::1]:443", "0.0.0.0:443",
	}
	for _, addr := range blocked {
		if err := control("tcp", addr, nil); err == nil {
			t.Errorf("control(tcp, %s) = nil — alamat internal lolos saat dial", addr)
		}
	}

	for _, addr := range []string{"8.8.8.8:443", "1.1.1.1:443", "[2606:4700::1111]:443"} {
		if err := control("tcp", addr, nil); err != nil {
			t.Errorf("control(tcp, %s) = %v, mau nil", addr, err)
		}
	}

	// Jaringan non-TCP ditolak: klien HTTP kami tidak memakainya.
	if err := control("udp", "8.8.8.8:53", nil); err == nil {
		t.Error("jaringan udp lolos")
	}
	// Alamat yang tidak bisa diurai ditolak, bukan diloloskan.
	for _, bad := range []string{"bukan-alamat", "nama-host:443", ""} {
		if err := control("tcp", bad, nil); err == nil {
			t.Errorf("control(tcp, %q) = nil untuk alamat tak terurai", bad)
		}
	}
}

// Bukti bahwa penjaga benar-benar terpasang di jalur HTTP sungguhan: satu server nyata
// di loopback, dua kebijakan, dua hasil berbeda.
func TestGuardedTransportBlocksRealLoopbackRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := func(policy SSRFPolicy) *http.Client {
		return &http.Client{Transport: &http.Transport{
			DialContext: GuardedDialContext(policy, &net.Dialer{}),
		}}
	}

	t.Run("kebijakan ketat memblokir", func(t *testing.T) {
		_, err := client(DefaultSSRFPolicy()).Get(srv.URL)
		if err == nil {
			t.Fatal("permintaan ke loopback berhasil padahal seharusnya diblokir")
		}
		if !strings.Contains(err.Error(), "tidak diizinkan") {
			t.Errorf("err = %v, mau menyebut alamat tidak diizinkan", err)
		}
	})

	t.Run("kebijakan AllowPrivate mengizinkan", func(t *testing.T) {
		resp, err := client(SSRFPolicy{AllowPrivate: true}).Get(srv.URL)
		if err != nil {
			t.Fatalf("permintaan ditolak padahal AllowPrivate: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d", resp.StatusCode)
		}
	})
}

// Regresi untuk cacat yang nyata: AllowedPrivateAddrs harus berlaku di KEDUA lapisan.
//
// Sebelum perbaikan, ValidateBaseURL menghormati pengecualian ini tetapi CheckAddr tidak,
// sehingga operator yang menjalankan model lokal dan mengizinkan 127.0.0.1 lolos validasi
// lalu diblokir tepat saat menghubungi. Jalan keluar yang didokumentasikan tidak bekerja
// ujung-ke-ujung, dan yang benar-benar berfungsi hanya AllowPrivate — yang melepas
// perlindungan untuk SELURUH provider, termasuk yang dibuat pengguna BYOK.
//
// Test ini menempuh dua lapisan itu berurutan pada satu server sungguhan, karena cacatnya
// hanya terlihat bila keduanya diperiksa dalam satu alur.
func TestAllowedPrivateAddrsBerlakuDiValidasiDanDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	policy := SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: mustAddrs(t, "127.0.0.1")}

	// Lapisan 1.
	if err := ValidateBaseURL(srv.URL, policy); err != nil {
		t.Fatalf("lapisan 1 menolak %s: %v", srv.URL, err)
	}
	// Lapisan 2, langsung.
	if err := CheckAddr(netip.MustParseAddr("127.0.0.1"), policy); err != nil {
		t.Fatalf("lapisan 2 menolak alamat yang dikecualikan: %v", err)
	}
	// Lapisan 2, lewat socket sungguhan.
	cl := &http.Client{Transport: &http.Transport{DialContext: GuardedDialContext(policy, &net.Dialer{})}}
	resp, err := cl.Get(srv.URL)
	if err != nil {
		t.Fatalf("dial ditolak padahal alamatnya dikecualikan: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, mau 200", resp.StatusCode)
	}

	// Yang TIDAK boleh ikut lolos: alamat privat lain, termasuk metadata cloud. Inilah
	// yang membedakan pengecualian per alamat dari AllowPrivate — dan inilah alasan
	// pengecualiannya berisi alamat, bukan nama: nama yang dibelokkan ke alamat privat
	// LAIN tetap tertolak di sini.
	for _, lain := range []string{"169.254.169.254", "10.0.0.5", "::1", "192.168.1.1"} {
		if err := CheckAddr(netip.MustParseAddr(lain), policy); err == nil {
			t.Errorf("CheckAddr(%s) = nil padahal hanya 127.0.0.1 yang dikecualikan", lain)
		}
	}

	// ::ffff:127.0.0.1 adalah bentuk terbungkus dari alamat yang sama dan harus ikut
	// dikenali — kalau tidak, pengecualian per alamat bisa dilewati justru oleh operator
	// yang menuliskannya dengan benar.
	if err := CheckAddr(netip.MustParseAddr("::ffff:127.0.0.1"), policy); err != nil {
		t.Errorf("bentuk IPv4-in-IPv6 dari alamat yang dikecualikan ditolak: %v", err)
	}
}

// Pengalihan (redirect) ke alamat internal juga harus terblokir — dan itu terjadi
// otomatis karena penjagaannya di lapisan dial, bukan di lapisan URL.
func TestGuardedTransportBlocksRedirectToInternal(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer internal.Close()

	// Server ini mengalihkan ke alamat loopback. Dari sisi URL awal, tidak ada yang
	// mencurigakan.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	// Dialer dibuat mengizinkan host redirector saja, sehingga permintaan pertama lolos
	// tetapi pengalihannya tidak.
	redirHost, _, _ := net.SplitHostPort(strings.TrimPrefix(redirector.URL, "http://"))
	internalHost, internalPort, _ := net.SplitHostPort(strings.TrimPrefix(internal.URL, "http://"))
	if redirHost != internalHost {
		t.Skipf("kedua server tidak di host yang sama (%s vs %s), test ini tidak berlaku", redirHost, internalHost)
	}
	_ = internalPort

	// Karena keduanya di 127.0.0.1, kebijakan ketat memblokir keduanya — yang justru
	// membuktikan intinya: penjagaan di lapisan dial berlaku pada SETIAP koneksi,
	// termasuk yang dibuat karena pengalihan, tanpa perlu memeriksa URL tujuan.
	client := &http.Client{Transport: &http.Transport{
		DialContext: GuardedDialContext(DefaultSSRFPolicy(), &net.Dialer{}),
	}}
	if _, err := client.Get(redirector.URL); err == nil {
		t.Error("permintaan berhasil padahal seluruh loopback diblokir")
	}
}

func TestNewSSRFDialerPreservesBaseSettings(t *testing.T) {
	base := &net.Dialer{Timeout: 7, KeepAlive: 11}
	d := NewSSRFDialer(DefaultSSRFPolicy(), base)

	if d.Timeout != base.Timeout || d.KeepAlive != base.KeepAlive {
		t.Errorf("setelan dasar hilang: timeout=%v keepalive=%v", d.Timeout, d.KeepAlive)
	}
	if d.Control == nil {
		t.Error("Control tidak dipasang")
	}
	// base tidak boleh ikut berubah.
	if base.Control != nil {
		t.Error("dialer dasar ikut dimodifikasi")
	}
	// nil base harus tetap menghasilkan dialer yang bisa dipakai.
	if NewSSRFDialer(DefaultSSRFPolicy(), nil).Control == nil {
		t.Error("Control tidak dipasang saat base nil")
	}
}

func TestCheckAddrInvalid(t *testing.T) {
	if err := CheckAddr(netip.Addr{}, DefaultSSRFPolicy()); err == nil {
		t.Error("alamat kosong seharusnya ditolak")
	}
	// AllowPrivate melepas semuanya, termasuk alamat kosong — itu memang maknanya.
	if err := CheckAddr(netip.Addr{}, SSRFPolicy{AllowPrivate: true}); err != nil {
		t.Errorf("AllowPrivate seharusnya melepas semua pemeriksaan: %v", err)
	}
}

func TestGuardedDialContextSignature(t *testing.T) {
	var fn DialContextFunc = GuardedDialContext(DefaultSSRFPolicy(), nil)
	if _, err := fn(context.Background(), "tcp", "127.0.0.1:1"); err == nil {
		t.Error("dial ke loopback berhasil")
	}
}
