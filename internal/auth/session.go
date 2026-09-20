package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Nilai bawaan layanan sesi.
const (
	// DefaultSessionTTL mengikuti masa berlaku sesi bawaan lapisan data, supaya cookie
	// dan baris sessions tidak pernah berbeda pendapat soal kapan sesi berakhir.
	DefaultSessionTTL = identity.DefaultSessionTTL

	// DefaultTouchInterval adalah selang minimum antara dua pembaruan last_seen_at
	// untuk satu sesi. Lihat touchIfStale untuk alasan angkanya.
	DefaultTouchInterval = 5 * time.Minute

	// maxEmailLen membatasi panjang email yang diproses jalur login. Nilainya batas
	// praktis alamat email menurut RFC 5321 (64 lokal + @ + 255 domain). Batas ini bukan
	// validasi format, hanya penjaga: email berasal dari body request, ikut ke cuplikan
	// pelaku di audit_logs, dan kolom itu bertipe text tanpa batas panjang.
	maxEmailLen = 320
)

// Aksi audit milik paket ini.
//
// Dua nilai ini melengkapi daftar aksi di identity/audit.go, yang belum memuat
// penggantian password. Ditulis sebagai konstanta — bukan string literal di tempat
// pemakaian — karena alasan yang sama dengan daftar di sana: nilai inilah yang dipakai
// menyaring audit log, dan satu salah ketik membuat aksinya tidak pernah muncul pada
// penyaringan apa pun.
const ()

// Service adalah layanan sesi dan login admin.
//
// Satu instance dibuat saat start lalu dibagikan: seluruh field-nya hanya dibaca setelah
// konstruksi, dan repository di bawahnya aman dipakai bersamaan.
type Service struct {
	pool   *pgxpool.Pool
	cfg    *config.Config
	logger *slog.Logger

	users    *identity.Users
	sessions *identity.Sessions
	audit    *identity.Audit

	cookies *Cookies
	csrf    *CSRF

	sessionTTL    time.Duration
	touchInterval time.Duration
	lockThreshold int
	lockDuration  time.Duration

	// limiter boleh nil; method-nya aman dipanggil pada penerima nil dan berarti tanpa
	// pembatasan laju.
	limiter *LoginLimiter
}

// String menyamarkan seluruh isi Service.
//
// cfg adalah field TAK DIEKSPOR yang memuat DATABASE_URL, REDIS_URL, dan SESSION_SECRET,
// dan fmt tidak boleh memanggil metode pada nilai yang diperoleh dari field tak diekspor.
// Tanpa metode di tingkat struct, "%s" pada Service menempuh jalur verb-salah milik fmt
// yang membongkar isi struct beserta ketiga nilai itu. Dijaga TestRedaksiFieldTakDiekspor
// di internal/security.
func (s Service) String() string { return "auth.Service{[REDACTED]}" }

// GoString menutup jalur "%#v".
func (s Service) GoString() string { return s.String() }

// LogValue menutup jalur slog.
func (s Service) LogValue() slog.Value { return slog.StringValue(s.String()) }

// Option menyetel Service saat konstruksi.
type Option func(*Service)

// WithSessionTTL mengubah masa berlaku sesi. Nilai ≤ 0 diabaikan.
func WithSessionTTL(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.sessionTTL = d
		}
	}
}

// WithTouchInterval mengubah selang minimum pembaruan last_seen_at. Nol mematikan
// pembaruan sepenuhnya, yang berguna bila jejak "terakhir terlihat" tidak dipakai.
func WithTouchInterval(d time.Duration) Option {
	return func(s *Service) {
		if d >= 0 {
			s.touchInterval = d
		}
	}
}

// WithLoginLockout mengubah ambang dan lama penguncian akun. Nilai ≤ 0 memakai bawaan
// lapisan data (identity.DefaultLoginFailureThreshold dan DefaultLoginLockDuration).
func WithLoginLockout(threshold int, lockFor time.Duration) Option {
	return func(s *Service) {
		s.lockThreshold = threshold
		s.lockDuration = lockFor
	}
}

// WithLoginLimiter memasang pembatas laju login. nil berarti tanpa pembatasan di lapisan
// ini — penguncian per akun di database tetap berjalan.
func WithLoginLimiter(l *LoginLimiter) Option {
	return func(s *Service) { s.limiter = l }
}

// NewService membuat layanan sesi di atas pool database.
//
// pool wajib ada. cfg dipakai untuk menentukan atribut cookie dan kunci HMAC token CSRF;
// nil diperlakukan sebagai development tanpa kunci sesi, yang hanya masuk akal di test.
// logger nil jatuh ke slog.Default().
func NewService(pool *pgxpool.Pool, cfg *config.Config, logger *slog.Logger, opts ...Option) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{
		pool:   pool,
		cfg:    cfg,
		logger: logger,

		users:    identity.NewUsers(pool),
		sessions: identity.NewSessions(pool),
		audit:    identity.NewAudit(pool),

		sessionTTL:    DefaultSessionTTL,
		touchInterval: DefaultTouchInterval,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	// Cookie dan CSRF dirakit setelah opsi diterapkan supaya Max-Age cookie mengikuti
	// masa berlaku sesi yang sebenarnya dipakai.
	s.cookies = NewCookies(cfg, s.sessionTTL)
	s.csrf = NewCSRF(cfg, s.cookies, logger)
	return s
}

