-- Migrasi 0025 — jenis event finish_reason_unknown (B-248).
--
-- Finish reason upstream yang berada di luar kosakata kanonik (mis.
-- sebab berhenti khas vendor) dilipat permukaan Anthropic menjadi
-- end_turn, karena kosakata stop_reason Anthropic tertutup dan tidak
-- punya padanan jujur untuk nilai asing. Tanpa jenis event ini,
-- pelipatan itu tidak tercatat di mana pun dan sebab berhenti yang
-- sebenarnya hilang dari timeline request — operator hanya melihat
-- end_turn yang tampak normal. Constraint diganti dengan daftar lama
-- + satu nilai baru; tidak ada baris lama yang ditulis ulang.

alter table request_events
    drop constraint if exists request_events_kind_valid;

alter table request_events
    add constraint request_events_kind_valid check (kind in (
        'attempt_started', 'attempt_succeeded', 'attempt_failed',
        'retry_scheduled', 'failover', 'circuit_open', 'rate_limited',
        'budget_exceeded', 'content_blocked', 'stream_started', 'stream_completed',
        'cache_hit', 'finish_reason_unknown'
    ));
