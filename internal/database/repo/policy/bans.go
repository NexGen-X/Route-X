package policy

import (
	"context"
	"fmt"
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

// CreateBan memblokir satu subjek.
//
// Blokir baru untuk subjek yang sudah diblokir TIDAK ditolak, dan itu disengaja: tabelnya
// tidak punya keunikan pada (subject_kind, subject), sehingga riwayat blokir satu subjek
// tetap utuh alih-alih ditimpa. Penegakannya memeriksa keberadaan blokir yang masih
// berlaku, jadi baris berlapis tidak mengubah hasilnya.
func (r *Repo) CreateBan(ctx context.Context, p CreateBanParams) (*Ban, error) {
	const op = "memblokir subjek"

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