// Cookies mengembalikan penyusun cookie milik layanan ini, supaya router dan handler
// memakai nama serta atribut yang sama persis.
func (s *Service) Cookies() *Cookies { return s.cookies }

// CSRF mengembalikan penjaga CSRF milik layanan ini.
func (s *Service) CSRF() *CSRF { return s.csrf }

// SessionTTL mengembalikan masa berlaku sesi yang dipakai.
func (s *Service) SessionTTL() time.Duration { return s.sessionTTL }

// TouchInterval mengembalikan selang minimum pembaruan last_seen_at.
func (s *Service) TouchInterval() time.Duration { return s.touchInterval }

// --- Principal ---------------------------------------------------------------

// Principal adalah identitas yang sudah terautentikasi beserta kewenangannya.
//
// Isinya cuplikan pada saat sesi diperiksa, bukan penunjuk hidup ke database: satu
// request memakai satu gambaran kewenangan yang tetap, sehingga perubahan peran di tengah
// request tidak membuat separuh handler memakai izin lama dan separuh lagi izin baru.
// Perubahan itu berlaku pada request berikutnya.
type Principal struct {
	// User adalah baris pengguna. PasswordHash-nya bertipe security.Secret dan
	// bertanda json:"-", jadi struct ini aman diserialisasi ke respons API.
	User identity.User `json:"user"`
	// SessionID adalah sesi yang membawa request ini. Sama dengan Session.ID; disediakan
	// terpisah karena itulah satu-satunya bagian sesi yang dibutuhkan mayoritas
	// pemanggil (pengikat token CSRF, target pencabutan saat logout).
	SessionID string `json:"session_id"`
	// Permissions adalah gabungan izin seluruh peran, terurut dan tanpa duplikat.
	// Selalu non-nil supaya terserialisasi sebagai [] alih-alih null.
	Permissions []string `json:"permissions"`
	// Roles adalah nama peran yang dimiliki, yang paling berkuasa lebih dulu.
	Roles []string `json:"roles"`
	// Session adalah sesi yang dipakai, untuk keperluan menampilkan masa berlaku dan
	// jejak perangkat di dashboard.
	Session identity.Session `json:"session"`
}

// Can melaporkan apakah principal memiliki satu izin.
// Can melaporkan apakah principal memiliki satu izin.
// Dalam arsitektur single-admin, seluruh pengguna terautentikasi adalah administrator
// dengan akses penuh.
func (p *Principal) Can(permission string) bool {
	if p == nil || permission == "" {
		return false
	}
	return true
}

// CanAny melaporkan apakah principal memiliki setidaknya satu dari izin yang diberikan.
func (p *Principal) CanAny(permissions ...string) bool {
	if p == nil || len(permissions) == 0 {
		return false
	}
	return true
}

// MustChangePassword melaporkan apakah pengguna wajib mengganti passwordnya lebih dulu.
func (p *Principal) MustChangePassword() bool { return p != nil && p.User.MustChangePassword }

// TopRole mengembalikan nama peran terkuat yang dimiliki ("Admin").
func (p *Principal) TopRole() string {
	if p == nil {
		return ""
	}
	if len(p.Roles) > 0 {
		return p.Roles[0]
	}
	return "Admin"
}

// Actor menyusun cuplikan pelaku untuk catatan audit.
func (p *Principal) Actor() identity.Actor {
	if p == nil {
		return identity.Actor{}
	}
	return identity.ActorOf(p.User, p.TopRole())
}

// Result adalah hasil login yang berhasil.
type Result struct {
	// Principal adalah identitas yang baru terautentikasi.
	Principal *Principal
	// Token adalah token sesi mentah, dan ini satu-satunya kesempatan membacanya —
	// database hanya menyimpan hash-nya. Kirimkan langsung sebagai cookie lewat
	// Cookies.SetSession dan jangan simpan ke mana pun.
	Token security.Secret
}

// --- Login -------------------------------------------------------------------

