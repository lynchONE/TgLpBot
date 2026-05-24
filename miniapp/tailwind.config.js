/** @type {import('tailwindcss').Config} */
export default {
    darkMode: 'class',
    content: ['./index.html', './src/**/*.{js,jsx}'],
    theme: {
        extend: {
            colors: {
                surface: {
                    1: '#0f1116',
                    2: '#14171c',
                    3: '#1c2026',
                },
                pnl: {
                    pos: '#34d399',
                    neg: '#f87171',
                },
            },
            borderRadius: {
                card: '1rem',
                chip: '0.625rem',
            },
            boxShadow: {
                card: '0 1px 0 rgba(255,255,255,0.04) inset, 0 8px 24px -12px rgba(0,0,0,0.4)',
                pop: '0 12px 40px -8px rgba(0,0,0,0.55)',
            },
            fontSize: {
                micro: ['11px', '14px'],
                caption: ['12px', '16px'],
            },
        },
    },
    plugins: [],
};
