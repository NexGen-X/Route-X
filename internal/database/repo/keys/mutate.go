// Operasi mutasi API key: rotasi, pencabutan, perubahan status, pembaruan sebagian,
// penghapusan permanen, penonaktifan key yang kedaluwarsa, dan penelusuran rantai
// rotasi.
//
// Semuanya jalur admin dan worker. Jalur request inferensi hanya memakai Authenticate
// dan TouchUsage di keys.go, jadi tidak ada di berkas ini yang berjalan sekali per
// permintaan.
package keys

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// ErrNeedsTx dikembalikan operasi yang wajib berjalan di dalam satu transaksi tetapi
// diberi Querier yang bukan transaksi. Lihat Repo.Rotate; pemanggil yang hanya memegang
// pool memakai RotateKey.
var ErrNeedsTx = errors.New("operasi ini wajib dijalankan di dalam transaksi")

// ErrNoChanges dikembalikan Update yang tidak diberi satu pun field untuk diubah.
// Dilaporkan sebagai error, bukan diam-diam sukses, karena hampir selalu berarti
// pemanggil salah merakit permintaannya.
var ErrNoChanges = errors.New("tidak ada perubahan yang diminta")

// RevokeReasonRotated adalah alasan yang dipasang Rotate pada key yang digantikannya.
// Penggantinya selalu bisa ditemukan lewat rotated_from, jadi alasannya tidak perlu
// memuat pengenal apa pun.
const RevokeReasonRotated = "digantikan oleh key hasil rotasi"

// MaxLineageDepth membatasi kedalaman penelusuran Lineage pada setiap arah.
const MaxLineageDepth = 64

