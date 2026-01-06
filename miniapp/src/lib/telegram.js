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

/**
 * 触觉反馈类型
 * - impact: 物理碰撞反馈 (light, medium, heavy, rigid, soft)
 * - notification: 通知反馈 (success, warning, error)
 * - selection: 选择变化反馈
 */

// 碰撞反馈 - 按钮点击
export function hapticImpact(style = 'light') {
    const tg = getTelegramWebApp();
    try {
        tg?.HapticFeedback?.impactOccurred?.(style);
    } catch {
        // ignore - 设备可能不支持
    }
}

// 通知反馈 - 操作结果
export function hapticNotification(type = 'success') {
    const tg = getTelegramWebApp();
    try {
        tg?.HapticFeedback?.notificationOccurred?.(type);
    } catch {
        // ignore
    }
}

// 选择反馈 - 轻触反馈
export function hapticSelection() {
    const tg = getTelegramWebApp();
    try {
        tg?.HapticFeedback?.selectionChanged?.();
    } catch {
        // ignore
    }
}
