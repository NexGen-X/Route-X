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
	// AllowedPrivateAddrs adalah jalan tengah yang lebih baik daripada AllowPrivate:
	// hanya ALAMAT tertentu yang dikecualikan, sisanya tetap dijaga. Operator yang
	// menjalankan Ollama di 127.0.0.1:11434 memasang 127.0.0.1/32 di sini dan tidak
	// perlu melepas perlindungan untuk provider lain.
	//
	// Isinya ALAMAT, bukan nama host, dan itu keputusan yang sengaja diambil. Pengecualian
	// ini harus berlaku di DUA tempat: saat base URL divalidasi, dan saat socket benar-benar
	// dibuka. Di tempat kedua nama host sudah tidak ada — yang tersisa hanya hasil
	// resolusinya. Pengecualian bernama karena itu cuma bisa dijalankan dengan meresolusi
	// nama saat kebijakan dibuat, lalu mempercayai hasilnya saat menghubungi; dan itu
	// tepat DNS rebinding yang dijaga lapisan dial. Pengecualian per alamat tidak punya
	// celah itu: nama yang dibelokkan ke alamat privat LAIN tetap ditolak.
	//
	// Pakai ParsePrivateAddrs untuk mengubah masukan operator menjadi nilai ini; ia menolak
	// nama host dengan pesan yang menjelaskan mengapa.
	AllowedPrivateAddrs []netip.Prefix
}

// DefaultSSRFPolicy adalah kebijakan paling ketat: hanya https, tanpa alamat internal.
func DefaultSSRFPolicy() SSRFPolicy { return SSRFPolicy{} }

// ParsePrivateAddrs mengurai daftar literal IP atau CIDR menjadi prefix.
//
// Nama host ditolak, bukan diresolusi: lihat alasannya di AllowedPrivateAddrs. Pesannya
// menyebutkan jalan keluarnya supaya operator tidak menyimpulkan pengecualian ini tidak
// bisa dipakai untuk layanan yang ia kenal lewat nama.
func ParsePrivateAddrs(values []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if pfx, err := netip.ParsePrefix(v); err == nil {
			out = append(out, pfx.Masked())
			continue
		}
		addr, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("%w: %q bukan alamat IP atau CIDR — pengecualian ini "+
				"berlaku per alamat, bukan per nama host; tulis alamat hasil resolusinya",
				ErrInvalidBaseURL, v)
		}
		// Alamat tunggal disimpan sebagai prefix /32 atau /128 supaya pemeriksaannya
		// seragam dengan bentuk CIDR.
		addr = unmap4In6(addr)
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// allowsAddr melaporkan apakah alamat ini dikecualikan dari penjagaan rentang.
func (p SSRFPolicy) allowsAddr(addr netip.Addr) bool {
	if p.AllowPrivate {
		return true
	}
	addr = unmap4In6(addr)
	for _, pfx := range p.AllowedPrivateAddrs {
		// Perbandingan hanya bermakna bila keluarga alamatnya sama.
		if pfx.Addr().Is4() == addr.Is4() && pfx.Contains(addr) {
			return true
		}
	}
	return false
}

// allowsLoopbackName melaporkan apakah nama yang pasti menunjuk ke mesin ini boleh lolos
// validasi URL.
//
// Diperlukan supaya "http://localhost:11434" tetap bisa dipakai operator yang sudah
// mengecualikan loopback lewat alamat. Ini hanya melonggarkan LAPISAN PERTAMA; alamat
// hasil resolusinya tetap diperiksa saat dial, jadi nama yang ternyata menunjuk ke tempat
// lain tetap ditolak di sana.
func (p SSRFPolicy) allowsLoopbackName() bool {
	return p.allowsAddr(netip.MustParseAddr("127.0.0.1")) ||
		p.allowsAddr(netip.MustParseAddr("::1"))
}

// unmap4In6 mengembalikan bentuk IPv4 dari alamat IPv4 yang dibungkus IPv6.
//
// Tanpa ini, ::ffff:127.0.0.1 melewati seluruh daftar rentang IPv4 hanya karena
// dituliskan dalam bentuk terbungkus.
func unmap4In6(addr netip.Addr) netip.Addr {
	if addr.Is4In6() {
		return addr.Unmap()
	}
	return addr
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

	// Bila host sudah berupa alamat IP, bisa langsung diperiksa — termasuk terhadap
	// AllowedPrivateAddrs, karena CheckAddr yang menghormatinya. Bila berupa nama, tidak
	// diresolusi di sini: hasil resolusi saat menyimpan tidak menjamin apa pun tentang
	// hasil resolusi saat menghubungi, dan pemeriksaan yang sesungguhnya ada di Dialer.
	if addr, err := netip.ParseAddr(host); err == nil {
		return CheckAddr(addr, policy)
	}

	// Nama yang terang-terangan menunjuk ke dalam ditolak lebih awal supaya operator
	// mendapat pesan yang jelas, bukan kegagalan koneksi yang membingungkan — kecuali
	// operator memang sudah mengecualikan loopback lewat alamat, yang berarti dia sedang
	// menjalankan model lokal dan menyebutnya "localhost".
	if isObviousLocalName(host) && !policy.allowsLoopbackName() {
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
//
// Inilah satu-satunya tempat keputusan "boleh atau tidak" diambil, dan karena itu ia yang
// menghormati AllowedPrivateAddrs. Sebelumnya pengecualian itu hanya dibaca ValidateBaseURL,
// sehingga operator yang mengizinkan 127.0.0.1 lolos validasi lalu diblokir tepat saat
// menghubungi — jalan keluar yang tidak bekerja ujung-ke-ujung, dan satu-satunya yang
// benar-benar berfungsi adalah AllowPrivate yang melepas perlindungan untuk SELURUH
// provider termasuk BYOK.
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
	addr = unmap4In6(addr)

	// Pengecualian diperiksa SEBELUM daftar terlarang: seluruh gunanya adalah melepas
	// alamat yang justru ada di daftar itu.
	if policy.allowsAddr(addr) {
		return nil
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
