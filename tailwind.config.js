/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: ['./internal/web/templates/**/*.html'],
  theme: {
    extend: {
      colors: {
        brand: {
          400: '#818cf8',
          500: '#6366f1',
          600: '#4f46e5',
          700: '#4338ca',
        },
      },
      minHeight: {
        touch: '2.75rem',
      },
    },
  },
  plugins: [],
};
