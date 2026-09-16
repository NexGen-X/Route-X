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

// State menyimpan status konfigurasi runtime Xray secara persisten.
// Seluruh parameter ini disimpan di database (identity.SettingsRepo) agar tetap bertahan
// meskipun kontainer Docker di-restart atau di-deploy ulang.
type State struct {
	UUID              string    `json:"uuid"`
	RealityPrivateKey string    `json:"reality_private_key"`
	RealityPublicKey  string    `json:"reality_public_key"`
	RealityShortID    string    `json:"reality_short_id"`
	RealityDest       string    `json:"reality_dest"`
	RealityServerName string    `json:"reality_server_name"`
	Domain            string    `json:"domain"`
	SocksPort         int       `json:"socks_port"`
	HTTPPort          int       `json:"http_port"`
	RealityPort       int       `json:"reality_port"`
	UpdatedAt         time.Time `json:"updated_at"`
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
			// 1. SOCKS5 Bridge Internal
			map[string]any{
				"tag":      "socks-internal",
				"port":     s.SocksPort,
				"listen":   "0.0.0.0",
				"protocol": "socks",
				"settings": map[string]any{
					"auth": "noauth",
					"udp":  true,
				},
			},
			// 2. HTTP Proxy Bridge Internal
			map[string]any{
				"tag":      "http-internal",
				"port":     s.HTTPPort,
				"listen":   "0.0.0.0",
				"protocol": "http",
				"settings": map[string]any{
					"allowTransparent": false,
				},
			},
			// 3. VLESS-Reality Mandiri (Port 8443 Direct TCP)
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

	// 2. Bridge Internal
	socksInternal := fmt.Sprintf("socks5://xray:%d", s.SocksPort)
	httpInternal := fmt.Sprintf("http://xray:%d", s.HTTPPort)

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
			PathOrSNI:   fmt.Sprintf("xray:%d", s.SocksPort),
			ShareLink:   socksInternal,
			EgressURL:   socksInternal,
			Description: "Jembatan internal antar-kontainer Docker untuk routing proxy upstream AI Route-X.",
		},
		{
			ID:          "http-internal",
			Name:        "⚡ Xray Internal HTTP Proxy Bridge",
			Protocol:    "http",
			Transport:   "tcp",
			Security:    "none",
			Port:        s.HTTPPort,
			PathOrSNI:   fmt.Sprintf("xray:%d", s.HTTPPort),
			ShareLink:   httpInternal,
			EgressURL:   httpInternal,
			Description: "Jembatan HTTP CONNECT internal untuk aplikasi yang hanya mendukung proxy HTTP standar.",
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

// SyncToFileCandidates mencoba menulis data konfigurasi ke salah satu kandidat path yang dapat ditulis.
func SyncToFileCandidates(data []byte, candidatePaths ...string) (string, error) {
	var lastErr error
	for _, p := range candidatePaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := WriteConfigFile(p, data); err == nil {
			return p, nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("tidak ada kandidat lokasi berkas konfigurasi Xray yang dapat ditulis")
}
