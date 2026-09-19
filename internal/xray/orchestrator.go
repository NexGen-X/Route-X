package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BridgeUser adalah nama pengguna tetap bridge SOCKS5/HTTP internal Xray. Namanya
// sengaja tetap (bukan acak) supaya mudah dikenali di log akses Xray; keamanannya
// bergantung pada kata sandi acak per-instalasi (NewBridgePassword).
const BridgeUser = "routex"

// DefaultBridgeHost adalah alamat jembatan Xray untuk deployment native:
// gateway & Xray berbagi host yang sama sehingga loopback cukup dan aman.
// Docker Compose wajib menyetel XRAY_BRIDGE_HOST=xray (container terpisah) —
// lihat internal/config/config.go & admin.Config.
const DefaultBridgeHost = "127.0.0.1"

// bridgeListenAddress memutuskan alamat listen Xray dari host koneksi yang
// dikonfigurasi. Xray hanya bisa listen pada alamat IP, bukan nama DNS, jadi
// deployment Docker (XRAY_BRIDGE_HOST=xray) harus listen 0.0.0.0 agar container
// xray terjangkau lewat nama layanannya. Itu aman karena tiga lapisan: tidak ada
// port yang di-publish ke host (docker-compose.yml), network bridge terisolasi,
// dan bridge tetap wajib autentikasi (auth: password). Deployment native tetap
// listen 127.0.0.1 murni.
func bridgeListenAddress(bridgeHost string) string {
	if bridgeHost == "127.0.0.1" || bridgeHost == "localhost" || bridgeHost == "::1" {
		return bridgeHost
	}
	return "0.0.0.0"
}

// bridgePasswordAlphabet adalah kumpulan karakter kata sandi bridge Xray: hanya
// alfanumerik tanpa karakter ambigu (O/0/I/l) supaya mudah ditulis manual dan
// aman ditanam di URL tanpa percent-encoding.
const bridgePasswordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// bridgePasswordLength adalah panjang kata sandi bridge: 24 karakter alfabet 56
// memberi ~140 bit entropi, jauh di atas kebutuhan brute-force lokal.
const bridgePasswordLength = 24

// State menyimpan status konfigurasi runtime Xray secara persisten.
// Seluruh parameter ini disimpan di database (identity.SettingsRepo) agar tetap bertahan
// meskipun kontainer Docker di-restart atau di-deploy ulang.
type State struct {
	UUID              string `json:"uuid"`
	RealityPrivateKey string `json:"reality_private_key"`
	RealityPublicKey  string `json:"reality_public_key"`
	RealityShortID    string `json:"reality_short_id"`
	RealityDest       string `json:"reality_dest"`
	RealityServerName string `json:"reality_server_name"`
	Domain            string `json:"domain"`
	// BridgeHost adalah host jembatan SOCKS/HTTP internal: alamat listen di config
	// Xray sekaligus host yang gateway hubungi. Bawaan 127.0.0.1 (satu host);
	// Docker Compose menyetel XRAY_BRIDGE_HOST=xray (container terpisah).
	BridgeHost    string    `json:"bridge_host"`
	SocksUser     string    `json:"socks_user"`
	SocksPassword string    `json:"socks_password"`
	SocksPort     int       `json:"socks_port"`
	HTTPPort      int       `json:"http_port"`
	RealityPort   int       `json:"reality_port"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProtocolItem merangkum metadata satu jenis konfigurasi protokol Xray siap pakai
// untuk disajikan pada UI pengaturan maupun pemilih preset Egress Proxy Pool.
type ProtocolItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`    // vless, socks5, http
	Transport   string `json:"transport"`   // tcp
	Security    string `json:"security"`    // reality, none
	Port        int    `json:"port"`        // 8443, 10808, 10809
	PathOrSNI   string `json:"path_or_sni"` // www.apple.com, xray:10808, dll.
	ShareLink   string `json:"share_link"`  // URI impor standar (vless://, socks5://, http://)
	EgressURL   string `json:"egress_url"`  // URL format egress proxy (misal: socks5://xray:10808)
	Description string `json:"description"` // Penjelasan keunggulan protokol
}

// Links memuat tautan share URL standar VLESS-Reality siap salin untuk klien luar
// beserta koleksi ProtocolItem untuk antarmuka selektor modern.
type Links struct {
	VlessReality  string         `json:"vless_reality"`
	SocksInternal string         `json:"socks_internal"`
	HTTPInternal  string         `json:"http_internal"`
	Protocols     []ProtocolItem `json:"protocols"`
}