// Rotate membuat key pengganti yang mewarisi seluruh setelan key lama, menandainya
// sebagai penerus lewat rotated_from, lalu mencabut key lama.
//
// WAJIB dijalankan di dalam satu transaksi, dan itu ditegakkan, bukan sekadar
// didokumentasikan: kedua akibatnya berbahaya kalau salah satunya berdiri sendiri. Key
// baru tanpa pencabutan key lama berarti dua kredensial hidup untuk satu klien —
// kredensial yang dikira sudah mati justru yang paling lama tidak diperhatikan kalau
// bocor. Pencabutan tanpa key baru berarti klien mati mendadak tanpa ada yang bisa
// dipasang sebagai penggantinya. Pakai RotateKey bila pemanggil hanya memegang pool.
//
// Yang diwarisi: nama, pemilik, cakupan, seluruh batas laju dan token, ip_allowlist,
// expires_at, metadata, serta daftar putih model dan provider. Yang TIDAK diwarisi:
// status — pengganti selalu lahir aktif — riwayat pemakaian, dan seluruh kolom
// pencabutan. Key baru bukan kelanjutan riwayat key lama, hanya salinan setelannya.
// created_by adalah pelaku rotasi, bukan pembuat key lama.
//
// Live atau test-nya pengganti mengikuti key lama: prefix yang berubah saat rotasi akan
// menembus konfigurasi klien yang membedakan lingkungan dari prefix key.
//
// expires_at diwarisi apa adanya, jadi key yang sudah kedaluwarsa menghasilkan pengganti
// yang juga sudah kedaluwarsa. Itu memang bawaan "mewarisi seluruh setelan";
// memperpanjangnya lewat Update adalah langkah terpisah yang harus disengaja.
//
// Key yang sudah dicabut tidak bisa diputar (repo.ErrConflict). Pagarnya ada di dalam
// pernyataan pencabutan, sehingga dua rotasi bersamaan atas key yang sama tidak bisa
// keduanya berhasil dan mengeluarkan dua kredensial pengganti.
func (r *Repo) Rotate(ctx context.Context, id string, createdBy *string) (*Created, error) {
	const op = "memutar API key"

	if _, inTx := r.q.(pgx.Tx); !inTx {
		return nil, fmt.Errorf("%s: %w", op, ErrNeedsTx)
	}
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if err := refID(op, "created_by", createdBy); err != nil {
		return nil, err
	}

	// Pencabutan dikerjakan lebih dulu karena satu pernyataan ini sekaligus mengunci
	// baris key lama sampai transaksi selesai, memastikan key itu masih hidup,
	// mencabutnya, dan memberikan prefix yang menentukan apakah penggantinya key live
	// atau test. Urutannya tidak terlihat dari luar — keduanya commit bersama — dan
	// kalau pembuatan pengganti gagal, pencabutan ini ikut dibatalkan.
	var prefix string
	err := r.q.QueryRow(ctx, `
		update api_keys set
			status = 'revoked', revoked_at = now(), revoked_by = $2, revoke_reason = $3
		where id = $1 and status <> 'revoked'
		returning key_prefix`,
		id, createdBy, RevokeReasonRotated,
	).Scan(&prefix)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.notFoundOrRevoked(ctx, op, id,
				"key sudah dicabut, sedangkan rotasi hanya untuk key yang masih hidup")
		}
		return nil, repo.Err(op, err)
	}

	generated, err := security.GenerateAPIKey(prefix == security.KeyPrefixLive, r.pepper)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Setelan disalin di dalam database lewat insert ... select, bukan dibaca ke Go lalu
	// dikirim kembali: kolom yang kelak ditambahkan ke tabel bisa ikut tersalin dengan
	// satu baris perubahan di sini, dan tidak ada nilai yang berubah bentuk di
	// perjalanan. Kolom yang sengaja TIDAK disebut adalah kolom yang harus kembali ke
	// nilai bawaannya: status, revoked_at, revoked_by, revoke_reason, last_used_at, dan
	// last_used_ip.
	row := r.q.QueryRow(ctx, `
		insert into api_keys (
			name, key_hash, key_prefix, last4, owner_user_id, scopes,
			rate_limit_rps, rate_limit_rpm, rate_limit_tpm,
			daily_request_limit, monthly_request_limit, daily_token_limit, monthly_token_limit,
			ip_allowlist, expires_at, metadata, rotated_from, created_by
		)
		select
			src.name, $2, $3, $4, src.owner_user_id, src.scopes,
			src.rate_limit_rps, src.rate_limit_rpm, src.rate_limit_tpm,
			src.daily_request_limit, src.monthly_request_limit,
			src.daily_token_limit, src.monthly_token_limit,
			src.ip_allowlist, src.expires_at, src.metadata, src.id, $5
		from api_keys src
		where src.id = $1
		returning `+keyColumns,
		id, generated.Hash, generated.Prefix, generated.Last4, createdBy,
	)

	key, err := scanKey(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}

	// Daftar putih disalin dengan cara yang sama dan dengan alasan yang sama.
	if _, err := r.q.Exec(ctx, `
		insert into api_key_allowed_models (api_key_id, model_id)
		select $1, model_id from api_key_allowed_models where api_key_id = $2`,
		key.ID, id); err != nil {
		return nil, repo.Err("menyalin daftar putih model", err)
	}
	if _, err := r.q.Exec(ctx, `
		insert into api_key_allowed_providers (api_key_id, provider_id)
		select $1, provider_id from api_key_allowed_providers where api_key_id = $2`,
		key.ID, id); err != nil {
		return nil, repo.Err("menyalin daftar putih provider", err)
	}

	return &Created{Key: key, Raw: generated.Raw}, nil
}

