export function normalizeWalletAddress(value) {
    const raw = String(value || '').trim();
    if (!/^0x[0-9a-fA-F]{40}$/.test(raw)) return '';
    return `0x${raw.slice(2).toLowerCase()}`;
}

export function walletAvatarIdx(addr, iconCount) {
    if (!addr || addr.length < 6 || !Number.isFinite(Number(iconCount)) || Number(iconCount) <= 0) return 0;
    return parseInt(addr.slice(-4), 16) % Number(iconCount);
}

export function shortAddr(addr) {
    if (!addr || addr.length < 10) return addr || '';
    return addr.slice(0, 6) + '...' + addr.slice(-4);
}

export function tailAddr(value) {
    const raw = String(value || '').trim();
    if (!raw) return '--';
    return raw.slice(-4);
}

export function isHexAddressValue(value) {
    return /^0x[a-fA-F0-9]{40}$/.test(String(value || '').trim());
}

export function walletSourceLabel(source) {
    const value = String(source || '').trim();
    if (value === 'manual') return '\u624b\u52a8\u6dfb\u52a0';
    if (value === 'contract_interaction') return '\u5408\u7ea6\u53d1\u73b0';
    return value || '\u672a\u6807\u8bb0\u6765\u6e90';
}

export function walletSourceBadgeClass(source) {
    return String(source || '').trim() === 'manual'
        ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
        : 'border-white/10 bg-zinc-800/80 text-zinc-300';
}

export function walletSourceContractLabel(value) {
    const address = normalizeWalletAddress(value);
    return address ? `\u6765\u6e90\u5408\u7ea6 ${shortAddr(address)}` : '';
}
