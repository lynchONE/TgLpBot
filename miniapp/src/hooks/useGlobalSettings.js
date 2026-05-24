import { useCallback, useEffect, useState } from 'react';
import { getTelegramWebApp } from '../lib/telegram';
import { normalizeAccentTheme } from '../lib/brand';

const STORAGE_THEME = 'tglp_theme';
const STORAGE_ACCENT_THEME = 'tglp_accent_theme';

function readStorage(key) {
    try {
        return window.localStorage?.getItem(key) ?? null;
    } catch {
        return null;
    }
}

function writeStorage(key, value) {
    try {
        window.localStorage?.setItem(key, value);
    } catch {
        // ignore
    }
}

/**
 * 管理用户的全局主题偏好（dark/light + accent）+ 同步 Telegram WebApp 颜色。
 *
 * 行为：
 * - 首次挂载从 localStorage 读取偏好，没有则默认 dark + lime
 * - theme/accentTheme 变化时：写回 localStorage、切 document.html 的 dark class、同步 Telegram WebApp header/background 颜色
 */
export function useGlobalSettings() {
    const [theme, setTheme] = useState('dark');
    const [accentTheme, setAccentTheme] = useState(() => normalizeAccentTheme(readStorage(STORAGE_ACCENT_THEME)));

    useEffect(() => {
        const savedTheme = readStorage(STORAGE_THEME);
        if (savedTheme === 'light' || savedTheme === 'dark') {
            setTheme(savedTheme);
        } else {
            setTheme('dark');
        }
        setAccentTheme(normalizeAccentTheme(readStorage(STORAGE_ACCENT_THEME)));
    }, []);

    useEffect(() => {
        const isDark = theme === 'dark';
        document.documentElement.classList.toggle('dark', isDark);
        writeStorage(STORAGE_THEME, isDark ? 'dark' : 'light');
        writeStorage(STORAGE_ACCENT_THEME, accentTheme);

        const tg = getTelegramWebApp();
        try {
            tg?.setHeaderColor?.(isDark ? '#0b0f14' : '#fafafa');
            tg?.setBackgroundColor?.(isDark ? '#0b0f14' : '#fafafa');
        } catch {
            // ignore
        }
    }, [accentTheme, theme]);

    const toggleTheme = useCallback(() => {
        setTheme((t) => (t === 'dark' ? 'light' : 'dark'));
    }, []);

    return { theme, setTheme, toggleTheme, accentTheme, setAccentTheme };
}

export default useGlobalSettings;
