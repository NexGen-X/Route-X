package policy

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Ban adalah satu baris bans: satu subjek yang tidak boleh memakai gateway.
//
// Penegakannya TIDAK dijalankan dari sini — lihat catatan konstanta BanSubject* di
// policy.go. Tipe ini dipakai untuk mengelola dan menampilkan blokir.
type Ban struct {
	ID          string
	SubjectKind string
	Subject     string
	Reason      string

	ExpiresAt *time.Time
	LiftedAt  *time.Time

	CreatedAt time.Time
}

// Active melaporkan apakah blokir ini masih berlaku pada waktu tertentu.
func (b *Ban) Active(now time.Time) bool {
	return b.LiftedAt == nil && (b.ExpiresAt == nil || b.ExpiresAt.After(now))
}

const banColumns = `id::text, subject_kind, subject, reason, expires_at, lifted_at, created_at`

// scanBan membaca satu baris bans sesuai banColumns.
func scanBan(s interface{ Scan(...any) error }) (*Ban, error) {
	var b Ban
	if err := s.Scan(&b.ID, &b.SubjectKind, &b.Subject, &b.Reason,
		&b.ExpiresAt, &b.LiftedAt, &b.CreatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}

// CreateBanParams adalah masukan pembuatan blokir.
type CreateBanParams struct {
	SubjectKind string
	Subject     string
	Reason      string
	// ExpiresAt nil berarti permanen sampai dicabut manual.
	ExpiresAt *time.Time
	CreatedBy string
}

// validasiSubjekBan memeriksa bentuk subjek sesuai jenisnya (B-192).
//
// Pemeriksaan ini adalah penjaga satu-satunya antara permukaan admin dan
// query autentikasi API key: penegakan blokir mengevaluasi cast atas
// `b.subject` di dalam subquery bans pada Authenticate, jadi satu baris
// ip/ip_range bersubjek teks tak sah membuat cast-nya melempar error dan
// SELURUH autentikasi gagal (jalur /v1 menjawab 500 untuk key lain yang
// sah). Constraint DB hanya memeriksa jenis dikenal & subjek non-kosong —
// bentuknya tidak. Karena itu bentuk divalidasi di sini, di titik tulis
// bersama semua pemanggil:
//
//   - ip: alamat IP tunggal yang bisa diurai (netip.ParseAddr; notasi
//     CIDR dan zona ditolak — keduanya tak pernah setara dengan inet
//     polos yang dibandingkan Authenticate);
//   - ip_range: prefiks CIDR yang bisa diurai (netip.ParsePrefix) DAN
//     sudah berbentuk jaringan (tanpa bit host, B-269): `10.0.0.1/24`
//     ditolak karena cast `::cidr` PostgreSQL menolaknya dengan 22P02 —
//     bentuk yang sama yang dulu meracuni Authenticate. Operator yang
//     bermaksud memblokir satu host memakai jenis ip; yang bermaksud
//     jaringan menulis alamat jaringannya (`10.0.0.0/24`);
//   - api_key / user: UUID entitas yang diblokir.
func validasiSubjekBan(kind, subject string) error {
	_, err := normalisasiSubjekBan(kind, subject)
	return err
}

// normalisasiSubjekBan menjalankan pemeriksaan validasiSubjekBan
// dan sekaligus mengembalikan subjek KANONIS yang harus disimpan
// (B-342). Perbedaannya pada keluarga 4-in-6: penegakan memeriksa
// alamat klien yang sudah di-Unmap (httpx), sementara subjek ip
// "::ffff:10.0.0.9" tersimpan apa adanya tidak akan pernah setara
// dengannya — blokir tampil aktif, tidak menegakkan apa pun.
// Keputusan B-342: bentuk 4-in-6 DINORMALKAN ke IPv4 polos di
// titik tulis (bukan ditolak): alamatnya menunjuk jaringan yang
// sama, dan nilai tersimpan selalu dalam bentuk yang benar-benar
// dibandingkan penegakan. Khusus ip_range, prefiks 4-in-6 yang
// bitnya < 96 mencakup ruang di luar peta IPv4-dalam-IPv6 sehingga
// tidak punya bentuk IPv4 yang setara — itu tetap ditolak.
func normalisasiSubjekBan(kind, subject string) (string, error) {
	if strings.TrimSpace(subject) == "" {
		return "", fmt.Errorf("subjek blokir kosong: %w", repo.ErrInvalidReference)
	}
	switch kind {
	case BanSubjectIP:
		addr, err := netip.ParseAddr(subject)
		if err != nil || addr.Zone() != "" {
			return "", fmt.Errorf("subjek blokir ip %q bukan alamat IP yang sah: %w", subject, repo.ErrInvalidReference)
		}
		return addr.Unmap().String(), nil
	case BanSubjectIPRange:
		p, err := netip.ParsePrefix(subject)
		if err != nil {
			return "", fmt.Errorf("subjek blokir ip_range %q bukan prefiks CIDR yang sah: %w", subject, repo.ErrInvalidReference)
		}
		if p.Addr().Is4In6() {
			if p.Bits() < 96 {
				return "", fmt.Errorf("subjek blokir ip_range %q mencakup ruang di luar peta IPv4-dalam-IPv6 dan tidak punya bentuk IPv4 yang setara: %w", subject, repo.ErrInvalidReference)
			}
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		if p != p.Masked() {
			return "", fmt.Errorf("subjek blokir ip_range %q mengandung bit host (bentuk jaringannya %s): %w", subject, p.Masked(), repo.ErrInvalidReference)
		}
		return p.String(), nil
	case BanSubjectAPIKey, BanSubjectUser:
		if !idOK(subject) {
			return "", fmt.Errorf("subjek blokir %s %q bukan UUID: %w", kind, subject, repo.ErrInvalidReference)
		}
		return subject, nil
	default:
		return "", fmt.Errorf("jenis subjek blokir %q tidak dikenal: %w", kind, repo.ErrInvalidReference)
	}
}

// CreateBan memblokir satu subjek.
//
// Blokir baru untuk subjek yang MASIH diblokir aktif DITOLAK oleh
// indeks unik parsial bans_active_subject_key (migrasi 0005,
// `where lifted_at is null`): hanya boleh ada satu blokir aktif per
// (subject_kind, subject), dan konfliknya terpetakan sebagai
// "data sudah ada". Riwayat tidak hilang oleh penolakan itu — yang
// ditolak hanya duplikat aktif; blokir yang sudah dicabut tidak lagi
// menempati keunikan, jadi subjek yang sama bisa diblokir lagi
// sesudah blokir lamanya dicabut. (Komentar lama di sini mengklaim
// duplikat "tidak ditolak, disengaja" — keliru, diperbaiki B-357:
// perilaku DB-lah yang selalu menang.)
func (r *Repo) CreateBan(ctx context.Context, p CreateBanParams) (*Ban, error) {
	const op = "memblokir subjek"

	// B-342: subjek yang disimpan adalah bentuk kanonisnya
	// (4-in-6 menjadi IPv4 polos) — lihat normalisasiSubjekBan.
	subjekKanonis, err := normalisasiSubjekBan(p.SubjectKind, p.Subject)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	p.Subject = subjekKanonis

	var createdBy any
	if p.CreatedBy != "" {
		if !idOK(p.CreatedBy) {
			return nil, fmt.Errorf("%s: %w: created_by bukan UUID", op, repo.ErrInvalidReference)
		}
		createdBy = p.CreatedBy
	}

	row := r.q.QueryRow(ctx, `
		insert into bans (subject_kind, subject, reason, expires_at, created_by)
		values ($1, $2, $3, $4, $5)
		returning `+banColumns,
		p.SubjectKind, p.Subject, p.Reason, p.ExpiresAt, createdBy)

	b, err := scanBan(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// Lift mencabut satu blokir.
//
// Dicabut dengan mengisi lifted_at, bukan dengan menghapus barisnya: siapa memblokir, kapan,
// dengan alasan apa, dan siapa yang mencabutnya adalah jejak yang justru paling dibutuhkan
// ketika seorang pelanggan bertanya mengapa aksesnya pernah mati.
//
// Bersyarat pada lifted_at yang masih NULL, sehingga pencabutan kedua mengembalikan
// ErrNotFound alih-alih memindahkan waktu pencabutan yang sudah tercatat.
func (r *Repo) Lift(ctx context.Context, id, liftedBy string) (*Ban, error) {
	const op = "mencabut blokir"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var by any
	if liftedBy != "" {
		if !idOK(liftedBy) {
			return nil, fmt.Errorf("%s: %w: lifted_by bukan UUID", op, repo.ErrInvalidReference)
		}
		by = liftedBy
	}

	row := r.q.QueryRow(ctx, `
		update bans set lifted_at = now(), lifted_by = $2
		where id = $1 and lifted_at is null
		returning `+banColumns, id, by)

	b, err := scanBan(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// ActiveBans mengembalikan seluruh blokir yang masih berlaku, terbaru lebih dulu.
//
// Dipakai dashboard dan pemeriksaan manual, bukan jalur request.
func (r *Repo) ActiveBans(ctx context.Context, page repo.Page) ([]*Ban, error) {
	const op = "mengambil daftar blokir aktif"

	rows, err := r.q.Query(ctx, `
		select `+banColumns+`
		from bans
		where lifted_at is null and (expires_at is null or expires_at > now())
		order by created_at desc, id desc
		limit $1`, page.Normalize())
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Ban
	for rows.Next() {
		b, err := scanBan(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// BanFor mencari blokir yang masih berlaku untuk satu subjek tertentu.
//
// Untuk menjawab "mengapa akses ini ditolak" dari sisi operator. Jalur request tidak
// memakainya: di sana pemeriksaan blokir menempel pada query autentikasi.
func (r *Repo) BanFor(ctx context.Context, subjectKind, subject string) (*Ban, error) {
	const op = "mencari blokir subjek"

	row := r.q.QueryRow(ctx, `
		select `+banColumns+`
		from bans
		where subject_kind = $1 and subject = $2
		  and lifted_at is null and (expires_at is null or expires_at > now())
		order by created_at desc
		limit 1`, subjectKind, subject)

	b, err := scanBan(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return b, nil
}

// PurgeExpired menghapus blokir yang kedaluwarsa lebih lama dari umur yang diberikan.
//
// Yang dihapus hanya yang sudah lewat masanya, bukan yang dicabut manual: pencabutan manual
// adalah keputusan orang dan jejaknya bernilai, sedangkan blokir sementara yang habis
// sendiri adalah kebisingan yang menumpuk.
func (r *Repo) PurgeExpired(ctx context.Context, olderThan time.Duration) (int64, error) {
	const op = "membersihkan blokir kedaluwarsa"

	tag, err := r.q.Exec(ctx, `
		delete from bans
		where lifted_at is null
		  and expires_at is not null
		  and expires_at < now() - make_interval(secs => $1)`, olderThan.Seconds())
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return tag.RowsAffected(), nil
}
