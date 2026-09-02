package upstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// harga merakit Price untuk test perhitungan biaya. Nilai nil berarti harga khusus itu
// tidak diberikan penyedia.
func priceOf(input, output string, cached, reasoning *string) Price {
	p := Price{
		Input:    MustParseUSD(input),
		Output:   MustParseUSD(output),
		Currency: DefaultCurrency,
	}
	if cached != nil {
		p.CachedInput = ptr(MustParseUSD(*cached))
	}
	if reasoning != nil {
		p.Reasoning = ptr(MustParseUSD(*reasoning))
	}
	return p
}

// Perhitungan biaya dengan angka yang bisa diperiksa dengan tangan.
//
// Rumusnya selalu jumlah_token × harga_per_juta ÷ 1.000.000, jadi setiap baris di bawah
// bisa dihitung ulang di kepala: 1.000 token dengan harga 2,50 dolar per juta token
// berarti seperseribu dari 2,50 dolar, yaitu 0,0025 dolar.
func TestPriceCost(t *testing.T) {
	for _, tc := range []struct {
		name  string
		price Price
		usage TokenUsage
		want  CostBreakdown
	}{
		{
			name:  "input dan output biasa",
			price: priceOf("2.50", "10.00", nil, nil),
			usage: TokenUsage{Input: 1000, Output: 500},
			want: CostBreakdown{
				Input:  MustParseUSD("0.0025"), // 1.000 × 2,50 ÷ 1jt
				Output: MustParseUSD("0.005"),  // 500 × 10 ÷ 1jt
				Total:  MustParseUSD("0.0075"),
			},
		},
		{
			name:  "token cache dengan harga sendiri",
			price: priceOf("2.50", "10.00", ptr("0.25"), nil),
			usage: TokenUsage{Input: 10_000, CachedInput: 90_000, Output: 1_000},
			want: CostBreakdown{
				Input:       MustParseUSD("0.025"),  // 10rb × 2,50 ÷ 1jt
				CachedInput: MustParseUSD("0.0225"), // 90rb × 0,25 ÷ 1jt
				Output:      MustParseUSD("0.01"),   // 1rb × 10 ÷ 1jt
				Total:       MustParseUSD("0.0575"),
			},
		},
		{
			name:  "token cache tanpa harga khusus memakai harga input",
			price: priceOf("2.50", "10.00", nil, nil),
			usage: TokenUsage{CachedInput: 1_000},
			want: CostBreakdown{
				CachedInput: MustParseUSD("0.0025"),
				Total:       MustParseUSD("0.0025"),
			},
		},
		{
			name:  "token penalaran dengan harga sendiri",
			price: priceOf("2.50", "10.00", nil, ptr("15.00")),
			usage: TokenUsage{Output: 1_000, Reasoning: 2_000},
			want: CostBreakdown{
				Output:    MustParseUSD("0.01"), // 1rb × 10 ÷ 1jt
				Reasoning: MustParseUSD("0.03"), // 2rb × 15 ÷ 1jt
				Total:     MustParseUSD("0.04"),
			},
		},
		{
			name:  "token penalaran tanpa harga memakai harga output",
			price: priceOf("2.50", "10.00", nil, nil),
			usage: TokenUsage{Reasoning: 2_000},
			want: CostBreakdown{
				Reasoning: MustParseUSD("0.02"), // 2rb × 10 ÷ 1jt
				Total:     MustParseUSD("0.02"),
			},
		},
		{
			name:  "biaya di bawah satu mikro-dolar tetap tersimpan",
			price: priceOf("0.15", "0.60", nil, nil),
			usage: TokenUsage{Input: 1},
			want: CostBreakdown{
				// 1 × 0,15 ÷ 1jt = 0,00000015 dolar; dengan presisi enam desimal angka ini
				// akan menjadi nol.
				Input: MustParseUSD("0.00000015"),
				Total: MustParseUSD("0.00000015"),
			},
		},
		{
			name:  "setengah satuan dibulatkan ke atas",
			price: priceOf("0.005", "0.005", nil, nil),
			usage: TokenUsage{Input: 1, Output: 3},
			want: CostBreakdown{
				// 1 × 0,005 ÷ 1jt = 0,000000005 → setengah satuan, dibulatkan ke atas.
				Input: MustParseUSD("0.00000001"),
				// 3 × 0,005 ÷ 1jt = 0,000000015 → satu setengah satuan, menjadi dua.
				Output: MustParseUSD("0.00000002"),
				// Total adalah jumlah rincian yang sudah dibulatkan, sehingga tabel di
				// dashboard benar-benar berjumlah sama dengan totalnya.
				Total: MustParseUSD("0.00000003"),
			},
		},
		{
			name:  "semua jenis token sekaligus",
			price: priceOf("3.00", "15.00", ptr("0.30"), ptr("60.00")),
			usage: TokenUsage{Input: 2_000, CachedInput: 8_000, Output: 1_500, Reasoning: 500},
			want: CostBreakdown{
				Input:       MustParseUSD("0.006"),  // 2rb × 3 ÷ 1jt
				CachedInput: MustParseUSD("0.0024"), // 8rb × 0,30 ÷ 1jt
				Output:      MustParseUSD("0.0225"), // 1,5rb × 15 ÷ 1jt
				Reasoning:   MustParseUSD("0.03"),   // 500 × 60 ÷ 1jt
				Total:       MustParseUSD("0.0609"),
			},
		},
		{
			name:  "tanpa token tidak ada biaya",
			price: priceOf("2.50", "10.00", nil, nil),
			usage: TokenUsage{},
			want:  CostBreakdown{},
		},
		{
			name:  "model gratis",
			price: priceOf("0", "0", nil, nil),
			usage: TokenUsage{Input: 1_000_000, Output: 1_000_000},
			want:  CostBreakdown{},
		},
		{
			name:  "satu juta token berharga tepat satu kali harganya",
			price: priceOf("2.50", "10.00", nil, nil),
			usage: TokenUsage{Input: 1_000_000, Output: 1_000_000},
			want: CostBreakdown{
				Input:  MustParseUSD("2.50"),
				Output: MustParseUSD("10.00"),
				Total:  MustParseUSD("12.50"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.price.Cost(tc.usage)
			if err != nil {
				t.Fatalf("Cost: %v", err)
			}
			if got != tc.want {
				t.Errorf("Cost() =\n  input=%s cached=%s output=%s reasoning=%s total=%s\ningin\n  input=%s cached=%s output=%s reasoning=%s total=%s",
					got.Input, got.CachedInput, got.Output, got.Reasoning, got.Total,
					tc.want.Input, tc.want.CachedInput, tc.want.Output, tc.want.Reasoning, tc.want.Total)
			}
			// Rincian wajib berjumlah sama dengan total; inilah janji yang dipegang
			// dashboard tagihan.
			if sum := got.Input + got.CachedInput + got.Output + got.Reasoning; sum != got.Total {
				t.Errorf("jumlah rincian = %s, total = %s", sum, got.Total)
			}
		})
	}
}

func TestPriceCostRejectsInvalidInput(t *testing.T) {
	p := priceOf("2.50", "10.00", nil, nil)

	if _, err := p.Cost(TokenUsage{Input: -1}); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("Cost dengan token negatif = %v, ingin ErrInvalidAmount", err)
	}

	// Hasil di luar rentang int64 lebih baik ditolak daripada meluap menjadi angka negatif.
	huge := Price{Input: USD(1) << 62, Output: 0}
	if _, err := huge.Cost(TokenUsage{Input: 1 << 40}); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("Cost yang meluap = %v, ingin ErrInvalidAmount", err)
	}
}

