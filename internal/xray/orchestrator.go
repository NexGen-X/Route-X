package xray

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State menyimpan status konfigurasi runtime Xray.
// Seluruh parameter ini disimpan di database (identity.SettingsRepo) agar tetap bertahan
// meskipun kontainer Docker di-restart.
type State struct {
	UUID             string    `json:"uuid"`
	TrojanPassword   string    `json:"trojan_password"`
	Domain           string    `json:"domain"`
	SocksPort        int       `json:"socks_port"`
	VlessWSPort      int       `json:"vless_ws_port"`
	VlessGRPCPort    int       `json:"vless_grpc_port"`
	TrojanWSPort     int       `json:"trojan_ws_port"`
	VlessWSPath      string    `json:"vless_ws_path"`
	VlessGRPCService string    `json:"vless_grpc_service"`
	TrojanWSPath     string    `json:"trojan_ws_path"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Links memuat tautan share URL standar V2Ray/Xray yang dapat disalin oleh pengguna
// ke aplikasi client seperti v2rayN, Shadowrocket, Sing-box, atau Nekoray.
type Links struct {
	VlessWS   string `json:"vless_ws"`
	VlessGRPC string `json:"vless_grpc"`
	TrojanWS  string `json:"trojan_ws"`
}

// DefaultState mengembalikan nilai awal konfigurasi Xray dengan porta standar
// dan path tersembunyi (stealth path) untuk reverse proxy Caddy.
func DefaultState(domain string) State {
	return State{
		UUID:             NewUUID(),
		TrojanPassword:   NewSecretToken(24),
		Domain:           domain,
		SocksPort:        10808,
		VlessWSPort:      10001,
		VlessGRPCPort:    10002,
		TrojanWSPort:     10003,
		VlessWSPath:      "/routex-xray-ws",
		VlessGRPCService: "routex-grpc",
		TrojanWSPath:     "/routex-trojan-ws",
		UpdatedAt:        time.Now().UTC(),
	}
}

// NewUUID menghasilkan UUIDv4 kriptografis murni tanpa dependensi eksternal.
// Menggunakan crypto/rand agar ID aman dari serangan prediksi acak.
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // versi 4
	b[8] = (b[8] & 0x3f) | 0x80 // varian RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewSecretToken membuat string acak heksadesimal aman untuk password Trojan.
func NewSecretToken(nBytes int) string {
	if nBytes <= 0 {
		nBytes = 16
	}
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateConfig menghasilkan konfigurasi lengkap config.json yang valid untuk Xray-core.
//
// Keputusan Desain:
//  1. Inbound SOCKS5 (porta 10808) dibuka tanpa autentikasi khusus untuk jaringan internal Docker
//     (routex_internal), sehingga routex-gateway dapat melakukan routing lalu lintas keluar
//     langsung tanpa overhead enkripsi berulang.
//  2. Inbound VLESS dan Trojan dibuka pada porta internal yang dipetakan ke Caddy reverse-proxy.
//     Caddy bertindak sebagai pengelola sertifikat TLS resmi (Let's Encrypt / ZeroSSL).
//  3. Outbound menggunakan protokol "freedom" untuk akses langsung ke internet global.
func GenerateConfig(s State) ([]byte, error) {
	if s.UUID == "" {
		s.UUID = NewUUID()
	}
	if s.TrojanPassword == "" {
		s.TrojanPassword = NewSecretToken(24)
	}
	if s.SocksPort == 0 {
		s.SocksPort = 10808
	}
	if s.VlessWSPort == 0 {
		s.VlessWSPort = 10001
	}
	if s.VlessGRPCPort == 0 {
		s.VlessGRPCPort = 10002
	}
	if s.TrojanWSPort == 0 {
		s.TrojanWSPort = 10003
	}
	if s.VlessWSPath == "" {
		s.VlessWSPath = "/routex-xray-ws"
	}
	if s.VlessGRPCService == "" {
		s.VlessGRPCService = "routex-grpc"
	}
	if s.TrojanWSPath == "" {
		s.TrojanWSPath = "/routex-trojan-ws"
	}

	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"inbounds": []any{
			// Inbound 1: SOCKS5 Internal Bridge untuk Egress Route-X Gateway
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
			// Inbound 2: VLESS WebSocket Inbound (dimultipleks oleh Caddy di Port 443)
			map[string]any{
				"tag":      "vless-ws-in",
				"port":     s.VlessWSPort,
				"listen":   "0.0.0.0",
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{
							"id":    s.UUID,
							"level": 0,
						},
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
			// Inbound 3: VLESS gRPC Inbound (dimultipleks oleh Caddy di Port 443)
			map[string]any{
				"tag":      "vless-grpc-in",
				"port":     s.VlessGRPCPort,
				"listen":   "0.0.0.0",
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{
							"id":    s.UUID,
							"level": 0,
						},
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
			// Inbound 4: Trojan WebSocket Inbound (dimultipleks oleh Caddy di Port 443)
			map[string]any{
				"tag":      "trojan-ws-in",
				"port":     s.TrojanWSPort,
				"listen":   "0.0.0.0",
				"protocol": "trojan",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{
							"password": s.TrojanPassword,
							"level":    0,
						},
					},
				},
				"streamSettings": map[string]any{
					"network": "ws",
					"wsSettings": map[string]any{
						"path": s.TrojanWSPath,
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

// GenerateShareLinks membuat URI shareable standar yang siap diimpor ke client proxy luar.
func GenerateShareLinks(s State) Links {
	if s.Domain == "" || s.UUID == "" {
		return Links{}
	}

	vlessWS := fmt.Sprintf(
		"vless://%s@%s:443?type=ws&security=tls&path=%s#RouteX-VLESS-WS",
		s.UUID,
		s.Domain,
		url.QueryEscape(s.VlessWSPath),
	)

	vlessGRPC := fmt.Sprintf(
		"vless://%s@%s:443?type=grpc&security=tls&serviceName=%s#RouteX-VLESS-gRPC",
		s.UUID,
		s.Domain,
		url.QueryEscape(s.VlessGRPCService),
	)

	trojanWS := fmt.Sprintf(
		"trojan://%s@%s:443?type=ws&security=tls&path=%s#RouteX-Trojan-WS",
		s.TrojanPassword,
		s.Domain,
		url.QueryEscape(s.TrojanWSPath),
	)

	return Links{
		VlessWS:   vlessWS,
		VlessGRPC: vlessGRPC,
		TrojanWS:  trojanWS,
	}
}

// WriteConfigFile menulis berkas konfigurasi secara atomik untuk mencegah berkas terbaca
// setengah jadi oleh proses Xray.
func WriteConfigFile(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("gagal membuat direktori konfigurasi Xray %s: %w", dir, err)
	}

	tmpFile := targetPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("gagal menulis berkas sementara Xray %s: %w", tmpFile, err)
	}

	_ = os.Chmod(tmpFile, 0666)

	if err := os.Rename(tmpFile, targetPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("gagal memindahkan konfigurasi Xray ke %s: %w", targetPath, err)
	}

	_ = os.Chmod(targetPath, 0666)
	return nil
}

// SyncToFileCandidates mencoba menulis data konfigurasi ke salah satu kandidat path yang dapat ditulis.
// Ini memastikan sistem bekerja baik di dalam Docker (/etc/xray/config.json)
// maupun saat dijalankan di lingkungan lokal/pengembangan (./deploy/xray/config.json).
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
