/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: 'var(--bg)',
        surface: 'var(--surface)',
        'surface-muted': 'var(--surface-muted)',
        line: 'var(--border)',
        'line-strong': 'var(--border-strong)',
        ink: 'var(--text)',
        muted: 'var(--text-muted)',
        blue: 'var(--brand-blue)',
        'blue-hover': 'var(--brand-blue-hover)',
        navy: 'var(--brand-navy)',
        sky: 'var(--brand-sky)',
        yellow: 'var(--brand-yellow)',
        'yellow-hover': 'var(--brand-yellow-hover)',
        success: 'var(--success)',
        'success-bg': 'var(--success-bg)',
        warning: 'var(--warning)',
        'warning-bg': 'var(--warning-bg)',
        danger: 'var(--danger)',
        'danger-bg': 'var(--danger-bg)',
        info: 'var(--info)',
        'info-bg': 'var(--info-bg)',
        neutral: 'var(--neutral)',
        'neutral-bg': 'var(--neutral-bg)',
      },
      fontFamily: {
        sans: ['Inter', 'Helvetica Neue', 'Arial', 'system-ui', 'sans-serif'],
        mono: ['ui-monospace', 'SF Mono', 'Menlo', 'monospace'],
      },
      fontSize: {
        meta: ['13px', { lineHeight: '18px' }],
        body: ['14px', { lineHeight: '22px' }],
        section: ['18px', { lineHeight: '26px' }],
        title: ['28px', { lineHeight: '36px' }],
        hero: ['32px', { lineHeight: '40px' }],
      },
      borderRadius: {
        sm: '6px',
        md: '10px',
        lg: '16px',
        pill: '999px',
      },
      boxShadow: {
        pop: '0 8px 24px rgba(0, 48, 135, 0.12)',
      },
      maxWidth: {
        content: '1120px',
        prose: '72ch',
      },
      spacing: {
        18: '4.5rem',
      },
    },
  },
  plugins: [],
};
