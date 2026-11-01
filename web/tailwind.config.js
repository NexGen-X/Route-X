/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-base': '#181818',
        'bg-sidebar': '#161616',
        'bg-surface': '#1C1C1C',
        'bg-surface-1': '#1F1F1F',
        'bg-surface-2': '#2A2A2A',
        'bg-surface-3': '#323232',
        border: '#2A2A2A',
        'border-subtle': '#232323',
        'border-hover': '#3A3A3A',
        'border-glass': 'rgba(255, 255, 255, 0.08)',
        'border-glass-hover': 'rgba(255, 255, 255, 0.14)',
        'border-glass-active': '#4A4A4A',
        'text-primary': '#ECECEC',
        'text-secondary': '#B9B9B9',
        // #969696: >= 4.85:1 di atas semua permukaan tema (audit F8) —
        // #737373 lama hanya 3.0-3.7:1 dan gagal WCAG AA untuk teks kecil.
        'text-muted': '#969696',
        accent: {
          DEFAULT: '#0070E0',
          bg: '#1F1F1F',
          hover: '#1A7FE6',
          // #5BACE6: varian aksen KHUSUS TEKS (audit kontras axe Fase 3 P6) —
          // #0070E0 sebagai teks hanya 2,6–3,6:1 di permukaan gelap dan
          // gagal WCAG AA; #5BACE6 >= 4,87:1 di semua permukaan tema
          // termasuk latar bertinta aksen terburuk (#22384E).
          text: '#5BACE6',
          active: '#005BB8',
          glow: 'rgba(0, 112, 224, 0.3)',
          subtle: 'rgba(0, 112, 224, 0.12)',
        },
        cyan: {
          DEFAULT: '#4DA3E8',
          subtle: 'rgba(77, 163, 232, 0.12)',
          glow: 'rgba(77, 163, 232, 0.3)',
        },
        status: {
          success: '#3ECF8E',
          error: '#F26D6D',
          warn: '#F5B84C',
          info: '#5BACE6',
        },
        // Penyelarasan Fase 6 ke palet sampel Calm Console yang disetujui
        // pemilik repo: shade Tailwind yang dipakai kelas-kelas status di
        // seluruh komponen dipetakan ke keluarga pastel mockup (hijau mint
        // #3ECF8E, koral #F26D6D, kuning lembut #F5B84C, biru teks #5BACE6).
        emerald: { 300: '#7CE3B1', 400: '#3ECF8E', 500: '#2EB87E' },
        red: { 300: '#F6A8A8', 400: '#F26D6D', 500: '#E45757', 600: '#D64545' },
        rose: { 300: '#F6A8A8', 400: '#F26D6D', 500: '#E45757' },
        amber: { 200: '#FBE3B0', 300: '#FAD48F', 400: '#F5B84C', 500: '#F0A832' },
        sky: { 300: '#A5D8F7', 400: '#5BACE6', 500: '#3E94D8' },
        teal: { 400: '#3ECF8E' },
        green: { 400: '#3ECF8E' },
      },
      borderRadius: {
        card: '16px',
        inner: '10px',
        nav: '10px',
        chip: '9999px',
        button: '10px',
      },
      boxShadow: {
        'elevation-glass': '0 4px 12px 0 rgba(0, 0, 0, 0.5), inset 0 1px 0 0 rgba(255, 255, 255, 0.05)',
        'glow-ai': '0 0 20px -3px rgba(0, 112, 224, 0.25)',
        'glow-accent': '0 0 24px -4px rgba(0, 112, 224, 0.35)',
        'glow-cyan': '0 0 24px -4px rgba(77, 163, 232, 0.35)',
        'glow-subtle': '0 0 16px -2px rgba(0, 112, 224, 0.15)',
        'glow-emerald': '0 0 20px -3px rgba(16, 185, 129, 0.25)',
        'glow-purple': '0 0 20px -3px rgba(0, 112, 224, 0.25)',
        'glow-rose': '0 0 20px -3px rgba(244, 63, 94, 0.25)',
        'glow-amber': '0 0 20px -3px rgba(245, 158, 11, 0.25)',
        'surface-elevated': '0 4px 16px -4px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(39, 39, 42, 0.8)',
        card: '0 1px 3px 0 rgba(0, 0, 0, 0.4), 0 1px 2px -1px rgba(0, 0, 0, 0.4)',
        glass: '0 4px 16px 0 rgba(0, 0, 0, 0.4)',
      },
      backgroundImage: {
        'glass-gradient': 'linear-gradient(135deg, rgba(255, 255, 255, 0.03) 0%, rgba(255, 255, 255, 0.01) 100%)',
        'shimmer-gradient': 'linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.06), transparent)',
        'aurora-radial': 'radial-gradient(ellipse 60% 40% at 50% 0%, rgba(255, 255, 255, 0.04), transparent 70%)',
        'radial-ambient': 'radial-gradient(ellipse 50% 30% at 50% 100%, rgba(255, 255, 255, 0.03), transparent 70%)',
        'radial-card': 'radial-gradient(circle at 50% 50%, rgba(255, 255, 255, 0.02), transparent 80%)',
      },
      fontFamily: {
        sans: ['"Noto Sans"', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
      },
      keyframes: {
        'toast-progress': {
          from: { width: '100%' },
          to: { width: '0%' },
        },
        'fade-in': {
          from: { opacity: '0', transform: 'translateY(4px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        'sheet-up': {
          from: { opacity: '0', transform: 'translateY(28px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        'pulse-subtle': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.6' },
        },
        'ping-subtle': {
          '75%, 100%': { transform: 'scale(1.8)', opacity: '0' },
        },
        'pulse-glow': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.6' },
        },
        shimmer: {
          '100%': { transform: 'translateX(100%)' },
        },
      },
      animation: {
        'toast-progress': 'toast-progress linear forwards',
        'fade-in': 'fade-in 0.15s cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'sheet-up': 'sheet-up 0.28s cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'pulse-subtle': 'pulse-subtle 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'ping-subtle': 'ping-subtle 2s cubic-bezier(0, 0, 0.2, 1) infinite',
        'pulse-glow': 'pulse-glow 2s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2.5s infinite',
      },
    },
  },
  plugins: [],
};
