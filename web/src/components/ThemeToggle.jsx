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
            className="relative inline-flex h-9 w-16 items-center rounded-full border border-slate-200/70 bg-white/70 p-1 shadow-sm backdrop-blur transition-colors hover:bg-white focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-50 dark:border-slate-800/70 dark:bg-[#151718]/60 dark:hover:bg-[#151718]/80 dark:focus-visible:ring-offset-[#0d0e10]"
        >
            <span className="pointer-events-none absolute left-2 text-slate-500 dark:text-slate-400">
                <SunIcon className="h-4 w-4" />
            </span>
            <span className="pointer-events-none absolute right-2 text-slate-500 dark:text-slate-400">
                <MoonIcon className="h-4 w-4" />
            </span>
            <span
                className={`grid h-7 w-7 transform place-items-center rounded-full bg-slate-900 text-white shadow transition-transform duration-200 dark:bg-white dark:text-slate-900 ${isDark ? 'translate-x-7' : 'translate-x-0'}`}
            >
                {isDark ? <MoonIcon className="h-4 w-4" /> : <SunIcon className="h-4 w-4" />}
            </span>
        </button>
    );
};

export default ThemeToggle;
