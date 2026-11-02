package config

import (
	"strings"
	"testing"
	"time"
)

// --- B-042 / V-AU-11: SESSION_TTL terbaca dari env ---------------------------

func TestSessionTTLDariEnv(t *testing.T) {
	// Bawaan tetap 12 jam saat tidak disetel.
	cfg, err := loadEnv(t, valid())
	if err != nil {
		t.Fatalf("load bawaan: %v", err)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Errorf("SessionTTL bawaan = %v, mau 12 jam", cfg.SessionTTL)
	}

	env := valid()
	env["SESSION_TTL"] = "30m"
	cfg, err = loadEnv(t, env)
	if err != nil {
		t.Fatalf("load SESSION_TTL=30m: %v", err)
	}
	if cfg.SessionTTL != 30*time.Minute {
		t.Errorf("SessionTTL = %v, mau 30 menit", cfg.SessionTTL)
	}

	// Di luar rentang wajar (5 menit – 7 hari) ditolak saat boot, bukan
	// diam-diam dipakai.
	for _, buruk := range []string{"1m", "200h", "-5m"} {
		env := valid()
		env["SESSION_TTL"] = buruk
		if _, err := loadEnv(t, env); err == nil || !strings.Contains(err.Error(), "SESSION_TTL") {
			t.Errorf("SESSION_TTL=%q: err = %v, mau penolakan yang menyebut SESSION_TTL", buruk, err)
		}
	}
}

// --- B-044 / V-LN-30: ENCRYPTION_KEY_PREVIOUS opsional tetapi ketat ----------

func TestEncryptionKeyPrevious(t *testing.T) {
	// Tidak disetel: nil, tanpa error (masa tenggang mati).
	cfg, err := loadEnv(t, valid())
	if err != nil {
		t.Fatalf("load tanpa previous: %v", err)
	}
	if cfg.EncryptionKeyPrevious != nil {
		t.Errorf("EncryptionKeyPrevious = %d byte, mau nil", len(cfg.EncryptionKeyPrevious))
	}

	// Disetel dengan kunci 32 byte yang sah: terbaca.
	env := valid()
	env["ENCRYPTION_KEY_PREVIOUS"] = key32B
	cfg, err = loadEnv(t, env)
	if err != nil {
		t.Fatalf("load dengan previous sah: %v", err)
	}
	if len(cfg.EncryptionKeyPrevious) != 32 {
		t.Errorf("EncryptionKeyPrevious = %d byte, mau 32", len(cfg.EncryptionKeyPrevious))
	}

	// Disetel tetapi cacat: kesalahan konfigurasi, bukan ditelan diam-diam —
	// masa tenggang yang tidak benar-benar bekerja lebih berbahaya daripada
	// boot yang gagal.
	env = valid()
	env["ENCRYPTION_KEY_PREVIOUS"] = key16B
	if _, err := loadEnv(t, env); err == nil || !strings.Contains(err.Error(), "ENCRYPTION_KEY_PREVIOUS") {
		t.Errorf("previous 16 byte: err = %v, mau penolakan yang menyebut ENCRYPTION_KEY_PREVIOUS", err)
	}
	env = valid()
	env["ENCRYPTION_KEY_PREVIOUS"] = "bukan-base64!!"
	if _, err := loadEnv(t, env); err == nil {
		t.Error("previous bukan base64 harus ditolak")
	}
}
