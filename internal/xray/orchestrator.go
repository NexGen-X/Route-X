package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
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
	TrojanPassword    string    `json:"trojan_password"`
	ShadowsocksPass   string    `json:"shadowsocks_password"`
	RealityPrivateKey string    `json:"reality_private_key"`
	RealityPublicKey  string    `json:"reality_public_key"`
	RealityShortID    string    `json:"reality_short_id"`
	RealityDest       string    `json:"reality_dest"`
	RealityServerName string    `json:"reality_server_name"`
	Domain            string    `json:"domain"`
	SocksPort         int       `json:"socks_port"`
	HTTPPort          int       `json:"http_port"`
	VlessWSPort       int       `json:"vless_ws_port"`
	VlessGRPCPort     int       `json:"vless_grpc_port"`
	TrojanWSPort      int       `json:"trojan_ws_port"`
	TrojanGRPCPort    int       `json:"trojan_grpc_port"`
	VmessWSPort       int       `json:"vmess_ws_port"`
	VmessGRPCPort     int       `json:"vmess_grpc_port"`
	ShadowsocksWSPort int       `json:"shadowsocks_ws_port"`
	VlessXHTTPPort    int       `json:"vless_xhttp_port"`
	RealityPort       int       `json:"reality_port"`
	ShadowsocksPort   int       `json:"shadowsocks_port"`
	VlessWSPath       string    `json:"vless_ws_path"`
	VlessGRPCService  string    `json:"vless_grpc_service"`
	TrojanWSPath      string    `json:"trojan_ws_path"`
	TrojanGRPCService string    `json:"trojan_grpc_service"`
	VmessWSPath       string    `json:"vmess_ws_path"`
	VmessGRPCService  string    `json:"vmess_grpc_service"`
	ShadowsocksWSPath string    `json:"shadowsocks_ws_path"`
	VlessXHTTPPath    string    `json:"vless_xhttp_path"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ProtocolItem merangkum metadata satu jenis konfigurasi protokol Xray siap pakai
// untuk disajikan pada UI pengaturan maupun pemilih preset Egress Proxy Pool.
type ProtocolItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`    // vless, trojan, vmess, shadowsocks, socks5, http
	Transport   string `json:"transport"`   // grpc, ws, splithttp, tcp
	Security    string `json:"security"`    // tls, reality, none
	Port        int    `json:"port"`        // 443, 8443, 8388, 10808, 10809
	PathOrSNI   string `json:"path_or_sni"` // /routex-xray-ws, routex-grpc, www.apple.com, dll.
	ShareLink   string `json:"share_link"`  // URI impor standar (vless://, trojan://, vmess://, ss://)
	EgressURL   string `json:"egress_url"`  // URL format egress proxy (misal: socks5://xray:10808)
	Description string `json:"description"` // Penjelasan keunggulan protokol
}

// Links memuat seluruh tautan share URL standar V2Ray/Xray siap salin untuk klien luar
// beserta koleksi ProtocolItem untuk antarmuka selektor modern.
type Links struct {
	VlessWS       string         `json:"vless_ws"`
	VlessGRPC     string         `json:"vless_grpc"`
	VlessXHTTP    string         `json:"vless_xhttp"`
	VlessReality  string         `json:"vless_reality"`
	TrojanWS      string         `json:"trojan_ws"`
	TrojanGRPC    string         `json:"trojan_grpc"`
	VmessWS       string         `json:"vmess_ws"`
	VmessGRPC     string         `json:"vmess_grpc"`
	ShadowsocksWS string         `json:"shadowsocks_ws"`
	Shadowsocks   string         `json:"shadowsocks"`
	SocksInternal string         `json:"socks_internal"`
	HTTPInternal  string         `json:"http_internal"`
	Protocols     []ProtocolItem `json:"protocols"`
}

