// Swap module shared metadata, presets, and formatting helpers.
// Adapted from webapp SwapPanel.jsx — kept minimal for miniapp.

import bnbLogo from '../image/bnb.svg';
import baseLogo from '../image/base.svg';

export const NATIVE_PSEUDO_ADDRESS = '0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee';
export const RECENT_STORAGE_KEY = 'tg_lp_bot_swap_recent_tokens_v1';
export const AUTO_QUOTE_REFRESH_MS = 8000;
export const MIN_WALLET_TOKEN_VALUE_USD = 0.1;

export const SLIPPAGE_PRESETS = ['0.5', '1.0', '2.0'];
export const AMOUNT_PRESETS = [0.25, 0.5, 0.75, 1];

export const CHAIN_META = {
    bsc: {
        key: 'bsc',
        label: 'BNB Chain',
        shortLabel: 'BSC',
        emoji: '🟡',
        nativeSymbol: 'BNB',
        nativeLogoUrl: bnbLogo,
        wrappedNativeAddress: '0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c',
        stable: {
            symbol: 'USDT',
            name: 'Tether USD',
            address: '0x55d398326f99059fF775485246999027B3197955',
            color: '#26a17b',
        },
        presets: [
            { symbol: 'USDT', name: 'Tether USD', address: '0x55d398326f99059fF775485246999027B3197955', color: '#26a17b' },
            { symbol: 'WBNB', name: 'Wrapped BNB', address: '0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c', color: '#f0b90b' },
            { symbol: 'USDC', name: 'USD Coin', address: '0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d', color: '#2775ca' },
            { symbol: 'BTCB', name: 'Bitcoin BEP20', address: '0x7130d2A12B9BCbfae4f2634d864A1Ee1Ce3Ead9c', color: '#f7931a' },
            { symbol: 'ETH', name: 'Ethereum Token', address: '0x2170Ed0880ac9A755fd29B2688956BD959F933F8', color: '#627eea' },
        ],
    },
    base: {
        key: 'base',
        label: 'Base',
        shortLabel: 'Base',
        emoji: '🔵',
        nativeSymbol: 'ETH',
        nativeLogoUrl: baseLogo,
        wrappedNativeAddress: '0x4200000000000000000000000000000000000006',
        stable: {
            symbol: 'USDC',
            name: 'USD Coin',
            address: '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913',
            color: '#2775ca',
        },
        presets: [
            { symbol: 'USDC', name: 'USD Coin', address: '0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913', color: '#2775ca' },
            { symbol: 'WETH', name: 'Wrapped Ether', address: '0x4200000000000000000000000000000000000006', color: '#627eea' },
            { symbol: 'cbBTC', name: 'Coinbase Wrapped BTC', address: '0xcbb7c0000ab88b473b1f5afd9ef808440eed33bf', color: '#f7931a' },
        ],
    },
};

export function getChainConfig(chain) {
    return CHAIN_META[String(chain || '').toLowerCase()] || CHAIN_META.bsc;
}

export function normalizeHex(address) {
    if (!address) return '';
    const s = String(address).trim().toLowerCase();
    if (!/^0x[0-9a-f]{40}$/.test(s)) return '';
    return s;
}

export function shortAddress(address, left = 6, right = 4) {
    const s = String(address || '').trim();
    if (s.length < left + right + 2) return s || '--';
    return `${s.slice(0, left)}…${s.slice(-right)}`;
}

export function dedupeTokens(tokens) {
    const seen = new Set();
    const out = [];
    for (const token of tokens || []) {
        const address = normalizeHex(token?.address);
        if (!address || seen.has(address)) continue;
        seen.add(address);
        out.push({ ...token, address });
    }
    return out;
}

export function makeCustomToken(address) {
    const normalized = normalizeHex(address);
    if (!normalized) return null;
    return {
        address: normalized,
        symbol: shortAddress(normalized, 4, 4),
        name: '自定义合约地址',
        color: '#7c8aa6',
        custom: true,
    };
}

export function getNativePresetToken(chain) {
    const chainConfig = getChainConfig(chain);
    const wrapped = (chainConfig.presets || []).find((token) =>
        String(token?.symbol || '').trim().toUpperCase().startsWith('W'),
    );
    return {
        address: NATIVE_PSEUDO_ADDRESS,
        symbol: chainConfig.nativeSymbol || 'NATIVE',
        name: chainConfig.nativeSymbol || 'NATIVE',
        color: wrapped?.color || '#7c8aa6',
        logoUrl: String(chainConfig.nativeLogoUrl || '').trim(),
        native: true,
        canSwap: true,
    };
}

export function getPresetTokens(chain) {
    return dedupeTokens([getNativePresetToken(chain), ...getChainConfig(chain).presets]);
}

export function resolveTokenMeta(address, tokens) {
    const normalized = normalizeHex(address);
    if (!normalized) return null;
    const pool = dedupeTokens(tokens);
    return pool.find((item) => item.address === normalized) || makeCustomToken(normalized);
}

export function shouldFetchTokenMetadata(token) {
    const address = normalizeHex(token?.address);
    if (!address) return false;
    if (Boolean(token?.native)) return false;
    const symbol = String(token?.symbol || '').trim();
    const name = String(token?.name || '').trim();
    const logoUrl = String(token?.logoUrl || '').trim();
    if (!logoUrl) return true;
    if (Boolean(token?.custom)) return true;
    return !symbol || !name || name === symbol;
}

