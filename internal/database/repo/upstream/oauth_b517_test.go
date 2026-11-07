package upstream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Keluarga pengunci B-517: penilaian pergeseran pemenang pada jalur
// transaksional OAuth (SimpanHasilRefresh/SimpanHasilExchange)
// dinilai ulang SESUDAH commit, bukan dari dalam transaksi.
//
// Pola probe Putaran Bersih #9 (W1-K1) yang dilatih di sini persis:
// keadaan ditata SERI GANDA — refresh atas B sendirian tidak
// menggeser pemenang dan MarkAuthFailure(A) sendirian tidak
// menggeser, tetapi gabungannya menggeser A→B. Di kode lama,
// MarkAuthFailure yang berjalan di jendela commit (sesudah
// UpdateSecret kembali, sebelum InTx commit) menilai dari keadaan
// ter-commit lama dan tidak melapor, refresh menilai dari dalam
// transaksinya dan juga tidak melapor: pemenang akhir bergeser
// dengan total 0 laporan. Jendela diperlebar deterministik dengan
// trigger pg_sleep pada penulisan provider_oauth_sessions —
// persis pola probe putaran itu, bukan proksi.

// tataSeriB517 menata keadaan seri ganda pada dua kredensial:
// A (auth_failures=0, last_used_at lebih dulu) adalah pemenang;
// B (auth_failures=1, last_used_at lebih kemudian). Kunci urut
// Active strategi non-priority: auth_failures asc, lalu
// last_used_at asc nulls first, lalu created_at asc.
func tataSeriB517(ctx context.Context, t *testing.T, pool *pgxpool.Pool, providerID, credAID, credBID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		update provider_credentials set auth_failures = 0, last_used_at = now() - interval '2 hours'
		where id = $1`, credAID); err != nil {
		t.Fatalf("menata A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		update provider_credentials set auth_failures = 1, last_used_at = now() - interval '1 hour'
		where id = $1`, credBID); err != nil {
		t.Fatalf("menata B: %v", err)
	}
	_ = providerID
}

// pasangJendelaCommitB517 memasang trigger pg_sleep(4) pada
// INSERT/UPDATE provider_oauth_sessions. Di SimpanHasilRefresh
// penulisan sesi (UpdateRefreshStatus) berjalan SESUDAH
// UpdateSecret di dalam transaksi yang sama; di
// SimpanHasilExchange gilirannya UpsertSession. Tidur di sana
// membuka jendela commit selebar 4 detik secara deterministik.
func pasangJendelaCommitB517(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		create or replace function tidur_jendela_b517() returns trigger as $$
		begin
			perform pg_sleep(4);
			return new;
		end;
		$$ language plpgsql`); err != nil {
		t.Fatalf("membuat fungsi jendela: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		create trigger trg_jendela_b517
		before insert or update on provider_oauth_sessions
		for each row execute function tidur_jendela_b517()`); err != nil {
		t.Fatalf("memasang trigger jendela: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, `drop trigger if exists trg_jendela_b517 on provider_oauth_sessions`)
		_, _ = pool.Exec(cleanupCtx, `drop function if exists tidur_jendela_b517()`)
	})
}

// penampungLaporan menghitung laporan pergeseran secara aman
// untuk goroutine.
type penampungLaporan struct {
	mu     sync.Mutex
	daftar []string
}

func (p *penampungLaporan) callback(_ context.Context, providerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.daftar = append(p.daftar, providerID)
}

func (p *penampungLaporan) jumlah() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.daftar)
}

