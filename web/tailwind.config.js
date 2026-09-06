/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-base': '#0A0A0A',
        'bg-sidebar': '#0C0C0C',
        'bg-surface': '#121212',
        'bg-surface-1': '#121212',
        'bg-surface-2': '#161616',
        'bg-surface-3': '#1E1E1E',
        'surface': '#121212',
        'surface-dark': '#0E0E0E',
        border: '#1F1F1F',
        'border-subtle': '#1C1C1C',
        'text-primary': '#EDEDED',
        'text-secondary': '#8E8E93',
        'text-muted': '#9CA3AF',
        accent: {
          DEFAULT: '#BEF264',
          bg: 'rgba(190, 242, 100, 0.10)',
          hover: '#a3e635',
        },
        status: {
          success: '#4ADE80',
          error: '#F87171',
          warn: '#FBBF24',
          warning: '#FBBF24',
          info: '#60A5FA',
        },
      },
      borderRadius: {
        card: '14px',
        inner: '12px',
        nav: '10px',
        chip: '9999px',
        box: '10px',
        button: '8px',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
};
