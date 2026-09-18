package upstream

import (
	"context"
	"fmt"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// CredentialMeta adalah kredensial provider sebagaimana boleh dilihat manusia.
//
// Tipe ini sengaja tidak punya field untuk plaintext maupun ciphertext. Dashboard dan
// respons API hanya pernah menerima bentuk ini, sehingga tidak ada jalur yang bisa
// membocorkan blob kredensial karena seseorang menambahkan satu field ke handler.
type CredentialMeta struct {
	ID         string
	ProviderID string
	Label      string
	// MaskedHint bukan rahasia, mis. "sk-****9a21".
	MaskedHint string
	// EncryptionKeyID menunjukkan kunci mana yang mengenkripsi baris ini; dipakai
	// memantau kemajuan rotasi kunci.
	EncryptionKeyID string

	Enabled      bool
	Priority     int
	LastUsedAt   *time.Time
	ExpiresAt    *time.Time
	AuthFailures int

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy *string
}

// ActiveCredential adalah kredensial yang sudah didekripsi, siap dipakai memanggil
// upstream.
//
// Secret bertipe security.Secret supaya tidak bisa tercetak ke log karena kelalaian;
// nilai aslinya hanya keluar lewat Reveal(), tepat di titik pemakaian.
type ActiveCredential struct {
	ID        string
	Label     string
	Secret    security.Secret
	ExpiresAt *time.Time
}

// CredentialRepo adalah akses data kredensial provider.
//
// Cipher-nya wajib ada: tanpa itu paket ini tidak punya cara menulis kredensial, dan
// menyimpannya sebagai teks biasa bukan pilihan yang boleh ada.
type CredentialRepo struct {
	q      repo.Querier
	cipher *security.Cipher
}

// NewCredentialRepo membuat CredentialRepo. cipher berasal dari config.EncryptionKey.
func NewCredentialRepo(q repo.Querier, cipher *security.Cipher) (*CredentialRepo, error) {
	if cipher == nil {
		return nil, fmt.Errorf("kredensial provider tidak bisa dilayani tanpa cipher: %w",
			security.ErrInvalidKeyLength)
	}
	return &CredentialRepo{q: q, cipher: cipher}, nil
}

// WithQuerier membuat salinan CredentialRepo yang berjalan di atas Querier baru (misal transaksi pgx.Tx).
func (r *CredentialRepo) WithQuerier(q repo.Querier) *CredentialRepo {
	return &CredentialRepo{q: q, cipher: r.cipher}
}

// ActiveKeyID mengembalikan pengenal kunci enkripsi yang sedang aktif. Baris yang
// encryption_key_id-nya berbeda masih memakai kunci lama dan perlu diputar.
func (r *CredentialRepo) ActiveKeyID() string { return r.cipher.KeyID() }

// credentialColumns tidak memuat ciphertext: bentuk baris untuk dashboard dan bentuk
// baris untuk dekripsi memang harus berbeda.
const credentialColumns = `
	id::text, provider_id::text, label, masked_hint, encryption_key_id,
	enabled, priority, last_used_at, expires_at, auth_failures,
	created_at, updated_at, created_by::text`

// scanCredential membaca satu baris sesuai credentialColumns.
func scanCredential(row pgxRow) (*CredentialMeta, error) {
	var c CredentialMeta
	err := row.Scan(
		&c.ID, &c.ProviderID, &c.Label, &c.MaskedHint, &c.EncryptionKeyID,
		&c.Enabled, &c.Priority, &c.LastUsedAt, &c.ExpiresAt, &c.AuthFailures,
		&c.CreatedAt, &c.UpdatedAt, &c.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateCredentialParams adalah masukan penambahan kredensial.
type CreateCredentialParams struct {
	ProviderID string
	// Label membedakan beberapa kredensial pada satu provider; kosong menjadi "primary".
	Label string
	// Secret adalah kredensial apa adanya. Nilainya tidak pernah disimpan, hanya
	// ciphertext-nya.
	Secret security.Secret
	// ExpiresAt opsional, dipakai mengingatkan rotasi sebelum kredensial mati.
	ExpiresAt *time.Time
	// Priority menentukan prioritas saat provider menggunakan strategi priority (makin kecil makin utama).
	Priority  *int
	CreatedBy *string
}

// Create mengenkripsi kredensial lalu menyimpannya.
//
// Pengenal barisnya dibuat di Go sebelum enkripsi, bukan diserahkan ke
// gen_random_uuid(). Alasannya AAD: ciphertext diikat ke "provider_credential:<id>",
// jadi id harus sudah pasti saat enkripsi terjadi. Pilihan lainnya adalah menyisipkan
// baris dengan ciphertext sementara lalu memperbaruinya di transaksi yang sama, tetapi
// itu berarti ada saat ketika tabel kredensial memuat baris yang isinya tidak sah — dan
// satu kegagalan di tengah transaksi cukup untuk meninggalkannya begitu. Membuat id
// lebih dulu membuat penyisipannya cukup satu pernyataan, tanpa keadaan setengah jadi.
func (r *CredentialRepo) Create(ctx context.Context, p CreateCredentialParams) (*CredentialMeta, error) {
	const op = "menambah kredensial provider"

	if !idOK(p.ProviderID) {
		return nil, fmt.Errorf("%s: %w: provider_id bukan UUID", op, repo.ErrInvalidReference)
	}
	if err := refID(op, "created_by", p.CreatedBy); err != nil {
		return nil, err
	}
	if p.Secret.IsZero() {
		return nil, fmt.Errorf("%s: %w: kredensial kosong", op, repo.ErrConstraint)
	}

	id, err := newID()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	ciphertext, err := r.cipher.EncryptSecret(p.Secret, security.CredentialAAD(id))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	row := r.q.QueryRow(ctx, `
		insert into provider_credentials (
			id, provider_id, label, ciphertext, encryption_key_id, masked_hint,
			expires_at, created_by, priority
		) values ($1, $2, $3, $4, $5, $6, $7, $8, coalesce($9, 100))
		returning `+credentialColumns,
		id, p.ProviderID, strOr(p.Label, DefaultCredentialLabel), ciphertext,
		r.cipher.KeyID(), maskCredential(p.Secret), p.ExpiresAt, p.CreatedBy, p.Priority,
	)

	meta, err := scanCredential(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return meta, nil
}

// Get mengambil metadata satu kredensial.
func (r *CredentialRepo) Get(ctx context.Context, id string) (*CredentialMeta, error) {
	const op = "mengambil kredensial provider"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	meta, err := scanCredential(r.q.QueryRow(ctx,
		`select `+credentialColumns+` from provider_credentials where id = $1`, id))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return meta, nil
}

// List mengembalikan metadata seluruh kredensial satu provider, terbaru dulu.
//
// Tanpa paginasi dan tanpa plaintext: satu provider hanya punya beberapa kredensial
// (unique (provider_id, label) membatasi jumlahnya secara alami), jadi kursor di sini
// hanya akan menambah bentuk API yang tidak dipakai siapa pun.
func (r *CredentialRepo) List(ctx context.Context, providerID string) ([]*CredentialMeta, error) {
	const op = "mendaftar kredensial provider"
	if !idOK(providerID) {
		return nil, fmt.Errorf("%s: %w: provider_id bukan UUID", op, repo.ErrInvalidReference)
	}

	rows, err := r.q.Query(ctx, `select `+credentialColumns+`
		from provider_credentials where provider_id = $1
		order by created_at desc, id desc`, providerID)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*CredentialMeta
	for rows.Next() {
		meta, err := scanCredential(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// Active mengembalikan kredensial aktif provider dalam bentuk sudah terdekripsi.
//
// Pemilihannya: yang paling sedikit gagal autentikasi lebih dulu, lalu yang paling lama
// tidak dipakai. Dengan begitu provider yang punya beberapa kredensial memakainya
// bergiliran — berguna saat batas laju upstream dihitung per kredensial — dan kredensial
// yang sedang ditolak upstream tidak dicoba terus-menerus selama masih ada yang lain.
//
// Kredensial yang mati atau sudah kedaluwarsa tidak pernah dipilih. Bila baris masih
// dienkripsi kunci lama, errornya membungkus security.ErrKeyMismatch supaya operator
// tahu ini soal rotasi yang belum selesai, bukan data yang rusak.
//
// Pemilihan mendukung strategi universal yang diatur di tingkat provider:
// - "round_robin": beban diputar merata bergantian (Least Recently Used).
// - "priority": kredensial prioritas utama selalu dipilih selama sehat; cadangan hanya saat limit/error.
func (r *CredentialRepo) Active(ctx context.Context, providerID string) (*ActiveCredential, error) {
	const op = "mengambil kredensial aktif provider"
	if !idOK(providerID) {
		return nil, fmt.Errorf("%s: %w: provider_id bukan UUID", op, repo.ErrInvalidReference)
	}

	var (
		id, label, ciphertext string
		expiresAt             *time.Time
	)
	err := r.q.QueryRow(ctx, `
		select c.id::text, c.label, c.ciphertext, c.expires_at
		from provider_credentials c
		join providers p on p.id = c.provider_id
		where c.provider_id = $1
		  and c.enabled
		  and (c.expires_at is null or c.expires_at > now())
		order by
		  c.auth_failures asc,
		  case
		    when p.credential_strategy = 'priority' then c.priority
		    else null
		  end asc,
		  case
		    when p.credential_strategy = 'priority' then c.created_at
		    else null
		  end asc,
		  c.last_used_at asc nulls first,
		  c.created_at asc
		limit 1`, providerID).Scan(&id, &label, &ciphertext, &expiresAt)
	if err != nil {
		return nil, repo.Err(op, err)
	}

	secret, err := r.cipher.DecryptSecret(ciphertext, security.CredentialAAD(id))
	if err != nil {
		return nil, fmt.Errorf("%s (kredensial %s): %w", op, id, err)
	}
	return &ActiveCredential{ID: id, Label: label, Secret: secret, ExpiresAt: expiresAt}, nil
}

// UpdateSecret memperbarui ciphertext kredensial (misalnya setelah auto-refresh token OAuth) dan mereset kegagalan autentikasi.
func (r *CredentialRepo) UpdateSecret(ctx context.Context, id string, secret security.Secret, expiresAt *time.Time) error {
	const op = "memperbarui rahasia kredensial provider"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if secret.IsZero() {
		return fmt.Errorf("%s: %w: kredensial kosong", op, repo.ErrConstraint)
	}

	ciphertext, err := r.cipher.EncryptSecret(secret, security.CredentialAAD(id))
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	tag, err := r.q.Exec(ctx, `
		update provider_credentials
		set ciphertext = $2,
		    encryption_key_id = $3,
		    masked_hint = $4,
		    expires_at = $5,
		    auth_failures = 0,
		    updated_at = now()
		where id = $1`,
		id, ciphertext, r.cipher.KeyID(), maskCredential(secret), expiresAt,
	)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// SetPriority mengubah urutan prioritas kredensial saat provider memakai strategi priority.
func (r *CredentialRepo) SetPriority(ctx context.Context, id string, priority int) error {
	const op = "mengatur prioritas kredensial"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	tag, err := r.q.Exec(ctx, `update provider_credentials set priority = $2, updated_at = now() where id = $1`, id, priority)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}


// Reveal mendekripsi satu kredensial tertentu.
//
// Dipakai untuk uji koneksi dari dashboard dan untuk memindahkan kredensial ke penyedia
// lain. Jalur request memakai Active, yang sekaligus memilih kredensial mana yang layak
// dipakai. Hasilnya tidak boleh dikembalikan ke klien API dalam bentuk apa pun.
func (r *CredentialRepo) Reveal(ctx context.Context, id string) (security.Secret, error) {
	const op = "membuka kredensial provider"
	if !idOK(id) {
		return "", fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var ciphertext string
	if err := r.q.QueryRow(ctx,
		`select ciphertext from provider_credentials where id = $1`, id).Scan(&ciphertext); err != nil {
		return "", repo.Err(op, err)
	}

	secret, err := r.cipher.DecryptSecret(ciphertext, security.CredentialAAD(id))
	if err != nil {
		return "", fmt.Errorf("%s (kredensial %s): %w", op, id, err)
	}
	return secret, nil
}

// SetEnabled menyalakan atau mematikan satu kredensial.
func (r *CredentialRepo) SetEnabled(ctx context.Context, id string, enabled bool) (*CredentialMeta, error) {
	const op = "mengubah status kredensial provider"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	meta, err := scanCredential(r.q.QueryRow(ctx, `
		update provider_credentials set enabled = $2 where id = $1
		returning `+credentialColumns, id, enabled))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return meta, nil
}

// SetEnabledOfProvider mengubah status kredensial yang dipastikan milik provider tertentu.
func (r *CredentialRepo) SetEnabledOfProvider(ctx context.Context, providerID, credID string, enabled bool) (*CredentialMeta, error) {
	const op = "mengubah status kredensial provider bersarang"
	if !idOK(providerID) || !idOK(credID) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	meta, err := scanCredential(r.q.QueryRow(ctx, `
		update provider_credentials set enabled = $3 where id = $1 and provider_id = $2
		returning `+credentialColumns, credID, providerID, enabled))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return meta, nil
}

// Delete menghapus kredensial.
func (r *CredentialRepo) Delete(ctx context.Context, id string) error {
	const op = "menghapus kredensial provider"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from provider_credentials where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// DeleteOfProvider menghapus kredensial yang dipastikan milik provider tertentu.
// Memastikan integritas hierarki sumber daya bersarang /providers/{id}/credentials/{cred_id}.
func (r *CredentialRepo) DeleteOfProvider(ctx context.Context, providerID, credID string) error {
	const op = "menghapus kredensial provider bersarang"
	if !idOK(providerID) || !idOK(credID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from provider_credentials where id = $1 and provider_id = $2`, credID, providerID)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// MarkUsed mencatat bahwa kredensial baru saja dipakai.
//
// auth_failures sengaja tidak direset di sini: penghitungnya bersifat kumulatif seperti
// yang dijanjikan skema ("dinaikkan saat upstream menolak kredensial ini, agar rotasi
// bisa diingatkan"), dan pengingat rotasi justru hilang kalau satu pemakaian berhasil
// menghapus jejaknya.
func (r *CredentialRepo) MarkUsed(ctx context.Context, id string) error {
	const op = "mencatat pemakaian kredensial provider"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx,
		`update provider_credentials set last_used_at = now() where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// MarkAuthFailure menaikkan penghitung penolakan autentikasi dan mengembalikan nilai
// barunya, sehingga pemanggil bisa langsung memutuskan untuk mematikan kredensial atau
// memberi tahu operator tanpa query tambahan.
func (r *CredentialRepo) MarkAuthFailure(ctx context.Context, id string) (int, error) {
	const op = "mencatat kegagalan autentikasi kredensial provider"
	if !idOK(id) {
		return 0, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var failures int
	err := r.q.QueryRow(ctx, `
		update provider_credentials set auth_failures = auth_failures + 1
		where id = $1 returning auth_failures`, id).Scan(&failures)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return failures, nil
}

// CountByKeyID menghitung kredensial per kunci enkripsi.
//
// Dipakai memantau rotasi: selama masih ada kunci selain ActiveKeyID di hasilnya,
// rotasi belum selesai dan kunci lama masih harus tersedia bagi aplikasi.
func (r *CredentialRepo) CountByKeyID(ctx context.Context) (map[string]int, error) {
	const op = "menghitung kredensial per kunci enkripsi"

	rows, err := r.q.Query(ctx,
		`select encryption_key_id, count(*) from provider_credentials group by encryption_key_id`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var (
			keyID string
			n     int
		)
		if err := rows.Scan(&keyID, &n); err != nil {
			return nil, repo.Err(op, err)
		}
		out[keyID] = n
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// Reencrypt memindahkan kredensial dari kunci lama ke kunci aktif dan mengembalikan
// jumlah baris yang berpindah.
//
// Dipanggil berulang sampai mengembalikan nol, sehingga rotasi database besar berjalan
// bertahap dan tidak pernah menahan satu transaksi panjang. Setiap baris diperbarui
// sendiri-sendiri: kalau proses terhenti di tengah jalan, yang sudah berpindah tetap
// terbaca dengan kunci aktif dan sisanya tetap terbaca dengan kunci lama — tidak ada
// keadaan yang membuat kredensial tidak bisa dibuka sama sekali.
//
// Hanya baris yang benar-benar dienkripsi kunci lama yang diambil. Baris yang memakai
// kunci ketiga (dua kali rotasi tanpa menyelesaikan yang pertama) dibiarkan utuh dan
// akan terlihat di CountByKeyID, bukan menggagalkan rotasi yang lain.
//
// Pembaruannya bersyarat pada encryption_key_id yang lama, jadi dua proses rotasi yang
// berjalan bersamaan tidak akan menghitung baris yang sama dua kali.
func (r *CredentialRepo) Reencrypt(ctx context.Context, old *security.Cipher, limit int) (int, error) {
	const op = "memutar kunci kredensial provider"

	if old == nil {
		return 0, fmt.Errorf("%s: kunci lama tidak diberikan", op)
	}
	if old.KeyID() == r.cipher.KeyID() {
		return 0, fmt.Errorf("%s: kunci lama sama dengan kunci aktif (%s), tidak ada yang perlu diputar",
			op, r.cipher.KeyID())
	}
	if limit <= 0 {
		limit = repo.DefaultPageLimit
	}

	type staleRow struct {
		id         string
		ciphertext string
	}

	rows, err := r.q.Query(ctx, `
		select id::text, ciphertext from provider_credentials
		where encryption_key_id = $1
		order by created_at asc, id asc
		limit $2`, old.KeyID(), limit)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	// Barisnya dikumpulkan lebih dulu, baru diperbarui: satu koneksi pgx tidak bisa
	// menjalankan perintah lain selagi hasil query masih terbuka.
	var stale []staleRow
	for rows.Next() {
		var s staleRow
		if err := rows.Scan(&s.id, &s.ciphertext); err != nil {
			rows.Close()
			return 0, repo.Err(op, err)
		}
		stale = append(stale, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, repo.Err(op, err)
	}

	rotated := 0
	for _, s := range stale {
		// AAD-nya tetap sama karena pengenal barisnya tidak berubah; yang berganti hanya
		// kuncinya. Plaintext-nya tidak pernah menyentuh variabel di paket ini.
		next, err := security.Reencrypt(old, r.cipher, s.ciphertext, security.CredentialAAD(s.id))
		if err != nil {
			return rotated, fmt.Errorf("%s (kredensial %s): %w", op, s.id, err)
		}

		tag, err := r.q.Exec(ctx, `
			update provider_credentials set ciphertext = $2, encryption_key_id = $3
			where id = $1 and encryption_key_id = $4`,
			s.id, next, r.cipher.KeyID(), old.KeyID())
		if err != nil {
			return rotated, repo.Err(op, err)
		}
		if tag.RowsAffected() == 1 {
			rotated++
		}
	}
	return rotated, nil
}