export function resolveNativeLogoUrl(token, tokenMetaMap, chain) {
    const chainConfig = getChainConfig(chain);
    const wrappedAddress = normalizeHex(chainConfig?.wrappedNativeAddress);
    const wrappedLogoUrl = wrappedAddress ? String(tokenMetaMap?.[wrappedAddress]?.logoUrl || '').trim() : '';
    const currentLogoUrl = String(token?.logoUrl || '').trim();
    return currentLogoUrl || wrappedLogoUrl || String(chainConfig?.nativeLogoUrl || '').trim();
}

export function applyTokenMetadata(token, tokenMetaMap, chain) {
    if (!token) return token;
    const address = normalizeHex(token.address);
    if (!address) return token;
    if (Boolean(token.native)) {
        return {
            ...token,
            address,
            logoUrl: resolveNativeLogoUrl(token, tokenMetaMap, chain),
        };
    }
    const meta = tokenMetaMap?.[address];
    if (!meta) return token;

    const fallbackSymbol = shortAddress(address, 4, 4);
    const symbol = String(token.symbol || '').trim();
    const name = String(token.name || '').trim();
    const nextSymbol = symbol && symbol !== fallbackSymbol
        ? symbol
        : String(meta.symbol || symbol || fallbackSymbol).trim() || fallbackSymbol;
    const nextName = Boolean(token.custom) || !name || name === symbol
        ? String(meta.name || name || nextSymbol).trim() || nextSymbol
        : name;

    return {
        ...token,
        address,
        symbol: nextSymbol,
        name: nextName,
        logoUrl: String(token.logoUrl || meta.logoUrl || '').trim(),
    };
}

export function matchesToken(token, query) {
    const keyword = String(query || '').trim().toLowerCase();
    if (!keyword) return true;
    return [token?.symbol, token?.name, token?.address].some((v) =>
        String(v || '').toLowerCase().includes(keyword),
    );
}

export function loadRecentTokens() {
    try {
        const raw = localStorage.getItem(RECENT_STORAGE_KEY);
        if (!raw) return {};
        const parsed = JSON.parse(raw);
        if (parsed && typeof parsed === 'object') return parsed;
    } catch {
        /* ignore */
    }
    return {};
}

export function saveRecentTokens(map) {
    try {
        localStorage.setItem(RECENT_STORAGE_KEY, JSON.stringify(map || {}));
    } catch {
        /* ignore */
    }
}

export function pushRecentToken(chain, token) {
    if (!chain || !token?.address) return;
    const map = loadRecentTokens();
    const key = String(chain).toLowerCase();
    const list = Array.isArray(map[key]) ? map[key] : [];
    const filtered = list.filter((item) => normalizeHex(item?.address) !== token.address);
    filtered.unshift({
        address: token.address,
        symbol: token.symbol,
        name: token.name,
        logoUrl: token.logoUrl || '',
        native: Boolean(token.native),
        color: token.color || '#7c8aa6',
    });
    map[key] = filtered.slice(0, 8);
    saveRecentTokens(map);
}

export function getRecentTokensFor(chain) {
    const map = loadRecentTokens();
    const list = Array.isArray(map[String(chain || '').toLowerCase()]) ? map[String(chain).toLowerCase()] : [];
    return dedupeTokens(list);
}

export function formatTokenAmount(value, opts = {}) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return opts.zeroDash ? '--' : '0.0';
    if (num >= 1000) return num.toLocaleString('en-US', { maximumFractionDigits: 2 });
    if (num >= 1) return num.toLocaleString('en-US', { maximumFractionDigits: 6 });
    return num.toLocaleString('en-US', { maximumFractionDigits: 8 });
}

export function formatUSDCompact(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '$0';
    if (num >= 1000) return `$${num.toLocaleString('en-US', { maximumFractionDigits: 0 })}`;
    if (num >= 1) return `$${num.toLocaleString('en-US', { maximumFractionDigits: 2 })}`;
    return `$${num.toLocaleString('en-US', { maximumFractionDigits: 4 })}`;
}

export function formatGasCost(value, symbol) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    return `${formatTokenAmount(num)} ${symbol || ''}`.trim();
}

export function formatGasUSD(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    return `≈ $${num.toLocaleString('en-US', { maximumFractionDigits: num >= 1 ? 2 : 4 })}`;
}

export function formatQuoteGasCostSummary(quote, nativeSymbol) {
    if (!quote) return '--';
    const native = formatGasCost(quote?.estimated_gas_native, quote?.estimated_gas_symbol || nativeSymbol);
    const usd = formatGasUSD(quote?.estimated_gas_usd);
    if (native !== '--' && usd !== '--') return `${native} · ${usd}`;
    if (usd !== '--') return usd;
    if (native !== '--') return native;
    return '--';
}

export function formatQuoteRelativeTime(timestamp, tick = Date.now()) {
    const ts = Number(timestamp);
    if (!Number.isFinite(ts) || ts <= 0) return '';
    const diff = Math.max(0, Math.floor((tick - ts) / 1000));
    if (diff < 5) return '刚刚';
    if (diff < 60) return `${diff}s 前`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m 前`;
    return new Date(ts).toLocaleTimeString();
}

export function applyAmountPreset(balance, ratio) {
    const num = Number(balance);
    if (!Number.isFinite(num) || num <= 0) return '';
    const value = num * ratio;
    if (value <= 0) return '';
    // Avoid floating dust: keep 8 decimals max, strip trailing zeros
    const fixed = value.toFixed(value >= 1 ? 6 : 8);
    return fixed.replace(/\.?0+$/, '');
}

export function isNativeAddress(address) {
    return normalizeHex(address) === NATIVE_PSEUDO_ADDRESS;
}
