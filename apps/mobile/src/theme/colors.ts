export const colors = {
  // Latar Belakang & Kanvas (Obsidian Deep Dark)
  bgCanvas: '#0B0F17',
  bgSurface: '#0F1523',
  bgElevated: '#182238',
  border: '#1E293B',
  borderSubtle: '#151F32',

  // Aksen Route-X (DeepSeek / Hermes Celestial Blue)
  accentPrimary: '#3B82F6', // Blue-500
  accentLime: '#60A5FA',    // Sky-400
  accentGreenSubtle: 'rgba(59, 130, 246, 0.12)',
  accentGreenBorder: 'rgba(59, 130, 246, 0.35)',

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

  // Aliases kustom
  bgBase: '#0A0B0D',
  bgCard: '#14171D',
  danger: '#EF4444',
  warning: '#EAB308',
} as const;

export type ColorKeys = keyof typeof colors;
