package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// key base64 dengan panjang byte tepat untuk masing-masing kebutuhan.
const (
	key32B = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 byte
	key16B = "MDEyMzQ1Njc4OWFiY2RlZg=="                     // 16 byte
	sess48 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// valid mengembalikan environment minimal yang seharusnya lolos validasi.
func valid() map[string]string {
	return map[string]string{
		"DATABASE_URL":   "postgres://routex:pw@127.0.0.1:5432/routex?sslmode=disable",
		"REDIS_URL":      "redis://127.0.0.1:6379/0",
		"SESSION_SECRET": sess48,
		"ENCRYPTION_KEY": key32B,
		"API_KEY_PEPPER": key32B,
	}
}

func lookupFrom(env map[string]string) lookupFunc {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

func loadEnv(t *testing.T, env map[string]string) (*Config, error) {
	t.Helper()
	return loadFrom(lookupFrom(env))
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := loadEnv(t, valid())
	if err != nil {
		t.Fatalf("konfigurasi valid ditolak: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"AppEnv", cfg.AppEnv, EnvDevelopment},
		{"Port", cfg.Port, 8080},
		{"LogLevel", cfg.LogLevel, slog.LevelInfo},
		{"DBMaxConns", cfg.DBMaxConns, int32(20)},
		{"DBMinConns", cfg.DBMinConns, int32(2)},
		{"MaxRequestBytes", cfg.MaxRequestBytes, int64(10 << 20)},
		{"UpstreamTimeout", cfg.UpstreamTimeout, 120 * time.Second},
		{"ShutdownGrace", cfg.ShutdownGrace, 25 * time.Second},
		{"WriteTimeout nol untuk SSE", cfg.WriteTimeout, time.Duration(0)},
		{"RequestLogRetentionDays", cfg.RequestLogRetentionDays, 30},
		{"Addr", cfg.Addr(), ":8080"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, mau %v", c.name, c.got, c.want)
		}
	}
	if len(cfg.EncryptionKey) != encryptionKeyLen {
		t.Errorf("EncryptionKey panjang %d, mau %d", len(cfg.EncryptionKey), encryptionKeyLen)
	}
}

// Semua masalah harus dilaporkan sekaligus, bukan satu per restart.
func TestLoadAggregatesAllErrors(t *testing.T) {
	_, err := loadEnv(t, map[string]string{})
	if err == nil {
		t.Fatal("environment kosong seharusnya ditolak")
	}
	for _, want := range []string{"DATABASE_URL", "REDIS_URL", "SESSION_SECRET", "ENCRYPTION_KEY", "API_KEY_PEPPER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error tidak menyebut %s:\n%v", want, err)
		}
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantMsg string
	}{
		{"ENCRYPTION_KEY kurang dari 32 byte", func(e map[string]string) { e["ENCRYPTION_KEY"] = key16B }, "tepat 32 byte"},
		{"ENCRYPTION_KEY bukan base64", func(e map[string]string) { e["ENCRYPTION_KEY"] = "bukan base64!!" }, "base64"},
		{"SESSION_SECRET terlalu pendek", func(e map[string]string) { e["SESSION_SECRET"] = "pendek" }, "terlalu pendek"},
		{"API_KEY_PEPPER kurang panjang", func(e map[string]string) { e["API_KEY_PEPPER"] = key16B }, "minimal 32 byte"},
		{"PORT di luar rentang", func(e map[string]string) { e["PORT"] = "70000" }, "di luar rentang"},
		{"PORT bukan angka", func(e map[string]string) { e["PORT"] = "delapan" }, "bukan bilangan bulat"},
		{"APP_ENV tidak dikenal", func(e map[string]string) { e["APP_ENV"] = "prod" }, "tidak dikenal"},
		{"LOG_LEVEL tidak dikenal", func(e map[string]string) { e["LOG_LEVEL"] = "verbose" }, "tidak dikenal"},
		{"durasi tidak sah", func(e map[string]string) { e["UPSTREAM_TIMEOUT"] = "120" }, "bukan durasi"},
		{"durasi negatif", func(e map[string]string) { e["UPSTREAM_TIMEOUT"] = "-5s" }, "tidak boleh negatif"},
		{"skema DATABASE_URL salah", func(e map[string]string) { e["DATABASE_URL"] = "mysql://x/y" }, "skema"},
		{"skema REDIS_URL salah", func(e map[string]string) { e["REDIS_URL"] = "http://127.0.0.1" }, "skema"},
		{"min conns melebihi max", func(e map[string]string) { e["DB_MIN_CONNS"] = "50"; e["DB_MAX_CONNS"] = "10" }, "DB_MIN_CONNS"},
		{"retensi body melebihi log", func(e map[string]string) {
			e["REQUEST_LOG_RETENTION_DAYS"] = "7"
			e["REQUEST_BODY_RETENTION_DAYS"] = "30"
		}, "REQUEST_BODY_RETENTION_DAYS"},
		{"email admin tanpa @", func(e map[string]string) { e["INITIAL_ADMIN_EMAIL"] = "admin" }, "email"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := valid()
			tc.mutate(env)
			_, err := loadEnv(t, env)
			if err == nil {
				t.Fatalf("seharusnya ditolak")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("pesan error tidak memuat %q:\n%v", tc.wantMsg, err)
			}
		})
	}
}

