package config

import (
	"strings"
	"testing"
)

// TestWarningsAppEnvHilang membuktikan audit3 V-LN-20: APP_ENV yang HILANG
// TOTAL menghasilkan peringatan keras (dulu diasumsikan development diam-diam).
func TestWarningsAppEnvHilang(t *testing.T) {
	cfg, err := loadEnv(t, valid())
	if err != nil {
		t.Fatalf("konfigurasi valid ditolak: %v", err)
	}
	ada := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "APP_ENV") {
			ada = true
		}
	}
	if !ada {
		t.Fatalf("APP_ENV hilang total harus menghasilkan peringatan, dapat %v", cfg.Warnings)
	}
}

// TestWarningsAppEnvEksplisitTanpaPeringatan: APP_ENV yang disetel eksplisit
// (walau development) tidak memicu peringatan APP_ENV.
func TestWarningsAppEnvEksplisitTanpaPeringatan(t *testing.T) {
	env := valid()
	env["APP_ENV"] = "development"
	cfg, err := loadEnv(t, env)
	if err != nil {
		t.Fatalf("konfigurasi valid ditolak: %v", err)
	}
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "APP_ENV tidak disetel") {
			t.Fatalf("peringatan APP_ENV muncul padahal disetel eksplisit: %q", w)
		}
	}
}

// TestWarningsShutdownGraceNol membuktikan audit3 V-LN-21: SHUTDOWN_GRACE=0
// yang diganti bawaan secara diam-diam kini dinyatakan sebagai peringatan.
func TestWarningsShutdownGraceNol(t *testing.T) {
	env := valid()
	env["APP_ENV"] = "development"
	env["SHUTDOWN_GRACE"] = "0s"
	cfg, err := loadEnv(t, env)
	if err != nil {
		t.Fatalf("konfigurasi valid ditolak: %v", err)
	}
	ada := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "SHUTDOWN_GRACE") {
			ada = true
		}
	}
	if !ada {
		t.Fatalf("SHUTDOWN_GRACE=0s harus menghasilkan peringatan, dapat %v", cfg.Warnings)
	}
}
