package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
)

// Penjaga SSRF untuk base URL provider.
//
// Base URL provider berasal dari operator, dan pada BYOK bisa berasal dari pengguna
// biasa. Tanpa penjagaan, siapa pun yang boleh membuat provider dapat menyuruh gateway
// menghubungi alamat internal — endpoint metadata cloud di 169.254.169.254 yang
// mengembalikan kredensial instans, layanan admin di localhost, atau apa pun di jaringan
// privat yang mengira semua peminta dari dalam jaringan bisa dipercaya.
//
// Ada DUA lapisan, dan lapisan kedua yang benar-benar penting:
//
//  1. Validasi URL saat disimpan (ValidateBaseURL) — menolak skema aneh, kredensial di
//     URL, dan nama host yang jelas menunjuk ke dalam.
//  2. Pemeriksaan ulang saat dial (Dialer) — memeriksa alamat IP yang BENAR-BENAR
//     dihubungi. Lapisan pertama sendirian bisa dilewati DNS rebinding: nama host yang
//     lolos pemeriksaan bisa diarahkan ke 127.0.0.1 pada saat koneksi dibuka. Hanya
//     pemeriksaan pada IP hasil resolusi yang menutup celah itu.

var (
	// ErrBlockedScheme dikembalikan untuk skema URL yang tidak diizinkan.
	ErrBlockedScheme = errors.New("skema URL tidak diizinkan")
	// ErrCredentialsInURL dikembalikan bila URL memuat userinfo.
	ErrCredentialsInURL = errors.New("URL tidak boleh memuat kredensial")
	// ErrBlockedAddress dikembalikan bila alamat tujuan berada di rentang terlarang.
	ErrBlockedAddress = errors.New("alamat tujuan tidak diizinkan")
	// ErrInvalidBaseURL dikembalikan untuk URL yang tidak bisa diurai.
	ErrInvalidBaseURL = errors.New("base URL tidak sah")
)

// blockedPrefixes adalah rentang yang tidak boleh dihubungi.
//
// Daftarnya disusun eksplisit alih-alih mengandalkan netip.Addr.IsPrivate() saja, karena
// beberapa rentang yang berbahaya di sini tidak dianggap privat oleh definisi itu —
// khususnya 100.64.0.0/10 (CGNAT, dipakai jaringan penyedia cloud) dan
// 192.0.0.0/24 (IETF protocol assignments).
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),          // "this host on this network"
	netip.MustParsePrefix("10.0.0.0/8"),         // privat
	netip.MustParsePrefix("100.64.0.0/10"),      // CGNAT
	netip.MustParsePrefix("127.0.0.0/8"),        // loopback
	netip.MustParsePrefix("169.254.0.0/16"),     // link-local, termasuk metadata cloud
	netip.MustParsePrefix("172.16.0.0/12"),      // privat
	netip.MustParsePrefix("192.0.0.0/24"),       // IETF
	netip.MustParsePrefix("192.0.2.0/24"),       // dokumentasi
	netip.MustParsePrefix("192.168.0.0/16"),     // privat
	netip.MustParsePrefix("198.18.0.0/15"),      // benchmark
	netip.MustParsePrefix("198.51.100.0/24"),    // dokumentasi
	netip.MustParsePrefix("203.0.113.0/24"),     // dokumentasi
	netip.MustParsePrefix("224.0.0.0/4"),        // multicast
	netip.MustParsePrefix("240.0.0.0/4"),        // dicadangkan
	netip.MustParsePrefix("255.255.255.255/32"), // broadcast
	netip.MustParsePrefix("::/128"),             // unspecified
	netip.MustParsePrefix("::1/128"),            // loopback
	netip.MustParsePrefix("fc00::/7"),           // unique local
	netip.MustParsePrefix("fe80::/10"),          // link-local
	netip.MustParsePrefix("ff00::/8"),           // multicast
	netip.MustParsePrefix("2001:db8::/32"),      // dokumentasi
	// 64:ff9b::/96 memetakan seluruh IPv4 ke IPv6 (NAT64) dan bisa dipakai menjangkau
	// alamat privat lewat alamat yang tampak publik.
	netip.MustParsePrefix("64:ff9b::/96"),
}

// SSRFPolicy mengatur alamat mana yang boleh dihubungi.
type SSRFPolicy struct {
	// AllowHTTP mengizinkan skema http. Bawaannya hanya https: base URL provider
	// membawa kredensial di header, dan http berarti kredensial itu melintas terbuka.
	AllowHTTP bool
	// AllowPrivate melepas seluruh penjagaan rentang.
	//
	// Ada karena kebutuhan yang sah: operator yang menjalankan model lokal (Ollama,
	// vLLM) memang harus bisa menunjuk ke 127.0.0.1. Tetapi ini melepas perlindungan
	// untuk SELURUH provider, termasuk yang dibuat pengguna BYOK — jadi pemakaiannya
	// harus keputusan sadar operator, bukan bawaan.
	AllowPrivate bool
	// AllowedPrivateHosts adalah jalan tengah yang lebih baik daripada AllowPrivate:
	// hanya host tertentu (mis. "127.0.0.1", "localhost", "ollama.internal") yang
	// dikecualikan, sisanya tetap dijaga.
	AllowedPrivateHosts []string
}

// DefaultSSRFPolicy adalah kebijakan paling ketat: hanya https, tanpa alamat internal.
func DefaultSSRFPolicy() SSRFPolicy { return SSRFPolicy{} }

