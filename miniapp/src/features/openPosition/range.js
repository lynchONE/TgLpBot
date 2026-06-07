export function parseRangeInput(lowerRaw, upperRaw) {
    const lower = Number(String(lowerRaw || '').trim());
    const upper = Number(String(upperRaw || '').trim());
    if (!Number.isFinite(lower) || !Number.isFinite(upper)) return null;
    return { lower: Math.abs(lower), upper: Math.abs(upper) };
}
