/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {},
  },
  plugins: [require('daisyui')],
  daisyui: {
    themes: [
      {
        'rose-pine-moon': {
          // Rosé Pine Moon - Dark theme (muted)
          primary: '#c4a7e7', // Iris
          'primary-content': '#232136', // Base
          secondary: '#3e8fb0', // Pine
          'secondary-content': '#e0def4', // Text
          accent: '#ea9a97', // Rose
          'accent-content': '#232136', // Base
          neutral: '#6e6a86', // Muted
          'neutral-content': '#e0def4', // Text
          'base-100': '#232136', // Base
          'base-200': '#2a273f', // Surface
          'base-300': '#393552', // Overlay
          'base-content': '#e0def4', // Text
          info: '#9ccfd8', // Foam
          'info-content': '#232136', // Base
          success: '#3e8fb0', // Pine
          'success-content': '#e0def4', // Text
          warning: '#f6c177', // Gold
          'warning-content': '#232136', // Base
          error: '#eb6f92', // Love
          'error-content': '#e0def4', // Text
        },
      },
      {
        'rose-pine-dawn': {
          // Rosé Pine Dawn - Light theme
          primary: '#907aa9', // Iris
          'primary-content': '#faf4ed', // Base
          secondary: '#286983', // Pine
          'secondary-content': '#faf4ed', // Base
          accent: '#d7827e', // Rose
          'accent-content': '#faf4ed', // Base
          neutral: '#9893a5', // Muted
          'neutral-content': '#575279', // Text
          'base-100': '#faf4ed', // Base
          'base-200': '#fffaf3', // Surface
          'base-300': '#f2e9e1', // Overlay
          'base-content': '#575279', // Text
          info: '#56949f', // Foam
          'info-content': '#faf4ed', // Base
          success: '#286983', // Pine
          'success-content': '#faf4ed', // Base
          warning: '#ea9d34', // Gold
          'warning-content': '#faf4ed', // Base
          error: '#b4637a', // Love
          'error-content': '#faf4ed', // Base
        },
      },
    ],
  },
};
