import React from 'react';
import { useTheme } from '../context/ThemeContext';

const SunIcon = (props) => (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" {...props}>
        <path
            d="M12 18a6 6 0 1 0 0-12 6 6 0 0 0 0 12Z"
            stroke="currentColor"
            strokeWidth="2"
        />
        <path
            d="M12 2v2m0 16v2m10-10h-2M4 12H2m16.95-6.95-1.41 1.41M6.46 17.54l-1.41 1.41m0-12.72 1.41 1.41m12.08 12.08 1.41 1.41"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
        />
    </svg>
);

const MoonIcon = (props) => (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" {...props}>
        <path
            d="M21 12.8A8.5 8.5 0 1 1 11.2 3a6.5 6.5 0 0 0 9.8 9.8Z"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinejoin="round"
        />
    </svg>
);

const ThemeToggle = () => {
    const { theme, toggleTheme } = useTheme();
    const isDark = theme === 'dark';

    return (
        <button
            type="button"
            onClick={toggleTheme}
            aria-label={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
            title={isDark ? 'Light mode' : 'Dark mode'}
            className="relative inline-flex h-9 w-16 items-center rounded-full border border-border bg-secondary/50 p-1 shadow-sm backdrop-blur-sm transition-all hover:bg-secondary focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
        >
            <span className="pointer-events-none absolute left-2 text-muted-foreground">
                <SunIcon className="h-4 w-4" />
            </span>
            <span className="pointer-events-none absolute right-2 text-muted-foreground">
                <MoonIcon className="h-4 w-4" />
            </span>
            <span
                className={`grid h-7 w-7 transform place-items-center rounded-full bg-background text-foreground shadow-md ring-1 ring-black/5 transition-transform duration-300 ${isDark ? 'translate-x-7' : 'translate-x-0'}`}
            >
                {isDark ? <MoonIcon className="h-4 w-4" /> : <SunIcon className="h-4 w-4" />}
            </span>
        </button>
    );
};

export default ThemeToggle;
