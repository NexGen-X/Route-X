-- Memisahkan penanda peringatan ambang (threshold) dan terlampaui (exceeded) pada tabel budgets.
-- alerted_at menandai bahwa notifikasi ambang batas (budget.threshold) sudah dikirim,
-- sedangkan exceeded_alerted_at menandai bahwa notifikasi batas anggaran terlampaui (budget.exceeded) sudah dikirim.

alter table budgets add column if not exists exceeded_alerted_at timestamptz;

comment on column budgets.alerted_at is 'Waktu saat notifikasi ambang batas (budget.threshold) dikirim.';
comment on column budgets.exceeded_alerted_at is 'Waktu saat notifikasi terlampaui (budget.exceeded) dikirim.';
