-- ==============================================================================
-- Route-X Migration 0010: Drop RBAC & Roles for Single-Admin Architecture
-- ==============================================================================
-- Route-X disederhanakan untuk personal single-admin / open-source.
-- Seluruh tabel pembagian peran dan izin teknis dihapus dari skema.
-- ==============================================================================

DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
DROP TABLE IF EXISTS permissions CASCADE;
