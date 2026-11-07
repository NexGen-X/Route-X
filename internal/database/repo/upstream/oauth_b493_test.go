package upstream

import (
	"context"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB493SimpanHasilRefreshMelaporkanPergeseran adalah pengunci
// B-493 untuk jalur refresh: ia memanggil SimpanHasilRefresh
// SUNGGUHAN ujung-ke-ujung (querier pool → transaksi → WithQuerier →
// konstruksi CredentialRepo di dalam OAuthRepo), BUKAN proksinya.
// Kegagalan asli B-493: test Fase 20 berlabel jalur ini tetapi
// memanggil UpdateSecret langsung pada repo bercallback, sehingga
// konstruksi literal tanpa callback di OAuthRepo lolos.
//
// Keadaan diatur deterministik (kunci urut Active: auth_failures,
// lalu last_used_at NULLS FIRST, lalu created_at): kredensial manual
// A dibuat lebih dulu (pemenang awal); kredensial sesi OAuth dibuat
// kedua. MarkUsed(A) menyerahkan pemenang ke kredensial sesi
// (last_used NULL lebih dulu), MarkAuthFailure atas kredensial sesi
// mengembalikannya ke A. Refresh atas kredensial sesi mereset
// auth_failures-nya ke 0 lewat UpdateSecret di dalam
// SimpanHasilRefresh — seri dipecah last_used_at: kredensial sesi
// (NULL) menyalip A. Pergeseran itu WAJIB dilaporkan lewat callback
// yang tinggal di OAuthRepo; refresh kedua yang tidak menggeser
// pemenang tidak boleh melapor lagi.
//
// Instance CredentialRepo penata keadaan SENGAJA tanpa callback:
// setiap laporan yang terhitung hanya bisa datang dari jalur
// OAuthRepo yang diuji.
func TestB493SimpanHasilRefreshMelaporkanPergeseran(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xB9)
	oauthRepo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	penata, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}

	var laporan []string
	oauthRepo.SetSesudahPemenangBergeser(func(_ context.Context, providerID string) {
		laporan = append(laporan, providerID)
	})

	provider := seedProvider(ctx, t, pool, "provider-b493-refresh")
	admin := seedUser(ctx, t, pool)
	kedaluwarsa := time.Now().Add(30 * 24 * time.Hour)

	credA, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "manual-b493",
		Secret: security.Secret("sk" + "-b493-aa"), ExpiresAt: &kedaluwarsa,
		CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	credOAuth, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "oauth:b493@contoh.test",
		Secret: security.Secret("sk" + "-b493-bb"), ExpiresAt: &kedaluwarsa,
		CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create OAuth: %v", err)
	}
	sesi, err := oauthRepo.UpsertSession(ctx, UpsertOAuthSessionParams{
		ProviderID:   provider.ID,
		CredentialID: credOAuth.ID,
		AccountEmail: "b493@contoh.test",
		ClientID:     "client-b493",
		RefreshToken: security.Secret("refresh-b493"),
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
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang awal = %s, mau A (%s)", got, credA.ID)
	}
	if err := penata.MarkUsed(ctx, credA.ID); err != nil {
		t.Fatalf("MarkUsed A: %v", err)
	}
	if got := pemenang(); got != credOAuth.ID {
		t.Fatalf("pemenang sesudah MarkUsed(A) = %s, mau kredensial sesi", got)
	}
	if _, err := penata.MarkAuthFailure(ctx, credOAuth.ID); err != nil {
		t.Fatalf("MarkAuthFailure sesi: %v", err)
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang sesudah MarkAuthFailure(sesi) = %s, mau A", got)
	}
	if len(laporan) != 0 {
		t.Fatalf("penataan keadaan memicu %d laporan, mau 0", len(laporan))
	}

	// Jalur bernama sungguhan: SimpanHasilRefresh atas kredensial
	// sesi. UpdateSecret di dalamnya mereset auth_failures → pemenang
	// bergeser A → kredensial sesi → WAJIB satu laporan.
	if err := oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
		SessionID:    sesi.ID,
		CredentialID: credOAuth.ID,
		AccessToken:  security.Secret("akses-b493-refresh-1"),
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SimpanHasilRefresh: %v", err)
	}
	if got := pemenang(); got != credOAuth.ID {
		t.Fatalf("pemenang sesudah refresh = %s, mau kredensial sesi (%s)", got, credOAuth.ID)
	}
	if len(laporan) != 1 || laporan[0] != provider.ID {
		t.Fatalf("laporan sesudah refresh penggeser = %v, mau [%s]", laporan, provider.ID)
	}

	// Refresh kedua atas pemenang yang sama: tidak ada pergeseran,
	// tidak ada laporan baru.
	if err := oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
		SessionID:    sesi.ID,
		CredentialID: credOAuth.ID,
		AccessToken:  security.Secret("akses-b493-refresh-2"),
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}); err != nil {
		t.Fatalf("SimpanHasilRefresh #2: %v", err)
	}
	if got := pemenang(); got != credOAuth.ID {
		t.Fatalf("pemenang sesudah refresh #2 = %s, mau kredensial sesi", got)
	}
	if len(laporan) != 1 {
		t.Fatalf("refresh tanpa pergeseran memicu laporan: total %d, mau 1", len(laporan))
	}
}

