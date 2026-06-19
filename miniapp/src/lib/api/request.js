export async function readErrorDetails(resp) {
    const text = await resp.text().catch(() => '');
    if (!text) {
        return { message: `HTTP ${resp.status}`, payload: null };
    }
    try {
        const parsed = JSON.parse(text);
        if (parsed && typeof parsed === 'object') {
            return {
                message: parsed?.message ? String(parsed.message) : `HTTP ${resp.status}`,
                payload: parsed,
            };
        }
    } catch {
        // ignore JSON parse errors
    }
    return { message: text, payload: null };
}

export async function requestJson(url, options) {
    const resp = await fetch(url, options);
    if (!resp.ok) {
        const detail = await readErrorDetails(resp);
        const err = new Error(detail.message);
        err.status = resp.status;
        if (detail.payload && typeof detail.payload === 'object') {
            err.payload = detail.payload;
            Object.assign(err, detail.payload);
        }
        throw err;
    }
    return resp.json();
}

export function timeoutSignal(parentSignal, timeoutMs, reason = 'request timeout') {
    const ms = Number(timeoutMs);
    if (!Number.isFinite(ms) || ms <= 0) {
        throw new Error('invalid timeout');
    }
    const controller = new AbortController();
    const timer = setTimeout(() => {
        controller.abort(new Error(reason));
    }, ms);
    let settled = false;
    let detachParent = () => {};
    const clear = () => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        detachParent();
    };

    if (parentSignal) {
        if (parentSignal.aborted) {
            controller.abort(parentSignal.reason);
            clear();
        } else {
            const abortFromParent = () => {
                controller.abort(parentSignal.reason);
                clear();
            };
            parentSignal.addEventListener('abort', abortFromParent, { once: true });
            detachParent = () => parentSignal.removeEventListener('abort', abortFromParent);
        }
    }

    controller.signal.addEventListener('abort', clear, { once: true });
    return { signal: controller.signal, clear };
}