// TestB517RefreshJendelaCommitTetapMelapor adalah pengunci utama
// B-517 untuk kaki refresh: ia memanggil SimpanHasilRefresh
// SUNGGUHAN (pool → InTx → WithQuerier → UpdateSecret) sementara
// MarkAuthFailure(A) berjalan di dalam jendela commitnya.
// Sesudah commit, pemenang WAJIB B dan total laporan WAJIB tepat
// 1 — di kode lama totalnya 0 (bocor). Kontrol sekuensial atas
// keadaan yang sama juga WAJIB tepat 1 (persis kontrol probe
// putaran).
func TestB517RefreshJendelaCommitTetapMelapor(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xC5)
	oauthRepo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	penata, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	laporan := &penampungLaporan{}
	oauthRepo.SetSesudahPemenangBergeser(laporan.callback)

	provider := seedProvider(ctx, t, pool, "provider-b517-refresh")
	admin := seedUser(ctx, t, pool)
	kedaluwarsa := time.Now().Add(30 * 24 * time.Hour)

	credA, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "manual-b517",
		Secret:    security.Secret("akses-b517-a"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	credB, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "oauth:b517@contoh.test",
		Secret:    security.Secret("akses-b517-b"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	sesi, err := oauthRepo.UpsertSession(ctx, UpsertOAuthSessionParams{
		ProviderID:   provider.ID,
		CredentialID: credB.ID,
		AccountEmail: "b517@contoh.test",
		ClientID:     "client-b517",
		RefreshToken: security.Secret("refresh-b517"),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	pemenang := func() string {
		t.Helper()
		aktif, err := penata.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		return aktif.ID
	}

	// --- Fase bersamaan: MarkAuthFailure di jendela commit. ---
	tataSeriB517(ctx, t, pool, provider.ID, credA.ID, credB.ID)
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang awal = %s, mau A", got)
	}
	pasangJendelaCommitB517(ctx, t, pool)

	selesaiRefresh := make(chan error, 1)
	go func() {
		selesaiRefresh <- oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
			SessionID:    sesi.ID,
			CredentialID: credB.ID,
			AccessToken:  security.Secret("akses-b517-b2"),
			ExpiresAt:    kedaluwarsa,
		})
	}()
	// UpdateRefreshStatus di dalam transaksi tidur 4 detik lewat
	// trigger; pada +1.5 detik transaksi pasti sudah melewati
	// UpdateSecret (kunci per-provider terlepas) dan belum commit.
	time.Sleep(1500 * time.Millisecond)
	if _, err := penata.MarkAuthFailure(ctx, credA.ID); err != nil {
		t.Fatalf("MarkAuthFailure di jendela commit: %v", err)
	}
	if err := <-selesaiRefresh; err != nil {
		t.Fatalf("SimpanHasilRefresh: %v", err)
	}
	if got := pemenang(); got != credB.ID {
		t.Fatalf("pemenang akhir fase bersamaan = %s, mau B", got)
	}
	if n := laporan.jumlah(); n != 1 {
		t.Fatalf("laporan fase bersamaan = %d, mau tepat 1 (0 = bocor pola B-517)", n)
	}

	// --- Kontrol sekuensial: urutan yang sama tanpa tumpang-tindih. ---
	tataSeriB517(ctx, t, pool, provider.ID, credA.ID, credB.ID)
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang awal kontrol = %s, mau A", got)
	}
	// Trigger masih terpasang: refresh sekuensial hanya lebih
	// lambat (tidur 4 detik), semantiknya tidak berubah.
	if err := oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
		SessionID:    sesi.ID,
		CredentialID: credB.ID,
		AccessToken:  security.Secret("akses-b517-b3"),
		ExpiresAt:    kedaluwarsa,
	}); err != nil {
		t.Fatalf("SimpanHasilRefresh kontrol: %v", err)
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("refresh sendirian menggeser pemenang ke %s, mau tetap A (seri)", got)
	}
	// MarkAuthFailure kontrol berjalan lewat repo PABRIK bercallback
	// (seperti jalur request produksi): pergeseran sekuensial ini
	// memang kewajibannya sendiri untuk melaporkan — berbeda dari
	// fase bersamaan, di mana MarkAuthFailure sengaja lewat penata
	// tanpa callback agar satu-satunya sumber laporan adalah
	// penilaian sesudah-commit milik refresh.
	pelapor, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo pelapor: %v", err)
	}
	pelapor.SetSesudahPemenangBergeser(laporan.callback)
	if _, err := pelapor.MarkAuthFailure(ctx, credA.ID); err != nil {
		t.Fatalf("MarkAuthFailure kontrol: %v", err)
	}
	if got := pemenang(); got != credB.ID {
		t.Fatalf("pemenang akhir kontrol = %s, mau B", got)
	}
	if n := laporan.jumlah(); n != 2 {
		t.Fatalf("laporan total sesudah kontrol = %d, mau 2 (kontrol menyumbang tepat 1)", n)
	}
}

