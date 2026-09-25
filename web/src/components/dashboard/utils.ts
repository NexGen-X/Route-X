export const formatBytes = (bytes?: number): string => {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
};

export const formatUptime = (sec?: number): string => {
  if (!sec || sec < 0) return '0s';
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = Math.floor(sec % 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
};

export const formatTimeAgo = (dateStr: string): string => {
  const t = new Date(dateStr).getTime();
  if (Number.isNaN(t)) return 'waktu tak dikenal';
  const diff = Math.floor((Date.now() - t) / 1000);
  if (diff < 5) return 'baru saja';
  if (diff < 60) return `${diff} detik lalu`;
  const m = Math.floor(diff / 60);
  if (m < 60) return `${m}m lalu`;
  const h = Math.floor(m / 60);
  return `${h}j lalu`;
};