// TestB493SimpanHasilExchangeMelaporkanPergeseran adalah pengunci
// B-493 untuk jalur exchange: ia memanggil SimpanHasilExchange
// SUNGGUHAN dua kali — pertama membuat kredensial sesi (cabang
// Create), kedua mengenai label yang sama (cabang UpdateSecret di
// dalam exchange). Pergeseran pemenang yang disebabkan cabang kedua
// WAJIB dilaporkan lewat callback OAuthRepo; konstruksi literal di
// SimpanHasilExchange adalah situs kedua B-493 (oauth.go) dan tidak
// boleh lolos hanya karena kaki refresh sudah tertutup.
func TestB493SimpanHasilExchangeMelaporkanPergeseran(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xB9)
	oauthRepo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	penata, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}

	var laporan []string
	oauthRepo.SetSesudahPemenangBergeser(func(_ context.Context, providerID string) {
		laporan = append(laporan, providerID)
	})

	provider := seedProvider(ctx, t, pool, "provider-b493-exchange")
	admin := seedUser(ctx, t, pool)
	kedaluwarsa := time.Now().Add(30 * 24 * time.Hour)

	credA, err := penata.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "manual-b493-x",
		Secret: security.Secret("sk" + "-b493x-aa"), ExpiresAt: &kedaluwarsa,
		CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}

	exchange := func(token string) string {
		t.Helper()
		credID, _, err := oauthRepo.SimpanHasilExchange(ctx, SimpanHasilExchangeParams{
			ProviderID:   provider.ID,
			Label:        "oauth:b493x@contoh.test",
			AccountEmail: "b493x@contoh.test",
			AccessToken:  security.Secret(token),
			RefreshToken: security.Secret("refresh-b493x"),
			ExpiresAt:    time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("SimpanHasilExchange(%s): %v", token, err)
		}
		return credID
	}

	pemenang := func() string {
		t.Helper()
		aktif, err := penata.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		return aktif.ID
	}

	// Exchange pertama: cabang Create — kredensial sesi lahir sebagai
	// kredensial kedua; pemenang tetap A dan tidak ada laporan
	// (Create memang di luar himpunan pelapor B-474).
	credOAuth := exchange("akses-b493x-1")
	if credOAuth == "" || credOAuth == credA.ID {
		t.Fatalf("exchange pertama mengembalikan kredensial %q, mau kredensial sesi baru", credOAuth)
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang sesudah exchange pertama = %s, mau A", got)
	}
	if len(laporan) != 0 {
		t.Fatalf("exchange pertama memicu %d laporan, mau 0", len(laporan))
	}

	// Tata keadaan kalah untuk kredensial sesi (tanpa callback di
	// repo penata): MarkUsed(A) menyerahkan pemenang ke sesi,
	// MarkAuthFailure(sesi) mengembalikannya ke A.
	if err := penata.MarkUsed(ctx, credA.ID); err != nil {
		t.Fatalf("MarkUsed A: %v", err)
	}
	if got := pemenang(); got != credOAuth {
		t.Fatalf("pemenang sesudah MarkUsed(A) = %s, mau kredensial sesi", got)
	}
	if _, err := penata.MarkAuthFailure(ctx, credOAuth); err != nil {
		t.Fatalf("MarkAuthFailure sesi: %v", err)
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang sesudah MarkAuthFailure(sesi) = %s, mau A", got)
	}

	// Exchange kedua, label sama: cabang UpdateSecret di dalam
	// SimpanHasilExchange mereset auth_failures kredensial sesi →
	// pemenang bergeser A → sesi → WAJIB satu laporan.
	if got := exchange("akses-b493x-2"); got != credOAuth {
		t.Fatalf("exchange kedua mengembalikan %s, mau kredensial sesi %s", got, credOAuth)
	}
	if got := pemenang(); got != credOAuth {
		t.Fatalf("pemenang sesudah exchange kedua = %s, mau kredensial sesi", got)
	}
	if len(laporan) != 1 || laporan[0] != provider.ID {
		t.Fatalf("laporan sesudah exchange penggeser = %v, mau [%s]", laporan, provider.ID)
	}
}
