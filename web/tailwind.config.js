/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'bg-base': '#0A0A0A',
        'bg-sidebar': '#0C0C0C',
        'bg-surface': '#101010',
        'bg-surface-2': '#141414',
        border: '#1F1F1F',
        'border-subtle': '#1C1C1C',
        'text-primary': '#F5F5F5',
        'text-secondary': '#A1A1AA',
        'text-muted': '#6B7280',
        accent: {
          DEFAULT: '#BEF264',
          bg: 'rgba(190, 242, 100, 0.10)',
          hover: '#a3e635',
        },
        status: {
          success: '#4ADE80',
          error: '#F87171',
          warn: '#FBBF24',
          info: '#60A5FA',
        },
      },
      borderRadius: {
        card: '14px',
        inner: '12px',
        nav: '10px',
        chip: '9999px',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'Roboto', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
};