// Login memverifikasi kredensial lalu membuat sesi baru.
//
// Seluruh kegagalan mengembalikan *LoginError yang membungkus ErrLoginFailed dengan pesan
// yang persis sama; sebabnya hanya ada di field Reason, untuk log dan audit. Kegagalan
// yang bukan soal kredensial (database mati, hash tidak bisa dibaca) tetap dibedakan:
// yang pertama keluar sebagai error biasa supaya pemanggil menjawab 500, yang kedua
// menjadi LoginError dengan ReasonBrokenHash karena bagi pengguna hasilnya sama saja —
// akun itu tidak bisa dimasuki sampai passwordnya direset.
//
// ip dan userAgent hanya untuk jejak: keduanya berasal dari klien, disimpan apa adanya di
// baris sesi dan audit, dan tidak pernah dipakai sebagai dasar keputusan.
func (s *Service) Login(ctx context.Context, email string, password security.Secret, ip, userAgent string) (*Result, error) {
	const op = "login"

	email = strings.TrimSpace(email)
	// Email dipendekkan lebih dulu supaya nilai raksasa dari body request tidak ikut ke
	// pesan log maupun ke kolom actor_email.
	auditEmail := truncateEmail(email)

	// Batas laju diperiksa paling awal: seluruh langkah di bawahnya melibatkan satu kerja
	// argon2 yang memakan 64 MiB, jadi menolak di sini adalah satu-satunya titik yang
	// benar-benar menghemat sumber daya saat ada yang menyapu daftar password.
	if allowed, retryAfter := s.limiter.Allow(ctx, ip, email); !allowed {
		s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "percobaan login ditolak batas laju",
			slog.String("email", auditEmail), slog.String("ip", ip),
			slog.Duration("retry_after", retryAfter))
		s.writeAudit(ctx, s.failureEvent(ctx, identity.Actor{Email: auditEmail}, "", ip, userAgent,
			ReasonRateLimited, nil))
		return nil, &LoginError{Reason: ReasonRateLimited, RetryAfter: retryAfter}
	}

	// Email kosong atau tidak masuk akal panjangnya tetap membayar waktu argon2. Kalau
	// tidak, permintaan yang salah bentuk akan selesai seketika dan itu sendiri sudah
	// menjadi sinyal yang bisa dibandingkan penyerang.
	if email == "" || len(email) > maxEmailLen {
		security.BurnVerifyTime()
		s.writeAudit(ctx, s.failureEvent(ctx, identity.Actor{Email: auditEmail}, "", ip, userAgent,
			ReasonUnknownEmail, nil))
		return nil, loginFailure(ReasonUnknownEmail)
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			// Kegagalan database bukan kegagalan kredensial. Membalasnya sebagai "email
			// atau password salah" akan membuat pengguna mengira passwordnya bermasalah
			// dan mencoba terus sampai akunnya benar-benar terkunci.
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		// Inti pertahanan enumerasi akun: kerja argon2 tetap dilakukan walaupun tidak ada
		// hash yang perlu diverifikasi, sehingga lamanya jawaban untuk email yang tidak
		// terdaftar sebanding dengan lamanya jawaban untuk password yang salah.
		security.BurnVerifyTime()
		s.writeAudit(ctx, s.failureEvent(ctx, identity.Actor{Email: auditEmail}, "", ip, userAgent,
			ReasonUnknownEmail, nil))
		return nil, loginFailure(ReasonUnknownEmail)
	}

	actor := identity.Actor{UserID: u.ID, Email: u.Email}

	// Status dan penguncian diperiksa sebelum password. Waktu argon2 tetap dibayar di
	// kedua jalur, dengan alasan yang sama seperti di atas: kalau akun yang dinonaktifkan
	// dijawab lebih cepat, statusnya bisa dibaca dari luar.
	switch {
	case u.Status != identity.UserStatusActive:
		security.BurnVerifyTime()
		s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "login ditolak: akun tidak aktif",
			slog.String("user_id", u.ID), slog.String("status", string(u.Status)))
		s.writeAudit(ctx, s.failureEvent(ctx, actor, u.ID, ip, userAgent, ReasonDisabled,
			map[string]any{"status": string(u.Status)}))
		return nil, loginFailure(ReasonDisabled)

	case u.Locked(time.Now()):
		security.BurnVerifyTime()
		s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "login ditolak: akun terkunci sementara",
			slog.String("user_id", u.ID), slog.Time("locked_until", *u.LockedUntil))
		s.writeAudit(ctx, s.failureEvent(ctx, actor, u.ID, ip, userAgent, ReasonLocked,
			map[string]any{"locked_until": u.LockedUntil}))
		return nil, loginFailure(ReasonLocked)
	}

	verify, err := security.VerifyPassword(password.Reveal(), u.PasswordHash.Reveal())
	if err != nil {
		// Hash tersimpan tidak bisa diparse: kerusakan data, bukan serangan. Penghitung
		// kegagalan sengaja tidak dinaikkan — pemilik akun tidak melakukan apa pun yang
		// salah, dan menguncinya hanya menambah satu masalah lagi.
		s.log(ctx).LogAttrs(ctx, slog.LevelError, "hash password tersimpan tidak bisa dibaca",
			slog.String("user_id", u.ID), slog.String("error", err.Error()))
		s.writeAudit(ctx, s.failureEvent(ctx, actor, u.ID, ip, userAgent, ReasonBrokenHash, nil))
		return nil, loginFailure(ReasonBrokenHash)
	}

	if !verify.Match {
		s.recordFailure(ctx, u, actor, ip, userAgent)
		return nil, loginFailure(ReasonBadPassword)
	}

	// Peran dan izin dimuat sebelum transaksi. Keduanya pembacaan murni, dan nama peran
	// dibutuhkan sebagai cuplikan pelaku di catatan audit login.
	roles, permissions, err := s.loadAuthorization(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	actor.Role = firstOr(roles, "")

	// Pencatatan login berhasil, pembuatan sesi, dan catatan auditnya satu transaksi.
	// Kalau dipisah, kegagalan di tengah bisa menghasilkan keadaan yang menyesatkan:
	// penghitung kegagalan sudah dinolkan tetapi sesinya tidak pernah ada, atau sesi
	// hidup tanpa jejak audit siapa pun yang membuatnya.
	var created identity.CreatedSession
	err = repo.InTx(ctx, s.pool, func(q repo.Querier) error {
		if err := identity.NewUsers(q).RecordLoginSuccess(ctx, u.ID, ip); err != nil {
			return err
		}
		var cErr error
		created, cErr = identity.NewSessions(q).Create(ctx, identity.NewSession{
			UserID: u.ID, TTL: s.sessionTTL, IP: ip, UserAgent: userAgent,
		})
		if cErr != nil {
			return cErr
		}
		return identity.NewAudit(q).Write(ctx, identity.Event{
			Actor:        actor,
			Action:       identity.ActionLogin,
			ResourceType: identity.ResourceSession,
			ResourceID:   created.ID,
			IP:           ip,
			UserAgent:    userAgent,
			RequestID:    observability.RequestIDFrom(ctx),
			Metadata: map[string]any{
				"expires_at":           created.ExpiresAt,
				"must_change_password": u.MustChangePassword,
			},
		})
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Rehash transparan dikerjakan SETELAH transaksi, bukan di dalamnya. Ini bukan bagian
	// dari login: hash yang gagal ditulis ulang hanya berarti percobaan berikutnya
	// mencobanya lagi, sedangkan kegagalan di dalam transaksi akan membatalkan sesi yang
	// sudah sah dan membuat pengguna tidak bisa masuk karena alasan yang tidak ada
	// hubungannya dengan kredensialnya.
	if verify.NeedsRehash {
		s.rehashPassword(ctx, u.ID, password)
	}

	// Penghitung batas laju untuk email ini dilepas setelah login berhasil, supaya
	// beberapa salah ketik hari ini tidak menghalangi pemilik akun besok. Penghitung per
	// IP sengaja tidak dilepas: satu login berhasil dari satu alamat NAT tidak boleh
	// menjadi cara membersihkan jejak penyapuan password dari alamat yang sama.
	s.limiter.ResetEmail(ctx, email)

	// Kedua field ini pasti sudah dinolkan RecordLoginSuccess di dalam transaksi di atas,
	// jadi cuplikan yang dibawa Principal disesuaikan alih-alih membaca ulang barisnya.
	// last_login_at sengaja dibiarkan apa adanya: nilainya adalah login SEBELUM ini, dan
	// itu justru yang berguna ditampilkan ("terakhir masuk kemarin 10:12").
	u.FailedLoginAttempts = 0
	u.LockedUntil = nil

	principal := &Principal{
		User:        u,
		SessionID:   created.ID,
		Permissions: permissions,
		Roles:       roles,
		Session:     created.Session,
	}
	s.log(ctx).LogAttrs(ctx, slog.LevelInfo, "login berhasil",
		slog.String("user_id", u.ID), slog.String("session_id", created.ID),
		slog.String("role", actor.Role), slog.Bool("must_change_password", u.MustChangePassword))

	return &Result{Principal: principal, Token: created.Token}, nil
}

// recordFailure mencatat satu percobaan login gagal beserta auditnya.
//
// Kenaikan penghitung dan catatan auditnya satu transaksi supaya keduanya tidak bisa
// terpisah — audit tanpa penghitung membuat penguncian tampak tidak pernah terjadi, dan
// penghitung tanpa audit menghilangkan satu-satunya jejak bahwa ada yang mencoba.
//
// Kegagalan pencatatan TIDAK mengubah hasil login: yang gagal tetap gagal. Ia hanya
// dilaporkan ke log, karena kalau kegagalan ini diteruskan sebagai error internal,
// penyerang bisa membedakan "password salah" dari "password salah dan pencatatan gagal".
func (s *Service) recordFailure(ctx context.Context, u identity.User, actor identity.Actor, ip, userAgent string) {
	var failure identity.LoginFailure
	err := repo.InTx(ctx, s.pool, func(q repo.Querier) error {
		var rErr error
		failure, rErr = identity.NewUsers(q).RecordLoginFailure(ctx, u.ID, s.lockThreshold, s.lockDuration)
		if rErr != nil {
			return rErr
		}
		return identity.NewAudit(q).Write(ctx, s.failureEvent(ctx, actor, u.ID, ip, userAgent,
			ReasonBadPassword, map[string]any{
				"attempts":     failure.Attempts,
				"locked":       failure.Locked,
				"locked_until": failure.LockedUntil,
			}))
	})
	if err != nil {
		s.log(ctx).LogAttrs(ctx, slog.LevelError, "gagal mencatat percobaan login yang gagal",
			slog.String("user_id", u.ID), slog.String("error", err.Error()))
		return
	}

	level := slog.LevelWarn
	if failure.Locked {
		// Penguncian adalah kejadian yang layak memicu perhatian, bukan hanya satu baris
		// di antara peringatan lain.
		level = slog.LevelError
	}
	s.log(ctx).LogAttrs(ctx, level, "login gagal: password salah",
		slog.String("user_id", u.ID), slog.Int("attempts", failure.Attempts),
		slog.Bool("locked", failure.Locked))
}

// rehashPassword menulis ulang hash password dengan parameter biaya terkini.
//
// Dipanggil hanya setelah verifikasi berhasil, jadi password yang di-hash sudah terbukti
// benar. Kegagalannya tidak pernah dilaporkan ke pemanggil: hash lama masih sah dan masih
// bisa diverifikasi, jadi satu-satunya akibat adalah percobaan rehash terulang pada login
// berikutnya.
func (s *Service) rehashPassword(ctx context.Context, userID string, password security.Secret) {
	hash, err := security.HashPassword(password.Reveal())
	if err != nil {
		s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "gagal menghitung ulang hash password",
			slog.String("user_id", userID), slog.String("error", err.Error()))
		return
	}
	if err := s.users.UpdatePasswordHash(ctx, userID, security.Secret(hash)); err != nil {
		s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "gagal menyimpan hash password baru",
			slog.String("user_id", userID), slog.String("error", err.Error()))
		return
	}
	s.log(ctx).LogAttrs(ctx, slog.LevelInfo, "hash password diperbarui ke parameter biaya terkini",
		slog.String("user_id", userID))
}

