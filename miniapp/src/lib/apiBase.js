export function resolveApiBaseUrl() {
    const queryApiBase = new URLSearchParams(window.location.search).get('apiBaseUrl');
    if (queryApiBase && queryApiBase.trim()) return queryApiBase.trim();

    const envBase = String(import.meta.env.VITE_API_BASE_URL || '').trim();
    if (envBase) {
        try {
            const pageProto = window.location.protocol;
            const envProto = new URL(envBase).protocol;
            if (pageProto === 'https:' && envProto === 'http:') {
                return '';
            }
        } catch {
            // ignore URL parse errors and keep envBase as-is
        }
        return envBase;
    }

    const host = window.location.hostname;
    if (host === 'localhost' || host === '127.0.0.1') {
        return 'http://localhost:8080';
    }

    // Production default: same-origin `/api/*` (e.g. via Vercel Function proxy)
    return '';
}

export function resolveAllowEmptyInitData() {
    const queryAllow = new URLSearchParams(window.location.search).get('allowEmptyInitData');
    if (queryAllow && ['1', 'true', 'yes', 'y', 'on'].includes(queryAllow.toLowerCase())) {
        return true;
    }

    const envAllow = String(import.meta.env.VITE_ALLOW_EMPTY_INITDATA || '').trim().toLowerCase();
    if (['1', 'true', 'yes', 'y', 'on'].includes(envAllow)) {
        return true;
    }

    const host = window.location.hostname;
    return host === 'localhost' || host === '127.0.0.1';
}

export function localizeWebAppError(message, allowEmptyInitData = false) {
    const text = String(message || '').trim();
    if (!text) return '';
    if (text.includes('missing initData')) {
        if (allowEmptyInitData) {
            return '当前缺少 Telegram initData。若这是本地调试，可在 backend/.env 中设置 TELEGRAM_WEBAPP_ALLOW_EMPTY_INITDATA=1。';
        }
        return '当前缺少 Telegram initData，请从 Telegram Mini App 内打开。';
    }
    if (text.includes('invalid initData')) {
        return 'Telegram initData 校验失败，请检查 backend 侧 TELEGRAM_BOT_TOKEN 配置。';
    }
    return text;
}
