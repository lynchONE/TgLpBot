export function parsePositionCreatedTime(position) {
    const raw = String(position?.running_since || position?.created_at || '').trim();
    if (!raw) return null;
    const ts = Date.parse(raw);
    return Number.isFinite(ts) ? ts : null;
}

export function comparePositionsByCreatedAt(a, b) {
    const aTime = parsePositionCreatedTime(a);
    const bTime = parsePositionCreatedTime(b);
    if (aTime !== null && bTime !== null && aTime !== bTime) return aTime - bTime;
    if (aTime !== null && bTime === null) return -1;
    if (aTime === null && bTime !== null) return 1;

    const aTaskId = Number(a?.task_id || 0);
    const bTaskId = Number(b?.task_id || 0);
    if (aTaskId !== bTaskId) return aTaskId - bTaskId;

    const aKey = [
        String(a?.title || ''),
        String(a?.pool_id || a?.pool_address || '').toLowerCase(),
        String(a?.position_id || ''),
        String(a?.version || ''),
        String(a?.exchange || '').toLowerCase(),
    ].join(':');
    const bKey = [
        String(b?.title || ''),
        String(b?.pool_id || b?.pool_address || '').toLowerCase(),
        String(b?.position_id || ''),
        String(b?.version || ''),
        String(b?.exchange || '').toLowerCase(),
    ].join(':');
    return aKey.localeCompare(bKey, undefined, { numeric: true });
}