// --- Authenticate ------------------------------------------------------------

// Authenticate menukar token sesi mentah dengan principal lengkap.
//
// Seluruh sebab kegagalan yang berkaitan dengan sesi keluar sebagai ErrNoSession tanpa
// dibedakan: cookie tidak ada, token tidak dikenal, sesi kedaluwarsa, sesi dicabut,
// pengguna sudah dihapus, atau akunnya dinonaktifkan. Perbedaan itu memberi tahu pemegang
// token curian apakah tebakannya pernah bernilai.
//
// Ini pembacaan murni: last_seen_at tidak disentuh di sini. Lihat touchIfStale.
func (s *Service) Authenticate(ctx context.Context, token security.Secret) (*Principal, error) {
	const op = "memeriksa sesi"

	sess, err := s.sessions.Lookup(ctx, token)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrNoSession
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// Baris sessions memakai ON DELETE CASCADE, jadi keadaan ini praktis tidak
			// mungkin. Ditangani sebagai "tidak ada sesi", bukan sebagai error internal,
			// supaya jalur autentikasi tidak pernah punya cabang yang meloloskan
			// permintaan hanya karena bentuk kegagalannya tidak terduga.
			return nil, ErrNoSession
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if u.Status != identity.UserStatusActive {
		// Sesi yang masih hidup milik akun yang sudah dimatikan langsung dicabut, bukan
		// hanya ditolak. Tanpa pencabutan, penolakannya harus dihitung ulang pada setiap
		// request sampai cookie-nya kedaluwarsa sendiri; dengan pencabutan, Lookup
		// berikutnya sudah menyaringnya di query.
		if err := s.sessions.Revoke(ctx, sess.ID); err != nil && !errors.Is(err, repo.ErrNotFound) {
			s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "gagal mencabut sesi milik akun tidak aktif",
				slog.String("session_id", sess.ID), slog.String("error", err.Error()))
		}
		s.log(ctx).LogAttrs(ctx, slog.LevelInfo, "sesi dicabut karena akun tidak aktif",
			slog.String("user_id", u.ID), slog.String("status", string(u.Status)))
		return nil, ErrNoSession
	}

	// Penguncian sementara akibat percobaan login gagal SENGAJA tidak memutus sesi yang
	// sudah berjalan. Penguncian itu menjawab penebakan password, bukan menyatakan sesi
	// yang sedang dipakai tidak sah — kalau ia ikut memutus, siapa pun yang tahu email
	// seorang admin bisa mengeluarkannya dari dashboard kapan saja hanya dengan menebak
	// passwordnya lima kali. Untuk mengeluarkan seseorang, cabut sesinya atau matikan
	// akunnya; keduanya keputusan sadar, bukan efek samping.

	roles, permissions, err := s.loadAuthorization(ctx, u.ID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Principal{
		User:        u,
		SessionID:   sess.ID,
		Permissions: permissions,
		Roles:       roles,
		Session:     sess,
	}, nil
}

