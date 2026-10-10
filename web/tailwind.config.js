/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-base': '#05070B',
        'bg-sidebar': '#070A10',
        'bg-surface': '#0B0F19',
        'bg-surface-1': '#0F1626',
        'bg-surface-2': '#141E34',
        'bg-surface-3': '#1D2A48',
        border: '#1E293B',
        'border-subtle': '#141D2E',
        'border-hover': '#2C3E5F',
        'border-glass': 'rgba(255, 255, 255, 0.08)',
        'border-glass-hover': 'rgba(255, 255, 255, 0.16)',
        'border-glass-active': 'rgba(59, 130, 246, 0.35)',
        'text-primary': '#F8FAFC',
        'text-secondary': '#94A3B8',
        'text-muted': '#64748B',
        accent: {
          DEFAULT: '#3B82F6',
          bg: 'rgba(59, 130, 246, 0.12)',
          hover: '#2563EB',
          active: '#1D4ED8',
          glow: 'rgba(59, 130, 246, 0.35)',
          subtle: 'rgba(59, 130, 246, 0.08)',
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
        button: '8px',
      },
      boxShadow: {
        'elevation-glass': '0 16px 40px -12px rgba(0,0,0,0.85), inset 0 1px 0 0 rgba(255,255,255,0.08)',
        'glow-ai': '0 0 32px -4px rgba(59,130,246,0.35), 0 0 16px -2px rgba(6,182,212,0.2)',
        'glow-accent': '0 0 24px -4px rgba(59, 130, 246, 0.35), 0 0 12px -2px rgba(59, 130, 246, 0.2)',
        'glow-subtle': '0 0 16px -2px rgba(59, 130, 246, 0.16)',
        'glow-emerald': '0 0 20px -3px rgba(16, 185, 129, 0.3)',
        'glow-purple': '0 0 20px -3px rgba(168, 85, 247, 0.3)',
        'glow-rose': '0 0 20px -3px rgba(244, 63, 94, 0.3)',
        'glow-amber': '0 0 20px -3px rgba(245, 158, 11, 0.3)',
        'surface-elevated': '0 16px 40px -12px rgba(0, 0, 0, 0.75), 0 0 0 1px rgba(255, 255, 255, 0.08)',
        glass: '0 8px 32px 0 rgba(0, 0, 0, 0.37)',
      },
      backgroundImage: {
        'aurora-radial': 'radial-gradient(ellipse 80% 50% at 50% -20%, rgba(59, 130, 246, 0.22), transparent 70%)',
        'radial-glow': 'radial-gradient(circle at 50% 0%, rgba(59, 130, 246, 0.18) 0%, transparent 70%)',
        'radial-ambient': 'radial-gradient(ellipse at top, rgba(59, 130, 246, 0.1) 0%, transparent 60%)',
        'radial-card': 'radial-gradient(600px circle at 50% 0%, rgba(59, 130, 246, 0.07), transparent 40%)',
        'glass-gradient': 'linear-gradient(135deg, rgba(255, 255, 255, 0.05) 0%, rgba(255, 255, 255, 0.01) 100%)',
        'shimmer-gradient': 'linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.12), transparent)',
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
        'ping-subtle': {
          '75%, 100%': { transform: 'scale(1.8)', opacity: '0' },
        },
        'pulse-glow': {
          '0%, 100%': { opacity: '1', filter: 'drop-shadow(0 0 6px currentColor)' },
          '50%': { opacity: '0.6', filter: 'drop-shadow(0 0 2px currentColor)' },
        },
        shimmer: {
          '100%': { transform: 'translateX(100%)' },
        },
      },
      animation: {
        'toast-progress': 'toast-progress linear forwards',
        'fade-in': 'fade-in 0.2s cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'pulse-subtle': 'pulse-subtle 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'ping-subtle': 'ping-subtle 2s cubic-bezier(0, 0, 0.2, 1) infinite',
        'pulse-glow': 'pulse-glow 2s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2.5s infinite',
      },
    },
  },
  plugins: [],
};
