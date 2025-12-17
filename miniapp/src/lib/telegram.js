export function getTelegramWebApp() {
    if (typeof window === 'undefined') return null;
    return window.Telegram?.WebApp ?? null;
}

export function openLink(url) {
    const tg = getTelegramWebApp();
    if (tg?.openLink) {
        tg.openLink(url);
        return;
    }
    window.open(url, '_blank', 'noopener,noreferrer');
}

export function copyToClipboard(text) {
    const tg = getTelegramWebApp();
    if (tg?.clipboard?.writeText) {
        tg.clipboard.writeText(text);
        return Promise.resolve();
    }
    if (navigator?.clipboard?.writeText) {
        return navigator.clipboard.writeText(text);
    }
    return Promise.reject(new Error('Clipboard API not available'));
}

