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

function legacyCopy(text) {
    if (typeof document === 'undefined') {
        return Promise.reject(new Error('Clipboard API not available'));
    }
    return new Promise((resolve, reject) => {
        try {
            const el = document.createElement('textarea');
            el.value = String(text ?? '');
            el.setAttribute('readonly', '');
            el.style.position = 'fixed';
            el.style.top = '0';
            el.style.left = '0';
            el.style.opacity = '0';
            el.style.pointerEvents = 'none';
            document.body.appendChild(el);
            el.focus();
            el.select();
            el.setSelectionRange(0, el.value.length);
            const ok = typeof document.execCommand === 'function' ? document.execCommand('copy') : false;
            document.body.removeChild(el);
            if (ok) resolve();
            else reject(new Error('Copy command rejected'));
        } catch (err) {
            reject(err);
        }
    });
}

export function copyToClipboard(text) {
    const tg = getTelegramWebApp();
    const value = String(text ?? '');

    if (typeof tg?.clipboard?.writeText === 'function') {
        try {
            const res = tg.clipboard.writeText(value);
            if (res && typeof res.then === 'function') return res;
            return Promise.resolve();
        } catch {
            // fallthrough
        }
    }
    if (navigator?.clipboard?.writeText) {
        return navigator.clipboard.writeText(value).catch(() => legacyCopy(value));
    }
    return legacyCopy(value);
}
