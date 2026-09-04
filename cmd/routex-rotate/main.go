package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/security/rotation"
)

func main() {
	var (
		oldKey = flag.String("old-key", os.Getenv("OLD_ENCRYPTION_KEY"), "Kunci enkripsi lama (32-byte base64)")
		newKey = flag.String("new-key", os.Getenv("NEW_ENCRYPTION_KEY"), "Kunci enkripsi baru (32-byte base64)")
		dbURL  = flag.String("db-url", os.Getenv("DATABASE_URL"), "URL koneksi basis data PostgreSQL")
		dryRun = flag.Bool("dry-run", false, "Simulasi rotasi tanpa menulis perubahan apa pun ke basis data")
	)
	flag.Parse()

	if *oldKey == "" || *newKey == "" {
		fmt.Fprintln(os.Stderr, "Error: Flag --old-key dan --new-key wajib disediakan (atau lewat variabel lingkungan).")
		flag.Usage()
		os.Exit(1)
	}

	if *dbURL == "" {
		fmt.Fprintln(os.Stderr, "Error: URL basis data wajib disediakan lewat flag --db-url atau variabel DATABASE_URL.")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Println("==================================================================")
	fmt.Println("         Route-X — Perkakas Rotasi Kunci Enkripsi Data            ")
	fmt.Println("==================================================================")
	if *dryRun {
		fmt.Println("Mode Operasi : SIMULASI (DRY-RUN) — Tidak ada modifikasi pada database")
	} else {
		fmt.Println("Mode Operasi : EKSEKUSI NYATA — Seluruh perubahan akan di-commit")
	}

	// Buat pool koneksi basis data mandiri
	poolCfg, err := pgxpool.ParseConfig(*dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Gagal mengurai URL basis data: %v\n", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = 5
	poolCfg.MinConns = 1

	connectCtx, connectCancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := pgxpool.NewWithConfig(connectCtx, poolCfg)
	connectCancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Gagal menghubungkan ke basis data: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	rotator, err := rotation.NewRotator(rotation.Config{
		DB:     pool,
		OldKey: *oldKey,
		NewKey: *newKey,
		DryRun: *dryRun,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Konfigurasi rotasi tidak valid: %v\n", err)
		os.Exit(1)
	}

	res, err := rotator.Execute(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Gagal menjalankan rotasi kunci enkripsi: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Kunci Lama ID : %s\n", res.OldKeyID)
	fmt.Printf("Kunci Baru ID : %s\n", res.NewKeyID)
	fmt.Println("------------------------------------------------------------------")
	fmt.Println("Hasil Pemrosesan Tabel:")
	for _, t := range res.Tables {
		if res.DryRun {
			fmt.Printf("  • %-22s: %d baris terdeteksi memakai kunci lama (siap dirotasi)\n", t.TableName, t.Scanned)
		} else {
			fmt.Printf("  • %-22s: %d baris dipindai, %d baris berhasil dirotasi ke kunci baru\n", t.TableName, t.Scanned, t.Rotated)
		}
	}
	fmt.Println("------------------------------------------------------------------")
	if res.DryRun {
		fmt.Printf("Total baris yang membutuhkan rotasi: %d baris.\n", res.TotalRotated)
		fmt.Println("Simulasi selesai. Seluruh ciphertext valid dan dapat didekripsi dengan kunci lama.")
	} else {
		fmt.Printf("Total baris berhasil dirotasi: %d baris.\n", res.TotalRotated)
		fmt.Println("✅ Sukses: Seluruh data terenkripsi telah diperbarui dengan kunci baru.")
	}
	fmt.Println("==================================================================")
}