// touchIfStale memperbarui last_seen_at hanya bila jejaknya sudah cukup tua.
//
// Menyegarkannya pada setiap request akan berarti satu UPDATE per request dashboard:
// setiap UPDATE PostgreSQL menulis versi baru barisnya, meninggalkan versi lama untuk
// dibersihkan autovacuum, dan menambah WAL yang ikut direplikasi. Untuk data yang hanya
// dipakai menampilkan "terakhir terlihat" di daftar perangkat, biaya itu tidak sebanding.
//
// Ambangnya DefaultTouchInterval (5 menit). Yang hilang hanyalah ketelitian jejak sampai
// selang itu — cukup teliti untuk daftar perangkat, dan tidak berpengaruh sama sekali pada
// keamanan, karena masa berlaku sesi ditentukan expires_at yang tetap, bukan oleh jejak
// ini. Kegagalannya diabaikan dengan sengaja: last_seen_at bukan alasan menolak request
// yang kredensialnya sudah terbukti sah.
func (s *Service) touchIfStale(ctx context.Context, p *Principal) {
	if p == nil || s.touchInterval <= 0 {
		return
	}
	if time.Since(p.Session.LastSeenAt) < s.touchInterval {
		return
	}
	if err := s.sessions.Touch(ctx, p.SessionID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// Sesi dicabut oleh permintaan lain di sela-selanya. Bukan kesalahan.
			return
		}
		s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "gagal menyegarkan jejak sesi",
			slog.String("session_id", p.SessionID), slog.String("error", err.Error()))
	}
}

