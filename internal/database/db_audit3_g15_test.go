package database

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
)

// TestApplyPoolSettingsTimeoutServer membuktikan audit3 V-LN-09: pool
// aplikasi memasang statement_timeout & lock_timeout sisi server, dan nilai
// eksplisit dari DSN operator selalu menang.
func TestApplyPoolSettingsTimeoutServer(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://routex:pw@127.0.0.1:5432/routex")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	applyPoolSettings(poolCfg, &config.Config{DBMaxConns: 2, DBMinConns: 1})
	if got := poolCfg.ConnConfig.RuntimeParams["statement_timeout"]; got != "120s" {
		t.Errorf("statement_timeout = %q, mau 120s", got)
	}
	if got := poolCfg.ConnConfig.RuntimeParams["lock_timeout"]; got != "15s" {
		t.Errorf("lock_timeout = %q, mau 15s", got)
	}

	poolCfg2, err := pgxpool.ParseConfig("postgres://routex:pw@127.0.0.1:5432/routex?statement_timeout=30s")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	applyPoolSettings(poolCfg2, &config.Config{DBMaxConns: 2, DBMinConns: 1})
	if got := poolCfg2.ConnConfig.RuntimeParams["statement_timeout"]; got != "30s" {
		t.Errorf("statement_timeout dari DSN harus menang, dapat %q", got)
	}
}

// TestCheckSchemaVersionIntegration membuktikan audit3 V-LN-07: sesudah
// migrasi penuh CheckSchemaVersion lolos; pada skema kosong ia gagal.
func TestCheckSchemaVersionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	baseDSN := testDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	dsn, _ := newTestSchema(ctx, t, baseDSN)
	db, err := Connect(ctx, testConfig(dsn), discardLogger())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	// Skema kosong: ledger migrasi bahkan belum ada → belum siap.
	if err := db.CheckSchemaVersion(ctx); err == nil {
		t.Fatal("CheckSchemaVersion pada skema kosong harus gagal")
	}

	if _, err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.CheckSchemaVersion(ctx); err != nil {
		t.Fatalf("CheckSchemaVersion sesudah migrasi penuh harus lolos: %v", err)
	}
}