func TestTokenUsageTotal(t *testing.T) {
	u := TokenUsage{Input: 10, CachedInput: 20, Output: 30, Reasoning: 40}
	if u.Total() != 100 {
		t.Errorf("Total() = %d, ingin 100", u.Total())
	}
}

// Memasang harga baru harus menutup harga lama dalam satu transaksi, dan riwayatnya harus
// bisa dibaca kembali per titik waktu.
func TestPricingSetIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	pricing := NewPricingRepo(pool)
	admin := seedUser(ctx, t, pool)

	model := seedModel(ctx, t, pool, "gpt-5", CapText)
	provider := seedProvider(ctx, t, pool, "openai")
	pm := seedAttachment(ctx, t, pool, model.ID, provider.ID, nil)

	// Waktu dipotong ke mikrodetik karena itulah resolusi timestamptz PostgreSQL.
	now := time.Now().UTC().Truncate(time.Microsecond)
	longAgo := now.Add(-48 * time.Hour)
	yesterday := now.Add(-24 * time.Hour)

	oldPrice, err := SetPrice(ctx, pool, SetPriceParams{
		ProviderModelID: pm.ID,
		Input:           MustParseUSD("1.25"),
		Output:          MustParseUSD("5.00"),
		CachedInput:     ptr(MustParseUSD("0.125")),
		EffectiveFrom:   longAgo,
		Source:          "official",
		CreatedBy:       &admin,
	})
	if err != nil {
		t.Fatalf("SetPrice pertama: %v", err)
	}

	switch {
	case oldPrice.Input != MustParseUSD("1.25"):
		t.Errorf("input = %s, ingin 1.25", oldPrice.Input)
	case oldPrice.CachedInput == nil || *oldPrice.CachedInput != MustParseUSD("0.125"):
		t.Errorf("cached_input = %v, ingin 0.125", oldPrice.CachedInput)
	case oldPrice.Reasoning != nil:
		t.Errorf("reasoning = %v, ingin nil", oldPrice.Reasoning)
	case !oldPrice.IsCurrent():
		t.Error("harga pertama seharusnya sedang berlaku")
	case oldPrice.Currency != "USD" || oldPrice.Source != "official":
		t.Errorf("currency/source = %q/%q", oldPrice.Currency, oldPrice.Source)
	case !oldPrice.EffectiveFrom.Equal(longAgo):
		t.Errorf("effective_from = %s, ingin %s", oldPrice.EffectiveFrom, longAgo)
	}

	t.Run("harga dengan enam angka desimal tersimpan tepat", func(t *testing.T) {
		precise, err := SetPrice(ctx, pool, SetPriceParams{
			ProviderModelID: pm.ID,
			Input:           MustParseUSD("12.345678"),
			Output:          MustParseUSD("0.000001"),
			EffectiveFrom:   yesterday.Add(-time.Hour),
		})
		if err != nil {
			t.Fatalf("SetPrice: %v", err)
		}
		if precise.Input != MustParseUSD("12.345678") || precise.Output != MustParseUSD("0.000001") {
			t.Errorf("harga terbaca = %s / %s, ingin 12.345678 / 0.000001", precise.Input, precise.Output)
		}
	})

	newPrice, err := SetPrice(ctx, pool, SetPriceParams{
		ProviderModelID: pm.ID,
		Input:           MustParseUSD("2.50"),
		Output:          MustParseUSD("10.00"),
		Reasoning:       ptr(MustParseUSD("30.00")),
		EffectiveFrom:   yesterday,
	})
	if err != nil {
		t.Fatalf("SetPrice kedua: %v", err)
	}

	t.Run("harga lama tertutup pada saat harga baru mulai berlaku", func(t *testing.T) {
		history, next, err := pricing.History(ctx, pm.ID, repo.Page{})
		if err != nil {
			t.Fatalf("History: %v", err)
		}
		if len(history) != 3 {
			t.Fatalf("jumlah riwayat = %d, ingin 3", len(history))
		}
		if next != "" {
			t.Errorf("kursor = %q, ingin kosong", next)
		}
		if !history[0].IsCurrent() || history[0].ID != newPrice.ID {
			t.Errorf("riwayat teratas bukan harga yang sedang berlaku")
		}

		closed := history[1]
		if closed.EffectiveTo == nil {
			t.Fatal("harga sebelumnya masih terbuka")
		}
		if !closed.EffectiveTo.Equal(newPrice.EffectiveFrom) {
			t.Errorf("effective_to lama = %s, ingin sama dengan effective_from baru (%s) supaya garis waktunya tidak berlubang",
				closed.EffectiveTo, newPrice.EffectiveFrom)
		}
	})

	t.Run("hanya satu harga berlaku", func(t *testing.T) {
		var openRows int
		err := pool.QueryRow(ctx,
			`select count(*) from model_pricing where provider_model_id = $1 and effective_to is null`,
			pm.ID).Scan(&openRows)
		if err != nil {
			t.Fatalf("menghitung harga terbuka: %v", err)
		}
		if openRows != 1 {
			t.Errorf("harga terbuka = %d, ingin 1", openRows)
		}

		current, err := pricing.Current(ctx, pm.ID)
		if err != nil {
			t.Fatalf("Current: %v", err)
		}
		if current.ID != newPrice.ID {
			t.Errorf("Current mengembalikan harga yang salah")
		}
	})

	t.Run("harga per titik waktu", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			saat  time.Time
			want  USD
			noRow bool
		}{
			{"sebelum harga pertama ada", longAgo.Add(-time.Hour), 0, true},
			{"saat harga pertama mulai", longAgo, MustParseUSD("1.25"), false},
			{"sesaat sebelum harga baru", yesterday.Add(-time.Microsecond), MustParseUSD("12.345678"), false},
			{"tepat saat harga baru mulai", yesterday, MustParseUSD("2.50"), false},
			{"sekarang", now, MustParseUSD("2.50"), false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := pricing.At(ctx, pm.ID, tc.saat)
				if tc.noRow {
					if !errors.Is(err, repo.ErrNotFound) {
						t.Fatalf("At = %v, ingin ErrNotFound", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("At: %v", err)
				}
				if got.Input != tc.want {
					t.Errorf("harga input = %s, ingin %s", got.Input, tc.want)
				}
			})
		}
	})

	t.Run("biaya request lampau memakai harga saat itu", func(t *testing.T) {
		// Inilah gunanya At: laporan bulan lalu tidak boleh berubah karena harga naik.
		past, err := pricing.At(ctx, pm.ID, longAgo.Add(time.Hour))
		if err != nil {
			t.Fatalf("At: %v", err)
		}
		cost, err := past.Cost(TokenUsage{Input: 1_000_000, Output: 1_000_000})
		if err != nil {
			t.Fatalf("Cost: %v", err)
		}
		if cost.Total != MustParseUSD("6.25") { // 1,25 + 5,00
			t.Errorf("total biaya = %s, ingin 6.25", cost.Total)
		}
	})

	t.Run("paginasi riwayat", func(t *testing.T) {
		seen := map[string]bool{}
		cursor := ""
		for range 5 {
			page, next, err := pricing.History(ctx, pm.ID, repo.Page{Limit: 2, Cursor: cursor})
			if err != nil {
				t.Fatalf("History: %v", err)
			}
			for _, p := range page {
				if seen[p.ID] {
					t.Fatalf("harga %s muncul dua kali", p.ID)
				}
				seen[p.ID] = true
			}
			if next == "" {
				break
			}
			cursor = next
		}
		if len(seen) != 3 {
			t.Errorf("baris riwayat terbaca = %d, ingin 3", len(seen))
		}
	})

	t.Run("harga mundur ditolak", func(t *testing.T) {
		_, err := SetPrice(ctx, pool, SetPriceParams{
			ProviderModelID: pm.ID,
			Input:           MustParseUSD("1.00"),
			Output:          MustParseUSD("1.00"),
			EffectiveFrom:   yesterday.Add(-time.Hour),
		})
		if !errors.Is(err, repo.ErrConflict) {
			t.Errorf("SetPrice mundur = %v, ingin ErrConflict", err)
		}
	})

	t.Run("harga ikut terhapus bersama pemetaannya", func(t *testing.T) {
		if err := NewModelRepo(pool).DetachProvider(ctx, pm.ID); err != nil {
			t.Fatalf("DetachProvider: %v", err)
		}
		if _, err := pricing.Current(ctx, pm.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Current setelah pemetaan dihapus = %v, ingin ErrNotFound", err)
		}
	})
}