// --- Logout ------------------------------------------------------------------

// Logout mencabut satu sesi dan mencatatnya di audit.
//
// Sesi yang memang sudah tidak ada tidak dilaporkan sebagai kegagalan: logout dua kali,
// atau logout atas sesi yang baru dicabut admin, adalah permintaan yang maksudnya sudah
// terpenuhi. Catatan auditnya tetap ditulis, karena yang menarik untuk ditelusuri adalah
// siapa yang meminta keluar dan dari mana, bukan berapa baris yang berubah.
//
// actor dibawa dari pemanggil alih-alih dibaca ulang dari database: pemanggil sudah
// memegang principal dari middleware, dan cuplikan pelaku di audit memang harus
// menggambarkan keadaan saat aksi terjadi.
func (s *Service) Logout(ctx context.Context, sessionID string, actor identity.Actor, ip, userAgent string) error {
	const op = "logout"

	err := repo.InTx(ctx, s.pool, func(q repo.Querier) error {
		if err := identity.NewSessions(q).Revoke(ctx, sessionID); err != nil &&
			!errors.Is(err, repo.ErrNotFound) {
			return err
		}
		return identity.NewAudit(q).Write(ctx, identity.Event{
			Actor:        actor,
			Action:       identity.ActionLogout,
			ResourceType: identity.ResourceSession,
			ResourceID:   sessionID,
			IP:           ip,
			UserAgent:    userAgent,
			RequestID:    observability.RequestIDFrom(ctx),
		})
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	s.log(ctx).LogAttrs(ctx, slog.LevelInfo, "logout",
		slog.String("user_id", actor.UserID), slog.String("session_id", sessionID))
	return nil
}

// --- Ganti password ----------------------------------------------------------

// ChangePassword mengganti password pengguna setelah memverifikasi password lamanya, lalu
// mencabut sesi-sesinya yang lain. Nilai kembalinya adalah jumlah sesi yang dicabut.
//
// keepSessionID adalah sesi yang dibiarkan hidup — biasanya sesi yang sedang dipakai
// pemanggil, sehingga penggantian password tidak mengeluarkannya sendiri dari dashboard.
// String kosong berarti seluruh sesi dicabut, yang tepat untuk reset password oleh admin
// lain.
//
// Pencabutan sesi lain adalah inti dari operasi ini, bukan pelengkap. Alasan orang
// mengganti password justru dugaan bahwa kredensialnya sudah diketahui orang lain; kalau
// sesi yang mungkin sudah dibajak tetap hidup, penggantian password tidak mengubah apa
// pun bagi pembajaknya sampai sesi itu kedaluwarsa sendiri.
//
// Penggantian password, pencabutan sesi, dan catatan auditnya satu transaksi: password
// baru tanpa pencabutan sesi adalah keadaan yang membuat pengguna merasa aman padahal
// tidak.
func (s *Service) ChangePassword(ctx context.Context, userID string, current, next security.Secret,
	keepSessionID, ip, userAgent string) (int, error) {
	const op = "mengganti password"

	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	actor := identity.Actor{UserID: u.ID, Email: u.Email}

	verify, err := security.VerifyPassword(current.Reveal(), u.PasswordHash.Reveal())
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	if !verify.Match {
		// Dicatat di audit: permintaan ganti password dengan password lama yang salah,
		// dari sesi yang sah, adalah pola khas sesi yang dipakai orang lain.
		s.writeAudit(ctx, identity.Event{
			Actor:        actor,
			Action:       identity.ActionPasswordChangeFailed,
			ResourceType: identity.ResourceUser,
			ResourceID:   u.ID,
			IP:           ip,
			UserAgent:    userAgent,
			RequestID:    observability.RequestIDFrom(ctx),
			Metadata:     map[string]any{"reason": string(ReasonBadPassword)},
		})
		s.log(ctx).LogAttrs(ctx, slog.LevelWarn, "ganti password ditolak: password lama salah",
			slog.String("user_id", u.ID), slog.String("session_id", keepSessionID))
		return 0, fmt.Errorf("%s: %w", op, ErrWrongPassword)
	}

	// Password yang tidak berubah ditolak sebelum apa pun disentuh. Meloloskannya akan
	// mencabut seluruh sesi lain tanpa mengubah kredensial apa pun — kerugian tanpa
	// manfaat, dan biasanya memang salah kirim dari form.
	// Perbandingannya constant-time (subtle): == berhenti di byte pertama yang
	// berbeda sehingga waktu respons membocorkan seberapa jauh kecocokannya.
	curr, nxt := current.Reveal(), next.Reveal()
	if len(curr) == len(nxt) && subtle.ConstantTimeCompare([]byte(curr), []byte(nxt)) == 1 {
		return 0, fmt.Errorf("%s: %w", op, ErrPasswordUnchanged)
	}
	// Kekuatan password diperiksa di sini walaupun identity.Users.ChangePassword juga
	// memeriksanya: pemeriksaan di sini yang menghasilkan error terbungkus ErrWeakPassword
	// sehingga lapisan HTTP bisa menjawab 400 dengan aturan yang dilanggar, alih-alih
	// menerjemahkan error lapisan data.
	if err := security.ValidatePasswordStrength(next.Reveal()); err != nil {
		return 0, weakPasswordError(op, err)
	}

	var revoked int
	err = repo.InTx(ctx, s.pool, func(q repo.Querier) error {
		revoked = 0
		// mustChange=false: pemilik akun sendiri yang menggantinya, jadi kewajiban ganti
		// password (mis. dari admin bootstrap) selesai di sini.
		if err := identity.NewUsers(q).ChangePassword(ctx, userID, next, false); err != nil {
			return err
		}

		sessions := identity.NewSessions(q)
		if keepSessionID == "" {
			n, err := sessions.RevokeAllOfUser(ctx, userID)
			if err != nil {
				return err
			}
			revoked = n
		} else {
			// Dicabut satu per satu karena RevokeAllOfUser tidak punya pengecualian, dan
			// menambahkan SQL sendiri di lapisan ini akan memindahkan perakitan query ke
			// luar paket repository. Jumlah sesi aktif seorang admin adalah jumlah
			// perangkatnya — hitungan tangan, bukan skala yang menuntut satu pernyataan.
			active, err := sessions.ListActive(ctx, userID)
			if err != nil {
				return err
			}
			for _, sess := range active {
				if sess.ID == keepSessionID {
					continue
				}
				if err := sessions.Revoke(ctx, sess.ID); err != nil {
					return err
				}
				revoked++
			}
		}

		return identity.NewAudit(q).Write(ctx, identity.Event{
			Actor:        actor,
			Action:       identity.ActionPasswordChange,
			ResourceType: identity.ResourceUser,
			ResourceID:   u.ID,
			IP:           ip,
			UserAgent:    userAgent,
			RequestID:    observability.RequestIDFrom(ctx),
			Metadata: map[string]any{
				"revoked_sessions": revoked,
				"kept_session_id":  keepSessionID,
			},
		})
	})
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	s.log(ctx).LogAttrs(ctx, slog.LevelInfo, "password diganti",
		slog.String("user_id", u.ID), slog.Int("revoked_sessions", revoked))
	return revoked, nil
}

// loadAuthorization memuat nama peran dan izin efektif seorang pengguna.
// Dalam arsitektur single-admin, seluruh pengguna terautentikasi adalah Admin dengan izin penuh.
func (s *Service) loadAuthorization(_ context.Context, _ string) (roles, permissions []string, err error) {
	return []string{"Admin"}, []string{"*"}, nil
}

// SetupHintResponse adalah informasi onboarding awal untuk halaman login.
//
// Hanya berisi petunjuk, bukan kredensial. Alamat email admin pertama boleh diberitahu
// karena ia tidak menghasilkan akses apa pun dan justru memudahkan onboarding, tetapi
// password default TIDAK PERNAH dikirim ke klien.
//
// Sebelumnya field default_password membuat rute publik tanpa autentikasi ini
// menyerahkan kredensial admin penuh dalam plaintext kepada siapa pun yang memintanya.
// Dalam arsitektur single-admin setiap pengguna terautentikasi adalah Admin dengan izin
// penuh (*), jadi satu permintaan GET cukup untuk menguasai seluruh instance sebelum
// password diganti. Password hanya boleh didapat dari sumber tepercaya: output instalasi,
// berkas .env, atau perintah routex-rotate.
type SetupHintResponse struct {
	HasDefaultAdmin bool   `json:"has_default_admin"`
	DefaultEmail    string `json:"default_email,omitempty"`
}

// SetupHint memeriksa apakah akun admin default masih dalam keadaan belum diganti passwordnya.
func (s *Service) SetupHint(ctx context.Context) (SetupHintResponse, error) {
	defaultEmail := "admin@routex.local"
	if s.cfg != nil && s.cfg.InitialAdminEmail != "" {
		defaultEmail = s.cfg.InitialAdminEmail
	}

	u, err := s.users.GetByEmail(ctx, defaultEmail)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return SetupHintResponse{HasDefaultAdmin: false}, nil
		}
		return SetupHintResponse{}, err
	}

	if !u.MustChangePassword {
		return SetupHintResponse{HasDefaultAdmin: false}, nil
	}

	// Password default sengaja TIDAK disertakan di respons. Membacanya dari
	// s.cfg.InitialAdminPassword.Reveal() seperti sebelumnya hanya akan mengirim
	// kredensial admin penuh ke pemanggil mana pun, termasuk yang tidak terautentikasi.
	// Halaman login hanya diberi tahu bahwa admin pertama masih wajib ganti password,
	// lalu mengarahkan operator mengambil kredensial dari sumber tepercaya.
	return SetupHintResponse{
		HasDefaultAdmin: true,
		DefaultEmail:    defaultEmail,
	}, nil
}