// DefaultState mengembalikan konfigurasi awal Xray dengan porta standar,
// kunci kriptografi Reality X25519, dan kredensial aman.
func DefaultState(domain string) State {
	privB64, pubB64, _ := NewX25519KeyPair()
	return State{
		UUID:              NewUUID(),
		RealityPrivateKey: privB64,
		RealityPublicKey:  pubB64,
		RealityShortID:    NewShortID(),
		RealityDest:       "www.apple.com:443",
		RealityServerName: "www.apple.com",
		Domain:            domain,
		BridgeHost:        DefaultBridgeHost,
		SocksUser:         BridgeUser,
		SocksPassword:     NewBridgePassword(),
		SocksPort:         10808,
		HTTPPort:          10809,
		RealityPort:       8443,
		UpdatedAt:         time.Now().UTC(),
	}
}

// EnsureDefaults memastikan bahwa seluruh field yang belum ada pada state yang diambil dari DB lama
// otomatis diisi nilai default yang valid dan aman.
func (s *State) EnsureDefaults(domain string) {
	def := DefaultState(domain)
	if s.Domain == "" {
		s.Domain = domain
	}
	if s.UUID == "" {
		s.UUID = def.UUID
	}
	if s.RealityPrivateKey == "" || s.RealityPublicKey == "" {
		s.RealityPrivateKey = def.RealityPrivateKey
		s.RealityPublicKey = def.RealityPublicKey
	}
	if s.RealityShortID == "" {
		s.RealityShortID = def.RealityShortID
	}
	if s.RealityDest == "" {
		s.RealityDest = def.RealityDest
	}
	if s.RealityServerName == "" {
		s.RealityServerName = def.RealityServerName
	}
	// State lama (pra-PR ini) belum memuat host bridge: isi default loopback.
	// Deployment Docker menyetel nilai ini via XRAY_BRIDGE_HOST saat sinkronisasi.
	if s.BridgeHost == "" {
		s.BridgeHost = DefaultBridgeHost
	}
	if s.SocksUser == "" {
		s.SocksUser = def.SocksUser
	}
	// State lama (pra-PR ini) belum memuat kata sandi bridge: isi sekarang supaya
	// instalasi yang sudah berjalan langsung mendapat autentikasi saat sync berikut.
	if s.SocksPassword == "" {
		s.SocksPassword = def.SocksPassword
	}
	if s.SocksPort == 0 {
		s.SocksPort = def.SocksPort
	}
	if s.HTTPPort == 0 {
		s.HTTPPort = def.HTTPPort
	}
	if s.RealityPort == 0 {
		s.RealityPort = def.RealityPort
	}
}

// NewUUID menghasilkan UUIDv4 murni berbasis crypto/rand tanpa dependensi pustaka luar
// agar aman dari celah prediksi keacakan.
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // versi 4
	b[8] = (b[8] & 0x3f) | 0x80 // varian RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewShortID membuat 8 karakter heksadesimal acak (4 byte) untuk parameter Reality Short ID.
func NewShortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x", b)
}

// NewBridgePassword menghasilkan kata sandi acak untuk autentikasi bridge
// SOCKS5/HTTP internal Xray murni dari crypto/rand. Sampling memakai rejection
// terhadap sisa pembagian alfabet supaya tiap karakter berpeluang sama (tidak
// ada bias modulo), sehingga entropi penuhnya dapat diandalkan.
func NewBridgePassword() string {
	out := make([]byte, 0, bridgePasswordLength)
	// 256 % len(alphabet) = 32; hanya nilai < 224 yang dipakai.
	limit := 256 - (256 % len(bridgePasswordAlphabet))
	buf := make([]byte, bridgePasswordLength*2)
	for len(out) < bridgePasswordLength {
		_, _ = rand.Read(buf)
		for _, c := range buf {
			if int(c) < limit {
				out = append(out, bridgePasswordAlphabet[int(c)%len(bridgePasswordAlphabet)])
				if len(out) == bridgePasswordLength {
					break
				}
			}
		}
	}
	return string(out)
}

// NewX25519KeyPair menghasilkan pasangan kunci privat dan publik kurva eliptis X25519
// dengan format Base64 RawURL standar Xray Reality.
func NewX25519KeyPair() (privB64, pubB64 string, err error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("gagal men-generate pasangan kunci X25519: %w", err)
	}
	privB64 = base64.RawURLEncoding.EncodeToString(priv.Bytes())
	pubB64 = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
	return privB64, pubB64, nil
}