// TestB517ExchangeJendelaCommitTetapMelapor melatih kaki exchange
// dengan pola yang sama: SimpanHasilExchange SUNGGUHAN atas
// kredensial berlabel sama (cabang UpdateSecret), MarkAuthFailure
// berjalan di jendela commit (tidur pada UpsertSession), dan
// pergeseran gabungan WAJIB tetap tepat 1 laporan.
func TestB517ExchangeJendelaCommitTetapMelapor(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xC6)
	oauthRepo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	penata, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	laporan := &penampungLaporan{}
	oauthRepo.SetSesudahPemenangBergeser(laporan.callback)

	provider := seedProvider(ctx, t, pool, "provider-b517-exchange")
	admin := seedUser(ctx, t, pool)
	kedaluwarsa := time.Now().Add(30 * 24 * time.Hour)

	credA, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "manual-b517x",
		Secret:    security.Secret("akses-b517x-a"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	credB, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "oauth:b517x@contoh.test",
		Secret:    security.Secret("akses-b517x-b"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	tataSeriB517(ctx, t, pool, provider.ID, credA.ID, credB.ID)
	aktif, err := penata.Active(ctx, provider.ID)
	if err != nil || aktif.ID != credA.ID {
		t.Fatalf("pemenang awal bukan A (aktif=%v, err=%v)", aktif, err)
	}
	pasangJendelaCommitB517(ctx, t, pool)

	type hasil struct {
		credID string
		err    error
	}
	selesaiExchange := make(chan hasil, 1)
	go func() {
		credID, _, err := oauthRepo.SimpanHasilExchange(ctx, SimpanHasilExchangeParams{
			ProviderID:   provider.ID,
			Label:        credB.Label,
			AccountEmail: "b517x@contoh.test",
			AccessToken:  security.Secret("akses-b517x-b2"),
			RefreshToken: security.Secret("refresh-b517x"),
			ExpiresAt:    kedaluwarsa,
		})
		selesaiExchange <- hasil{credID: credID, err: err}
	}()
	time.Sleep(1500 * time.Millisecond)
	if _, err := penata.MarkAuthFailure(ctx, credA.ID); err != nil {
		t.Fatalf("MarkAuthFailure di jendela commit: %v", err)
	}
	h := <-selesaiExchange
	if h.err != nil {
		t.Fatalf("SimpanHasilExchange: %v", h.err)
	}
	if h.credID != credB.ID {
		t.Fatalf("exchange memperbarui kredensial %s, mau B (%s)", h.credID, credB.ID)
	}
	aktif, err = penata.Active(ctx, provider.ID)
	if err != nil || aktif.ID != credB.ID {
		t.Fatalf("pemenang akhir bukan B (aktif=%v, err=%v)", aktif, err)
	}
	if n := laporan.jumlah(); n != 1 {
		t.Fatalf("laporan kaki exchange = %d, mau tepat 1 (0 = bocor pola B-517)", n)
	}
}

// TestB517RollbackTanpaLaporanMubazir mengunci faset kedua B-517:
// callback tidak lagi berjalan sebelum commit, jadi transaksi
// yang ROLLBACK tidak menaikkan generasi secara mubazir. Keadaan
// ditata agar refresh di DALAM transaksi memang menggeser
// pemenang (pandangan dalam-tx bergeser) — di kode lama callback
// terlanjur terpanggil dari keadaan itu walau transaksi batal.
func TestB517RollbackTanpaLaporanMubazir(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xC7)
	oauthRepo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	penata, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	laporan := &penampungLaporan{}
	oauthRepo.SetSesudahPemenangBergeser(laporan.callback)

	provider := seedProvider(ctx, t, pool, "provider-b517-rollback")
	admin := seedUser(ctx, t, pool)
	kedaluwarsa := time.Now().Add(30 * 24 * time.Hour)

	credA, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "manual-b517r",
		Secret:    security.Secret("akses-b517r-a"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	credB, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "oauth:b517r@contoh.test",
		Secret:    security.Secret("akses-b517r-b"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	// Keadaan TIDAK seri: A pemenang lewat auth_failures (0 vs 1)
	// tetapi last_used_at B lebih dulu — refresh B (reset
	// auth_failures ke 0) di dalam transaksi menggeser ke B.
	if _, err := pool.Exec(ctx, `
		update provider_credentials set auth_failures = 0, last_used_at = now() - interval '1 hour'
		where id = $1`, credA.ID); err != nil {
		t.Fatalf("menata A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		update provider_credentials set auth_failures = 1, last_used_at = now() - interval '2 hours'
		where id = $1`, credB.ID); err != nil {
		t.Fatalf("menata B: %v", err)
	}

	// SessionID sah-format tetapi tidak ada: UpdateRefreshToken
	// gagal SESUDAH UpdateSecret berhasil di dalam transaksi →
	// seluruh SimpanHasilRefresh rollback.
	sesiHantu, err := newID()
	if err != nil {
		t.Fatalf("newID: %v", err)
	}
	rotasi := security.Secret("refresh-b517r-baru")
	err = oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
		SessionID:        sesiHantu,
		CredentialID:     credB.ID,
		AccessToken:      security.Secret("akses-b517r-b2"),
		ExpiresAt:        kedaluwarsa,
		RefreshTokenBaru: &rotasi,
	})
	if err == nil {
		t.Fatal("SimpanHasilRefresh atas sesi hantu harus gagal")
	}
	if n := laporan.jumlah(); n != 0 {
		t.Fatalf("laporan sesudah rollback = %d, mau 0 (callback pra-commit = kenaikan mubazir)", n)
	}
	aktif, err := penata.Active(ctx, provider.ID)
	if err != nil || aktif.ID != credA.ID {
		t.Fatalf("pemenang sesudah rollback bukan A (aktif=%v, err=%v)", aktif, err)
	}
	metaB, err := penata.Get(ctx, credB.ID)
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if metaB.AuthFailures != 1 {
		t.Fatalf("auth_failures B sesudah rollback = %d, mau 1 (UpdateSecret ikut rollback)", metaB.AuthFailures)
	}
}