// failureEvent menyusun catatan audit untuk satu percobaan login yang gagal.
//
// Alasan sebenarnya masuk ke metadata, bukan ke pesan yang dilihat klien. Di sinilah
// tempatnya: audit log hanya bisa dibaca pemegang izin audit:read, dan tanpa alasan yang
// tercatat, operator tidak punya cara membedakan seseorang yang lupa passwordnya dari
// seseorang yang sedang mencari-cari alamat email yang terdaftar.
func (s *Service) failureEvent(ctx context.Context, actor identity.Actor, userID, ip, userAgent string,
	reason FailureReason, extra map[string]any) identity.Event {
	metadata := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		metadata[k] = v
	}
	metadata["reason"] = string(reason)

	return identity.Event{
		Actor:        actor,
		Action:       identity.ActionLoginFailed,
		ResourceType: identity.ResourceUser,
		ResourceID:   userID,
		IP:           ip,
		UserAgent:    userAgent,
		RequestID:    observability.RequestIDFrom(ctx),
		Metadata:     metadata,
	}
}

// writeAudit menulis catatan audit dan hanya melaporkan kegagalannya ke log.
//
// Dipakai di jalur yang tidak punya transaksi untuk diikuti — percobaan login yang gagal
// sebelum ada baris apa pun yang berubah. Kegagalan menulisnya tidak boleh mengubah
// jawaban ke klien, karena perbedaan jawaban itu sendiri sudah menjadi keterangan.
func (s *Service) writeAudit(ctx context.Context, e identity.Event) {
	if err := s.audit.Write(ctx, e); err != nil {
		s.log(ctx).LogAttrs(ctx, slog.LevelError, "gagal menulis catatan audit",
			slog.String("action", e.Action), slog.String("error", err.Error()))
	}
}