// DefaultState mengembalikan konfigurasi komprehensif awal Xray dengan porta standar,
// path stealth untuk Caddy reverse-proxy, kunci kriptografi Reality X25519, dan kredensial aman.
func DefaultState(domain string) State {
	privB64, pubB64, _ := NewX25519KeyPair()
	return State{
		UUID:              NewUUID(),
		TrojanPassword:    NewSecretToken(24),
		ShadowsocksPass:   NewSecretToken(16),
		RealityPrivateKey: privB64,
		RealityPublicKey:  pubB64,
		RealityShortID:    NewShortID(),
		RealityDest:       "www.apple.com:443",
		RealityServerName: "www.apple.com",
		Domain:            domain,
		SocksPort:         10808,
		HTTPPort:          10809,
		VlessWSPort:       10001,
		VlessGRPCPort:     10002,
		TrojanWSPort:      10003,
		TrojanGRPCPort:    10004,
		VmessWSPort:       10005,
		VmessGRPCPort:     10006,
		ShadowsocksWSPort: 10007,
		VlessXHTTPPort:    10008,
		RealityPort:       8443,
		ShadowsocksPort:   8388,
		VlessWSPath:       "/routex-xray-ws",
		VlessGRPCService:  "routex-grpc",
		TrojanWSPath:      "/routex-trojan-ws",
		TrojanGRPCService: "routex-trojan-grpc",
		VmessWSPath:       "/routex-vmess-ws",
		VmessGRPCService:  "routex-vmess-grpc",
		ShadowsocksWSPath: "/routex-ss-ws",
		VlessXHTTPPath:    "/routex-xhttp",
		UpdatedAt:         time.Now().UTC(),
	}
}