// allowsHost melaporkan apakah host ini dikecualikan dari penjagaan rentang.
func (p SSRFPolicy) allowsHost(host string) bool {
	if p.AllowPrivate {
		return true
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range p.AllowedPrivateHosts {
		if strings.ToLower(strings.TrimSuffix(h, ".")) == host {
			return true
		}
	}
	return false
}

// ValidateBaseURL memeriksa base URL provider sebelum disimpan.
//
// Ini lapisan pertama: menolak yang jelas salah lebih awal, dengan pesan yang bisa
// dipahami operator. Ia TIDAK cukup sendirian — lihat catatan di atas soal DNS
// rebinding.
func ValidateBaseURL(raw string, policy SSRFPolicy) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%w", ErrInvalidBaseURL)
	}
	// Skema diperiksa lebih dulu, sebelum keberadaan host. Untuk masukan seperti
	// "file:///etc/passwd" atau "api.example.com" (tanpa skema), url.Parse menghasilkan
	// host kosong — dan melaporkan "tidak ada host" menyembunyikan masalah yang
	// sebenarnya dari operator, yang sedang salah menempelkan jenis URL.
	switch u.Scheme {
	case "https":
	case "http":
		if !policy.AllowHTTP {
			return fmt.Errorf("%w: http hanya diizinkan bila dinyalakan operator, karena kredensial provider akan melintas terbuka", ErrBlockedScheme)
		}
	default:
		return fmt.Errorf("%w: %q", ErrBlockedScheme, u.Scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("%w: tidak ada host", ErrInvalidBaseURL)
	}

	// Kredensial di URL akan ikut tercatat di log mana pun yang mencetak base URL.
	if u.User != nil {
		return ErrCredentialsInURL
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: host kosong", ErrInvalidBaseURL)
	}
	if policy.allowsHost(host) {
		return nil
	}

	// Bila host sudah berupa alamat IP, bisa langsung diperiksa. Bila berupa nama, tidak
	// diresolusi di sini: hasil resolusi saat menyimpan tidak menjamin apa pun tentang
	// hasil resolusi saat menghubungi, dan pemeriksaan yang sesungguhnya ada di Dialer.
	if addr, err := netip.ParseAddr(host); err == nil {
		if err := CheckAddr(addr, policy); err != nil {
			return err
		}
	}
	// Nama yang terang-terangan menunjuk ke dalam ditolak lebih awal supaya operator
	// mendapat pesan yang jelas, bukan kegagalan koneksi yang membingungkan.
	if isObviousLocalName(host) {
		return fmt.Errorf("%w: %q menunjuk ke mesin ini", ErrBlockedAddress, host)
	}
	return nil
}

// isObviousLocalName melaporkan nama host yang pasti menunjuk ke mesin lokal.
func isObviousLocalName(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost" || strings.HasSuffix(h, ".localhost") || h == "localhost.localdomain"
}

// CheckAddr memeriksa satu alamat IP terhadap kebijakan.
func CheckAddr(addr netip.Addr, policy SSRFPolicy) error {
	if policy.AllowPrivate {
		return nil
	}
	if !addr.IsValid() {
		return fmt.Errorf("%w: alamat tidak sah", ErrBlockedAddress)
	}

	// IPv4 yang dibungkus IPv6 (::ffff:127.0.0.1) harus diperiksa sebagai IPv4-nya,
	// kalau tidak seluruh daftar rentang IPv4 bisa dilewati hanya dengan menuliskan
	// alamat dalam bentuk terbungkus.
	if addr.Is4In6() {
		addr = addr.Unmap()
	}

	for _, p := range blockedPrefixes {
		// Perbandingan hanya bermakna bila keluarga alamatnya sama.
		if p.Addr().Is4() == addr.Is4() && p.Contains(addr) {
			return fmt.Errorf("%w: %s berada di rentang terlarang %s", ErrBlockedAddress, addr, p)
		}
	}
	// Jaring pengaman untuk rentang khusus yang mungkin lolos daftar di atas.
	if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsUnspecified() {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
	}
	return nil
}

// NewSSRFControl mengembalikan fungsi Control untuk net.Dialer.
//
// Inilah lapisan yang menutup DNS rebinding: Control dipanggil setelah nama host
// diresolusi dan tepat sebelum socket tersambung, dengan alamat yang benar-benar akan
// dihubungi. Nama host yang tadi lolos validasi tidak menolong penyerang di titik ini,
// karena yang diperiksa adalah hasil resolusinya.
//
// Kembalian error di sini membatalkan koneksi.
func NewSSRFControl(policy SSRFPolicy) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		switch network {
		case "tcp", "tcp4", "tcp6":
		default:
			// Jaringan selain TCP tidak dipakai klien HTTP kami; menolaknya menutup
			// jalur yang tidak pernah diniatkan ada.
			return fmt.Errorf("%w: jaringan %q tidak diizinkan", ErrBlockedAddress, network)
		}

		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%w: alamat %q tidak bisa diurai", ErrBlockedAddress, address)
		}
		addr, err := netip.ParseAddr(host)
		if err != nil {
			// Pada titik ini host SUDAH berupa alamat IP hasil resolusi. Bila tidak,
			// ada yang tidak sesuai asumsi dan menolak adalah pilihan yang aman.
			return fmt.Errorf("%w: %q bukan alamat IP", ErrBlockedAddress, host)
		}
		return CheckAddr(addr, policy)
	}
}

// NewSSRFDialer membuat dialer yang menolak alamat internal.
func NewSSRFDialer(policy SSRFPolicy, base *net.Dialer) *net.Dialer {
	d := &net.Dialer{}
	if base != nil {
		*d = *base
	}
	d.Control = NewSSRFControl(policy)
	return d
}

// DialContextFunc adalah bentuk fungsi dial yang dipakai http.Transport.
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

// GuardedDialContext mengembalikan fungsi dial berpenjaga untuk http.Transport.
func GuardedDialContext(policy SSRFPolicy, base *net.Dialer) DialContextFunc {
	return NewSSRFDialer(policy, base).DialContext
}
