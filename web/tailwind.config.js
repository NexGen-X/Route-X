/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-base': '#0B0F17',
        'bg-sidebar': '#080C14',
        'bg-surface': '#0F1523',
        'bg-surface-1': '#131B2E',
        'bg-surface-2': '#182238',
        'bg-surface-3': '#1E2B45',
        border: '#1E293B',
        'border-subtle': '#151F32',
        'border-hover': '#2E3D59',
        'text-primary': '#F8FAFC',
        'text-secondary': '#94A3B8',
        'text-muted': '#64748B',
        accent: {
          DEFAULT: '#2563EB',
          bg: 'rgba(37, 99, 235, 0.12)',
          hover: '#1D4ED8',
          glow: 'rgba(37, 99, 235, 0.25)',
          subtle: 'rgba(37, 99, 235, 0.08)',
        },
        status: {
          success: '#10B981',
          error: '#F43F5E',
          warn: '#F59E0B',
          info: '#38BDF8',
        },
      },
      borderRadius: {
        card: '14px',
        inner: '10px',
        nav: '10px',
        chip: '9999px',
      },
      boxShadow: {
        'glow-accent': '0 0 24px -4px rgba(59, 130, 246, 0.28)',
        'glow-subtle': '0 0 16px -2px rgba(59, 130, 246, 0.15)',
        'surface-elevated': '0 12px 32px -8px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(255, 255, 255, 0.05)',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
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
        'pulse-subtle': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.6' },
        },
      },
      animation: {
        'toast-progress': 'toast-progress linear forwards',
        'fade-in': 'fade-in 0.2s cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'pulse-subtle': 'pulse-subtle 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
      },
    },
  },
  plugins: [],
};
