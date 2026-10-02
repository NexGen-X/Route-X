export const colors = {
  // Latar Belakang & Kanvas (AMOLED Dark)
  bgCanvas: '#0A0B0D',
  bgSurface: '#14171D',
  bgElevated: '#1C2029',
  border: '#212631',
  borderSubtle: '#212631',

  // Aksen Route-X
  accentPrimary: '#22C55E', // Green-500
  accentLime: '#A3E635',    // Lime-400
  accentGreenSubtle: 'rgba(34, 197, 94, 0.12)',
  accentGreenBorder: 'rgba(34, 197, 94, 0.35)',

  // Tipografi
  textPrimary: '#FFFFFF',
  textSecondary: '#94A3B8',
  textMuted: '#64748B',

  // Status & Indikator
  statusOnline: '#22C55E',
  statusOffline: '#64748B',
  statusWarning: '#EAB308',
  statusError: '#EF4444',

  // Aksen Avatar Pengguna
  avatarAdmin: '#22C55E',    // Hijau
  avatarOperator: '#3B82F6', // Biru
  avatarDev: '#A855F7',      // Ungu
  avatarUser: '#06B6D4',     // Cyan
} as const;

export type ColorKeys = keyof typeof colors;
