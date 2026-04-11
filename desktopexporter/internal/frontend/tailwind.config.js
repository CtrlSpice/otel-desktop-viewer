/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Atkinson Hyperlegible Next"', 'system-ui', 'Segoe UI', 'sans-serif'],
        mono: ['"Atkinson Hyperlegible Mono"', 'ui-monospace', 'monospace'],
      },
      boxShadow: {
        surface: '0 1px 2px rgb(0 0 0 / 0.05), 0 8px 28px rgb(0 0 0 / 0.07)',
        'surface-sm': '0 1px 2px rgb(0 0 0 / 0.04)',
      },
    },
  },
}