// RotateKey menjalankan Repo.Rotate di dalam transaksinya sendiri.
//
// Disediakan supaya pemanggil yang hanya memegang pool tidak perlu merakit transaksi
// sendiri — dan supaya tidak ada yang tergoda memanggil Rotate di luar transaksi.
// pepper-nya sama dengan yang diberikan ke New; tanpa itu pengganti tidak bisa dibuat.
func RotateKey(ctx context.Context, pool *pgxpool.Pool, pepper []byte, id string, createdBy *string) (*Created, error) {
	var out *Created
	err := repo.InTx(ctx, pool, func(q repo.Querier) error {
		r, err := New(q, pepper)
		if err != nil {
			return err
		}
		created, err := r.Rotate(ctx, id, createdBy)
		if err != nil {
			return err
		}
		out = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Revoke mencabut key selamanya: status menjadi 'revoked' dan waktu, pelaku, serta
// alasan pencabutan dicatat.
//
// Idempoten. Mencabut key yang sudah dicabut bukan error dan tidak mengubah apa pun
// selain updated_at — pencabutan yang pertama yang menentukan siapa, kapan, dan kenapa.
// Itu penting karena pencabutan datang dari tempat yang bisa mengulang pekerjaannya:
// worker yang jaringannya terputus setelah pernyataan terkirim, admin yang menekan
// tombol dua kali, atau penanganan insiden yang mencabut sederet key sekaligus. Kalau
// pengulangan menimpa jejaknya, alasan sebenarnya ("dilaporkan bocor di repositori
// publik") bisa tertimpa alasan susulan yang tidak menjelaskan apa-apa.
//
// Keempat kolom disetel dalam satu pernyataan supaya tidak ada saat ketika baris
// melanggar api_keys_revoked_consistent, yang memaksa status dan revoked_at sejalan.
//
// reason kosong disimpan sebagai NULL, bukan teks kosong: kolom itu berarti "tidak ada
// alasan yang dicatat", dan dua bentuk untuk satu arti hanya menyulitkan pembacanya.
func (r *Repo) Revoke(ctx context.Context, id string, revokedBy *string, reason string) (*Key, error) {
	const op = "mencabut API key"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if err := refID(op, "revoked_by", revokedBy); err != nil {
		return nil, err
	}

	key, err := scanKey(r.q.QueryRow(ctx, `
		update api_keys set
			status        = 'revoked',
			revoked_at    = case when status = 'revoked' then revoked_at    else now() end,
			revoked_by    = case when status = 'revoked' then revoked_by    else $2   end,
			revoke_reason = case when status = 'revoked' then revoke_reason else $3   end
		where id = $1
		returning `+keyColumns,
		id, revokedBy, nullIfEmpty(reason)))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return key, nil
}

// SetStatus mematikan atau menghidupkan kembali key: 'active' ↔ 'disabled'.
//
// 'revoked' tidak bisa disetel dari sini dan key yang sudah dicabut tidak bisa
// dihidupkan lagi. Keduanya ditolak, bukan diizinkan dengan hati-hati:
//
//   - Menyetel 'revoked' lewat jalur ini akan menghasilkan pencabutan tanpa waktu,
//     pelaku, dan alasan — tepatnya keterangan yang paling dibutuhkan ketika sebuah
//     pencabutan dipertanyakan berbulan-bulan kemudian. Pakai Revoke.
//   - Menghidupkan kembali key yang dicabut berarti membatalkan keputusan keamanan.
//     Key dicabut karena bocor atau karena pemiliknya pergi; kalau pencabutan bisa
//     dibatalkan, "sudah dicabut" tidak lagi berarti "tidak bisa dipakai lagi" dan
//     setiap catatan insiden yang menyebutnya kehilangan artinya. Yang benar adalah
//     membuat key baru.
//
// Pagar terhadap kebangkitan itu ada di dalam pernyataan UPDATE, bukan sebagai
// pemeriksaan terpisah lebih dulu: baca-lalu-tulis bisa disalip pencabutan yang berjalan
// bersamaan, dan hasilnya justru key yang baru saja dicabut hidup kembali.
//
// Key yang kedaluwarsa boleh disetel 'active' dan itu tidak membuatnya bisa dipakai —
// Authenticate menolak kedaluwarsa dari expires_at, bukan dari status. Perpanjang
// expires_at lewat Update.
func (r *Repo) SetStatus(ctx context.Context, id, status string) (*Key, error) {
	const op = "mengubah status API key"

	switch status {
	case StatusActive, StatusDisabled:
	case StatusRevoked:
		return nil, fmt.Errorf("%s: %w: status %q hanya bisa disetel lewat Revoke, yang sekaligus mencatat waktu, pelaku, dan alasan pencabutan",
			op, repo.ErrConstraint, status)
	default:
		return nil, fmt.Errorf("%s: %w: status %q tidak dikenali, hanya %q dan %q yang bisa disetel di sini",
			op, repo.ErrConstraint, status, StatusActive, StatusDisabled)
	}
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	key, err := scanKey(r.q.QueryRow(ctx, `
		update api_keys set status = $2
		where id = $1 and status <> 'revoked'
		returning `+keyColumns, id, status))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, r.notFoundOrRevoked(ctx, op, id,
				"pencabutan bersifat final, key yang sudah dicabut tidak bisa diaktifkan lagi — buat key baru")
		}
		return nil, repo.Err(op, err)
	}
	return key, nil
}

// notFoundOrRevoked memilih error yang tepat ketika UPDATE berpagar "status <> revoked"
// tidak mengenai satu baris pun: barisnya memang tidak ada, atau ada tetapi sudah
// dicabut. Tanpa pembedaan ini, menyentuh key yang sudah mati dan menyentuh key yang
// tidak pernah ada akan tampak sama di antarmuka admin, padahal yang pertama adalah
// keadaan yang bisa dijelaskan kepada operator.
func (r *Repo) notFoundOrRevoked(ctx context.Context, op, id, revokedMessage string) error {
	var status string
	if err := r.q.QueryRow(ctx, `select status from api_keys where id = $1`, id).Scan(&status); err != nil {
		// Termasuk pgx.ErrNoRows, yang di sini berarti barisnya benar-benar tidak ada.
		return repo.Err(op, err)
	}
	if status == StatusRevoked {
		return fmt.Errorf("%s: %w: %s", op, repo.ErrConflict, revokedMessage)
	}
	// Barisnya ada dan tidak dicabut, jadi statusnya berubah di antara dua pernyataan
	// ini. Yang bisa dilaporkan hanya bahwa permintaan ini kalah dari yang bersamaan.
	return fmt.Errorf("%s: %w: status key berubah bersamaan dengan permintaan ini, coba lagi",
		op, repo.ErrConflict)
}

// UpdateParams adalah pembaruan sebagian sebuah API key.
//
// Field bertipe Opt memisahkan tiga permintaan yang berbeda: Opt kosong berarti biarkan
// seperti sekarang, Set(v) berarti ubah menjadi v, dan Clear berarti kosongkan. Untuk
// batas, dikosongkan berarti TANPA batas pada key ini — pemakaiannya lalu hanya dibatasi
// baris rate_limits pada cakupan yang lebih luas — dan itu jelas bukan hal yang sama
// dengan "jangan diubah". Pointer biasa tidak bisa menyatakan keduanya sekaligus.
//
// Field bertipe slice memakai pembedaan yang sama dalam bentuk yang cocok untuk kolom
// yang tidak boleh NULL: nil berarti biarkan, slice kosong yang bukan nil berarti
// kosongkan.
//
// Yang sengaja tidak bisa diubah dari sini: pemilik, prefix, dan nilai key itu sendiri.
// Memindahkan kepemilikan key berarti menyerahkan kredensial yang masih dipegang pemilik
// lama kepada orang lain; yang benar adalah key baru untuk pemilik baru. Nilai key hanya
// berubah lewat Rotate, dan daftar putih model serta provider lewat SetAllowed.
type UpdateParams struct {
	// Name nil berarti biarkan. Nama kosong ditolak constraint api_keys_name_not_empty.
	Name *string
	// Scopes nil berarti biarkan; slice kosong yang bukan nil mencabut seluruh cakupan,
	// termasuk inference — key tetap hidup tetapi tidak boleh melakukan apa pun.
	Scopes []string

	RateLimitRPS        Opt[int]
	RateLimitRPM        Opt[int]
	RateLimitTPM        Opt[int]
	DailyRequestLimit   Opt[int64]
	MonthlyRequestLimit Opt[int64]
	DailyTokenLimit     Opt[int64]
	MonthlyTokenLimit   Opt[int64]

	// IPAllowlist nil berarti biarkan; slice kosong yang bukan nil membuka key dari
	// alamat mana saja.
	IPAllowlist []netip.Prefix
	// ExpiresAt Clear berarti key tidak pernah kedaluwarsa.
	ExpiresAt Opt[time.Time]
}

// Update mengubah field yang diminta saja dan mengembalikan baris hasilnya.
//
// Key yang sudah dicabut tetap boleh diperbarui. Perubahannya tidak berpengaruh apa pun
// karena key itu tidak bisa dipakai lagi, tetapi memperbaiki nama pada baris yang
// terlanjur salah label adalah pekerjaan yang wajar dan tidak ada gunanya dilarang.
func (r *Repo) Update(ctx context.Context, id string, p UpdateParams) (*Key, error) {
	const op = "memperbarui API key"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	// Urutan kolom di klausa SET dijaga tetap: teks SQL yang stabil berarti satu entri di
	// cache prepared statement pgx per bentuk pembaruan, bukan satu per permutasi.
	set := newUpdateSet(id)
	if p.Name != nil {
		set.add("name", *p.Name)
	}
	if p.Scopes != nil {
		set.add("scopes", p.Scopes)
	}
	addOpt(set, "rate_limit_rps", p.RateLimitRPS)
	addOpt(set, "rate_limit_rpm", p.RateLimitRPM)
	addOpt(set, "rate_limit_tpm", p.RateLimitTPM)
	addOpt(set, "daily_request_limit", p.DailyRequestLimit)
	addOpt(set, "monthly_request_limit", p.MonthlyRequestLimit)
	addOpt(set, "daily_token_limit", p.DailyTokenLimit)
	addOpt(set, "monthly_token_limit", p.MonthlyTokenLimit)
	if p.IPAllowlist != nil {
		set.add("ip_allowlist", p.IPAllowlist)
	}
	addOpt(set, "expires_at", p.ExpiresAt)

	if set.empty() {
		return nil, fmt.Errorf("%s: %w", op, ErrNoChanges)
	}

	key, err := scanKey(r.q.QueryRow(ctx,
		`update api_keys set `+set.clause()+` where id = $1 returning `+keyColumns, set.args...))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return key, nil
}

// Delete menghapus baris key beserta daftar putihnya (lewat on delete cascade) secara
// permanen.
//
// Revoke hampir selalu pilihan yang benar. requests menyimpan api_key_id TANPA foreign
// key — disengaja, supaya log tidak ikut terhapus bersama keynya dan pencatatan tidak
// bergantung pada baris key yang masih ada — sehingga setelah Delete, baris log lama
// menunjuk pengenal yang tidak ada lagi. Akibatnya: laporan pemakaian kehilangan nama
// key, tagihan tidak bisa dijelaskan per key, dan penyelidikan insiden kehilangan
// pemiliknya. Key yang dicabut tetap ada, tetap tidak bisa dipakai, dan tetap bisa
// menjelaskan riwayatnya sendiri.
//
// Delete masuk akal untuk key yang salah dibuat dan belum pernah dipakai, untuk data uji,
// dan untuk permintaan penghapusan data pribadi. Selain itu: Revoke.
//
// Satu akibat lain yang perlu diketahui: rotated_from key penerus menjadi NULL (on delete
// set null), jadi rantai rotasi yang dibaca Lineage terputus di titik yang dihapus.
func (r *Repo) Delete(ctx context.Context, id string) error {
	const op = "menghapus API key"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from api_keys where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// ExpireDue mematikan semua key aktif yang sudah melewati expires_at dan mengembalikan
// jumlah baris yang berubah. Dipanggil worker pemeliharaan secara berkala.
//
// Statusnya menjadi 'disabled', bukan 'revoked'. Kedaluwarsa bukan peristiwa keamanan
// dan bukan keputusan siapa pun — hanya batas waktu yang tercapai — sedangkan pencabutan
// bersifat final. Dengan 'disabled', operator yang memperpanjang expires_at bisa
// menghidupkan kembali key yang sama lewat SetStatus dan kliennya tidak perlu memasang
// kredensial baru.
//
// Jalur request tidak bergantung pada worker ini: Authenticate menolak key kedaluwarsa
// dari expires_at menurut jam database, bukan dari status. Worker hanya membuat keadaan
// yang terlihat di dashboard sejalan dengan keadaan yang ditegakkan.
//
// Aman diulang — baris yang sudah dimatikan tidak lagi cocok dengan pagarnya, jadi
// pemanggilan kedua mengembalikan 0 — dan pagarnya dilayani indeks partial
// api_keys_expires_idx, sehingga biayanya sebanding dengan jumlah key yang benar-benar
// jatuh tempo, bukan dengan besar tabel.
func (r *Repo) ExpireDue(ctx context.Context) (int, error) {
	tag, err := r.q.Exec(ctx, `
		update api_keys set status = 'disabled'
		where status = 'active' and expires_at is not null and expires_at <= now()`)
	if err != nil {
		return 0, repo.Err("mematikan API key kedaluwarsa", err)
	}
	return int(tag.RowsAffected()), nil
}

// Lineage mengembalikan seluruh rantai rotasi yang memuat key ini, dari yang paling awal
// sampai yang terbaru.
//
// Ditelusuri dua arah: ke atas lewat rotated_from dan ke bawah lewat kebalikannya.
// Pemanggil biasanya memegang key yang sedang dipakai atau key yang sedang diselidiki,
// bukan pangkal rantainya, jadi penelusuran satu arah akan mengembalikan potongan yang
// berbeda tergantung key mana yang ditanyakan — sedangkan pertanyaannya selalu sama:
// "key ini datang dari mana dan digantikan apa".
//
// Kedalamannya dibatasi MaxLineageDepth pada setiap arah, sehingga hasilnya paling banyak
// 2×MaxLineageDepth+1 baris. Rantai nyata tidak pernah sepanjang itu; batasnya ada untuk
// data yang rusak. rotated_from menunjuk ke tabelnya sendiri dan satu-satunya constraint
// yang menjaganya hanya melarang penunjukan ke diri sendiri, jadi satu UPDATE langsung ke
// database cukup untuk membuat siklus — dan WITH RECURSIVE yang menabrak siklus tanpa
// batas kedalaman tidak akan pernah berhenti. Yang mati bukan hanya query itu: satu
// permintaan admin yang salah cukup untuk menahan satu koneksi pool selamanya.
//
// Urutannya menurut created_at karena penerus selalu dibuat setelah key yang
// digantikannya. Pada data bersiklus tidak ada "paling awal" yang bermakna, tetapi
// urutannya tetap sama setiap kali dan itulah yang bisa dijanjikan.
//
// Key yang belum pernah diputar mengembalikan dirinya sendiri, satu baris.
// repo.ErrNotFound bila keynya tidak ada.
func (r *Repo) Lineage(ctx context.Context, id string) ([]*Key, error) {
	const op = "menelusuri rantai rotasi API key"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	rows, err := r.q.Query(ctx, `
		with recursive
		-- Ke atas: dari key ini ke key yang digantikannya, terus sampai pangkal rantai.
		ancestors as (
			select k.id, k.rotated_from, 0 as depth
			from api_keys k
			where k.id = $1
			union all
			select prev.id, prev.rotated_from, a.depth + 1
			from api_keys prev
			join ancestors a on prev.id = a.rotated_from
			where a.depth < $2
		),
		-- Ke bawah: dari key ini ke key yang menggantikannya, terus sampai yang terbaru.
		descendants as (
			select k.id, 0 as depth
			from api_keys k
			where k.id = $1
			union all
			select next.id, d.depth + 1
			from api_keys next
			join descendants d on next.rotated_from = d.id
			where d.depth < $2
		)
		select `+keyColumns+`
		from api_keys
		where id in (select id from ancestors union select id from descendants)
		order by created_at, id`,
		id, MaxLineageDepth)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Key
	for rows.Next() {
		key, err := scanKey(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, key)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	if len(out) == 0 {
		// Key yang ada selalu ikut dalam hasilnya sendiri, jadi hasil kosong hanya
		// mungkin berarti keynya tidak ada.
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return out, nil
}

// --- pola pembaruan sebagian ----------------------------------------------------------
//
// Opt dan updateSet di bawah ini bentuknya sama persis dengan yang dipakai paket
// upstream. Tempat yang benar untuk keduanya adalah paket repo, supaya seluruh repository
// memakai satu definisi; memindahkannya adalah pekerjaan tersendiri karena menyentuh
// paket lain, dan menyalin polanya jauh lebih baik daripada membuat pola kedua yang
// artinya harus dipelajari ulang oleh pembaca berikutnya.

// Opt menyatakan sebuah field pembaruan sebagian.
//
// Pointer biasa tidak cukup untuk kolom yang boleh NULL: nil harus bisa berarti "biarkan
// seperti sekarang" sekaligus "kosongkan", dan keduanya adalah permintaan yang berbeda.
// Opt memisahkannya — Set menandai bahwa kolom ikut diubah, Value nil berarti diubah
// menjadi NULL.
type Opt[T any] struct {
	Set   bool
	Value *T
}

// Set menandai sebuah field untuk diubah menjadi v.
func Set[T any](v T) Opt[T] { return Opt[T]{Set: true, Value: &v} }

// Clear menandai sebuah field untuk dikosongkan (NULL).
func Clear[T any]() Opt[T] { return Opt[T]{Set: true} }

// arg mengembalikan nilai yang siap dikirim ke pgx: pointer nil menjadi NULL.
func (o Opt[T]) arg() any {
	if o.Value == nil {
		return nil
	}
	return *o.Value
}

// updateSet merakit klausa SET untuk pembaruan sebagian.
//
// Kolom yang tidak diminta berubah sengaja tidak ikut disebut, bukan ditulis ulang dengan
// nilainya sendiri lewat coalesce: dengan begitu trigger set_updated_at dan pembaca
// riwayat database hanya melihat kolom yang benar-benar berubah, dan tidak ada jalur yang
// bisa menimpa perubahan pengguna lain dengan nilai basi.
type updateSet struct {
	assigns []string
	args    []any
}

// newUpdateSet memulai perakitan dengan argumen pertama ($1) berisi kunci baris.
func newUpdateSet(key any) *updateSet {
	return &updateSet{args: []any{key}}
}

// add menambahkan "kolom = $n" beserta nilainya.
func (u *updateSet) add(column string, value any) {
	u.args = append(u.args, value)
	u.assigns = append(u.assigns, fmt.Sprintf("%s = $%d", column, len(u.args)))
}

// addOpt menambahkan kolom hanya bila pemanggil memintanya berubah. Fungsi bebas, bukan
// method, karena method di Go tidak boleh punya parameter tipe sendiri.
func addOpt[T any](u *updateSet, column string, o Opt[T]) {
	if o.Set {
		u.add(column, o.arg())
	}
}

// empty melaporkan apakah tidak ada kolom yang diubah.
func (u *updateSet) empty() bool { return len(u.assigns) == 0 }

// clause mengembalikan isi klausa SET.
func (u *updateSet) clause() string { return strings.Join(u.assigns, ", ") }

// --- helper ---------------------------------------------------------------------------

// idOK memeriksa bentuk UUID sebelum sebuah pengenal dikirim ke database.
//
// Pengenal yang salah bentuk (terpotong saat disalin dari dashboard, atau berasal dari
// URL yang dikarang) gagal di lapisan encoding pgx dan akan muncul sebagai kegagalan
// internal, padahal artinya sama dengan "tidak ada": tidak mungkin ada baris dengan
// pengenal seperti itu. Pemeriksaan ini yang membuat pemanggil mendapat repo.ErrNotFound
// atau repo.ErrInvalidReference, bukan error driver yang berakhir sebagai 500.
func idOK(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i := range len(id) {
		c := id[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

// refID memvalidasi pengenal referensi opsional sebelum dikirim ke database.
func refID(op, field string, id *string) error {
	if id != nil && !idOK(*id) {
		return fmt.Errorf("%s: %w: %s bukan UUID", op, repo.ErrInvalidReference, field)
	}
	return nil
}

// nullIfEmpty mengubah teks kosong menjadi NULL. Kolom teks opsional di skema ini memang
// bermakna "tidak ada", bukan "ada tapi kosong".
func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