// Penjagaan yang tidak boleh bisa dilewati: dua harga terbuka sekaligus, dan pemasangan
// harga di luar transaksi.
func TestPricingGuardsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	pricing := NewPricingRepo(pool)

	model := seedModel(ctx, t, pool, "gpt-5", CapText)
	provider := seedProvider(ctx, t, pool, "openai")
	pm := seedAttachment(ctx, t, pool, model.ID, provider.ID, nil)

	t.Run("Set di luar transaksi ditolak", func(t *testing.T) {
		_, err := pricing.Set(ctx, SetPriceParams{
			ProviderModelID: pm.ID,
			Input:           MustParseUSD("1.00"),
			Output:          MustParseUSD("2.00"),
		})
		if !errors.Is(err, ErrNeedsTx) {
			t.Errorf("Set di luar transaksi = %v, ingin ErrNeedsTx", err)
		}
	})

	t.Run("harga lebih presisi daripada kolomnya ditolak", func(t *testing.T) {
		err := repo.InTx(ctx, pool, func(q repo.Querier) error {
			_, err := NewPricingRepo(q).Set(ctx, SetPriceParams{
				ProviderModelID: pm.ID,
				Input:           MustParseUSD("0.00000015"),
				Output:          MustParseUSD("2.00"),
			})
			return err
		})
		if !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("Set dengan harga terlalu presisi = %v, ingin ErrConstraint", err)
		}
	})

	t.Run("pemetaan yang tidak ada menjadi ErrInvalidReference", func(t *testing.T) {
		ghostID, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		_, err = SetPrice(ctx, pool, SetPriceParams{
			ProviderModelID: ghostID,
			Input:           MustParseUSD("1.00"),
			Output:          MustParseUSD("2.00"),
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("SetPrice pada pemetaan hantu = %v, ingin ErrInvalidReference", err)
		}
	})

	// Harga pertama dipasang lewat jalur biasa.
	if _, err := SetPrice(ctx, pool, SetPriceParams{
		ProviderModelID: pm.ID,
		Input:           MustParseUSD("1.00"),
		Output:          MustParseUSD("2.00"),
	}); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	t.Run("baris terbuka kedua ditolak database", func(t *testing.T) {
		// Lewat SQL langsung, tanpa menutup baris lama: indeks unique partial
		// model_pricing_one_current_idx yang harus menolaknya, bukan kode Go.
		_, err := pool.Exec(ctx, `
			insert into model_pricing (provider_model_id, input_usd_per_mtok, output_usd_per_mtok)
			values ($1, 9.99, 99.99)`, pm.ID)
		if err == nil {
			t.Fatal("dua harga terbuka sekaligus berhasil disisipkan")
		}
		wrapped := repo.Err("menyisipkan harga kedua", err)
		if !errors.Is(wrapped, repo.ErrConflict) {
			t.Errorf("error = %v, ingin diterjemahkan menjadi ErrConflict", wrapped)
		}
		if !strings.Contains(wrapped.Error(), "model_pricing_one_current_idx") {
			t.Errorf("pesan tidak menyebut indeks penjaganya: %v", wrapped)
		}
	})
}
