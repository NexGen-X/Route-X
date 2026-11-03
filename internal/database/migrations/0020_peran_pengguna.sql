-- =============================================================================
-- Route-X Migration 0020: Peran pengguna — kolom is_admin (B-102)
-- =============================================================================
-- Migrasi 0010 menghapus seluruh RBAC (roles/permissions/user_roles) demi
-- arsitektur single-admin, dan akibatnya SETIAP akun yang dibuat lewat panel
-- menjadi admin penuh: lapisan auth memberi peran "Admin" + izin "*" kepada
-- siapa pun yang terautentikasi, tanpa satu pun kolom yang membedakan
-- kewenangan antar akun (terbukti live di Langkah 7: pengguna hasil
-- createUser bisa membaca GET /api/admin/access/users dengan 200).
--
-- Perbaikan ini SENGAJA tidak menghidupkan kembali RBAC bergranular: taksonomi
-- yang hidup di kode hanya mengenal dua tingkat — admin (peran "Admin", izin
-- "*") dan bukan admin (peran "User", tanpa izin admin). Satu kolom boolean
-- adalah representasi paling sederhana dan jujur untuk taksonomi itu.
--
-- Bawaan kolom FALSE (aman): baris baru yang ditulis jalur mana pun tanpa
-- menyebut peran adalah NON-admin. Pengguna yang SUDAH ada di-backfill TRUE:
-- sampai migrasi ini mereka memang admin penuh secara faktual, dan mencabut
-- kewenangan itu lewat migrasi akan mengunci operator keluar dari
-- instalasinya sendiri. Pencabutan sesudah ini adalah keputusan eksplisit
-- operator lewat update pengguna.
-- =============================================================================

alter table users add column if not exists is_admin boolean not null default false;

update users set is_admin = true where is_admin = false;

comment on column users.is_admin is 'true = admin penuh (peran Admin, izin *); false = pengguna biasa tanpa izin admin (B-102). Bawaan false: akun baru non-admin kecuali peran admin diminta eksplisit.';