// GenerateConfig menghasilkan berkas JSON lengkap yang valid dan siap dijalankan oleh daemon Xray-core.
func GenerateConfig(s State) ([]byte, error) {
	s.EnsureDefaults(s.Domain)

	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"inbounds": []any{
			// 1. SOCKS5 Bridge Internal — hanya loopback + wajib autentikasi.
			// Bridge ini konsumsinya internal (gateway Route-X di host yang sama),
			// jadi tidak boleh terbuka ke jaringan tanpa kata sandi (celah open proxy).
			map[string]any{
				"tag":      "socks-internal",
				"port":     s.SocksPort,
				"listen":   bridgeListenAddress(s.BridgeHost),
				"protocol": "socks",
				"settings": map[string]any{
					"auth": "password",
					"accounts": []any{
						map[string]any{
							"user": s.SocksUser,
							"pass": s.SocksPassword,
						},
					},
					"udp": true,
				},
			},
			// 2. HTTP Proxy Bridge Internal — loopback + basic auth (accounts).
			map[string]any{
				"tag":      "http-internal",
				"port":     s.HTTPPort,
				"listen":   bridgeListenAddress(s.BridgeHost),
				"protocol": "http",
				"settings": map[string]any{
					"accounts": []any{
						map[string]any{
							"user": s.SocksUser,
							"pass": s.SocksPassword,
						},
					},
					"allowTransparent": false,
				},
			},
			// 3. VLESS-Reality Mandiri (Port 8443 Direct TCP) — tetap mendengarkan
			//    0.0.0.0 karena ini adalah proxy keluar yang sah dari internet:
			//    autentikasinya kriptografis (UUID klien + handshake Reality), bukan
			//    open proxy.
			map[string]any{
				"tag":      "vless-reality-in",
				"port":     s.RealityPort,
				"listen":   "0.0.0.0",
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{
							"id":   s.UUID,
							"flow": "xtls-rprx-vision",
						},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]any{
						"show": false,
						"dest": s.RealityDest,
						"xver": 0,
						"serverNames": []string{
							s.RealityServerName,
							"apple.com",
						},
						"privateKey": s.RealityPrivateKey,
						"shortIds": []string{
							s.RealityShortID,
						},
					},
				},
			},
		},
		"outbounds": []any{
			map[string]any{
				"tag":      "direct",
				"protocol": "freedom",
				"settings": map[string]any{
					"domainStrategy": "UseIP",
				},
			},
			map[string]any{
				"tag":      "blocked",
				"protocol": "blackhole",
				"settings": map[string]any{},
			},
		},
		"routing": map[string]any{
			"domainStrategy": "IPIfNonMatch",
			"rules": []any{
				map[string]any{
					"type":        "field",
					"outboundTag": "blocked",
					"ip": []string{
						"geoip:private",
					},
				},
				map[string]any{
					"type":        "field",
					"outboundTag": "direct",
					"network":     "tcp,udp",
				},
			},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// BridgeEgressURL menyusun URL SOCKS5 bridge internal lengkap dengan kredensial,
// untuk dipakai gateway (Egress Pool) saat merutekan lalu lintas melalui Xray.
// Hasilnya tidak boleh masuk log maupun respons API; pemanggilnya sebaiknya
// langsung membungkusnya menjadi security.Secret.
func BridgeEgressURL(s State) string {
	s.EnsureDefaults(s.Domain)
	return fmt.Sprintf("socks5://%s:%s@%s:%d", s.SocksUser, s.SocksPassword, s.BridgeHost, s.SocksPort)
}

// GenerateShareLinks membuat tautan impor standar untuk protokol Xray aktif.
func GenerateShareLinks(s State) Links {
	s.EnsureDefaults(s.Domain)
	host := s.Domain
	if host == "" {
		host = "example.com"
	}

	// 1. VLESS Reality
	realitySNI := s.RealityServerName
	if realitySNI == "" {
		realitySNI = "www.apple.com"
	}
	realityPort := s.RealityPort
	if realityPort == 0 {
		realityPort = 8443
	}
	vlessReality := fmt.Sprintf(
		"vless://%s@%s:%d?type=tcp&security=reality&pbk=%s&fp=chrome&sni=%s&sid=%s&spx=%%2F&flow=xtls-rprx-vision#RouteX-VLESS-Reality",
		s.UUID, host, realityPort, s.RealityPublicKey, realitySNI, s.RealityShortID,
	)

	// 2. Bridge Internal — ditampilkan TANPA kredensial: kata sandi bridge adalah
	//    rahasia yang disimpan terenkripsi di database dan hanya dibaca gateway
	//    (lewat BridgeEgressURL) untuk Egress Pool. Tidak ada alasan UI/browser
	//    melihatnya, berbeda dengan UUID VLESS yang memang perlu disalin user.
	socksInternal := fmt.Sprintf("socks5://%s:%d", s.BridgeHost, s.SocksPort)
	httpInternal := fmt.Sprintf("http://%s:%d", s.BridgeHost, s.HTTPPort)

	// Susun daftar ProtocolItem (hanya protokol aktif: Reality & Local Bridges)
	protocols := []ProtocolItem{
		{
			ID:          "vless-reality",
			Name:        "VLESS Reality (XTLS-Vision)",
			Protocol:    "vless",
			Transport:   "tcp",
			Security:    "reality",
			Port:        realityPort,
			PathOrSNI:   realitySNI,
			ShareLink:   vlessReality,
			EgressURL:   socksInternal,
			Description: "Teknologi kamuflase TLS tanpa domain dengan meminjam handshake sertifikat situs raksasa dunia.",
		},
		{
			ID:          "socks-internal",
			Name:        "⚡ Xray Internal SOCKS5 Bridge",
			Protocol:    "socks5",
			Transport:   "tcp",
			Security:    "none",
			Port:        s.SocksPort,
			PathOrSNI:   fmt.Sprintf("%s:%d", s.BridgeHost, s.SocksPort),
			ShareLink:   socksInternal,
			EgressURL:   socksInternal,
			Description: "Jembatan internal loopback antar-komponen Route-X untuk routing proxy upstream AI. Wajib autentikasi; kredensial diisi otomatis dan disimpan terenkripsi.",
		},
		{
			ID:          "http-internal",
			Name:        "⚡ Xray Internal HTTP Proxy Bridge",
			Protocol:    "http",
			Transport:   "tcp",
			Security:    "none",
			Port:        s.HTTPPort,
			PathOrSNI:   fmt.Sprintf("%s:%d", s.BridgeHost, s.HTTPPort),
			ShareLink:   httpInternal,
			EgressURL:   httpInternal,
			Description: "Jembatan HTTP CONNECT loopback internal untuk aplikasi yang hanya mendukung proxy HTTP standar. Wajib autentikasi; kredensial diisi otomatis dan disimpan terenkripsi.",
		},
	}

	return Links{
		VlessReality:  vlessReality,
		SocksInternal: socksInternal,
		HTTPInternal:  httpInternal,
		Protocols:     protocols,
	}
}

// WriteConfigFile menulis berkas konfigurasi secara atomik untuk mencegah berkas terbaca
// setengah jadi oleh proses Xray.
func WriteConfigFile(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("gagal membuat direktori konfigurasi Xray %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("gagal membatasi izin direktori konfigurasi Xray %s: %w", dir, err)
	}

	tmpFile := targetPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("gagal menulis berkas sementara Xray %s: %w", tmpFile, err)
	}

	if err := os.Rename(tmpFile, targetPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("gagal memindahkan konfigurasi Xray ke %s: %w", targetPath, err)
	}
	if err := os.Chmod(targetPath, 0600); err != nil {
		return fmt.Errorf("gagal membatasi izin konfigurasi Xray %s: %w", targetPath, err)
	}
	return nil
}

// SyncToFileCandidates menulis data konfigurasi ke setiap kandidat path yang dapat
// ditulis, bukan hanya yang pertama. Alasannya topologi deployment berbeda-beda:
// systemd native membaca /var/lib/route-x/xray/config.json, sedangkan Docker
// Compose hanya mengekspos config.runtime.json lewat volume bersama ke container
// Xray terpisah. Menulis ke keduanya menjamin dua jalur itu tidak drift.
func SyncToFileCandidates(data []byte, candidatePaths ...string) (string, error) {
	var lastErr error
	written := ""
	wroteAny := false
	for _, p := range candidatePaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := WriteConfigFile(p, data); err == nil {
			if !wroteAny {
				written = p
			}
			wroteAny = true
		} else {
			lastErr = err
		}
	}
	if wroteAny {
		return written, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("tidak ada kandidat lokasi berkas konfigurasi Xray yang dapat ditulis")
}
