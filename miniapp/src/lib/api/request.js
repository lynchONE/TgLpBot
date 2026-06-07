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
