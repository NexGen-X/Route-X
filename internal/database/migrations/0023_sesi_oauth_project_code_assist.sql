-- ==============================================================================
-- Route-X Migration 0023: Project Code Assist pada Sesi OAuth
-- ==============================================================================
-- Menambahkan kolom code_assist_project ke provider_oauth_sessions (B-157).
--
-- Latar: daftar model Antigravity bersifat project-scoped — endpoint
-- fetchAvailableModels hanya mengembalikan model tergating-project (rollout/
-- preview per project, mis. "Argon" yang terlihat di agy) bila permintaan
-- meneruskan project Code Assist akun. Gateway tidak pernah menyelesaikan
-- project itu (loadCodeAssist tidak dipanggil), sehingga katalog hanya berisi
-- himpunan generik yang dijawab upstream untuk body kosong. Kolom ini
-- menyimpan hasil resolusi project per sesi agar bisa diteruskan pada
-- fetchAvailableModels dan amplop generate/stream.
--
-- Sifat kolom: project ID adalah IDENTIFIKATOR project Google Cloud, bukan
-- kredensial — perlakuannya mengikuti kolom identitas sesi lain yang teks
-- biasa (client_id, account_email), BUKAN refresh_token_encrypted. Nullable:
-- sesi lama dan sesi yang resolusinya belum/gagal berjalan membiarkannya NULL
-- dan gateway jatuh kembali ke perilaku generik lama (project kosong).
-- ==============================================================================

alter table provider_oauth_sessions
    add column if not exists code_assist_project text;

comment on column provider_oauth_sessions.code_assist_project is 'Project ID Code Assist (cloudaicompanionProject) hasil resolusi loadCodeAssist untuk sesi ini; diteruskan ke fetchAvailableModels dan amplop generate/stream Antigravity (B-157). NULL berarti belum/tidak terselesaikan — perilaku generik tanpa project berlaku.';
