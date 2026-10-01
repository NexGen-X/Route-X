/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-base': 'var(--bg-base)',
        'bg-sidebar': 'var(--bg-surface)',
        'bg-surface': 'var(--bg-surface)',
        'bg-surface-1': 'var(--bg-surface-1)',
        'bg-surface-2': 'var(--bg-surface-2)',
        'bg-surface-3': 'var(--bg-surface-3)',
        'glass-bg': 'var(--glass-bg)',
        'glass-border': 'var(--glass-border)',
        border: 'var(--border)',
        'border-subtle': 'var(--border-subtle)',
        'border-hover': 'var(--border-hover)',
        'text-primary': 'var(--text-primary)',
        'text-secondary': 'var(--text-secondary)',
        'text-muted': 'var(--text-muted)',
        accent: {
          DEFAULT: 'var(--accent)',
          bg: 'var(--accent-bg)',
          hover: 'var(--accent-hover)',
        },
        status: {
          success: '#4ADE80',
          error: '#F87171',
          warn: '#FBBF24',
          info: '#60A5FA',
        },
      },
      borderRadius: {
        card: 'var(--radius-card)',
        inner: 'var(--radius-inner)',
        nav: 'var(--radius-nav)',
        chip: 'var(--radius-chip)',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
      },
      transitionTimingFunction: {
        fluid: 'var(--ease-fluid)',
        spring: 'var(--ease-spring)',
      },
      keyframes: {
        'toast-progress': {
          from: { width: '100%' },
          to: { width: '0%' },
        },
      },
      animation: {
        'toast-progress': 'toast-progress linear forwards',
      },
      boxShadow: {
        'glow-accent': '0 0 20px -5px var(--accent)',
        'glass-panel': 'inset 0 1px 0 0 rgba(255,255,255,0.05), 0 8px 32px -4px rgba(0,0,0,0.5)',
      },
    },
  },
  plugins: [],
};
