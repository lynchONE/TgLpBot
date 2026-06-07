export const GMGN_STABLE_SYMBOLS = new Set(['usdc', 'usdt', 'busd', 'dai', 'frax', 'usdd', 'fdusd', 'wbnb', 'weth', 'wsol', 'bnb', 'eth', 'sol']);

export function getPairLabel(value) {
    const pair = String(value?.trading_pair || '').trim();
    if (pair && pair !== '/') return pair;
    const left = String(value?.token0_symbol || '').trim();
    const right = String(value?.token1_symbol || '').trim();
    if (left && right) return `${left}/${right}`;
    if (left) return left;
    if (right) return right;
    return '鏈瘑鍒氦鏄撳';
}

export function getPoolIdentifier(value) {
    return String(value?.pool_address || '').trim();
}

export function resolvePoolChain(value) {
    if (String(value?.chain || '').trim()) return String(value.chain).trim().toLowerCase();
    return Number(value?.chain_id) === 8453 ? 'base' : 'bsc';
}

export function pickGmgnTokenAddress(pool) {
    const pair = String(pool?.trading_pair || '').trim();
    const token0 = String(pool?.token0_address || '').trim();
    const token1 = String(pool?.token1_address || '').trim();
    if (!pair) return token0 || token1;

    const symbols = pair.split('/').map((part) => String(part || '').trim().toLowerCase());
    if (symbols.length !== 2) return token0 || token1;

    const [leftSymbol, rightSymbol] = symbols;
    const leftStable = GMGN_STABLE_SYMBOLS.has(leftSymbol);
    const rightStable = GMGN_STABLE_SYMBOLS.has(rightSymbol);
    if (leftStable && !rightStable) return token1 || token0;
    if (rightStable && !leftStable) return token0 || token1;
    return token0 || token1;
}

export function buildGmgnUrl(pool, fallbackChain = 'bsc') {
    const tokenAddress = pickGmgnTokenAddress(pool);
    if (!tokenAddress) return '';
    const chain = String(pool?.chain || fallbackChain || 'bsc').trim().toLowerCase() === 'base' ? 'base' : 'bsc';
    return `https://gmgn.ai/${chain}/token/${tokenAddress}`;
}

export function getPairInitials(value) {
    return getPairLabel(value)
        .split(/[/-]/)
        .map((part) => String(part || '').trim().charAt(0).toUpperCase())
        .join('')
        .slice(0, 2) || 'LP';
}
