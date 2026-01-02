/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'sans-serif'],
        display: ['Outfit', 'sans-serif'],
      },
      colors: {
        // V2 Neo-Futuristic Palette
        midnight: {
          950: '#020617', // Main background
          900: '#0f172a', // Card background
          800: '#1e293b', // Card hover
        },
        neon: {
          cyan: '#06b6d4',
          purple: '#a855f7',
          green: '#22c55e',
          blue: '#3b82f6',
        },
        surface: {
          dark: 'rgba(15, 23, 42, 0.6)',
          light: 'rgba(255, 255, 255, 0.05)',
        }
      },
      animation: {
        'wiggle': 'wiggle 1s ease-in-out infinite',
        'float': 'float 6s ease-in-out infinite',
        'pulse-slow': 'pulse 4s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'glow': 'glow 2s ease-in-out infinite alternate',
      },
      keyframes: {
        wiggle: {
          '0%, 100%': { transform: 'rotate(-3deg)' },
          '50%': { transform: 'rotate(3deg)' },
        },
        float: {
          '0%, 100%': { transform: 'translateY(0)' },
          '50%': { transform: 'translateY(-10px)' },
        },
        glow: {
          '0%': { boxShadow: '0 0 5px rgba(6, 182, 212, 0.5)' },
          '100%': { boxShadow: '0 0 20px rgba(6, 182, 212, 0.8), 0 0 10px rgba(168, 85, 247, 0.6)' },
        }
      }
    },
  },
  plugins: [],
}