// Pesan error tidak boleh membocorkan isi rahasia.
func TestLoadErrorsNeverLeakSecrets(t *testing.T) {
	env := valid()
	env["DATABASE_URL"] = "mysql://user:sangat-rahasia@host/db"
	_, err := loadEnv(t, env)
	if err == nil {
		t.Fatal("seharusnya ditolak")
	}
	if strings.Contains(err.Error(), "sangat-rahasia") {
		t.Fatalf("pesan error membocorkan kredensial: %v", err)
	}
}

func TestProductionHardening(t *testing.T) {
	prod := func() map[string]string {
		e := valid()
		e["APP_ENV"] = "production"
		e["PUBLIC_URL"] = "https://gateway.example.com"
		e["DATABASE_URL"] = "postgres://routex:pw@db.internal:5432/routex?sslmode=require"
		return e
	}

	t.Run("konfigurasi produksi yang benar diterima", func(t *testing.T) {
		cfg, err := loadEnv(t, prod())
		if err != nil {
			t.Fatalf("ditolak: %v", err)
		}
		if !cfg.AppEnv.IsProduction() {
			t.Error("IsProduction() = false")
		}
	})

	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantMsg string
	}{
		{"PUBLIC_URL wajib", func(e map[string]string) { delete(e, "PUBLIC_URL") }, "PUBLIC_URL wajib"},
		{"PUBLIC_URL harus https", func(e map[string]string) { e["PUBLIC_URL"] = "http://gateway.example.com" }, "https"},
		{"sslmode=disable dilarang", func(e map[string]string) {
			e["DATABASE_URL"] = "postgres://routex:pw@db/routex?sslmode=disable"
		}, "sslmode=disable"},
		{"password admin lemah", func(e map[string]string) { e["INITIAL_ADMIN_PASSWORD"] = "pendek123" }, "minimal 16 karakter"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := prod()
			tc.mutate(env)
			if _, err := loadEnv(t, env); err == nil {
				t.Fatal("seharusnya ditolak")
			} else if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("pesan error tidak memuat %q:\n%v", tc.wantMsg, err)
			}
		})
	}
}

// Aturan produksi tidak boleh diterapkan di development.
func TestDevelopmentAllowsInsecureLocalDefaults(t *testing.T) {
	env := valid() // sslmode=disable, tanpa PUBLIC_URL
	if _, err := loadEnv(t, env); err != nil {
		t.Fatalf("development seharusnya mengizinkan default lokal: %v", err)
	}
}

func TestTrustedProxies(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"tidak diset berarti nil", "", nil},
		{"satu cidr", "10.0.0.0/8", []string{"10.0.0.0/8"}},
		{"beberapa dengan spasi", " 10.0.0.0/8 , 192.168.0.0/16 ", []string{"10.0.0.0/8", "192.168.0.0/16"}},
		{"entri kosong dibuang", "10.0.0.0/8,,192.168.0.0/16,", []string{"10.0.0.0/8", "192.168.0.0/16"}},
		{"hanya koma", ",,,", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := valid()
			if tc.raw != "" {
				env["TRUSTED_PROXIES"] = tc.raw
			}
			cfg, err := loadEnv(t, env)
			if err != nil {
				t.Fatalf("ditolak: %v", err)
			}
			if len(cfg.TrustedProxies) != len(tc.want) {
				t.Fatalf("TrustedProxies = %v, mau %v", cfg.TrustedProxies, tc.want)
			}
			for i := range tc.want {
				if cfg.TrustedProxies[i] != tc.want[i] {
					t.Errorf("TrustedProxies[%d] = %q, mau %q", i, cfg.TrustedProxies[i], tc.want[i])
				}
			}
		})
	}
}

// Default aman: tanpa konfigurasi eksplisit, header X-Forwarded-For tidak dipercaya.
func TestTrustedProxiesDefaultsToEmpty(t *testing.T) {
	cfg, err := loadEnv(t, valid())
	if err != nil {
		t.Fatalf("ditolak: %v", err)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies default = %v, mau kosong", cfg.TrustedProxies)
	}
}