// log mengembalikan logger layanan yang sudah dibubuhi request_id bila context punya.
//
// Logger milik layanan yang dipakai sebagai dasar, bukan yang ada di context: yang
// dijanjikan konstruktor adalah logger itu, dan jalur non-HTTP (worker, perintah CLI)
// tidak punya logger di context sama sekali.
func (s *Service) log(ctx context.Context) *slog.Logger {
	if id := observability.RequestIDFrom(ctx); id != "" {
		return s.logger.With(slog.String("request_id", id))
	}
	return s.logger
}

// truncateEmail memendekkan email ke batas yang aman disimpan dan di-log.
//
// Pemotongan mundur ke awal rune. Tanpa itu, nilai yang dipotong di tengah rune
// multibyte menjadi UTF-8 yang tidak sah, dan PostgreSQL menolak seluruh INSERT audit
// dengan galat encoding — kegagalan yang muncul justru pada masukan yang paling perlu
// tercatat.
func truncateEmail(email string) string {
	if len(email) <= maxEmailLen {
		return email
	}
	cut := maxEmailLen
	for cut > 0 && !utf8.RuneStart(email[cut]) {
		cut--
	}
	return email[:cut]
}

// firstOr mengembalikan elemen pertama sebuah slice, atau nilai pengganti bila kosong.
func firstOr(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0]
}
