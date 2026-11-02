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

// muatKunciRotasi membaca pasangan kunci rotasi HANYA dari environment:
// OLD_ENCRYPTION_KEY dan NEW_ENCRYPTION_KEY.
//
// Flag -old-key/-new-key yang dulu ada DIHAPUS (audit3 V-LN-31): nilai yang
// dilewatkan sebagai argumen baris perintah terbaca oleh semua pengguna host
// lewat daftar proses (ps) dan terekam di riwayat shell, padahal ini kunci
// yang membuka seluruh kredensial provider. Environment tidak kebal bocor,
// tetapi tidak disiarkan ke daftar proses.
func muatKunciRotasi() (lama, baru string, err error) {
	lama = os.Getenv("OLD_ENCRYPTION_KEY")
	baru = os.Getenv("NEW_ENCRYPTION_KEY")
	if lama == "" || baru == "" {
		return "", "", fmt.Errorf("variabel lingkungan OLD_ENCRYPTION_KEY dan NEW_ENCRYPTION_KEY wajib disediakan (kunci tidak lagi diterima sebagai argumen perintah)")
	}
	return lama, baru, nil
}

func main() {
	var (
		dbURL  = flag.String("db-url", os.Getenv("DATABASE_URL"), "URL koneksi basis data PostgreSQL")
		dryRun = flag.Bool("dry-run", false, "Simulasi rotasi tanpa menulis perubahan apa pun ke basis data")
	)
	flag.Parse()

	// Perkakas ini TIDAK memiliki subcommand. Argumen posisional apa pun —
	// termasuk perintah hantu "set-admin-password" yang pernah tertulis di
	// pesan log seed dan mustahil bekerja — ditolak keras di sini, bukan
	// diabaikan diam-diam sementara rotasi tetap berjalan (audit3 V-LN-28/31).
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: routex-rotate tidak memiliki subcommand; argumen %q tidak dikenal. Perkakas ini hanya merotasi ENCRYPTION_KEY.\n", flag.Arg(0))
		os.Exit(2)
	}

	oldKey, newKey, err := muatKunciRotasi()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
		OldKey: oldKey,
		NewKey: newKey,
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
			fmt.Printf("  • %-22s: %d baris terdeteksi memakai kunci lama (siap dirotasi), %d baris sudah memakai kunci baru\n", t.TableName, t.Scanned, t.SudahBaru)
		} else {
			fmt.Printf("  • %-22s: %d baris dipindai, %d baris berhasil dirotasi ke kunci baru, %d baris sudah memakai kunci baru\n", t.TableName, t.Scanned, t.Rotated, t.SudahBaru)
		}
	}
	fmt.Println("------------------------------------------------------------------")
	if res.DryRun {
		fmt.Printf("Total baris yang membutuhkan rotasi: %d baris.\n", res.TotalRotated)
		fmt.Println("Simulasi selesai. Seluruh ciphertext valid dan dapat didekripsi dengan kunci lama.")
	} else {
		fmt.Printf("Total baris berhasil dirotasi: %d baris.\n", res.TotalRotated)
		fmt.Println("✅ Sukses: Seluruh data terenkripsi telah diperbarui dengan kunci baru.")
		fmt.Println("Langkah berikutnya (masa tenggang ENCRYPTION_KEY_PREVIOUS):")
		fmt.Println("  1. Setel ENCRYPTION_KEY=<kunci baru> dan ENCRYPTION_KEY_PREVIOUS=<kunci lama>, lalu restart gateway dan verifikasi kredensial provider terbaca.")
		fmt.Println("  2. Setelah terverifikasi, HAPUS ENCRYPTION_KEY_PREVIOUS dan restart sekali lagi — selama env itu terpasang, kunci lama masih dipercaya untuk dekripsi.")
	}
	fmt.Println("==================================================================")
}
