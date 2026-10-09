-- ==============================================================================
-- Route-X Migration 0016: Critical Performance Indexes (Audit Item P1-5)
-- ==============================================================================
-- Menambahkan indeks performa untuk mengeliminasi sequential scan:
-- 1. api_keys (rotated_from)       -> Optimasi query lineage rekursif
-- 2. sessions (revoked_at)        -> Optimasi worker pembersihan sesi tercabut
-- 3. requests (created_at, error) -> Partial index query error filter status_code >= 400 OR error_type IS NOT NULL
-- 4. sessions (user_id)           -> Lookup sesi aktif dan riwayat per pengguna
-- 5. providers (egress_pool_id)   -> Optimasi relasi foreign key egress pool pada providers
-- ==============================================================================

-- 1. Index parsial lineage API key
CREATE INDEX IF NOT EXISTS api_keys_rotated_from_idx
    ON api_keys (rotated_from)
    WHERE rotated_from IS NOT NULL;

-- 2. Index parsial pembersihan sesi tercabut
CREATE INDEX IF NOT EXISTS sessions_cleanup_revoked_idx
    ON sessions (revoked_at)
    WHERE revoked_at IS NOT NULL;

-- 3. Index komposit parsial filter error log request
CREATE INDEX IF NOT EXISTS requests_errors_all_idx
    ON requests (created_at DESC)
    WHERE status_code >= 400 OR error_type IS NOT NULL;

-- 4. Index pencarian sesi per user
CREATE INDEX IF NOT EXISTS sessions_user_all_idx
    ON sessions (user_id);

-- 5. Index parsial relasi egress pool pada providers
CREATE INDEX IF NOT EXISTS providers_egress_pool_idx
    ON providers (egress_pool_id)
    WHERE egress_pool_id IS NOT NULL;