// EnsureDefaults memastikan bahwa seluruh field yang belum ada pada state yang diambil dari DB lama
// otomatis diisi nilai default yang valid dan aman, mencegah bug path atau password kosong.
func (s *State) EnsureDefaults(domain string) {
	def := DefaultState(domain)
	if s.Domain == "" {
		s.Domain = domain
	}
	if s.UUID == "" {
		s.UUID = def.UUID
	}
	if s.TrojanPassword == "" {
		s.TrojanPassword = def.TrojanPassword
	}
	if s.ShadowsocksPass == "" {
		s.ShadowsocksPass = def.ShadowsocksPass
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
	if s.VlessWSPort == 0 {
		s.VlessWSPort = def.VlessWSPort
	}
	if s.VlessGRPCPort == 0 {
		s.VlessGRPCPort = def.VlessGRPCPort
	}
	if s.TrojanWSPort == 0 {
		s.TrojanWSPort = def.TrojanWSPort
	}
	if s.TrojanGRPCPort == 0 {
		s.TrojanGRPCPort = def.TrojanGRPCPort
	}
	if s.VmessWSPort == 0 {
		s.VmessWSPort = def.VmessWSPort
	}
	if s.VmessGRPCPort == 0 {
		s.VmessGRPCPort = def.VmessGRPCPort
	}
	if s.ShadowsocksWSPort == 0 {
		s.ShadowsocksWSPort = def.ShadowsocksWSPort
	}
	if s.VlessXHTTPPort == 0 {
		s.VlessXHTTPPort = def.VlessXHTTPPort
	}
	if s.RealityPort == 0 {
		s.RealityPort = def.RealityPort
	}
	if s.ShadowsocksPort == 0 {
		s.ShadowsocksPort = def.ShadowsocksPort
	}
	if s.VlessWSPath == "" {
		s.VlessWSPath = def.VlessWSPath
	}
	if s.VlessGRPCService == "" {
		s.VlessGRPCService = def.VlessGRPCService
	}
	if s.TrojanWSPath == "" {
		s.TrojanWSPath = def.TrojanWSPath
	}
	if s.TrojanGRPCService == "" {
		s.TrojanGRPCService = def.TrojanGRPCService
	}
	if s.VmessWSPath == "" {
		s.VmessWSPath = def.VmessWSPath
	}
	if s.VmessGRPCService == "" {
		s.VmessGRPCService = def.VmessGRPCService
	}
	if s.ShadowsocksWSPath == "" {
		s.ShadowsocksWSPath = def.ShadowsocksWSPath
	}
	if s.VlessXHTTPPath == "" {
		s.VlessXHTTPPath = def.VlessXHTTPPath
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

// NewSecretToken membuat string acak heksadesimal aman untuk kata sandi Trojan dan Shadowsocks.
func NewSecretToken(nBytes int) string {
	if nBytes <= 0 {
		nBytes = 16
	}
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewShortID membuat 8 karakter heksadesimal acak (4 byte) untuk parameter Reality Short ID.
func NewShortID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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
			// 3. VLESS over WebSocket (Port 443 Multiplexed)
			map[string]any{
				"tag":      "vless-ws-in",
				"port":     s.VlessWSPort,
				"listen":   "0.0.0.0",
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"id": s.UUID, "level": 0},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network": "ws",
					"wsSettings": map[string]any{
						"path": s.VlessWSPath,
					},
				},
			},
			// 4. VLESS over gRPC (Port 443 Multiplexed)
			map[string]any{
				"tag":      "vless-grpc-in",
				"port":     s.VlessGRPCPort,
				"listen":   "0.0.0.0",
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"id": s.UUID, "level": 0},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network": "grpc",
					"grpcSettings": map[string]any{
						"serviceName": s.VlessGRPCService,
					},
				},
			},
			// 5. Trojan over WebSocket (Port 443 Multiplexed)
			map[string]any{
				"tag":      "trojan-ws-in",
				"port":     s.TrojanWSPort,
				"listen":   "0.0.0.0",
				"protocol": "trojan",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"password": s.TrojanPassword, "level": 0},
					},
				},
				"streamSettings": map[string]any{
					"network": "ws",
					"wsSettings": map[string]any{
						"path": s.TrojanWSPath,
					},
				},
			},
			// 6. Trojan over gRPC (Port 443 Multiplexed)
			map[string]any{
				"tag":      "trojan-grpc-in",
				"port":     s.TrojanGRPCPort,
				"listen":   "0.0.0.0",
				"protocol": "trojan",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"password": s.TrojanPassword, "level": 0},
					},
				},
				"streamSettings": map[string]any{
					"network": "grpc",
					"grpcSettings": map[string]any{
						"serviceName": s.TrojanGRPCService,
					},
				},
			},
			// 7. VMess over WebSocket (Port 443 Multiplexed)
			map[string]any{
				"tag":      "vmess-ws-in",
				"port":     s.VmessWSPort,
				"listen":   "0.0.0.0",
				"protocol": "vmess",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"id": s.UUID, "alterId": 0},
					},
				},
				"streamSettings": map[string]any{
					"network": "ws",
					"wsSettings": map[string]any{
						"path": s.VmessWSPath,
					},
				},
			},
			// 8. VMess over gRPC (Port 443 Multiplexed)
			map[string]any{
				"tag":      "vmess-grpc-in",
				"port":     s.VmessGRPCPort,
				"listen":   "0.0.0.0",
				"protocol": "vmess",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"id": s.UUID, "alterId": 0},
					},
				},
				"streamSettings": map[string]any{
					"network": "grpc",
					"grpcSettings": map[string]any{
						"serviceName": s.VmessGRPCService,
					},
				},
			},
			// 9. Shadowsocks over WebSocket (Port 443 Multiplexed)
			map[string]any{
				"tag":      "ss-ws-in",
				"port":     s.ShadowsocksWSPort,
				"listen":   "0.0.0.0",
				"protocol": "shadowsocks",
				"settings": map[string]any{
					"method":   "aes-128-gcm",
					"password": s.ShadowsocksPass,
					"network":  "tcp",
				},
				"streamSettings": map[string]any{
					"network": "ws",
					"wsSettings": map[string]any{
						"path": s.ShadowsocksWSPath,
					},
				},
			},
			// 10. VLESS over SplitHTTP / XHTTP (Port 443 Multiplexed)
			map[string]any{
				"tag":      "vless-xhttp-in",
				"port":     s.VlessXHTTPPort,
				"listen":   "0.0.0.0",
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"id": s.UUID, "level": 0},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]any{
					"network": "splithttp",
					"splithttpSettings": map[string]any{
						"path": s.VlessXHTTPPath,
					},
				},
			},
			// 11. VLESS-Reality Mandiri (Port 8443 Direct TCP)
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
			// 12. Shadowsocks Mandiri (Port 8388 Standalone TCP & UDP)
			map[string]any{
				"tag":      "ss-standalone-in",
				"port":     s.ShadowsocksPort,
				"listen":   "0.0.0.0",
				"protocol": "shadowsocks",
				"settings": map[string]any{
					"method":   "aes-128-gcm",
					"password": s.ShadowsocksPass,
					"network":  "tcp,udp",
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

// GenerateShareLinks membuat tautan impor standar untuk seluruh kombinasi protokol Xray.
func GenerateShareLinks(s State) Links {
	s.EnsureDefaults(s.Domain)
	host := s.Domain
	if host == "" {
		host = "example.com"
	}

	// 1. VLESS WS
	vlessWS := fmt.Sprintf(
		"vless://%s@%s:443?type=ws&security=tls&path=%s#RouteX-VLESS-WS",
		s.UUID, host, url.QueryEscape(s.VlessWSPath),
	)

	// 2. VLESS gRPC
	vlessGRPC := fmt.Sprintf(
		"vless://%s@%s:443?type=grpc&security=tls&serviceName=%s#RouteX-VLESS-gRPC",
		s.UUID, host, url.QueryEscape(s.VlessGRPCService),
	)

	// 3. VLESS XHTTP (SplitHTTP)
	vlessXHTTP := fmt.Sprintf(
		"vless://%s@%s:443?type=splithttp&security=tls&path=%s#RouteX-VLESS-XHTTP",
		s.UUID, host, url.QueryEscape(s.VlessXHTTPPath),
	)

	// 4. VLESS Reality
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

	// 5. Trojan WS
	trojanWS := fmt.Sprintf(
		"trojan://%s@%s:443?type=ws&security=tls&path=%s#RouteX-Trojan-WS",
		s.TrojanPassword, host, url.QueryEscape(s.TrojanWSPath),
	)

	// 6. Trojan gRPC
	trojanGRPC := fmt.Sprintf(
		"trojan://%s@%s:443?type=grpc&security=tls&serviceName=%s#RouteX-Trojan-gRPC",
		s.TrojanPassword, host, url.QueryEscape(s.TrojanGRPCService),
	)

	// 7. VMess WS (Base64 JSON)
	vmessWSObj := map[string]any{
		"v":    "2",
		"ps":   "RouteX-VMess-WS",
		"add":  host,
		"port": 443,
		"id":   s.UUID,
		"aid":  0,
		"scy":  "auto",
		"net":  "ws",
		"type": "none",
		"host": host,
		"path": s.VmessWSPath,
		"tls":  "tls",
		"sni":  host,
		"alpn": "",
	}
	vmessWSBytes, _ := json.Marshal(vmessWSObj)
	vmessWS := "vmess://" + base64.StdEncoding.EncodeToString(vmessWSBytes)

	// 8. VMess gRPC (Base64 JSON)
	vmessGRPCObj := map[string]any{
		"v":    "2",
		"ps":   "RouteX-VMess-gRPC",
		"add":  host,
		"port": 443,
		"id":   s.UUID,
		"aid":  0,
		"scy":  "auto",
		"net":  "grpc",
		"type": "none",
		"host": "",
		"path": s.VmessGRPCService,
		"tls":  "tls",
		"sni":  host,
		"alpn": "",
	}
	vmessGRPCBytes, _ := json.Marshal(vmessGRPCObj)
	vmessGRPC := "vmess://" + base64.StdEncoding.EncodeToString(vmessGRPCBytes)

	// 9. Shadowsocks WS
	ssWSAuth := base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:" + s.ShadowsocksPass))
	ssWS := fmt.Sprintf(
		"ss://%s@%s:443?plugin=v2ray-plugin%%3Bpath%%3D%s%%3Bhost%%3D%s%%3Btls#RouteX-Shadowsocks-WS",
		ssWSAuth, host, url.QueryEscape(s.ShadowsocksWSPath), host,
	)

	// 10. Shadowsocks Standalone
	ssPort := s.ShadowsocksPort
	if ssPort == 0 {
		ssPort = 8388
	}
	ssAuth := base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:" + s.ShadowsocksPass))
	shadowsocks := fmt.Sprintf("ss://%s@%s:%d#RouteX-Shadowsocks-2022", ssAuth, host, ssPort)

	// 11. Bridge Internal
	socksInternal := "socks5://xray:10808"
	httpInternal := "http://xray:10809"

	// Susun daftar ProtocolItem
	protocols := []ProtocolItem{
		{
			ID:          "vless-grpc",
			Name:        "VLESS over gRPC",
			Protocol:    "vless",
			Transport:   "grpc",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.VlessGRPCService,
			ShareLink:   vlessGRPC,
			EgressURL:   socksInternal,
			Description: "Latensi ultra-rendah dengan multiplexing HTTP/2 gRPC resmi.",
		},
		{
			ID:          "vless-ws",
			Name:        "VLESS over WebSocket",
			Protocol:    "vless",
			Transport:   "ws",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.VlessWSPath,
			ShareLink:   vlessWS,
			EgressURL:   socksInternal,
			Description: "Kompatibilitas universal untuk melewati firewall dan CDN reverse proxy.",
		},
		{
			ID:          "vless-xhttp",
			Name:        "VLESS over SplitHTTP (XHTTP)",
			Protocol:    "vless",
			Transport:   "splithttp",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.VlessXHTTPPath,
			ShareLink:   vlessXHTTP,
			EgressURL:   socksInternal,
			Description: "Transport tercanggih di Xray versi terbaru, dirancang khusus anti pemblokiran CDN.",
		},
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
			Description: "Teknologi kamuflase TLS tanpa domain dengan meminjam sertifikat situs raksasa dunia.",
		},
		{
			ID:          "trojan-grpc",
			Name:        "Trojan over gRPC",
			Protocol:    "trojan",
			Transport:   "grpc",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.TrojanGRPCService,
			ShareLink:   trojanGRPC,
			EgressURL:   socksInternal,
			Description: "Protokol autentikasi sandi murni dengan transport multiplexing gRPC.",
		},
		{
			ID:          "trojan-ws",
			Name:        "Trojan over WebSocket",
			Protocol:    "trojan",
			Transport:   "ws",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.TrojanWSPath,
			ShareLink:   trojanWS,
			EgressURL:   socksInternal,
			Description: "Protokol penyamaran HTTPS dengan WebSocket di balik terminasi TLS Caddy.",
		},
		{
			ID:          "vmess-grpc",
			Name:        "VMess over gRPC",
			Protocol:    "vmess",
			Transport:   "grpc",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.VmessGRPCService,
			ShareLink:   vmessGRPC,
			EgressURL:   socksInternal,
			Description: "Protokol klasik V2Ray dengan transport modern gRPC dan enkripsi zero-alterId.",
		},
		{
			ID:          "vmess-ws",
			Name:        "VMess over WebSocket",
			Protocol:    "vmess",
			Transport:   "ws",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.VmessWSPath,
			ShareLink:   vmessWS,
			EgressURL:   socksInternal,
			Description: "Format tautan VMess Base64 terstandarisasi untuk semua aplikasi klien v2ray.",
		},
		{
			ID:          "ss-ws",
			Name:        "Shadowsocks over WebSocket",
			Protocol:    "shadowsocks",
			Transport:   "ws",
			Security:    "tls",
			Port:        443,
			PathOrSNI:   s.ShadowsocksWSPath,
			ShareLink:   ssWS,
			EgressURL:   socksInternal,
			Description: "Shadowsocks enkripsi AES-128-GCM dimultipleks dalam WebSocket Port 443.",
		},
		{
			ID:          "ss-standalone",
			Name:        "Shadowsocks 2022 Standalone",
			Protocol:    "shadowsocks",
			Transport:   "tcp",
			Security:    "none",
			Port:        ssPort,
			PathOrSNI:   "-",
			ShareLink:   shadowsocks,
			EgressURL:   socksInternal,
			Description: "Port langsung berkecepatan tinggi tanpa overhead TLS untuk router dan perangkat IoT.",
		},
		{
			ID:          "socks-internal",
			Name:        "⚡ Xray Internal SOCKS5 Bridge",
			Protocol:    "socks5",
			Transport:   "tcp",
			Security:    "none",
			Port:        10808,
			PathOrSNI:   "xray:10808",
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
			Port:        10809,
			PathOrSNI:   "xray:10809",
			ShareLink:   httpInternal,
			EgressURL:   httpInternal,
			Description: "Jembatan HTTP CONNECT internal untuk aplikasi yang hanya mendukung proxy HTTP standar.",
		},
	}

	return Links{
		VlessWS:       vlessWS,
		VlessGRPC:     vlessGRPC,
		VlessXHTTP:    vlessXHTTP,
		VlessReality:  vlessReality,
		TrojanWS:      trojanWS,
		TrojanGRPC:    trojanGRPC,
		VmessWS:       vmessWS,
		VmessGRPC:     vmessGRPC,
		ShadowsocksWS: ssWS,
		Shadowsocks:   shadowsocks,
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
