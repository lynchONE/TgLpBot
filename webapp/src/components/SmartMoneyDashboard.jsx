import React, { useState, useEffect, useCallback, useMemo, useRef, useReducer } from 'react';
import {
    Wallet, Search, Plus, ExternalLink, X, Check, Activity,
    ChevronRight, ChevronDown, ChevronLeft, Pause, Play, Trash2, Copy, Brain, Flame, Pencil, SlidersHorizontal,
    Users, Percent, DollarSign, Clock, Zap, AlertCircle, CheckCircle2, XCircle, Radar, Settings,
} from 'lucide-react';
import {
    fetchSMPools, fetchSMPoolStats, fetchSMPoolFeeHeatmap, fetchSMPositionDetail, fetchSMPositions, fetchSMWallets,
    fetchSMStats, addSMWallet, updateSMWallet, deleteSMWallet, fetchSMZombieWallets, deleteSMZombieWallets,
    fetchSMPoolLiquidityWalletCandidates, importSMPoolLiquidityWallets, streamSMPoolLiquidityWalletCandidates,
    fetchSMContracts, addSMContract, updateSMContract, deleteSMContract,
    uploadSMWalletAvatar, resolveSMAvatarAssetUrl,
    buildSMEventsWsUrl,
    fetchSMGoldenDogConfig, saveSMGoldenDogConfig, testSMGoldenDogConfig,
    fetchSMWatchActivity,
    fetchSMWatchOpenAlertConfig, saveSMWatchOpenAlertConfig, testSMWatchOpenAlertConfig,
    fetchSMAutoFollow, saveSMAutoFollowConfig, deleteSMAutoFollowConfig,
} from '../smartMoneyApi';
import { buildGmgnUrl, compactPrice, computePriceRange, formatDuration, formatUsd, shortAddress } from '../utils';
import uniswapLogo from '../img/uniswap.svg';
import pancakeLogo from '../img/pancake.svg';
import flashIcon from '../img/flash.svg';
import gmgnIcon from '../img/gmgn.svg';
import useSmartMoneyPositionPreviewMap, {
    buildRangeStatusSummary,
    getPositionSelectionKey,
    resolvePositionPreviewFeeUsd,
} from './smart-money/useSmartMoneyPositionPreviewMap';
import SmartMoneyShell from './smart-money/SmartMoneyShell';

const LazySmartMoneyAssetsPanel = React.lazy(() => import('./SmartMoneyAssetsPanel'));

const PROTOCOL_MAP = {
    pancake_v3: { version: 'V3', icon: pancakeLogo, color: '#d1884f' },
    uniswap_v3: { version: 'V3', icon: uniswapLogo, color: '#ff007a' },
    uniswap_v4: { version: 'V4', icon: uniswapLogo, color: '#ff007a' },
};

const WALLET_AVATAR_ICONS = Object.entries(
    import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
)
    .sort(([pathA], [pathB]) => pathA.localeCompare(pathB, undefined, { numeric: true }))
    .map(([, src]) => src);

const SMART_MONEY_AVATAR_ACCEPT = 'image/png,image/jpeg,image/webp';
const SMART_MONEY_AVATAR_MAX_BYTES = 5 * 1024 * 1024;

function StatCard({ label, value, color }) {
    const valueClassName = ['smd-stat-value', color].filter(Boolean).join(' ');
    return (
        <div className="smd-stat-card">
            <div className="smd-stat-label">{label}</div>
            <div className={valueClassName}>{value}</div>
        </div>
    );
}

function formatDateTimeLocalValue(date) {
    const pad = (value) => String(value).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function createDefaultTokenLiquidityRange() {
    const end = new Date();
    end.setMilliseconds(0);
    const start = new Date(end.getTime() - 24 * 60 * 60 * 1000);
    return {
        start: formatDateTimeLocalValue(start),
        end: formatDateTimeLocalValue(end),
    };
}

function tokenLiquidityLocalToISO(value) {
    const text = String(value || '').trim();
    if (!text) return '';
    const date = new Date(text);
    if (Number.isNaN(date.getTime())) return '';
    return date.toISOString();
}

function formatTokenLiquidityDateTimeRange(startValue, endValue) {
    const start = String(startValue || '').replace('T', ' ');
    const end = String(endValue || '').replace('T', ' ');
    if (!start || !end) return '请选择开始和结束时间';
    return `${start} → ${end}`;
}

const SMART_MONEY_RADAR_SCAN_TIMEOUT_MS = 50 * 1000;

function formatRadarElapsed(ms) {
    const totalSeconds = Math.max(0, Math.floor(Number(ms || 0) / 1000));
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    if (minutes <= 0) return `${seconds}s`;
    return `${minutes}m ${String(seconds).padStart(2, '0')}s`;
}

function createRadarLogEntry(text, tone = 'info') {
    const date = new Date();
    return {
        id: `${date.getTime()}-${Math.random().toString(16).slice(2)}`,
        time: date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }),
        text,
        tone,
    };
}

function parsePoolLiquidityInput(value) {
    const text = String(value || '').trim();
    if (/^0x[a-fA-F0-9]{40}$/.test(text)) {
        return { poolAddress: text.toLowerCase(), poolId: '' };
    }
    if (/^0x[a-fA-F0-9]{64}$/.test(text)) {
        return { poolAddress: '', poolId: text.toLowerCase() };
    }
    return null;
}

function upsertPoolLiquidityCandidate(list, candidate) {
    const wallet = String(candidate?.wallet_address || '').toLowerCase();
    if (!wallet) return Array.isArray(list) ? list : [];
    const next = Array.isArray(list) ? [...list] : [];
    const index = next.findIndex((item) => String(item?.wallet_address || '').toLowerCase() === wallet);
    if (index >= 0) next[index] = { ...next[index], ...candidate, wallet_address: wallet };
    else next.push({ ...candidate, wallet_address: wallet });
    return next.sort((a, b) => {
        const amountDiff = Number(b?.max_amount_usd || 0) - Number(a?.max_amount_usd || 0);
        if (amountDiff !== 0) return amountDiff;
        return String(b?.tx_time || '').localeCompare(String(a?.tx_time || ''));
    });
}

function walletAvatarIdx(addr) {
    if (!WALLET_AVATAR_ICONS.length || !addr || addr.length < 6) return 0;
    return parseInt(addr.slice(-4), 16) % WALLET_AVATAR_ICONS.length;
}

function resolveWalletAvatarSrc(address, avatarUrl) {
    const preferred = resolveSMAvatarAssetUrl(avatarUrl);
    if (preferred) return preferred;
    return WALLET_AVATAR_ICONS[walletAvatarIdx(address)] || WALLET_AVATAR_ICONS[0];
}

function shortAddr(addr) {
    if (!addr || addr.length < 10) return addr || '';
    return addr.slice(0, 6) + '...' + addr.slice(-4);
}

function tailAddr(value) {
    const raw = String(value || '').trim();
    if (!raw) return '--';
    return raw.slice(-4);
}

function normalizeWalletAddress(value) {
    const raw = String(value || '').trim();
    if (!/^0x[0-9a-fA-F]{40}$/.test(raw)) return '';
    return `0x${raw.slice(2).toLowerCase()}`;
}

function isHexAddressValue(value) {
    return /^0x[a-fA-F0-9]{40}$/.test(String(value || '').trim());
}

function getPairLabel(value) {
    const pair = String(value?.trading_pair || '').trim();
    if (pair && pair !== '/') return pair;
    const left = String(value?.token0_symbol || '').trim();
    const right = String(value?.token1_symbol || '').trim();
    if (left && right) return `${left}/${right}`;
    if (left) return left;
    if (right) return right;
    return '未识别交易对';
}

function getPoolIdentifier(value) {
    return String(value?.pool_address || '').trim();
}

function normalizePoolSelectionId(value) {
    return String(value?.pool_address || value?.pool_id || value || '').trim().toLowerCase();
}

function resolvePoolChain(value) {
    if (String(value?.chain || '').trim()) return String(value.chain).trim().toLowerCase();
    return Number(value?.chain_id) === 8453 ? 'base' : 'bsc';
}

function getPoolIdentifierLabel(value) {
    return isHexAddressValue(value) ? 'Pool Address' : 'Pool ID';
}

function getPairInitials(value) {
    const pair = getPairLabel(value);
    return pair
        .split(/[/-]/)
        .map((part) => String(part || '').trim().charAt(0).toUpperCase())
        .join('')
        .slice(0, 2) || 'LP';
}

function formatFeeTier(fee) {
    if (!fee) return '';
    return `${(Number(fee) / 10000).toFixed(4)}%`;
}

function formatUSDCompact(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    const abs = Math.abs(num);
    if (abs >= 1000000) return `$${(num / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
    if (abs >= 1000) return `$${(num / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
    if (abs >= 100) return `$${num.toFixed(0)}`;
    if (abs >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
    return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatWalletBalance(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    if (num === 0) return '$0';
    return formatUSDCompact(num);
}

function parseOptionalNumber(value) {
    const text = String(value ?? '').replace(/,/g, '').trim();
    if (!text) return null;
    const match = text.match(/-?\d+(\.\d+)?/);
    if (!match) return null;
    const num = Number(match[0]);
    if (!Number.isFinite(num)) return null;
    return Math.max(0, num);
}

function formatOptionalNumber(value) {
    return Number.isFinite(value) ? String(value) : '';
}

function parseMetricNumber(value) {
    if (value === null || value === undefined || value === '') return NaN;
    const raw = typeof value === 'string' ? value.replace(/,/g, '').trim() : value;
    const direct = Number(raw);
    if (Number.isFinite(direct)) return direct;
    const match = String(value).match(/-?\d+(\.\d+)?/);
    if (!match) return NaN;
    const parsed = Number(match[0]);
    return Number.isFinite(parsed) ? parsed : NaN;
}

function resolveSmartMoneyPoolMarketCapDisplay(pool) {
    const candidates = [
        pool?.fdv_usd,
        pool?.current_token_fdv_usd,
        pool?.market_cap_usd,
    ];
    for (const candidate of candidates) {
        const value = parseMetricNumber(candidate);
        if (Number.isFinite(value) && value > 0) return value;
    }
    return NaN;
}

function resolveSmartMoneyPoolMarketCapLabel(pool) {
    const fdv = parseMetricNumber(pool?.fdv_usd);
    if (Number.isFinite(fdv) && fdv > 0) return 'FDV';
    const legacyFDV = parseMetricNumber(pool?.current_token_fdv_usd);
    if (Number.isFinite(legacyFDV) && legacyFDV > 0) return 'FDV';
    return '市值';
}

const SMART_MONEY_POOL_FILTER_STORAGE_KEY = 'tglp_smart_money_pool_filter_v1';
const EMPTY_SMART_MONEY_POOL_FILTER = { minSmartMoneyUsd: null, maxFeeRate: null, minMarketCapUsd: null };
const SMART_MONEY_POOL_SOURCE_TABS = [
    { key: 'all', label: '全部', source: '' },
    { key: 'manual', label: '手动添加', source: 'manual' },
    { key: 'contract', label: '合约发现', source: 'contract_interaction' },
];
const SMART_MONEY_POOL_SOURCE_BY_KEY = Object.fromEntries(
    SMART_MONEY_POOL_SOURCE_TABS.map((item) => [item.key, item.source])
);

function normalizeStoredSmartMoneyPoolFilter(value) {
    if (!value || typeof value !== 'object') {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
    return {
        minSmartMoneyUsd: Number.isFinite(Number(value.minSmartMoneyUsd)) ? Number(value.minSmartMoneyUsd) : null,
        maxFeeRate: Number.isFinite(Number(value.maxFeeRate)) ? Number(value.maxFeeRate) : null,
        minMarketCapUsd: Number.isFinite(Number(value.minMarketCapUsd)) ? Number(value.minMarketCapUsd) : null,
    };
}

function readStoredSmartMoneyPoolFilter() {
    if (typeof window === 'undefined' || !window.localStorage) {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
    try {
        const raw = window.localStorage.getItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY);
        if (!raw) return { ...EMPTY_SMART_MONEY_POOL_FILTER };
        return normalizeStoredSmartMoneyPoolFilter(JSON.parse(raw));
    } catch {
        return { ...EMPTY_SMART_MONEY_POOL_FILTER };
    }
}

function writeStoredSmartMoneyPoolFilter(value) {
    if (typeof window === 'undefined' || !window.localStorage) {
        return;
    }
    try {
        const normalized = normalizeStoredSmartMoneyPoolFilter(value);
        const isEmpty = !Number.isFinite(normalized.minSmartMoneyUsd)
            && !Number.isFinite(normalized.maxFeeRate)
            && !Number.isFinite(normalized.minMarketCapUsd);
        if (isEmpty) {
            window.localStorage.removeItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY);
            return;
        }
        window.localStorage.setItem(SMART_MONEY_POOL_FILTER_STORAGE_KEY, JSON.stringify(normalized));
    } catch {
        // ignore storage failures
    }
}

function formatRangePercent(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 100) return `±${Math.round(num)}%`;
    if (num >= 10) return `±${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `±${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatRangePercentPlain(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function formatWatchActivityAction(value) {
    const eventType = String(value || '').trim();
    if (eventType === 'add') return '加 LP';
    if (eventType === 'remove') return '撤 LP';
    return eventType || 'LP 操作';
}

function getWatchActivityActionClass(value) {
    return String(value || '').trim() === 'remove' ? 'danger' : 'success';
}

function normalizeWatchWalletItems(resp, fallbackWallets = []) {
    const items = Array.isArray(resp?.items) ? resp.items : [];
    const seen = new Set();
    const normalized = [];

    items.forEach((item) => {
        const address = normalizeWalletAddress(item?.wallet_address);
        if (!address || seen.has(address)) return;
        seen.add(address);
        normalized.push({ ...item, wallet_address: address });
    });

    fallbackWallets.forEach((value) => {
        const address = normalizeWalletAddress(value);
        if (!address || seen.has(address)) return;
        seen.add(address);
        normalized.push({ wallet_address: address, wallet_color: '#7F77DD' });
    });

    return normalized;
}

function getDisplayedPriceRangeState(position, currentPrice) {
    const current = Number(currentPrice);
    const lower = Number(position?.price_lower);
    const upper = Number(position?.price_upper);
    if (!Number.isFinite(current) || !Number.isFinite(lower) || !Number.isFinite(upper)) return null;
    const rangeMin = Math.min(lower, upper);
    const rangeMax = Math.max(lower, upper);
    if (current >= rangeMin && current <= rangeMax) {
        return { inRange: true, outOfRange: null };
    }
    if (current > rangeMax) {
        const base = Math.abs(rangeMax) > 0 ? Math.abs(rangeMax) : 1;
        return {
            inRange: false,
            outOfRange: { direction: 'above', pct: ((current - rangeMax) / base) * 100 },
        };
    }
    const base = Math.abs(rangeMin) > 0 ? Math.abs(rangeMin) : 1;
    return {
        inRange: false,
        outOfRange: { direction: 'below', pct: ((rangeMin - current) / base) * 100 },
    };
}

function resolvePositionRangeStatus(position, preview, currentPrice) {
    return preview?.rangeStatus || buildRangeStatusSummary(getDisplayedPriceRangeState(position, currentPrice));
}

function getPositionAmountSummary(position, preview) {
    const liveTotal = Number(preview?.currentValueUsd);
    const netInvested = Number(preview?.netInvestedUsd ?? position?.position_amount_usd);
    if (Number.isFinite(liveTotal) && liveTotal > 0) {
        return {
            primaryLabel: '仓位总计',
            primaryValue: liveTotal,
            secondaryLabel: Number.isFinite(netInvested) && netInvested > 0 ? '净投入' : '',
            secondaryValue: Number.isFinite(netInvested) && netInvested > 0 ? netInvested : null,
        };
    }
    return {
        primaryLabel: '净投入',
        primaryValue: netInvested,
        secondaryLabel: '',
        secondaryValue: null,
    };
}

const POOL_CARD_RANGE_LIMIT = 5;
const POOL_LIST_PAGE_SIZE = 10;
const POSITION_LIST_PAGE_SIZE = 6;
const WALLET_LIST_PAGE_SIZE = 10;
const POOL_HEATMAP_PAGE_SIZE = 10;
const WATCH_ACTIVITY_PAGE_SIZE = 12;
const POOL_HEATMAP_WINDOWS = [
    { key: '30s', label: '30s' },
    { key: '1m', label: '1min' },
    { key: '5m', label: '5min' },
    { key: '1h', label: '1h' },
];
const POOL_HEATMAP_SORTS = [
    { key: 'rate', label: '速率' },
    { key: 'fee', label: '手续费' },
];

function getPoolCardRangeGroups(pool) {
    const groups = Array.isArray(pool?.range_groups)
        ? pool.range_groups.filter((item) => Number(item?.range_percent) > 0)
        : [];
    return groups;
}

function PoolCardRangeSummary({ pool }) {
    const [expanded, setExpanded] = useState(false);
    const groups = getPoolCardRangeGroups(pool);
    const visibleGroups = expanded ? groups : groups.slice(0, POOL_CARD_RANGE_LIMIT);
    const hiddenCount = Math.max(0, groups.length - visibleGroups.length);
    if (!groups.length) {
        return <div className="smd-pool-card-range-empty">暂无聪明钱区间聚合</div>;
    }
    return (
        <div className="smd-range-summary-stack">
            {visibleGroups.map((group, index) => (
                <div key={`${pool?.pool_address || 'pool'}:${Number(group?.range_percent || 0)}:${index}`} className="smd-range-summary-line">
                    <span className="smd-range-summary-pct">{formatRangePercentPlain(group.range_percent)}</span>
                    {Math.max(0, Number(group?.position_count) || 0) > 1 ? (
                        <span className="smd-range-summary-badge">{Number(group.position_count)}个</span>
                    ) : null}
                    <span className="smd-range-summary-amount">{formatUSDCompact(group.total_amount_usd)}</span>
                </div>
            ))}
            {groups.length > POOL_CARD_RANGE_LIMIT ? (
                <button
                    type="button"
                    className="smd-range-summary-toggle"
                    onClick={(event) => {
                        event.stopPropagation();
                        setExpanded((prev) => !prev);
                    }}
                >
                    {expanded ? '收起区间' : `展开全部区间${hiddenCount > 0 ? ` (+${hiddenCount})` : ''}`}
                </button>
            ) : null}
        </div>
    );
}

function getRefreshIntervalMs(refreshInterval) {
    const seconds = Number(refreshInterval);
    if (!Number.isFinite(seconds) || seconds <= 0) return 10000;
    return Math.max(Math.round(seconds), 2) * 1000;
}

function relativeTime(dateStr) {
    if (!dateStr) return '';
    const d = new Date(dateStr);
    const diff = (Date.now() - d.getTime()) / 1000;
    if (diff < 60) return '刚刚';
    if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
    return `${Math.floor(diff / 86400)}天前`;
}

function formatHeatmapUSD(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    if (Math.abs(num) < 0.005) return '$0';
    return formatUSDCompact(num);
}

function formatHeatmapRate(value) {
    const num = Number(value);
    if (!Number.isFinite(num)) return '--';
    if (num >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
    if (num >= 1) return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
    return `$${num.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function heatmapWindowLabel(value) {
    return POOL_HEATMAP_WINDOWS.find((item) => item.key === value)?.label || '1min';
}

function formatHeatmapAge(seconds) {
    const value = Number(seconds);
    if (!Number.isFinite(value) || value <= 0) return '--';
    if (value < 60) return `${Math.max(1, Math.round(value))}秒`;
    if (value < 3600) return `${Math.round(value / 60)}分钟`;
    if (value < 86400) return `${Math.round(value / 3600)}小时`;
    return `${Math.round(value / 86400)}天`;
}

function heatmapSampleText(row) {
    const status = String(row?.sample_status || '').trim();
    if (status === 'ok') return '样本完整';
    if (status === 'partial') return '部分样本';
    return '样本不足';
}

function walletSourceLabel(source) {
    const value = String(source || '').trim();
    if (value === 'manual') return '手动添加';
    if (value === 'contract_interaction') return '合约发现';
    if (value === 'token_liquidity_indexer') return '雷达发现';
    if (value === 'pool_liquidity_radar') return '池子雷达';
    return value || '未标记来源';
}

function walletSourceBadgeClass(source) {
    const value = String(source || '').trim();
    if (value === 'manual') return 'source-manual';
    if (value === 'token_liquidity_indexer' || value === 'pool_liquidity_radar') return 'source-token-liquidity';
    return 'source-contract';
}

function walletSourceContractLabel(value) {
    const address = normalizeWalletAddress(value);
    if (address) return `来源合约 ${shortAddr(address)}`;
    const poolId = String(value || '').trim();
    if (/^0x[a-fA-F0-9]{64}$/.test(poolId)) return `来源 poolId ${shortAddr(poolId)}`;
    return '';
}

function CopyBtn({ text }) {
    const [copied, setCopied] = useState(false);
    return (
        <button className="smd-copy-btn" onClick={e => {
            e.stopPropagation();
            navigator.clipboard.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        }}>
            {copied ? <Check size={12} /> : <Copy size={12} />}
        </button>
    );
}

function CopyTinyBtn({ text }) {
    const [copied, setCopied] = useState(false);
    if (!text) return null;
    return (
        <button className="smd-copy-btn smd-copy-btn--tiny" onClick={e => {
            e.stopPropagation();
            navigator.clipboard.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 1200);
        }} title="复制">
            {copied ? <Check size={10} /> : <Copy size={10} />}
        </button>
    );
}

function WalletAvatar({ address, color, size = 18, avatarUrl }) {
    const fallbackSrc = WALLET_AVATAR_ICONS[walletAvatarIdx(address)] || WALLET_AVATAR_ICONS[0];
    const preferredSrc = resolveWalletAvatarSrc(address, avatarUrl);
    const [iconSrc, setIconSrc] = useState(preferredSrc);

    useEffect(() => {
        setIconSrc(preferredSrc);
    }, [preferredSrc]);

    return (
        <span className="smd-wallet-avatar" style={{ borderColor: color, width: size, height: size }}>
            <img
                src={iconSrc}
                alt=""
                className="smd-wallet-avatar-icon"
                onError={() => {
                    if (iconSrc !== fallbackSrc) {
                        setIconSrc(fallbackSrc);
                    }
                }}
            />
        </span>
    );
}

function CompactIdentifier({ value, label = 'ID' }) {
    if (!value) return null;
    return (
        <span className="smd-inline-id">
            <span className="smd-inline-id-label">{label}</span>
            <span className="smd-inline-id-value">{tailAddr(value)}</span>
            <CopyTinyBtn text={value} />
        </span>
    );
}

function WalletIdentity({ address, color, label, avatarUrl, source, sourceContract, size = 16, onClick, showCopy = false, showSource = false }) {
    const sourceText = walletSourceLabel(source);
    const sourceContractText = walletSourceContractLabel(sourceContract);
    const content = (
        <>
            <WalletAvatar address={address} color={color} avatarUrl={avatarUrl} size={size} />
            <span className="smd-wallet-info-main">
                <span className="smd-wallet-info-name">{label && label !== address ? label : tailAddr(address)}</span>
                {showSource ? (
                    <span className={`smd-badge ${walletSourceBadgeClass(source)}`} title={sourceContractText || sourceText}>
                        {sourceText}
                    </span>
                ) : null}
            </span>
            {showCopy ? <CopyTinyBtn text={address} /> : null}
        </>
    );

    if (typeof onClick === 'function') {
        return (
            <button className="smd-wallet-info smd-wallet-btn" onClick={(event) => {
                event.stopPropagation();
                onClick(event);
            }}>
                {content}
            </button>
        );
    }

    return <div className="smd-wallet-info">{content}</div>;
}

function Badge({ children, cls = '', ...rest }) {
    return <span className={`smd-badge ${cls}`} {...rest}>{children}</span>;
}

function ProtocolBadge({ protocol }) {
    const info = PROTOCOL_MAP[protocol];
    if (!info) return <Badge cls="proto">{protocol}</Badge>;
    return (
        <span className="smd-badge proto smd-proto-icon" style={{ borderColor: info.color + '40' }}>
            <img src={info.icon} alt="" className="smd-proto-img" />
            {info.version}
        </span>
    );
}

function PairAvatar({ item, size = 'md' }) {
    const displayTokenLogoUrl = String(item?.display_token_logo_url || '').trim();
    const displayTokenSymbol = String(item?.display_token_symbol || '').trim();
    const avatarLabel = (displayTokenSymbol || getPairInitials(item) || 'LP').slice(0, 4).toUpperCase();
    const avatarSrc = displayTokenLogoUrl;

    return (
        <span className={`pool-avatar smd-pair-avatar smd-pair-avatar--${size}`}>
            {avatarSrc ? (
                <>
                    <img
                        src={avatarSrc}
                        alt=""
                        onError={(e) => {
                            e.currentTarget.style.display = 'none';
                            const fallback = e.currentTarget.parentElement?.querySelector('.pool-avatar-fallback');
                            if (fallback) fallback.style.display = 'flex';
                        }}
                    />
                    <span className="pool-avatar-fallback" style={{ display: 'none' }}>{avatarLabel}</span>
                </>
            ) : (
                <span className="pool-avatar-fallback">{avatarLabel}</span>
            )}
        </span>
    );
}

function PriceRangeChart({ positions, currentPrice }) {
    if (!positions?.length) return null;
    const valid = positions.filter(p => p.price_lower && p.price_upper);
    if (!valid.length) return null;

    const allPrices = valid.flatMap(p => [parseFloat(p.price_lower), parseFloat(p.price_upper)]);
    let minP = Math.min(...allPrices), maxP = Math.max(...allPrices);
    const pad = (maxP - minP) * 0.1 || 1;
    minP = Math.max(0, minP - pad);
    maxP += pad;
    const pct = p => Math.max(0, Math.min(100, ((p - minP) / (maxP - minP)) * 100));
    const parsedCurrentPrice = Number.parseFloat(currentPrice);
    const curPct = Number.isFinite(parsedCurrentPrice) ? pct(parsedCurrentPrice) : null;
    const walletIdx = {};
    const currentLabelStyle = curPct === null
        ? null
        : curPct >= 92
            ? { right: 0 }
            : curPct <= 8
                ? { left: 0 }
                : { left: `${curPct}%`, transform: 'translateX(-50%)' };

    return (
        <div className="smd-price-chart">
            <div className="smd-price-chart-area" style={{ minHeight: valid.length * 14 + 50 }}>
                {curPct !== null && (
                    <div className="smd-price-cur" style={{ left: `${curPct}%` }}>
                        <div className="smd-price-cur-label" style={currentLabelStyle || undefined}>{currentPrice}</div>
                    </div>
                )}
                {valid.map((p, i) => {
                    const l = pct(parseFloat(p.price_lower)), r = pct(parseFloat(p.price_upper));
                    const w = Math.max(r - l, 0.5);
                    const color = p.wallet_color || '#7F77DD';
                    walletIdx[p.wallet_address] = (walletIdx[p.wallet_address] || 0) + 1;
                    const idx = walletIdx[p.wallet_address];
                    const op = idx === 1 ? 0.85 : idx === 2 ? 0.6 : 0.4;
                    const inRange = currentPrice && parseFloat(p.price_lower) <= parseFloat(currentPrice) && parseFloat(currentPrice) <= parseFloat(p.price_upper);
                    return (
                        <div key={p.id || i} className="smd-price-bar" style={{
                            left: `${l}%`, width: `${w}%`, top: i * 14 + 20,
                            backgroundColor: color, opacity: inRange ? op : 0.35,
                        }} title={`${shortAddr(p.wallet_address)}: ${p.price_lower} - ${p.price_upper}`} />
                    );
                })}
                <div className="smd-price-axis">
                    {Array.from({ length: 5 }, (_, i) => (
                        <span key={i}>{(minP + ((maxP - minP) / 4) * i).toPrecision(4)}</span>
                    ))}
                </div>
            </div>
            <div className="smd-legend">
                {Object.entries(valid.reduce((a, p) => {
                    if (!a[p.wallet_address]) {
                        a[p.wallet_address] = {
                            color: p.wallet_color,
                            label: p.wallet_label,
                            source: p.wallet_source,
                            sourceContract: p.wallet_source_contract,
                        };
                    }
                    return a;
                }, {})).map(([addr, { color, label, source, sourceContract }]) => (
                    <span key={addr}>
                        <span className="smd-legend-dot" style={{ backgroundColor: color }} />
                        {label || shortAddr(addr)}
                        <span className={`smd-badge ${walletSourceBadgeClass(source)}`} title={walletSourceContractLabel(sourceContract) || walletSourceLabel(source)}>
                            {walletSourceLabel(source)}
                        </span>
                    </span>
                ))}
            </div>
        </div>
    );
}

function ConfirmDialog({ open, title, description, confirmLabel = '确认', busy = false, onConfirm, onCancel }) {
    if (!open) return null;

    return (
        <div className="smd-modal-overlay" onClick={busy ? undefined : onCancel}>
            <div className="smd-modal smd-confirm-modal" onClick={(e) => e.stopPropagation()}>
                <div className="smd-modal-header">
                    <h3 className="smd-modal-title">{title}</h3>
                    <button type="button" onClick={onCancel} disabled={busy} className="smd-modal-close">
                        <X size={18} />
                    </button>
                </div>
                <div className="smd-confirm-copy">{description}</div>
                <div className="smd-modal-actions">
                    <button type="button" onClick={onCancel} disabled={busy} className="smd-modal-cancel">取消</button>
                    <button type="button" onClick={onConfirm} disabled={busy} className="smd-modal-submit">
                        {busy ? '处理中...' : confirmLabel}
                    </button>
                </div>
            </div>
        </div>
    );
}

function formatZombieLastActive(value) {
    const raw = String(value || '').trim();
    if (!raw) return '从未活动';
    return relativeTime(raw) || raw.slice(0, 10);
}

function zombieHistoryCount(item) {
    return Number(item?.total_event_count || 0)
        + Number(item?.position_count || 0)
        + Number(item?.active_position_count || 0)
        + Number(item?.snapshot_count || 0)
        + Number(item?.transfer_event_count || 0)
        + Number(item?.daily_stat_count || 0)
        + Number(item?.live_state_count || 0);
}

function ZombieWalletModal({ open, candidates, selectedMap, busy, onToggle, onToggleAll, onClose, onDelete }) {
    if (!open) return null;
    const list = Array.isArray(candidates) ? candidates : [];
    const selectedCount = list.filter((item) => selectedMap[`${item.address}:${item.chain_id}`]).length;
    const allSelected = list.length > 0 && selectedCount === list.length;

    return (
        <div className="smd-modal-overlay" onClick={busy ? undefined : onClose}>
            <div className="smd-modal smd-zombie-modal" onClick={(e) => e.stopPropagation()}>
                <div className="smd-modal-header">
                    <h3 className="smd-modal-title">僵尸钱包</h3>
                    <button type="button" onClick={onClose} disabled={busy} className="smd-modal-close">
                        <X size={18} />
                    </button>
                </div>
                <div className="smd-confirm-copy">
                    最近 30 天没有 LP 开仓或撤仓的钱包。确认删除后会同时删除该钱包的聪明钱历史数据。
                </div>
                {list.length > 0 ? (
                    <>
                        <div className="smd-zombie-toolbar">
                            <button type="button" className="smd-modal-cancel" disabled={busy} onClick={() => onToggleAll(!allSelected)}>
                                {allSelected ? '取消全选' : '全选'}
                            </button>
                            <span>{selectedCount} / {list.length} 已选择</span>
                        </div>
                        <div className="smd-zombie-list">
                            {list.map((item) => {
                                const key = `${item.address}:${item.chain_id}`;
                                const checked = Boolean(selectedMap[key]);
                                return (
                                    <label key={key} className={`smd-zombie-row${checked ? ' selected' : ''}`}>
                                        <input
                                            type="checkbox"
                                            checked={checked}
                                            disabled={busy}
                                            onChange={() => onToggle(key)}
                                        />
                                        <WalletAvatar address={item.address} avatarUrl={item.avatar_url} size={26} />
                                        <div className="smd-zombie-main">
                                            <div className="smd-zombie-title">{item.label || shortAddr(item.address)}</div>
                                            <div className="smd-zombie-sub">
                                                {shortAddr(item.address)} / {walletSourceLabel(item.source)} / 最后活动 {formatZombieLastActive(item.last_active_at)}
                                            </div>
                                        </div>
                                        <div className="smd-zombie-meta">
                                            <strong>{zombieHistoryCount(item)}</strong>
                                            <span>历史项</span>
                                        </div>
                                    </label>
                                );
                            })}
                        </div>
                    </>
                ) : (
                    <div className="smd-empty" style={{ marginTop: 12 }}>没有找到僵尸钱包</div>
                )}
                <div className="smd-modal-actions">
                    <button type="button" onClick={onClose} disabled={busy} className="smd-modal-cancel">关闭</button>
                    <button type="button" onClick={onDelete} disabled={busy || selectedCount === 0} className="smd-modal-submit danger">
                        {busy ? '删除中...' : `删除 ${selectedCount} 个`}
                    </button>
                </div>
            </div>
        </div>
    );
}

function TokenLiquidityImportModal({ open, apiBaseUrl, onClose, onImported }) {
    const [poolInput, setPoolInput] = useState('');
    const [minAmountUsd, setMinAmountUsd] = useState('500');
    const [timeRange, setTimeRange] = useState(() => createDefaultTokenLiquidityRange());
    const [limit, setLimit] = useState('50');
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [data, setData] = useState(null);
    const [selected, setSelected] = useState({});
    const [importResult, setImportResult] = useState(null);
    const [scanStep, setScanStep] = useState('');
    const [scanStartedAt, setScanStartedAt] = useState(null);
    const [scanElapsedMs, setScanElapsedMs] = useState(0);
    const [scanTarget, setScanTarget] = useState(null);
    const [scanLogs, setScanLogs] = useState([]);
    const scanAbortRef = useRef(null);

    const appendScanLog = useCallback((text, tone = 'info') => {
        setScanLogs((prev) => [...prev.slice(-7), createRadarLogEntry(text, tone)]);
    }, []);

    useEffect(() => {
        if (!open) return;
        setError('');
        setImportResult(null);
        setScanStep('');
        setScanStartedAt(null);
        setScanElapsedMs(0);
        setScanTarget(null);
        setScanLogs([]);
        setTimeRange(createDefaultTokenLiquidityRange());
        scanAbortRef.current?.abort();
        scanAbortRef.current = null;
    }, [open]);

    useEffect(() => {
        if (!open || (!loading && !saving) || !scanStartedAt) return undefined;
        const tick = () => setScanElapsedMs(Date.now() - scanStartedAt);
        tick();
        const timer = window.setInterval(tick, 1000);
        return () => window.clearInterval(timer);
    }, [open, loading, saving, scanStartedAt]);

    const candidates = Array.isArray(data?.candidates) ? data.candidates : [];
    const selectedWallets = candidates
        .filter((item) => selected[item.wallet_address])
        .map((item) => item.wallet_address);
    const allSelected = candidates.length > 0 && selectedWallets.length === candidates.length;
    const currentRangeLabel = formatTokenLiquidityDateTimeRange(timeRange.start, timeRange.end);
    const scanProgressPct = (loading || saving)
        ? Math.min(96, Math.max(8, (scanElapsedMs / SMART_MONEY_RADAR_SCAN_TIMEOUT_MS) * 96))
        : (scanStep ? 100 : 0);
    const scanTargetText = scanTarget ? `${scanTarget.kind} ${shortAddr(scanTarget.value)}` : '';
    const scanStatusText = loading ? '运行中' : saving ? '导入中' : error ? '失败' : data ? '完成' : '就绪';

    const stopScan = useCallback(() => {
        scanAbortRef.current?.abort();
        scanAbortRef.current = null;
        setLoading(false);
        setScanStep('扫描已停止');
        appendScanLog('已停止当前扫描。', 'warn');
    }, [appendScanLog]);

    const closeModal = useCallback(() => {
        if (loading) {
            stopScan();
        }
        onClose?.();
    }, [loading, onClose, stopScan]);

    if (!open) return null;

    const preview = async () => {
        const poolTarget = parsePoolLiquidityInput(poolInput);
        if (!poolTarget) {
            setError('请输入有效的 V3 池子合约地址或 V4 poolId');
            setScanStep('扫描失败');
            setScanStartedAt(null);
            setScanElapsedMs(0);
            setScanTarget(null);
            setScanLogs([createRadarLogEntry('参数校验失败：池子地址或 poolId 格式不正确。', 'error')]);
            return;
        }
        setScanStep('校验扫描参数');
        const startTime = tokenLiquidityLocalToISO(timeRange.start);
        const endTime = tokenLiquidityLocalToISO(timeRange.end);
        if (!startTime || !endTime) {
            setError('请选择有效的开始和结束时间');
            setScanStep('扫描失败');
            setScanStartedAt(null);
            setScanElapsedMs(0);
            setScanTarget(null);
            setScanLogs([createRadarLogEntry('参数校验失败：开始时间和结束时间必须完整。', 'error')]);
            return;
        }
        if (new Date(endTime).getTime() <= new Date(startTime).getTime()) {
            setError('结束时间必须晚于开始时间');
            setScanStep('扫描失败');
            setScanStartedAt(null);
            setScanElapsedMs(0);
            setScanTarget(null);
            setScanLogs([createRadarLogEntry('参数校验失败：结束时间必须晚于开始时间。', 'error')]);
            return;
        }
        const startedAt = Date.now();
        const rangeHours = Math.max(1, Math.ceil((new Date(endTime).getTime() - new Date(startTime).getTime()) / (60 * 60 * 1000)));
        const targetValue = poolTarget.poolAddress || poolTarget.poolId;
        const targetKind = poolTarget.poolAddress ? 'V3 Pool' : 'V4 PoolId';
        setLoading(true);
        setError('');
        setImportResult(null);
        setData({ candidates: [], excluded_count: 0, warnings: [] });
        setSelected({});
        setScanStartedAt(startedAt);
        setScanElapsedMs(0);
        setScanTarget({
            kind: targetKind,
            value: targetValue,
            range: currentRangeLabel,
            rangeHours,
            minAmountUsd: Number(minAmountUsd),
            limit: Number(limit),
        });
        setScanLogs([
            createRadarLogEntry(`参数已校验：${targetKind} ${shortAddr(targetValue)}，约 ${rangeHours} 小时时间窗。`),
            createRadarLogEntry(`提交后端流式 RPC 扫描，最低金额 ${formatUSDCompact(Number(minAmountUsd))}，最多返回 ${Number(limit)} 个候选。`),
        ]);
        scanAbortRef.current?.abort();
        const controller = new AbortController();
        scanAbortRef.current = controller;
        const applyBatchResponse = (resp) => {
            setScanStep('整理候选钱包');
            const list = Array.isArray(resp?.candidates) ? resp.candidates : [];
            const excludedCount = Number(resp?.excluded_count || 0);
            const isPartial = Boolean(resp?.partial);
            appendScanLog(
                `后端返回：候选 ${list.length} 个，排除 ${excludedCount} 条事件。`,
                isPartial ? 'warn' : (list.length > 0 ? 'success' : 'info'),
            );
            if (isPartial) {
                appendScanLog('本次只返回部分扫描结果，列表可先查看/勾选；完整结果请缩小时间范围后再扫。', 'warn');
            }
            if (Array.isArray(resp?.warnings) && resp.warnings.length > 0) {
                appendScanLog(`扫描提示：${resp.warnings.slice(0, 2).join('；')}`, 'warn');
            }
            const nextSelected = {};
            list.forEach((item) => {
                if (!item.already_monitored) nextSelected[item.wallet_address] = true;
            });
            setData(resp);
            setSelected(nextSelected);
            setScanStep(isPartial
                ? `已返回部分结果，找到 ${list.length} 个候选钱包`
                : (list.length > 0 ? `扫描完成，找到 ${list.length} 个候选钱包` : '扫描完成，未找到符合条件的钱包'));
        };
        try {
            setScanStep('建立流式扫描连接');
            let lastStage = '';
            await streamSMPoolLiquidityWalletCandidates({
                apiBaseUrl,
                chain: 'bsc',
                ...poolTarget,
                minAmountUsd: Number(minAmountUsd),
                startTime,
                endTime,
                limit: Number(limit),
                signal: controller.signal,
                onStage: (event) => {
                    const stageText = event?.message || event?.stage || '扫描中';
                    if (event?.current_block && event?.to_block) {
                        setScanStep(`${stageText}：${event.current_block}/${event.to_block}`);
                    } else {
                        setScanStep(stageText);
                    }
                    if (event?.stage && event.stage !== lastStage) {
                        lastStage = event.stage;
                        appendScanLog(stageText);
                    }
                },
                onCandidate: (event) => {
                    const candidate = event?.candidate;
                    const wallet = String(candidate?.wallet_address || '').toLowerCase();
                    if (!wallet) return;
                    setData((prev) => ({
                        ...(prev || {}),
                        candidates: upsertPoolLiquidityCandidate(prev?.candidates, candidate),
                        excluded_count: Number(event?.excluded_count || prev?.excluded_count || 0),
                    }));
                    setSelected((prev) => {
                        if (candidate.already_monitored || Object.prototype.hasOwnProperty.call(prev, wallet)) return prev;
                        return { ...prev, [wallet]: true };
                    });
                    setScanStep(`实时发现 ${Number(event?.candidate_count || 1)} 个候选钱包`);
                    appendScanLog(`发现候选：${shortAddr(wallet)}，最大加池 ${formatUSDCompact(Number(candidate.max_amount_usd || 0))}。`, 'success');
                },
                onWarning: (event) => {
                    appendScanLog(`扫描提示：${event?.message || '扫描过程中出现提示'}`, 'warn');
                },
                onSummary: (event) => {
                    const resp = event?.response;
                    if (resp) {
                        setData(resp);
                    } else {
                        setData((prev) => ({
                            ...(prev || {}),
                            excluded_count: Number(event?.excluded_count || prev?.excluded_count || 0),
                            partial: Boolean(event?.partial),
                            warnings: Array.isArray(event?.warnings) ? event.warnings : (prev?.warnings || []),
                        }));
                    }
                    const count = Number(event?.candidate_count || resp?.candidates?.length || 0);
                    setScanStep(event?.partial ? `已返回部分结果，找到 ${count} 个候选钱包` : `扫描完成，找到 ${count} 个候选钱包`);
                },
                onDone: (event) => {
                    appendScanLog(event?.partial ? '流式扫描结束：当前为部分结果。' : '流式扫描完成。', event?.partial ? 'warn' : 'success');
                },
                onError: (event) => {
                    appendScanLog(`流式扫描失败：${event?.message || '未知错误'}`, 'error');
                },
            });
        } catch (err) {
            if (controller.signal.aborted) {
                return;
            }
            const message = String(err?.message || err || '预览失败');
            if (/不支持流式扫描/.test(message)) {
                appendScanLog('当前环境不支持流式扫描，降级为普通扫描。', 'warn');
                try {
                    const resp = await fetchSMPoolLiquidityWalletCandidates({
                        apiBaseUrl,
                        chain: 'bsc',
                        ...poolTarget,
                        minAmountUsd: Number(minAmountUsd),
                        startTime,
                        endTime,
                        limit: Number(limit),
                    });
                    applyBatchResponse(resp);
                } catch (fallbackErr) {
                    const fallbackMessage = String(fallbackErr?.message || fallbackErr || '预览失败');
                    setError(fallbackMessage);
                    setScanStep('扫描失败');
                    appendScanLog(`普通扫描失败：${fallbackMessage}`, 'error');
                }
                return;
            }
            setError(message);
            setScanStep('扫描失败');
            appendScanLog(`扫描失败：${message}`, 'error');
        } finally {
            if (scanAbortRef.current === controller) scanAbortRef.current = null;
            setScanElapsedMs(Date.now() - startedAt);
            setLoading(false);
        }
    };

    const importSelected = async () => {
        if (selectedWallets.length === 0) {
            setError('请选择至少一个钱包');
            return;
        }
        const poolTarget = parsePoolLiquidityInput(poolInput);
        if (!poolTarget) {
            setError('请输入有效的 V3 池子合约地址或 V4 poolId');
            return;
        }
        setSaving(true);
        setError('');
        setScanStep('导入选中的候选钱包');
        const startedAt = Date.now();
        setScanStartedAt(startedAt);
        setScanElapsedMs(0);
        appendScanLog(`开始导入 ${selectedWallets.length} 个候选钱包。`);
        try {
            const resp = await importSMPoolLiquidityWallets({
                apiBaseUrl,
                chain: 'bsc',
                ...poolTarget,
                wallets: selectedWallets,
                labelPrefix: '雷达',
            });
            setImportResult(resp);
            setScanStep('导入完成');
            appendScanLog(`导入完成：新增 ${resp.created || 0}，恢复 ${resp.reactivated || 0}，跳过 ${resp.skipped_existing || 0}。`, 'success');
            await onImported?.();
        } catch (err) {
            const message = String(err?.message || err || '导入失败');
            setError(message);
            setScanStep('导入失败');
            appendScanLog(`导入失败：${message}`, 'error');
        } finally {
            setScanElapsedMs(Date.now() - startedAt);
            setSaving(false);
        }
    };

    return (
        <div className="smd-modal-overlay" onClick={saving ? undefined : closeModal}>
            <div className="smd-modal smd-token-liquidity-modal" onClick={(e) => e.stopPropagation()}>
                <div className="smd-modal-header">
                    <div>
                        <h3 className="smd-modal-title">聪明钱雷达</h3>
                        <div className="smd-modal-subtitle">RPC 池子加池事件扫描 · 支持 V3/V4</div>
                    </div>
                    <button type="button" onClick={closeModal} disabled={saving} className="smd-modal-close">
                        <X size={18} />
                    </button>
                </div>
                <div className="smd-token-liquidity-form">
                    <label>
                        <span>池子地址 / V4 poolId</span>
                        <input placeholder="V3 0x... 或 V4 poolId" value={poolInput} onChange={(e) => setPoolInput(e.target.value)} />
                    </label>
                    <div className="smd-token-liquidity-window">
                        <div className="smd-token-liquidity-window-head">
                            <span>时间范围</span>
                            <strong>{currentRangeLabel}</strong>
                        </div>
                        <div className="smd-token-liquidity-datetime-grid">
                            <label>
                                <span>开始时间</span>
                                <input
                                    type="datetime-local"
                                    step="1"
                                    value={timeRange.start}
                                    onChange={(e) => setTimeRange((prev) => ({ ...prev, start: e.target.value }))}
                                />
                            </label>
                            <label>
                                <span>结束时间</span>
                                <input
                                    type="datetime-local"
                                    step="1"
                                    value={timeRange.end}
                                    onChange={(e) => setTimeRange((prev) => ({ ...prev, end: e.target.value }))}
                                />
                            </label>
                        </div>
                    </div>
                    <label>
                        <span>最低金额(USD)</span>
                        <input type="number" min="1" value={minAmountUsd} onChange={(e) => setMinAmountUsd(e.target.value)} />
                    </label>
                    <label>
                        <span>数量上限</span>
                        <input type="number" min="1" max="100" value={limit} onChange={(e) => setLimit(e.target.value)} />
                    </label>
                    <button type="button" className="smd-add-btn" onClick={preview} disabled={loading || saving}>
                        {loading ? '扫描中...' : '扫描候选钱包'}
                    </button>
                    {loading ? (
                        <button type="button" className="smd-modal-cancel" onClick={stopScan} disabled={saving}>
                            停止扫描
                        </button>
                    ) : null}
                </div>
                {(loading || saving || scanStep) ? (
                    <div className={`smd-token-liquidity-scan-state${error ? ' error' : ''}`}>
                        <div className="smd-token-liquidity-scan-head">
                            <span>{scanStep || (loading ? '准备扫描' : '等待操作')}</span>
                            <strong>{scanStatusText} · {formatRadarElapsed(scanElapsedMs)}</strong>
                        </div>
                        <div className="smd-token-liquidity-progress" aria-hidden="true">
                            <span className={loading || saving ? 'active' : ''} style={{ width: `${scanProgressPct}%` }} />
                        </div>
                        <div className="smd-token-liquidity-scan-meta">
                            {loading ? '后端正在流式扫描链上加池事件，找到候选会立即插入列表。' : null}
                            {saving ? '正在写入监控钱包，请保持弹窗打开。' : null}
                            {!loading && !saving && data ? `候选 ${candidates.length} 个，已排除 ${Number(data?.excluded_count || 0)} 条事件。` : null}
                            {!loading && !saving && error ? '请求未完成，参数保留，可直接重试。' : null}
                        </div>
                        {scanTarget ? (
                            <div className="smd-token-liquidity-scan-grid">
                                <span>
                                    <strong>目标</strong>
                                    <em>{scanTargetText}</em>
                                </span>
                                <span>
                                    <strong>时间窗</strong>
                                    <em>{scanTarget.rangeHours}h</em>
                                </span>
                                <span>
                                    <strong>阈值</strong>
                                    <em>{formatUSDCompact(scanTarget.minAmountUsd)}</em>
                                </span>
                                <span>
                                    <strong>上限</strong>
                                    <em>{scanTarget.limit}</em>
                                </span>
                            </div>
                        ) : null}
                        {scanLogs.length > 0 ? (
                            <div className="smd-token-liquidity-log">
                                {scanLogs.map((item) => (
                                    <div key={item.id} className={`smd-token-liquidity-log-row ${item.tone}`}>
                                        <span>{item.time}</span>
                                        <p>{item.text}</p>
                                    </div>
                                ))}
                            </div>
                        ) : null}
                    </div>
                ) : null}
                {error ? <div className="smd-inline-error">{error}</div> : null}
                {importResult ? (
                    <div className="smd-inline-success">
                        已新增 {importResult.created || 0}，已恢复 {importResult.reactivated || 0}，已跳过 {importResult.skipped_existing || 0}
                    </div>
                ) : null}
                {data ? (
                    <div className="smd-token-liquidity-toolbar">
                        <span>已排除 {Number(data?.excluded_count || 0)} 条事件</span>
                        {Array.isArray(data?.warnings) && data.warnings.length > 0 ? (
                            <span title={data.warnings.join('\n')}>{data.warnings.length} 条提示</span>
                        ) : null}
                    </div>
                ) : null}
                {candidates.length > 0 ? (
                    <>
                        <div className="smd-token-liquidity-toolbar">
                            <button
                                type="button"
                                className="smd-modal-cancel"
                                onClick={() => {
                                    if (allSelected) {
                                        setSelected({});
                                        return;
                                    }
                                    const next = {};
                                    candidates.forEach((item) => { next[item.wallet_address] = true; });
                                    setSelected(next);
                                }}
                            >
                                {allSelected ? '取消全选' : '全选'}
                            </button>
                            <span>{selectedWallets.length} / {candidates.length} 已选择</span>
                        </div>
                        <div className="smd-table-wrap smd-token-liquidity-table-wrap">
                            <table className="smd-table">
                                <thead>
                                    <tr>
                                        <th></th>
                                        <th>钱包</th>
                                        <th className="right">最大金额</th>
                                        <th>交易对</th>
                                        <th>协议</th>
                                        <th>金额来源</th>
                                        <th>池子</th>
                                        <th>交易</th>
                                        <th>状态</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {candidates.map((item) => (
                                        <tr key={`${item.wallet_address}:${item.tx_hash}`}>
                                            <td>
                                                <input
                                                    type="checkbox"
                                                    checked={Boolean(selected[item.wallet_address])}
                                                    onChange={(e) => setSelected((prev) => ({ ...prev, [item.wallet_address]: e.target.checked }))}
                                                />
                                            </td>
                                            <td className="mono">{shortAddr(item.wallet_address)}</td>
                                            <td className="right">{formatUsd(item.max_amount_usd)}</td>
                                            <td>{item.pair || '--'}</td>
                                            <td>{item.protocol || '--'}</td>
                                            <td>{item.amount_source || '--'}</td>
                                            <td className="mono">{shortAddr(item.pool_address)}</td>
                                            <td className="mono">{shortAddr(item.tx_hash)}</td>
                                            <td>{item.already_monitored ? '已存在' : '可导入'}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    </>
                ) : data ? (
                    <div className="smd-empty" style={{ marginTop: 12 }}>没有找到符合条件的钱包</div>
                ) : null}
                <div className="smd-modal-actions">
                    <button type="button" onClick={closeModal} disabled={saving} className="smd-modal-cancel">{loading ? '停止并关闭' : '关闭'}</button>
                    <button type="button" onClick={importSelected} disabled={loading || saving || selectedWallets.length === 0} className="smd-modal-submit">
                        {saving ? '导入中...' : `导入 ${selectedWallets.length} 个`}
                    </button>
                </div>
            </div>
        </div>
    );
}

function PositionAmountSummary({ position, preview, compact = false }) {
    const summary = getPositionAmountSummary(position, preview);
    return (
        <div className={`smd-pos-card-amount-wrap${compact ? ' compact' : ''}`}>
            <span className="smd-pos-card-amount-label">{summary.primaryLabel}</span>
            <span className="smd-pos-card-amount">{formatUSDCompact(summary.primaryValue)}</span>
            {!compact && summary.secondaryLabel && summary.secondaryValue !== null ? (
                <span className="smd-pos-card-amount-sub">
                    {summary.secondaryLabel} {formatUSDCompact(summary.secondaryValue)}
                </span>
            ) : null}
        </div>
    );
}

function PositionPreviewMetrics({ position, preview, currentPrice, compact = false }) {
    const runningText = formatDuration(preview?.runningSince || position?.opened_at) || '--';
    const previewFeeValue = Number(preview?.feeUsd);
    const feeValue = Number.isFinite(previewFeeValue) ? previewFeeValue : resolvePositionPreviewFeeUsd(null, position);
    const feeText = Number.isFinite(feeValue) ? formatUsd(feeValue) : '--';
    const feeTone = Number.isFinite(feeValue)
        ? (feeValue > 0 ? ' positive' : feeValue < 0 ? ' negative' : '')
        : '';
    const runtimeTone = runningText !== '--' ? ' positive' : '';
    const rangeStatus = resolvePositionRangeStatus(position, preview, currentPrice);
    const rangeTone = rangeStatus?.tone === 'positive'
        ? ' positive'
        : rangeStatus?.tone === 'negative'
            ? ' negative'
            : '';

    return (
        <div className={`smd-pos-card-preview${compact ? ' compact' : ''}`}>
            <span className={`smd-pos-card-metric${rangeTone}`}>
                <strong>区间状态</strong>
                <span>{rangeStatus?.text || '--'}</span>
            </span>
            <span className={`smd-pos-card-metric${feeTone}`}>
                <strong>手续费</strong>
                <span>{feeText}</span>
            </span>
            <span className={`smd-pos-card-metric${runtimeTone}`}>
                <strong>运行时间</strong>
                <span>{runningText}</span>
            </span>
        </div>
    );
}

function PositionPagination({ page, total, pageSize = POSITION_LIST_PAGE_SIZE, onChange }) {
    const totalPages = Math.max(1, Math.ceil(Number(total || 0) / pageSize));
    if (totalPages <= 1) return null;
    return (
        <div className="smd-filter-group" style={{ justifyContent: 'center', marginTop: 12 }}>
            <button type="button" className="smd-filter-btn" disabled={page <= 1} onClick={() => onChange(page - 1)}>
                上一页
            </button>
            <span className="smd-filter-btn active" style={{ cursor: 'default' }}>
                {page} / {totalPages}
            </span>
            <button type="button" className="smd-filter-btn" disabled={page >= totalPages} onClick={() => onChange(page + 1)}>
                下一页
            </button>
        </div>
    );
}

function buildPoolFromWatchActivity(event) {
    return {
        pool_address: event.pool_address,
        chain_id: event.chain_id,
        chain: resolvePoolChain(event),
        protocol: event.protocol,
        token0_address: event.token0_address,
        token1_address: event.token1_address,
        token0_symbol: event.token0_symbol,
        token1_symbol: event.token1_symbol,
        trading_pair: event.trading_pair,
        display_token_address: event.display_token_address,
        display_token_symbol: event.display_token_symbol,
        display_token_logo_url: event.display_token_logo_url,
        fee_tier: event.fee_tier,
    };
}

function WatchActivityCard({ event, onSelectWallet, onSelectPool }) {
    const walletAddress = normalizeWalletAddress(event?.wallet_address) || String(event?.wallet_address || '').trim();
    const poolAddress = String(event?.pool_address || '').trim();
    const amountValue = Number(event?.total_usd);
    const amountText = Number.isFinite(amountValue) && amountValue > 0 ? formatUSDCompact(amountValue) : '--';
    const pairLabel = getPairLabel(event);
    const rangeText = event?.tick_lower !== null && event?.tick_lower !== undefined && event?.tick_upper !== null && event?.tick_upper !== undefined
        ? `${event.tick_lower} - ${event.tick_upper}`
        : '--';
    const nftText = event?.nft_token_id ? `NFT #${event.nft_token_id}` : '';
    const canOpenWallet = Boolean(walletAddress);
    const canOpenPool = Boolean(poolAddress);

    return (
        <div className="smd-pool-card smd-watch-activity-card">
            <div className="smd-watch-activity-head">
                <PairAvatar item={event} size="sm" />
                <div className="smd-watch-activity-main">
                    <div className="smd-pool-card-badges">
                        <Badge cls={`smd-watch-action ${getWatchActivityActionClass(event?.event_type)}`}>
                            {formatWatchActivityAction(event?.event_type)}
                        </Badge>
                        <ProtocolBadge protocol={event?.protocol} />
                        {event?.fee_tier ? <Badge cls="fee">{formatFeeTier(event.fee_tier)}</Badge> : null}
                    </div>
                    <div className="smd-watch-activity-pair">{pairLabel}</div>
                    <WalletIdentity
                        address={walletAddress}
                        color={event?.wallet_color}
                        label={event?.wallet_label || walletAddress}
                        avatarUrl={event?.wallet_avatar_url}
                        source={event?.wallet_source}
                        sourceContract={event?.wallet_source_contract}
                        size={22}
                        showSource
                        onClick={canOpenWallet ? () => onSelectWallet?.(walletAddress) : undefined}
                    />
                </div>
                <div className="smd-watch-activity-amount">
                    <strong>{amountText}</strong>
                    <span>{relativeTime(event?.tx_timestamp)}</span>
                </div>
            </div>

            <div className="smd-watch-activity-metrics">
                <span><strong>金额</strong>{amountText}</span>
                <span><strong>Tick</strong>{rangeText}</span>
                {nftText ? <span>{nftText}</span> : null}
            </div>

            <div className="smd-watch-activity-foot">
                <div className="smd-watch-activity-ids">
                    <CompactIdentifier value={poolAddress} label="池子" />
                    {event?.tx_hash ? <CompactIdentifier value={event.tx_hash} label="TX" /> : null}
                </div>
                <div className="smd-watch-activity-actions">
                    {event?.explorer_url ? (
                        <a href={event.explorer_url} target="_blank" rel="noreferrer" className="smd-link">
                            浏览器 <ExternalLink size={10} />
                        </a>
                    ) : null}
                    {canOpenPool ? (
                        <button
                            type="button"
                            className="smd-action-chip"
                            onClick={() => onSelectPool?.(buildPoolFromWatchActivity(event))}
                        >
                            池子 <ExternalLink size={10} />
                        </button>
                    ) : null}
                </div>
            </div>
        </div>
    );
}

function WatchActivityPanel({
    apiBaseUrl,
    initData,
    watchedWallets = [],
    watchToggleMap = {},
    onToggleWatchWallet,
    onSelectWallet,
    onSelectPool,
    onOpenWallets,
    refreshInterval = 10,
}) {
    const canLoad = Boolean(String(initData || '').trim());
    const [activities, setActivities] = useState([]);
    const [walletItems, setWalletItems] = useState(() => normalizeWatchWalletItems(null, watchedWallets));
    const [selectedWallet, setSelectedWallet] = useState('');
    const [total, setTotal] = useState(0);
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(canLoad);
    const [error, setError] = useState('');
	const loadSeqRef = useRef(0);
	const refreshIntervalMs = useMemo(() => getRefreshIntervalMs(refreshInterval), [refreshInterval]);
	const activeWallet = normalizeWalletAddress(selectedWallet);
    const normalizedWatchedWallets = useMemo(
        () => Array.from(new Set((Array.isArray(watchedWallets) ? watchedWallets : [])
            .map((item) => normalizeWalletAddress(item))
            .filter(Boolean))).sort(),
        [watchedWallets],
    );

    useEffect(() => {
        loadSeqRef.current += 1;
        setWalletItems((prev) => {
            const allowed = new Set(normalizedWatchedWallets);
            if (allowed.size === 0) return [];
            return normalizeWatchWalletItems({
                items: prev.filter((item) => allowed.has(normalizeWalletAddress(item?.wallet_address))),
            }, normalizedWatchedWallets);
        });
    }, [normalizedWatchedWallets]);

    useEffect(() => {
        if (!activeWallet || normalizedWatchedWallets.includes(activeWallet)) return;
        setSelectedWallet('');
    }, [activeWallet, normalizedWatchedWallets]);

    const load = useCallback((silent = false) => {
        if (!canLoad) {
            setActivities([]);
            setTotal(0);
            setLoading(false);
            return;
        }

        const seq = ++loadSeqRef.current;
        if (!silent) {
            setLoading(true);
            setError('');
        }

        fetchSMWatchActivity({
            apiBaseUrl,
            initData,
            chain: 'bsc',
            wallet: activeWallet || undefined,
            page,
            size: WATCH_ACTIVITY_PAGE_SIZE,
        })
            .then((resp) => {
                if (seq !== loadSeqRef.current) return;
                if (!Array.isArray(resp?.list) || !Number.isFinite(Number(resp?.total))) {
                    throw new Error('特别关注动态格式错误');
                }
                const nextTotal = Number(resp.total);
                const totalPages = Math.max(1, Math.ceil(nextTotal / WATCH_ACTIVITY_PAGE_SIZE));
                setWalletItems(normalizeWatchWalletItems(resp, normalizedWatchedWallets));
                if (page > totalPages) {
                    setActivities([]);
                    setTotal(nextTotal);
                    setPage(totalPages);
                    return;
                }
                setActivities(resp.list);
                setTotal(nextTotal);
                setError('');
            })
            .catch((err) => {
                if (seq !== loadSeqRef.current) return;
                setActivities([]);
                setTotal(0);
                setError(String(err?.message || err || '加载特别关注动态失败'));
            })
            .finally(() => {
                if (!silent && seq === loadSeqRef.current) {
                    setLoading(false);
                }
            });
    }, [activeWallet, apiBaseUrl, canLoad, initData, normalizedWatchedWallets, page]);

    useEffect(() => {
        load();
    }, [load]);

    useEffect(() => {
        if (!canLoad) return undefined;
        const timer = setInterval(() => {
            load(true);
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [canLoad, load, refreshIntervalMs]);

    useEffect(() => {
        setPage(1);
    }, [activeWallet]);

    if (!canLoad) {
        return <div className="smd-empty">需要 Telegram initData 才能读取特别关注动态。</div>;
    }

    const emptyWallets = walletItems.length === 0;

    return (
        <div>
            <div className="smd-watch-wallet-strip">
                <button
                    type="button"
                    className={`smd-watch-wallet-chip${!activeWallet ? ' active' : ''}`}
                    onClick={() => setSelectedWallet('')}
                >
                    全部
                </button>
                {walletItems.map((item) => {
                    const address = normalizeWalletAddress(item.wallet_address);
                    const active = activeWallet === address;
                    const busy = Boolean(watchToggleMap[address]);
                    return (
                        <div
                            key={address}
                            className={`smd-watch-wallet-chip${active ? ' active' : ''}`}
                        >
                            <button
                                type="button"
                                className="smd-watch-wallet-chip-main"
                                onClick={() => setSelectedWallet(address)}
                            >
                                <WalletAvatar address={address} color={item.wallet_color} avatarUrl={item.wallet_avatar_url} size={22} />
                                <span>{item.wallet_label || shortAddr(address)}</span>
                            </button>
                            <button
                                type="button"
                                className="smd-watch-wallet-remove-btn"
                                disabled={busy}
                                onClick={(event) => {
                                    event.stopPropagation();
                                    onToggleWatchWallet?.(address, false);
                                    if (active) setSelectedWallet('');
                                }}
                                title="移除特别关注"
                                aria-label={`移除特别关注 ${shortAddr(address)}`}
                            >
                                {busy ? '...' : <X size={12} />}
                            </button>
                        </div>
                    );
                })}
            </div>

            {emptyWallets && !loading ? (
                <div className="smd-empty">
                    还没有特别关注钱包。
                    <button type="button" className="smd-filter-btn active smd-watch-empty-btn" onClick={onOpenWallets}>
                        去钱包视图添加
                    </button>
                </div>
            ) : loading ? (
                <div className="smd-loading">加载中...</div>
            ) : error ? (
                <div className="smd-empty smd-empty-error">{error}</div>
            ) : activities.length === 0 ? (
                <div className="smd-empty">当前范围暂无加 LP / 撤 LP 记录</div>
            ) : (
                <>
                    <div className="smd-pool-page-meta">
                        第 {page} 页 · 当前显示 {activities.length} 条 / 共 {total} 条记录
                    </div>
                    <div className="smd-pool-cards">
                        {activities.map((event) => (
                            <WatchActivityCard
                                key={`${event.tx_hash || event.id}:${event.log_index || 0}`}
                                event={event}
                                onSelectWallet={onSelectWallet}
                                onSelectPool={onSelectPool}
                            />
                        ))}
                    </div>
                </>
            )}

            <PositionPagination page={page} total={total} pageSize={WATCH_ACTIVITY_PAGE_SIZE} onChange={setPage} />
        </div>
    );
}

function SmartMoneyPositionDetailPanel({ apiBaseUrl, position, onClose }) {
    const [detail, setDetail] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');

    useEffect(() => {
        if (!position) return undefined;

        let timerId = 0;
        let cancelled = false;

        const load = async (silent = false) => {
            if (!silent) {
                setLoading(true);
                setError('');
            }
            try {
                const data = await fetchSMPositionDetail({
                    apiBaseUrl,
                    positionRef: position.position_ref,
                    positionId: position.id,
                });
                if (cancelled) return;
                setDetail(data || null);
                setError('');
                const pollSec = Math.max(Number(data?.poll_interval_sec || 1), 1);
                timerId = window.setTimeout(() => {
                    load(true);
                }, pollSec * 1000);
            } catch (err) {
                if (cancelled) return;
                setError(String(err?.message || err || 'detail load failed'));
                timerId = window.setTimeout(() => {
                    load(true);
                }, 3000);
            } finally {
                if (!cancelled) {
                    setLoading(false);
                }
            }
        };

        load(false);

        return () => {
            cancelled = true;
            window.clearTimeout(timerId);
        };
    }, [apiBaseUrl, position]);

    if (!position) return null;

    const token0 = detail?.token_rows?.[0];
    const token1 = detail?.token_rows?.[1];
    const totalVal = Number.isFinite(Number(detail?.current_value_usd))
        ? Number(detail.current_value_usd)
        : Number(detail?.totals?.position_usd || 0) + Number(detail?.totals?.fee_usd || 0);
    const statusLabel = String(detail?.status_label || (detail?.has_liquidity ? 'Open' : 'Closed'));
    const priceRange = detail ? computePriceRange(detail) : null;
    const detailRangeStatus = buildRangeStatusSummary(
        priceRange || (detail?.in_range === undefined ? null : { inRange: Boolean(detail.in_range) })
    );

    return (
        <div className="smd-pos-inline-panel">
            {error ? <div className="smd-inline-error" style={{ marginBottom: 12 }}>{error}</div> : null}
            {Array.isArray(detail?.warnings) && detail.warnings.length > 0 ? (
                <div className="smd-inline-error" style={{ marginBottom: 12, color: '#fbbf24', borderColor: 'rgba(251,191,36,0.25)' }}>
                    {detail.warnings.join(' / ')}
                </div>
            ) : null}

            {loading && !detail ? (
                <div className="smd-loading">读取链上仓位中...</div>
            ) : detail ? (
                <div className="pos-card sm-position-card sm-position-card--inline-detail">
                    <div className="pos-card-header">
                        <div className="pos-card-main sm-position-card-main">
                            <div className="pos-card-title-wrap sm-position-card-title-wrap">
                                <div className="sm-position-card-head-top">
                                    <div className="pos-pair-row">
                                        <span className="pos-pair-name">{detail?.title || shortAddress(detail?.pool_id || '')}</span>
                                        {detail?.tick_spacing ? (
                                            <span className="badge badge-fee">
                                                {formatFeeTier(({ 1: 100, 10: 500, 50: 2500, 60: 3000, 100: 5000, 200: 10000, 2000: 20000 }[Number(detail.tick_spacing)] || 0))}
                                            </span>
                                        ) : null}
                                    </div>
                                    <div className="sm-position-card-head-actions">
                                        <div className="pos-card-right-block">
                                            <div className="pos-metrics">
                                                <div className="pos-total">{formatUsd(totalVal)}</div>
                                            </div>
                                        </div>
                                        <button type="button" onClick={onClose} className="smd-pos-inline-panel-close" aria-label="收起详情">
                                            <X size={16} />
                                        </button>
                                    </div>
                                </div>
                                <div className="pos-status-row">
                                    <span className="status-pill">
                                        <span className="status-dot" />
                                        {statusLabel}
                                    </span>
                                    <span className="pos-wallet-chip">钱包 {shortAddress(detail?.wallet_address || '')}</span>
                                    <span className={`smd-badge ${walletSourceBadgeClass(detail?.wallet_source)}`} title={walletSourceContractLabel(detail?.wallet_source_contract) || walletSourceLabel(detail?.wallet_source)}>
                                        {walletSourceLabel(detail?.wallet_source)}
                                    </span>
                                    <span className={`range-pill ${detailRangeStatus?.tone === 'positive' ? 'in' : 'out'}`}>
                                        {detailRangeStatus?.text || '已离开区间'}
                                    </span>
                                </div>
                            </div>
                        </div>
                    </div>

                    {(token0 || token1) ? (
                        <div className="pos-token-table">
                            <div className="pos-token-head">
                                <span>Token</span><span>钱包</span><span>仓位</span><span>手续费</span>
                            </div>
                            {[token0, token1].filter(Boolean).map((tk) => (
                                <div key={tk.address || tk.symbol} className="pos-token-row">
                                    <div className="pos-tk-name">
                                        <div>{tk.symbol}</div>
                                        <div className="pos-tk-price">${Number(tk.price_usd || 0).toFixed(4)}</div>
                                    </div>
                                    <div className="pos-tk-cell">
                                        <div>{tk.wallet_amount ?? '--'}</div>
                                        <div className="pos-tk-usd">{formatUsd(tk.wallet_usd)}</div>
                                    </div>
                                    <div className="pos-tk-cell">
                                        <div>{tk.position_amount ?? '--'}</div>
                                        <div className="pos-tk-usd">{formatUsd(tk.position_usd)}</div>
                                    </div>
                                    <div className="pos-tk-cell fee">
                                        <div>{tk.fee_amount ?? '--'}</div>
                                        <div className="pos-tk-usd">{formatUsd(tk.fee_usd)}</div>
                                    </div>
                                </div>
                            ))}
                            <div className="pos-token-foot">
                                <span>小计</span>
                                <span>{formatUsd(detail?.totals?.wallet_usd)}</span>
                                <span>{formatUsd(detail?.totals?.position_usd)}</span>
                                <span className="fee">{formatUsd(detail?.totals?.fee_usd)}</span>
                            </div>
                        </div>
                    ) : null}

                    {priceRange ? (
                        <div className="pos-price-range">
                            <div className="pos-price-range-header">
                                <span className="pos-price-range-label">价格范围 ({priceRange.pairLabel}{priceRange.gridCount ? ` ${priceRange.gridCount}格` : ''})</span>
                                {Number.isFinite(priceRange.deviation) && priceRange.deviation > 0 ? (
                                    <span className="pos-price-range-dev">{priceRange.deviation.toFixed(2)}%</span>
                                ) : null}
                            </div>
                            <div className="pos-price-range-bar-wrap">
                                <div className="pos-price-range-bar">
                                    <div className="pos-price-range-limit lo" />
                                    <div className="pos-price-range-limit hi" />
                                    {priceRange.visibleGridLines?.map((pct, index) => (
                                        <div key={index} className="pos-price-range-grid" style={{ left: `calc(3% + ${pct * 0.94}%)` }} />
                                    ))}
                                    <div
                                        className={`pos-price-range-cursor ${priceRange.inRange ? 'in' : 'out'}`}
                                        style={{ left: `calc(3% + ${priceRange.percent * 0.94}%)` }}
                                    />
                                </div>
                            </div>
                            <div className="pos-price-range-labels">
                                <span className="lo">{compactPrice(priceRange.rangeMin)}</span>
                                <span className="cur">{compactPrice((priceRange.rangeMin + priceRange.rangeMax) / 2)}</span>
                                <span className="hi">{compactPrice(priceRange.rangeMax)}</span>
                            </div>
                        </div>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}

// ---- Pages ----

function PoolList({ apiBaseUrl, onSelect, onOpenDetail, onOpenPosition, activePoolAddress = '', refreshInterval = 10 }) {
    const [pools, setPools] = useState([]);
    const [poolsTotal, setPoolsTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const [proto, setProto] = useState('all');
    const [sourceScope, setSourceScope] = useState('all');
    const [page, setPage] = useState(1);
    const [filterOpen, setFilterOpen] = useState(false);
    const [poolFilter, setPoolFilter] = useState(readStoredSmartMoneyPoolFilter);
    const [poolFilterDraft, setPoolFilterDraft] = useState({ minSmartMoneyUsd: '', maxFeeRate: '', minMarketCapUsd: '' });
    const loadSeqRef = useRef(0);
    const searchKeyword = useMemo(() => String(search || '').trim(), [search]);
    const sourceFilter = SMART_MONEY_POOL_SOURCE_BY_KEY[sourceScope];
    const normalizedActivePoolAddress = useMemo(
        () => normalizePoolSelectionId(activePoolAddress),
        [activePoolAddress]
    );
    const refreshIntervalMs = useMemo(
        () => getRefreshIntervalMs(refreshInterval),
        [refreshInterval]
    );

    const loadPools = useCallback((silent = false) => {
        const seq = ++loadSeqRef.current;
        if (!silent) {
            setLoading(true);
            setError('');
        }
        return fetchSMPools({
            apiBaseUrl,
            page,
            size: POOL_LIST_PAGE_SIZE,
            keyword: searchKeyword || undefined,
            protocol: proto !== 'all' ? proto : undefined,
            source: sourceFilter,
            minSmartMoneyUsd: poolFilter.minSmartMoneyUsd,
            maxFeeRate: poolFilter.maxFeeRate,
            minMarketCapUsd: poolFilter.minMarketCapUsd,
        })
            .then((d) => {
                if (seq !== loadSeqRef.current) return;
                if (!Array.isArray(d?.list) || !Number.isFinite(Number(d?.total))) {
                    throw new Error('池子数据格式错误');
                }
                const total = Number(d.total);
                const list = d.list;
                const totalPages = Math.max(1, Math.ceil(total / POOL_LIST_PAGE_SIZE));
                if (page > totalPages) {
                    setPools([]);
                    setPoolsTotal(total);
                    setPage(totalPages);
                    return;
                }
                setPools(list);
                setPoolsTotal(total);
                setError('');
            })
            .catch((err) => {
                if (seq !== loadSeqRef.current) return;
                setPools([]);
                setPoolsTotal(0);
                setError(String(err?.message || err || '加载池子失败'));
            })
            .finally(() => {
                if (!silent && seq === loadSeqRef.current) setLoading(false);
            });
    }, [apiBaseUrl, page, poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd, proto, searchKeyword, sourceFilter]);

    useEffect(() => {
        loadPools();
    }, [loadPools]);

    useEffect(() => {
        const timer = setInterval(() => {
            loadPools(true);
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [loadPools, refreshIntervalMs]);

    useEffect(() => {
        setPage(1);
    }, [poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd, proto, searchKeyword, sourceScope]);

    const poolFilterActive = useMemo(
        () => Number.isFinite(poolFilter.minSmartMoneyUsd)
            || Number.isFinite(poolFilter.maxFeeRate)
            || Number.isFinite(poolFilter.minMarketCapUsd),
        [poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd]
    );
    const hasFilter = Boolean(searchKeyword) || proto !== 'all' || sourceScope !== 'all' || poolFilterActive;
    const openPoolFilter = useCallback(() => {
        setPoolFilterDraft({
            minSmartMoneyUsd: formatOptionalNumber(poolFilter.minSmartMoneyUsd),
            maxFeeRate: formatOptionalNumber(poolFilter.maxFeeRate),
            minMarketCapUsd: formatOptionalNumber(poolFilter.minMarketCapUsd),
        });
        setFilterOpen((prev) => !prev);
    }, [poolFilter.maxFeeRate, poolFilter.minMarketCapUsd, poolFilter.minSmartMoneyUsd]);
    const applyPoolFilter = useCallback(() => {
        const next = {
            minSmartMoneyUsd: parseOptionalNumber(poolFilterDraft.minSmartMoneyUsd),
            maxFeeRate: parseOptionalNumber(poolFilterDraft.maxFeeRate),
            minMarketCapUsd: parseOptionalNumber(poolFilterDraft.minMarketCapUsd),
        };
        setPoolFilter(next);
        writeStoredSmartMoneyPoolFilter(next);
        setFilterOpen(false);
        setPage(1);
    }, [poolFilterDraft.maxFeeRate, poolFilterDraft.minMarketCapUsd, poolFilterDraft.minSmartMoneyUsd]);
    const clearPoolFilter = useCallback(() => {
        const next = { ...EMPTY_SMART_MONEY_POOL_FILTER };
        setPoolFilter(next);
        writeStoredSmartMoneyPoolFilter(next);
        setPoolFilterDraft({ minSmartMoneyUsd: '', maxFeeRate: '', minMarketCapUsd: '' });
        setFilterOpen(false);
        setPage(1);
    }, []);

    return (
        <div>
            <div className="smd-source-tabs" role="tablist" aria-label="聪明钱来源范围">
                {SMART_MONEY_POOL_SOURCE_TABS.map((item) => (
                    <button
                        key={item.key}
                        type="button"
                        role="tab"
                        aria-selected={sourceScope === item.key}
                        className={`smd-source-tab${sourceScope === item.key ? ' active' : ''}`}
                        onClick={() => {
                            setSourceScope(item.key);
                            setPage(1);
                        }}
                    >
                        {item.label}
                    </button>
                ))}
            </div>
            <div className="smd-search-row">
                <div className="smd-search-input">
                    <Search size={14} />
                    <input
                        placeholder="搜索池子..."
                        value={search}
                        onChange={(e) => {
                            setSearch(e.target.value);
                            setPage(1);
                        }}
                    />
                </div>
                <div className="smd-filter-group">
                    {['all', 'pancake_v3', 'uniswap_v3', 'uniswap_v4'].map(p => {
                        const info = PROTOCOL_MAP[p];
                        return (
                            <button
                                key={p}
                                className={`smd-filter-btn${proto === p ? ' active' : ''}`}
                                onClick={() => {
                                    setProto(p);
                                    setPage(1);
                                }}
                            >
                                {info && <img src={info.icon} alt="" className="smd-proto-img" />}
                                {p === 'all' ? '全部' : info?.version || p}
                            </button>
                        );
                    })}
                    <div className="smd-pool-filter-wrap">
                        <button
                            type="button"
                            className={`smd-filter-btn${poolFilterActive ? ' active' : ''}`}
                            onClick={openPoolFilter}
                            aria-pressed={poolFilterActive}
                            title="筛选池子"
                        >
                            <SlidersHorizontal size={13} />
                            筛选
                        </button>
                        {filterOpen ? (
                            <div className="popover kline-filter-popover smd-pool-filter-popover">
                                <div className="kline-filter-popover-head">
                                    <div>
                                        <div className="kline-filter-popover-title">池子筛选</div>
                                        <div className="kline-filter-popover-sub">按聪明钱仓位和池子费率过滤全部池子</div>
                                    </div>
                                    <button
                                        type="button"
                                        className="icon-link"
                                        onClick={() => setFilterOpen(false)}
                                        title="Close"
                                    >
                                        <X size={14} />
                                    </button>
                                </div>

                                <label className="kline-filter-field">
                                    <span>聪明钱仓位 ≥ (USD)</span>
                                    <input
                                        value={poolFilterDraft.minSmartMoneyUsd}
                                        onChange={(e) => setPoolFilterDraft((prev) => ({ ...prev, minSmartMoneyUsd: e.target.value }))}
                                        inputMode="decimal"
                                        placeholder="可选"
                                    />
                                </label>

                                <label className="kline-filter-field">
                                    <span>池子费率 ≤ (%)</span>
                                    <input
                                        value={poolFilterDraft.maxFeeRate}
                                        onChange={(e) => setPoolFilterDraft((prev) => ({ ...prev, maxFeeRate: e.target.value }))}
                                        inputMode="decimal"
                                        placeholder="可选"
                                    />
                                </label>

                                <label className="kline-filter-field">
                                    <span>FDV ≥ (USD)</span>
                                    <input
                                        value={poolFilterDraft.minMarketCapUsd}
                                        onChange={(e) => setPoolFilterDraft((prev) => ({ ...prev, minMarketCapUsd: e.target.value }))}
                                        inputMode="decimal"
                                        placeholder="可选"
                                    />
                                </label>

                                <div className="kline-filter-actions">
                                    <button type="button" className="ghost-chip active" onClick={applyPoolFilter}>
                                        应用
                                    </button>
                                    <button type="button" className="ghost-chip" onClick={clearPoolFilter}>
                                        清空
                                    </button>
                                </div>
                            </div>
                        ) : null}
                    </div>
                </div>
            </div>
            {loading ? <div className="smd-loading">加载中...</div> : error ? (
                <div className="smd-empty smd-empty-error">{error}</div>
            ) : pools.length === 0 ? (
                <div className="smd-empty">{hasFilter ? '当前筛选条件下暂无池子' : '暂无活跃仓位的池子'}</div>
            ) : (
                <>
                    <div className="smd-pool-page-meta">
                        第 {page} 页 · 当前显示 {pools.length} 个 / 共 {poolsTotal} 个池子
                    </div>
                    <div className="smd-pool-cards">
                        {pools.map((p) => {
                            const isActive = normalizedActivePoolAddress && normalizePoolSelectionId(p) === normalizedActivePoolAddress;
                            const marketCap = resolveSmartMoneyPoolMarketCapDisplay(p);
                            const marketCapLabel = resolveSmartMoneyPoolMarketCapLabel(p);
                            const marketCapAvailable = Number.isFinite(marketCap) && marketCap > 0;
                            return (
                                <div
                                    key={p.pool_address}
                                    className={`smd-pool-card${isActive ? ' active' : ''}`}
                                    onClick={() => {
                                        if (typeof onSelect === 'function') {
                                            onSelect(p);
                                            return;
                                        }
                                        onOpenDetail?.(p);
                                    }}
                                >
                                    <div className="smd-pool-card-head">
                                        <PairAvatar item={p} size="sm" />
                                        <span className="smd-pool-card-pair">{getPairLabel(p)}</span>
                                        <div className="smd-pool-card-badges">
                                            <ProtocolBadge protocol={p.protocol} />
                                            {p.fee_tier && <Badge cls="fee">{formatFeeTier(p.fee_tier)}</Badge>}
                                        </div>
                                    </div>
                                    <div className="smd-pool-card-meta">
                                        <CompactIdentifier value={getPoolIdentifier(p)} />
                                        {p.total_position_amount_usd > 0 && (
                                            <span className="smd-pool-card-tvl">{formatUSDCompact(p.total_position_amount_usd)}</span>
                                        )}
                                        {marketCapAvailable ? (
                                            <span className="smd-pool-card-tvl smd-pool-card-market-cap">
                                                {marketCapLabel} {formatUSDCompact(marketCap)}
                                            </span>
                                        ) : null}
                                    </div>
                                    <div className="smd-pool-card-range-row">
                                        <PoolCardRangeSummary pool={p} />
                                        {typeof onOpenPosition === 'function' ? (
                                            <button
                                                type="button"
                                                className="pool-buy-btn smd-follow-open-btn"
                                                onClick={(event) => {
                                                    event.stopPropagation();
                                                    onOpenPosition(p);
                                                }}
                                            >
                                                <img src={flashIcon} alt="" className="open-lightning-icon" aria-hidden="true" />
                                                跟单
                                            </button>
                                        ) : null}
                                    </div>
                                    <div className="smd-pool-card-foot">
                                        <span>{p.wallet_count} 钱包</span>
                                        <span className="smd-dot-sep">·</span>
                                        <span>{p.open_position_count} 仓位</span>
                                        <span className="smd-pool-card-time">
                                            <span className={`smd-status-dot ${p.latest_event_at && (Date.now() - new Date(p.latest_event_at).getTime()) < 120000 ? 'green' : 'muted'}`}>
                                                {relativeTime(p.latest_event_at)}
                                            </span>
                                        </span>
                                        <button
                                            type="button"
                                            className="smd-link smd-pool-card-detail-btn"
                                            onClick={(event) => {
                                                event.stopPropagation();
                                                onOpenDetail?.(p);
                                            }}
                                        >
                                            详情 <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} />
                                        </button>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </>
            )}
            <PositionPagination page={page} total={poolsTotal} pageSize={POOL_LIST_PAGE_SIZE} onChange={setPage} />
        </div>
    );
}

function SmartMoneyPoolView({ apiBaseUrl, onSelect, onOpenDetail, onOpenPosition, activePoolAddress = '', refreshInterval = 10 }) {
    const [tab, setTab] = useState('active');
    return (
        <div>
            <div className="smd-subtabs">
                {[
                    { key: 'active', label: '活跃池子', icon: Activity },
                    { key: 'heatmap', label: '收益火焰图', icon: Flame },
                ].map(({ key, label, icon: Icon }) => (
                    <button
                        key={key}
                        type="button"
                        className={`smd-subtab${tab === key ? ' active' : ''}`}
                        onClick={() => setTab(key)}
                    >
                        <Icon size={14} />
                        {label}
                    </button>
                ))}
            </div>
            {tab === 'heatmap' ? (
                <PoolFeeHeatmap
                    apiBaseUrl={apiBaseUrl}
                    onSelect={onSelect}
                    onOpenDetail={onOpenDetail}
                    onOpenPosition={onOpenPosition}
                    refreshInterval={refreshInterval}
                />
            ) : (
                <PoolList
                    apiBaseUrl={apiBaseUrl}
                    onSelect={onSelect}
                    onOpenDetail={onOpenDetail}
                    activePoolAddress={activePoolAddress}
                    refreshInterval={refreshInterval}
                    onOpenPosition={onOpenPosition}
                />
            )}
        </div>
    );
}

function PoolFeeHeatmap({ apiBaseUrl, onSelect, onOpenDetail, onOpenPosition, refreshInterval = 10 }) {
    const [rows, setRows] = useState([]);
    const [total, setTotal] = useState(0);
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    const [proto, setProto] = useState('all');
    const [sort, setSort] = useState('rate');
    const [windowKey, setWindowKey] = useState('1m');
    const [updatedAt, setUpdatedAt] = useState('');
    const loadSeqRef = useRef(0);
    const searchKeyword = useMemo(() => String(search || '').trim(), [search]);
    const refreshIntervalMs = useMemo(
        () => getRefreshIntervalMs(refreshInterval),
        [refreshInterval]
    );

    const loadHeatmap = useCallback((silent = false) => {
        const seq = ++loadSeqRef.current;
        if (!silent) {
            setLoading(true);
            setError('');
        }
        fetchSMPoolFeeHeatmap({
            apiBaseUrl,
            window: windowKey,
            sort,
            keyword: searchKeyword || undefined,
            protocol: proto !== 'all' ? proto : undefined,
            page,
            size: POOL_HEATMAP_PAGE_SIZE,
        })
            .then((data) => {
                if (seq !== loadSeqRef.current) return;
                if (!Array.isArray(data?.list)) {
                    throw new Error('收益火焰图数据格式错误');
                }
                const nextTotal = Number(data.total || 0);
                const totalPages = Math.max(1, Math.ceil(nextTotal / POOL_HEATMAP_PAGE_SIZE));
                if (page > totalPages) {
                    setRows([]);
                    setTotal(nextTotal);
                    setPage(totalPages);
                    return;
                }
                setRows(data.list);
                setTotal(nextTotal);
                setUpdatedAt(data.updated_at || '');
                setError('');
            })
            .catch((err) => {
                if (seq !== loadSeqRef.current) return;
                setRows([]);
                setTotal(0);
                setError(String(err?.message || err || '加载收益火焰图失败'));
            })
            .finally(() => {
                if (!silent && seq === loadSeqRef.current) setLoading(false);
            });
    }, [apiBaseUrl, page, proto, searchKeyword, sort, windowKey]);

    useEffect(() => {
        setPage(1);
    }, [proto, searchKeyword, sort, windowKey]);

    useEffect(() => {
        loadHeatmap();
    }, [loadHeatmap]);

    useEffect(() => {
        const timer = setInterval(() => {
            loadHeatmap(true);
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [loadHeatmap, refreshIntervalMs]);

    const maxIntensity = useMemo(() => {
        return Math.max(...rows.map((row) => {
            if (sort === 'fee') {
                return Number(row?.fee_position_count || 0) > 0 ? Number(row?.fee_usd || 0) : 0;
            }
            return Number(row?.rate_position_count || 0) > 0 ? Number(row?.fee_rate_per_1k_usd_window || 0) : 0;
        }), 0);
    }, [rows, sort]);

    return (
        <div>
            <div className="smd-search-row">
                <div className="smd-search-input">
                    <Search size={14} />
                    <input
                        placeholder="搜索池子..."
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                    />
                </div>
                <div className="smd-filter-group smd-filter-group--wrap">
                    {POOL_HEATMAP_SORTS.map((item) => (
                        <button
                            key={item.key}
                            type="button"
                            className={`smd-filter-btn${sort === item.key ? ' active' : ''}`}
                            onClick={() => setSort(item.key)}
                        >
                            {item.key === 'rate' ? <Activity size={13} /> : <DollarSign size={13} />}
                            {item.label}
                        </button>
                    ))}
                    {POOL_HEATMAP_WINDOWS.map((item) => (
                        <button
                            key={item.key}
                            type="button"
                            className={`smd-filter-btn${windowKey === item.key ? ' active' : ''}`}
                            onClick={() => setWindowKey(item.key)}
                        >
                            <Clock size={12} />
                            {item.label}
                        </button>
                    ))}
                </div>
            </div>
            <div className="smd-filter-group smd-heatmap-protos">
                {['all', 'pancake_v3', 'uniswap_v3', 'uniswap_v4'].map((p) => {
                    const info = PROTOCOL_MAP[p];
                    return (
                        <button
                            key={p}
                            className={`smd-filter-btn${proto === p ? ' active' : ''}`}
                            onClick={() => setProto(p)}
                        >
                            {info && <img src={info.icon} alt="" className="smd-proto-img" />}
                            {p === 'all' ? '全部' : info?.version || p}
                        </button>
                    );
                })}
            </div>

            {loading ? <div className="smd-loading">加载中...</div> : error ? (
                <div className="smd-empty smd-empty-error">{error}</div>
            ) : rows.length === 0 ? (
                <div className="smd-empty">暂无可计算收益的池子</div>
            ) : (
                <>
                    <div className="smd-pool-page-meta">
                        第 {page} 页 · 当前显示 {rows.length} 个 / 共 {total} 个池子 · {sort === 'rate' ? `按 ${heatmapWindowLabel(windowKey)} 速率` : '按手续费总额'}
                    </div>
                    <div className="smd-heatmap-grid">
                        {rows.map((row, index) => (
                            <PoolFeeHeatmapCard
                                key={row.pool_address}
                                row={row}
                                rank={(page - 1) * POOL_HEATMAP_PAGE_SIZE + index + 1}
                                sort={sort}
                                windowKey={windowKey}
                                maxIntensity={maxIntensity}
                                onSelect={onSelect}
                                onOpenDetail={onOpenDetail}
                                onOpenPosition={onOpenPosition}
                            />
                        ))}
                    </div>
                    <PositionPagination page={page} total={total} pageSize={POOL_HEATMAP_PAGE_SIZE} onChange={setPage} />
                </>
            )}
        </div>
    );
}

function PoolFeeHeatmapCard({ row, rank, sort, windowKey, maxIntensity, onSelect, onOpenDetail, onOpenPosition }) {
    const hasFeeSample = Number(row?.fee_position_count || 0) > 0;
    const hasRateSample = Number(row?.rate_position_count || 0) > 0;
    const metricValue = sort === 'fee' && hasFeeSample
        ? Number(row?.fee_usd || 0)
        : sort !== 'fee' && hasRateSample
            ? Number(row?.fee_rate_per_1k_usd_window || 0)
            : 0;
    const intensity = maxIntensity > 0 ? Math.max(0.08, Math.min(1, metricValue / maxIntensity)) : 0.08;
    const reliable = String(row?.sample_status || '') === 'ok';
    const partial = String(row?.sample_status || '') === 'partial';
    const marketCap = resolveSmartMoneyPoolMarketCapDisplay(row);
    const marketCapLabel = resolveSmartMoneyPoolMarketCapLabel(row);
    const marketCapAvailable = Number.isFinite(marketCap) && marketCap > 0;
    return (
        <div className="smd-heatmap-card" onClick={() => onSelect?.(row)}>
            <div className="smd-heatmap-card-rail" style={{ opacity: 0.2 + intensity * 0.75 }} />
            <div className="smd-heatmap-card-glow" style={{ opacity: intensity * 0.75 }} />
            <div className="smd-heatmap-card-head">
                <span className="smd-heatmap-rank">#{rank}</span>
                <PairAvatar item={row} size="sm" />
                <span className="smd-pool-card-pair">{getPairLabel(row)}</span>
                <div className="smd-pool-card-badges">
                    <ProtocolBadge protocol={row.protocol} />
                    {row.fee_tier && <Badge cls="fee">{formatFeeTier(row.fee_tier)}</Badge>}
                </div>
            </div>
            <div className="smd-heatmap-metrics">
                <div className="smd-heatmap-metric">
                    <span>总手续费</span>
                    <strong>{hasFeeSample ? formatHeatmapUSD(row.fee_usd) : '--'}</strong>
                </div>
                <div className="smd-heatmap-metric accent">
                    <span>每1000U/{heatmapWindowLabel(windowKey)}</span>
                    <strong>{hasRateSample ? formatHeatmapRate(row.fee_rate_per_1k_usd_window) : '--'}</strong>
                </div>
            </div>
            <div className="smd-heatmap-bar">
                <span style={{ width: `${Math.max(6, intensity * 100)}%` }} />
            </div>
            <div className="smd-heatmap-meta">
                <span>{row.wallet_count} 钱包</span>
                <span>{row.open_position_count} 仓位</span>
                <span>仓位 {formatHeatmapUSD(row.total_position_amount_usd)}</span>
                {marketCapAvailable ? <span>{marketCapLabel} {formatUSDCompact(marketCap)}</span> : null}
                <span>均龄 {formatHeatmapAge(row.average_position_age_seconds)}</span>
            </div>
            <div className="smd-heatmap-foot">
                <span className={`smd-heatmap-sample ${reliable ? 'ok' : partial ? 'partial' : 'bad'}`}>
                    {heatmapSampleText(row)} · {row.rate_position_count || 0}/{row.open_position_count || 0} 仓
                </span>
                <div className="smd-heatmap-actions">
                    {typeof onOpenPosition === 'function' ? (
                        <button
                            type="button"
                            className="pool-buy-btn smd-follow-open-btn"
                            onClick={(event) => {
                                event.stopPropagation();
                                onOpenPosition(row);
                            }}
                        >
                            <img src={flashIcon} alt="" className="open-lightning-icon" aria-hidden="true" />
                            跟单
                        </button>
                    ) : null}
                    <button
                        type="button"
                        className="smd-link smd-pool-card-detail-btn"
                        onClick={(event) => {
                            event.stopPropagation();
                            onOpenDetail?.(row);
                        }}
                    >
                        详情 <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} />
                    </button>
                </div>
            </div>
        </div>
    );
}

function RangeSummary({ position }) {
    const isClosed = position.status === 'closed';
    const rangeText = position.price_lower && position.price_upper
        ? `${position.price_lower} - ${position.price_upper}`
        : '--';

    return (
        <div className="smd-range-cell">
            <div className={`smd-range-main mono muted${isClosed ? ' is-closed' : ''}`}>
                {rangeText}
            </div>
            <div className="smd-range-sub">{formatRangePercent(position.range_percent)}</div>
        </div>
    );
}

function PoolDetail({ apiBaseUrl, pool, onBack, onSelectWallet, refreshInterval = 10 }) {
    const [positions, setPositions] = useState([]);
    const [positionsTotal, setPositionsTotal] = useState(0);
    const [stats, setStats] = useState(null);
    const [status, setStatus] = useState('open');
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(true);
    const [selectedPosition, setSelectedPosition] = useState(null);
    const poolIdentifier = getPoolIdentifier(pool);
    const poolChain = resolvePoolChain(pool);
    const poolGmgnUrl = useMemo(() => buildGmgnUrl({ ...pool, chain: poolChain }, poolChain), [pool, poolChain]);
    const refreshIntervalMs = useMemo(
        () => getRefreshIntervalMs(refreshInterval),
        [refreshInterval]
    );
    const positionPreviews = useSmartMoneyPositionPreviewMap(apiBaseUrl, positions);

    const loadStats = useCallback(() => (
        fetchSMPoolStats({ apiBaseUrl, poolAddress: pool.pool_address }).then(setStats).catch(() => { })
    ), [apiBaseUrl, pool.pool_address]);

    const loadPositions = useCallback((silent = false) => {
        if (!silent) setLoading(true);
        return fetchSMPositions({
            apiBaseUrl,
            pool: pool.pool_address,
            status,
            page,
            size: POSITION_LIST_PAGE_SIZE,
            orderBy: 'position_amount_desc',
        })
            .then((d) => {
                setPositions(d?.list || []);
                setPositionsTotal(Number(d?.total || 0));
            })
            .catch(() => { })
            .finally(() => {
                if (!silent) setLoading(false);
            });
    }, [apiBaseUrl, page, pool.pool_address, status]);

    useEffect(() => {
        setPage(1);
    }, [pool.pool_address, status]);

    useEffect(() => {
        loadStats();
    }, [loadStats]);

    useEffect(() => {
        loadPositions();
    }, [loadPositions]);

    useEffect(() => {
        const timer = setInterval(() => {
            loadStats();
            loadPositions(true);
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [loadPositions, loadStats, refreshIntervalMs]);

    useEffect(() => {
        if (!selectedPosition) return;
        const selectedKey = getPositionSelectionKey(selectedPosition);
        if (positions.some((pos) => getPositionSelectionKey(pos) === selectedKey)) return;
        setSelectedPosition(null);
    }, [positions, selectedPosition]);
    const selectedPositionKey = selectedPosition ? getPositionSelectionKey(selectedPosition) : '';
    const statsMarketCap = resolveSmartMoneyPoolMarketCapDisplay(stats || pool);
    const statsMarketCapLabel = resolveSmartMoneyPoolMarketCapLabel(stats || pool);
    const statsMarketCapAvailable = Number.isFinite(statsMarketCap) && statsMarketCap > 0;

    return (
        <div>
            <button onClick={onBack} className="smd-back-btn">
                <ChevronLeft size={14} />
                <span>返回池子列表</span>
            </button>
            <div className="smd-detail-card">
                <div className="smd-detail-header">
                    <PairAvatar item={pool} size="lg" />
                    <div className="smd-detail-copy">
                        <div className="smd-detail-headline">
                            <h3 className="smd-detail-title">{getPairLabel(pool)}</h3>
                            <ProtocolBadge protocol={pool.protocol} />
                            {pool.fee_tier && <Badge cls="fee">{formatFeeTier(pool.fee_tier)}</Badge>}
                        </div>
                        <div className="smd-detail-meta">
                            <CompactIdentifier value={poolIdentifier} label={getPoolIdentifierLabel(poolIdentifier)} />
                            {poolGmgnUrl ? (
                                <a
                                    href={poolGmgnUrl}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="smd-link"
                                    title="在 GMGN 查看池子代币"
                                >
                                    <img src={gmgnIcon} alt="GMGN" style={{ width: 14, height: 14, verticalAlign: 'middle' }} />
                                    <span>GMGN</span>
                                </a>
                            ) : null}
                        </div>
                    </div>
                </div>
            </div>

            {stats && (
                <div className="smd-stats-grid smd-stats-grid--pool">
                    <StatCard label="当前价格" value={stats.current_price || '--'} />
                    <StatCard label="钱包数" value={stats.wallet_count} />
                    <StatCard label="持仓笔数" value={stats.open_position_count} />
                    <StatCard label="今日关闭" value={stats.closed_today_count} color="red" />
                    {statsMarketCapAvailable ? (
                        <StatCard label={statsMarketCapLabel} value={formatUSDCompact(statsMarketCap)} />
                    ) : null}
                </div>
            )}

            <PriceRangeChart positions={positions} currentPrice={stats?.current_price} />

            <div className="smd-section-header">
                <h4 className="smd-section-title">仓位列表</h4>
                <div className="smd-filter-group">
                    {['open', 'all'].map(s => (
                        <button key={s} className={`smd-filter-btn${status === s ? ' active' : ''}`} onClick={() => setStatus(s)}>
                            {s === 'open' ? '持仓中' : '全部'}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? <div className="smd-loading">加载中...</div> : positions.length === 0 ? (
                <div className="smd-empty">{status === 'open' ? '当前没有进行中的仓位，切换到“全部”查看历史' : '暂无仓位'}</div>
            ) : (
                <div className="smd-pos-list">
                    {positions.map(pos => {
                        const positionKey = getPositionSelectionKey(pos) || String(pos.id || '');
                        const isSelected = Boolean(positionKey) && positionKey === selectedPositionKey;
                        return (
                            <div key={positionKey || pos.id} className="smd-pos-row">
                                <div className={`smd-pos-card${pos.status === 'closed' ? ' closed' : ''}`} onClick={() => setSelectedPosition(pos)}>
                                    <div className="smd-pos-card-top">
                                        <WalletIdentity
                                            address={pos.wallet_address}
                                            color={pos.wallet_color}
                                            label={pos.wallet_label || pos.wallet_address}
                                            avatarUrl={pos.wallet_avatar_url}
                                            source={pos.wallet_source}
                                            sourceContract={pos.wallet_source_contract}
                                            size={28}
                                            showSource
                                            onClick={() => onSelectWallet(pos.wallet_address)}
                                        />
                                        <div className="smd-pos-card-top-right">
                                            <PositionAmountSummary position={pos} preview={positionPreviews[getPositionSelectionKey(pos)]} />
                                            <Badge cls={pos.status === 'open' ? 'status-open' : 'status-closed'}>
                                                {pos.status === 'open' ? '持仓中' : '已关闭'}
                                            </Badge>
                                        </div>
                                    </div>
                                    <div className="smd-pos-card-range smd-pos-card-range--detail">
                                        <span className={`smd-pos-card-prices${pos.status === 'closed' ? ' is-closed' : ''}`}>
                                            {pos.price_lower && pos.price_upper ? `${pos.price_lower} - ${pos.price_upper}` : '--'}
                                        </span>
                                        <div className="smd-pos-card-meta">
                                            <span>NFT #{pos.nft_token_id || '--'}</span>
                                            {pos.range_percent > 0 && <span>{formatRangePercent(pos.range_percent)}</span>}
                                        </div>
                                        {pos.bscscan_url ? (
                                            <a
                                                href={pos.bscscan_url}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="smd-link smd-pos-card-link"
                                                onClick={(event) => event.stopPropagation()}
                                            >
                                                查看交易 <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} />
                                            </a>
                                        ) : null}
                                    </div>
                                        <PositionPreviewMetrics
                                            position={pos}
                                            preview={positionPreviews[getPositionSelectionKey(pos)]}
                                            currentPrice={stats?.current_price}
                                        />
                                    </div>
                                {isSelected ? (
                                    <SmartMoneyPositionDetailPanel
                                        apiBaseUrl={apiBaseUrl}
                                        position={selectedPosition}
                                        onClose={() => setSelectedPosition(null)}
                                    />
                                ) : null}
                            </div>
                        );
                    })}
                </div>
            )}
            <PositionPagination page={page} total={positionsTotal} onChange={setPage} />
        </div>
    );
}

function WalletList({
    apiBaseUrl,
    onSelect,
    onAdd,
    refreshInterval = 10,
    watchedWalletSet = new Set(),
    watchToggleMap = {},
    onToggleWatchWallet,
}) {
    const [wallets, setWallets] = useState([]);
    const [walletsTotal, setWalletsTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [page, setPage] = useState(1);
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [editingWallet, setEditingWallet] = useState(null);
    const [zombieOpen, setZombieOpen] = useState(false);
    const [tokenLiquidityOpen, setTokenLiquidityOpen] = useState(false);
    const [zombieCandidates, setZombieCandidates] = useState([]);
    const [zombieSelected, setZombieSelected] = useState({});
    const loadSeqRef = useRef(0);
    const refreshIntervalMs = useMemo(
        () => getRefreshIntervalMs(refreshInterval),
        [refreshInterval]
    );
    const searchKeyword = useMemo(() => String(search || '').trim(), [search]);

    const load = useCallback((silent = false) => {
        const seq = ++loadSeqRef.current;
        if (!silent) setLoading(true);
        return fetchSMWallets({
            apiBaseUrl,
            page,
            size: WALLET_LIST_PAGE_SIZE,
            keyword: searchKeyword || undefined,
        })
            .then((d) => {
                if (seq !== loadSeqRef.current) return;
                const total = Number(d?.total || 0);
                const list = Array.isArray(d?.list) ? d.list : [];
                const totalPages = Math.max(1, Math.ceil(total / WALLET_LIST_PAGE_SIZE));
                if (page > totalPages) {
                    setPage(totalPages);
                    return;
                }
                setWallets(list);
                setWalletsTotal(total);
            })
            .catch(() => { })
            .finally(() => {
                if (!silent && seq === loadSeqRef.current) setLoading(false);
            });
    }, [apiBaseUrl, page, searchKeyword]);
    useEffect(() => { load(); }, [load]);
    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [load, refreshIntervalMs]);

    const runAction = async (key, action) => {
        setBusyKey(key);
        setActionError('');
        try {
            await action();
            await load();
        } catch (err) {
            setActionError(String(err?.message || err || '鎿嶄綔澶辫触'));
        } finally {
            setBusyKey('');
        }
    };

    const confirmDelete = async () => {
        if (!confirmState) return;
        const { key, action } = confirmState;
        setBusyKey(key);
        setActionError('');
        try {
            await action();
            await load();
            setConfirmState(null);
        } catch (err) {
            setConfirmState(null);
            setActionError(String(err?.message || err || '鎿嶄綔澶辫触'));
        } finally {
            setBusyKey('');
        }
    };

    const findZombieWallets = async () => {
        setBusyKey('wallet-zombies:find');
        setActionError('');
        try {
            const data = await fetchSMZombieWallets({ apiBaseUrl, days: 30 });
            const list = Array.isArray(data?.list) ? data.list : [];
            const selected = {};
            list.forEach((item) => {
                selected[`${item.address}:${item.chain_id}`] = true;
            });
            setZombieCandidates(list);
            setZombieSelected(selected);
            setZombieOpen(true);
        } catch (err) {
            setActionError(String(err?.message || err || '查找僵尸钱包失败'));
        } finally {
            setBusyKey('');
        }
    };

    const toggleZombieWallet = (key) => {
        setZombieSelected((prev) => ({ ...prev, [key]: !prev[key] }));
    };

    const toggleAllZombieWallets = (checked) => {
        const next = {};
        zombieCandidates.forEach((item) => {
            next[`${item.address}:${item.chain_id}`] = checked;
        });
        setZombieSelected(next);
    };

    const deleteSelectedZombieWallets = async () => {
        const walletsToDelete = zombieCandidates
            .filter((item) => zombieSelected[`${item.address}:${item.chain_id}`])
            .map((item) => ({ address: item.address, chain_id: item.chain_id }));
        if (walletsToDelete.length === 0) return;
        setBusyKey('wallet-zombies:delete');
        setActionError('');
        try {
            await deleteSMZombieWallets({ apiBaseUrl, wallets: walletsToDelete });
            setZombieOpen(false);
            setZombieCandidates([]);
            setZombieSelected({});
            await load();
        } catch (err) {
            setActionError(String(err?.message || err || '删除僵尸钱包失败'));
        } finally {
            setBusyKey('');
        }
    };

    return (
        <div>
            <div className="smd-search-row">
                <div className="smd-search-input">
                    <Search size={14} />
                    <input
                        placeholder="搜索钱包..."
                        value={search}
                        onChange={e => {
                            setSearch(e.target.value);
                            setPage(1);
                        }}
                    />
                </div>
                <button
                    type="button"
                    onClick={() => setTokenLiquidityOpen(true)}
                    className="smd-add-btn smd-token-liquidity-btn"
                    title="按代币扫描大额加池钱包"
                >
                    <Radar size={14} /> 聪明钱雷达
                </button>
                <button
                    type="button"
                    onClick={findZombieWallets}
                    disabled={busyKey === 'wallet-zombies:find' || busyKey === 'wallet-zombies:delete'}
                    className="smd-add-btn smd-zombie-btn"
                    title="查找最近 30 天没有 LP 开仓或撤仓的钱包"
                >
                    <Activity size={14} /> 僵尸钱包
                </button>
                <button onClick={onAdd} className="smd-add-btn">
                    <Plus size={14} /> 添加钱包
                </button>
            </div>
            {actionError ? <div className="smd-inline-error">{actionError}</div> : null}
            {loading ? <div className="smd-loading">加载中...</div> : wallets.length === 0 ? (
                <div className="smd-empty">暂无监控钱包，点击"添加钱包"开始</div>
            ) : (
                <div className="smd-table-wrap">
                    <table className="smd-table smd-table--wallets">
                        <thead>
                            <tr>
                                <th>钱包</th>
                                <th className="center">状态</th>
                                <th className="right">持仓</th>
                                <th className="right">池子</th>
                                <th className="right">操作</th>
                            </tr>
                        </thead>
                        <tbody>
                            {wallets.map(w => {
                                const normalizedAddress = normalizeWalletAddress(w.address) || w.address;
                                return (
                                <tr key={w.address} className="clickable" onClick={() => onSelect(w.address)}>
                                    <td>
                                        <WalletIdentity
                                            address={w.address}
                                            color={w.color}
                                            label={w.label || w.address}
                                            avatarUrl={w.avatar_url}
                                            source={w.source}
                                            sourceContract={w.source_contract}
                                            size={20}
                                            showCopy
                                            showSource
                                        />
                                    </td>
                                    <td className="center">
                                        <span className={`smd-status-dot ${w.is_active ? 'green' : 'muted'}`}>
                                            {w.is_active ? '监控中' : '已暂停'}
                                        </span>
                                    </td>
                                    <td className="right">{w.open_position_count}</td>
                                    <td className="right">{w.active_pool_count}</td>
                                    <td className="right">
                                        <div className="smd-action-row" style={{ justifyContent: 'flex-end' }}>
                                            <button
                                                type="button"
                                                className="smd-icon-btn"
                                                style={watchedWalletSet.has(normalizedAddress) ? {
                                                    color: '#ff5d73',
                                                    borderColor: 'rgba(255, 93, 115, 0.35)',
                                                    background: 'rgba(255, 93, 115, 0.08)',
                                                } : undefined}
                                                disabled={Boolean(watchToggleMap[normalizedAddress])}
                                                title={watchedWalletSet.has(normalizedAddress) ? '取消特别关注' : '加入特别关注'}
                                                onClick={e => {
                                                    e.stopPropagation();
                                                    onToggleWatchWallet?.(w.address);
                                                }}
                                            >
                                                {watchToggleMap[normalizedAddress] ? '…' : (watchedWalletSet.has(normalizedAddress) ? '♥' : '♡')}
                                            </button>
                                            <button type="button" className="smd-icon-btn" disabled={busyKey === `wallet-toggle:${w.address}` || busyKey === `wallet-delete:${w.address}`} onClick={e => {
                                                e.stopPropagation();
                                                setEditingWallet(w);
                                            }}><Pencil size={14} /></button>
                                            <button type="button" className="smd-icon-btn" disabled={busyKey === `wallet-toggle:${w.address}` || busyKey === `wallet-delete:${w.address}`} onClick={e => {
                                                e.stopPropagation();
                                                runAction(`wallet-toggle:${w.address}`, () => updateSMWallet({ apiBaseUrl, address: w.address, updates: { is_active: !w.is_active } }));
                                            }}>{w.is_active ? <Pause size={14} /> : <Play size={14} />}</button>
                                            <button type="button" className="smd-icon-btn danger" disabled={busyKey === `wallet-delete:${w.address}` || busyKey === `wallet-toggle:${w.address}`} onClick={e => {
                                                e.stopPropagation();
                                                setActionError('');
                                                setConfirmState({
                                                    key: `wallet-delete:${w.address}`,
                                                    title: '删除钱包',
                                                    description: `确认删除钱包 ${shortAddr(w.address)} 吗？该钱包的聪明钱历史数据也会删除。`,
                                                    action: () => deleteSMWallet({ apiBaseUrl, address: w.address }),
                                                });
                                            }}><Trash2 size={14} /></button>
                                        </div>
                                    </td>
                                </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            )}
            <PositionPagination page={page} total={walletsTotal} pageSize={WALLET_LIST_PAGE_SIZE} onChange={setPage} />
            <ConfirmDialog
                open={Boolean(confirmState)}
                title={confirmState?.title || '确认操作'}
                description={confirmState?.description || ''}
                confirmLabel="删除"
                busy={busyKey.startsWith('wallet-delete:')}
                onCancel={() => { if (!busyKey.startsWith('wallet-delete:')) setConfirmState(null); }}
                onConfirm={confirmDelete}
            />
            <ZombieWalletModal
                open={zombieOpen}
                candidates={zombieCandidates}
                selectedMap={zombieSelected}
                busy={busyKey === 'wallet-zombies:delete'}
                onToggle={toggleZombieWallet}
                onToggleAll={toggleAllZombieWallets}
                onClose={() => {
                    if (busyKey !== 'wallet-zombies:delete') setZombieOpen(false);
                }}
                onDelete={deleteSelectedZombieWallets}
            />
            <TokenLiquidityImportModal
                open={tokenLiquidityOpen}
                apiBaseUrl={apiBaseUrl}
                onClose={() => setTokenLiquidityOpen(false)}
                onImported={async () => {
                    await load();
                }}
            />
            <EditWalletModal
                open={Boolean(editingWallet)}
                apiBaseUrl={apiBaseUrl}
                wallet={editingWallet}
                onClose={() => setEditingWallet(null)}
                onSaved={async () => {
                    await load();
                    setEditingWallet(null);
                }}
            />
        </div>
    );
}

function WalletDetail({
    apiBaseUrl,
    addr,
    onBack,
    onSelectPool,
    refreshInterval = 10,
    watchedWalletSet = new Set(),
    watchToggleMap = {},
    onToggleWatchWallet,
}) {
    const [positions, setPositions] = useState([]);
    const [positionsTotal, setPositionsTotal] = useState(0);
    const [info, setInfo] = useState(null);
    const [status, setStatus] = useState('open');
    const [page, setPage] = useState(1);
    const [loading, setLoading] = useState(true);
    const [selectedPosition, setSelectedPosition] = useState(null);
    const refreshIntervalMs = useMemo(
        () => getRefreshIntervalMs(refreshInterval),
        [refreshInterval]
    );
    const positionPreviews = useSmartMoneyPositionPreviewMap(apiBaseUrl, positions);
    const loadInfo = useCallback(() => (
        fetchSMStats({ apiBaseUrl, address: addr }).then(setInfo).catch(() => { })
    ), [apiBaseUrl, addr]);

    const loadPositions = useCallback((silent = false) => {
        if (!silent) setLoading(true);
        return fetchSMPositions({
            apiBaseUrl,
            wallet: addr,
            status,
            page,
            size: POSITION_LIST_PAGE_SIZE,
            orderBy: 'position_amount_desc',
        })
            .then((d) => {
                setPositions(d?.list || []);
                setPositionsTotal(Number(d?.total || 0));
            })
            .catch(() => { })
            .finally(() => {
                if (!silent) setLoading(false);
            });
    }, [apiBaseUrl, addr, page, status]);

    useEffect(() => {
        setPage(1);
    }, [addr, status]);

    useEffect(() => {
        loadInfo();
    }, [loadInfo]);

    useEffect(() => {
        loadPositions();
    }, [loadPositions]);

    useEffect(() => {
        const timer = setInterval(() => {
            loadInfo();
            loadPositions(true);
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [loadInfo, loadPositions, refreshIntervalMs]);

    useEffect(() => {
        if (!selectedPosition) return;
        const selectedKey = getPositionSelectionKey(selectedPosition);
        if (positions.some((pos) => getPositionSelectionKey(pos) === selectedKey)) return;
        setSelectedPosition(null);
    }, [positions, selectedPosition]);
    const selectedPositionKey = selectedPosition ? getPositionSelectionKey(selectedPosition) : '';

    const groups = useMemo(() => {
        const m = {};
        (positions || []).forEach(p => {
            if (!m[p.pool_address]) m[p.pool_address] = {
                pool_address: p.pool_address,
                token0_symbol: p.token0_symbol,
                token1_symbol: p.token1_symbol,
                trading_pair: p.trading_pair,
                display_token_address: p.display_token_address,
                display_token_symbol: p.display_token_symbol,
                display_token_logo_url: p.display_token_logo_url,
                fee_tier: p.fee_tier,
                protocol: p.protocol,
                positions: [],
                hasOpen: false,
            };
            m[p.pool_address].positions.push(p);
            if (p.status === 'open') m[p.pool_address].hasOpen = true;
        });
        return Object.values(m).sort((a, b) => (a.hasOpen ? -1 : 1) - (b.hasOpen ? -1 : 1));
    }, [positions]);
    const normalizedAddr = normalizeWalletAddress(addr) || addr;

    return (
        <div>
            <button onClick={onBack} className="smd-back-btn">
                <ChevronLeft size={14} />
                <span>返回钱包列表</span>
            </button>
            {info && (
                <div className="smd-detail-card" style={{ marginBottom: 16 }}>
                    <div className="smd-detail-header">
                        <WalletAvatar address={addr} color={info.color || '#7F77DD'} avatarUrl={info.avatar_url} size={72} />
                        <div className="smd-detail-copy">
                            <h3 className="smd-detail-title">{info.label || `钱包 ${tailAddr(addr)}`}</h3>
                            <CompactIdentifier value={addr} label="钱包" />
                            <div className="smd-pool-card-badges" style={{ marginTop: 8 }}>
                                <Badge cls={walletSourceBadgeClass(info.source)} title={walletSourceContractLabel(info.source_contract) || walletSourceLabel(info.source)}>
                                    {walletSourceLabel(info.source)}
                                </Badge>
                                {walletSourceContractLabel(info.source_contract) ? (
                                    <Badge>{walletSourceContractLabel(info.source_contract)}</Badge>
                                ) : null}
                            </div>
                            <div style={{ marginTop: 10 }}>
                                <button
                                    type="button"
                                    className="smd-icon-btn"
                                    style={watchedWalletSet.has(normalizedAddr) ? {
                                        color: '#ff5d73',
                                        borderColor: 'rgba(255, 93, 115, 0.35)',
                                        background: 'rgba(255, 93, 115, 0.08)',
                                    } : undefined}
                                    disabled={Boolean(watchToggleMap[normalizedAddr])}
                                    onClick={() => onToggleWatchWallet?.(addr)}
                                >
                                    {watchToggleMap[normalizedAddr] ? '处理中...' : (watchedWalletSet.has(normalizedAddr) ? '♥ 已特别关注' : '♡ 加入特别关注')}
                                </button>
                            </div>
                        </div>
                    </div>
                    <div className="smd-stats-grid">
                        <StatCard label="钱包余额" value={formatWalletBalance(info.wallet_balance_usd)} />
                        <StatCard label="持仓笔数" value={info.open_position_count} />
                        <StatCard label="活跃池子" value={info.active_pool_count} />
                        <StatCard label="总加仓次数" value={info.total_add_count} />
                        <StatCard label="总减仓次数" value={info.total_remove_count} />
                    </div>
                </div>
            )}

			<div className="smd-section-header">
                <h4 className="smd-section-title">按池子分组</h4>
                <div className="smd-filter-group">
                    {['open', 'all'].map(s => (
                        <button key={s} className={`smd-filter-btn${status === s ? ' active' : ''}`} onClick={() => setStatus(s)}>
                            {s === 'open' ? '持仓中' : '全部'}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? <div className="smd-loading">加载中...</div> : groups.length === 0 ? (
                <div className="smd-empty">暂未检测到 LP 活动</div>
            ) : (
                <>
                    {groups.map(g => (
                        <div key={g.pool_address} className={`smd-pool-group${!g.hasOpen ? ' dim' : ''}`}>
                            <div className="smd-pool-group-header">
                                <div className="smd-pool-group-left">
                                    <div className="smd-pair-row smd-pair-row--group">
                                        <PairAvatar item={g} size="sm" />
                                        <span className="smd-pool-group-pair">{getPairLabel(g)}</span>
                                    </div>
                                    <CompactIdentifier value={g.pool_address} />
                                    {g.fee_tier && <Badge cls="fee">{formatFeeTier(g.fee_tier)}</Badge>}
                                    <ProtocolBadge protocol={g.protocol} />
                                    <span className="smd-pool-group-count">{g.positions.length} 个仓位</span>
                                </div>
                                <button className="smd-link smd-action-chip" onClick={() => onSelectPool({
                                    pool_address: g.pool_address, token0_symbol: g.token0_symbol, token1_symbol: g.token1_symbol, trading_pair: g.trading_pair, display_token_address: g.display_token_address, display_token_symbol: g.display_token_symbol, display_token_logo_url: g.display_token_logo_url, fee_tier: g.fee_tier, protocol: g.protocol,
                                })}>池子详情 <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} /></button>
                            </div>
                            <div className="smd-pos-list smd-pos-list--compact">
                                {g.positions.map(pos => {
                                    const positionKey = getPositionSelectionKey(pos) || String(pos.id || '');
                                    const isSelected = Boolean(positionKey) && positionKey === selectedPositionKey;
                                    return (
                                        <div key={positionKey || pos.id} className="smd-pos-row">
                                            <div className={`smd-pos-card smd-pos-card--compact${pos.status === 'closed' ? ' closed' : ''}`} onClick={() => setSelectedPosition(pos)}>
                                                <div className="smd-pos-card-compact-main">
                                                    <PositionAmountSummary
                                                        position={pos}
                                                        preview={positionPreviews[getPositionSelectionKey(pos)]}
                                                        compact
                                                    />
                                                    <span className={`smd-pos-card-prices${pos.status === 'closed' ? ' is-closed' : ''}`}>
                                                        {pos.price_lower && pos.price_upper ? `${pos.price_lower} - ${pos.price_upper}` : '--'}
                                                    </span>
                                                    {pos.range_percent > 0 && <span className="smd-pos-card-pct">{formatRangePercent(pos.range_percent)}</span>}
                                                </div>
                                                <PositionPreviewMetrics
                                                    position={pos}
                                                    preview={positionPreviews[getPositionSelectionKey(pos)]}
                                                    compact
                                                />
                                            </div>
                                            {isSelected ? (
                                                <SmartMoneyPositionDetailPanel
                                                    apiBaseUrl={apiBaseUrl}
                                                    position={selectedPosition}
                                                    onClose={() => setSelectedPosition(null)}
                                                />
                                            ) : null}
                                        </div>
                                    );
                                })}
                            </div>
                        </div>
                    ))}
                    <PositionPagination page={page} total={positionsTotal} onChange={setPage} />
                </>
            )}
        </div>
    );
}

function GoldenDogPanel({ apiBaseUrl, initData }) {
    return <GoldenDogPanelContent apiBaseUrl={apiBaseUrl} initData={initData} />;

    const hasInitData = Boolean(String(initData || '').trim());
    const [loading, setLoading] = useState(hasInitData);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [savedAt, setSavedAt] = useState('');
    const [status, setStatus] = useState(null);
    const [draft, setDraft] = useState({
        enabled: false,
        min_wallets: '3',
        window_minutes: '10',
        cooldown_minutes: '30',
    });

    const applyResponse = useCallback((resp) => {
        setStatus(resp || null);
        const cfg = resp?.config || {};
        setDraft({
            enabled: Boolean(cfg.enabled),
            min_wallets: String(cfg.min_wallets ?? 3),
            window_minutes: String(cfg.window_minutes ?? 10),
            cooldown_minutes: String(cfg.cooldown_minutes ?? 30),
        });
    }, []);

    const loadConfig = useCallback(async () => {
        if (!hasInitData) {
            setLoading(false);
            setStatus(null);
            return;
        }
        setLoading(true);
        setError('');
        try {
            applyResponse(await fetchSMGoldenDogConfig({ apiBaseUrl, initData, chain: 'bsc' }));
        } catch (err) {
            setError(String(err?.message || err || '加载失败'));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, applyResponse, hasInitData, initData]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    const barkStatusText = useMemo(() => {
        if (status?.bark_ready) return '已就绪';
        if (status?.bark_configured) return status?.bark_enabled ? '已配置未就绪' : '已配置未开启';
        return '未配置';
    }, [status]);

    const handleSave = useCallback(async () => {
        if (!hasInitData) {
            setError('请先登录 WebApp，拿到 initData 后才能保存监控通知。');
            return;
        }

        const minWallets = Number.parseInt(String(draft.min_wallets || '').trim(), 10);
        const windowMinutes = Number.parseInt(String(draft.window_minutes || '').trim(), 10);
        const cooldownMinutes = Number.parseInt(String(draft.cooldown_minutes || '').trim(), 10);
        if (!Number.isFinite(minWallets) || minWallets < 1) {
            setError('钱包数量必须大于等于 1。');
            return;
        }
        if (!Number.isFinite(windowMinutes) || windowMinutes < 1) {
            setError('统计窗口必须大于等于 1 分钟。');
            return;
        }
        if (!Number.isFinite(cooldownMinutes) || cooldownMinutes < 0) {
            setError('冷却时间不能小于 0。');
            return;
        }

        setSaving(true);
        setError('');
        setSavedAt('');
        try {
            const resp = await saveSMGoldenDogConfig({
                apiBaseUrl,
                initData,
                chain: 'bsc',
                config: {
                    enabled: Boolean(draft.enabled),
                    min_wallets: minWallets,
                    window_minutes: windowMinutes,
                    cooldown_minutes: cooldownMinutes,
                },
            });
            applyResponse(resp);
            setSavedAt('配置已保存');
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, applyResponse, draft, hasInitData, initData]);

    return (
        <div>
            <div className="smd-detail-card" style={{
                marginBottom: 16,
                padding: 18,
                border: '1px solid rgba(251, 191, 36, 0.18)',
                background: 'radial-gradient(circle at top left, rgba(251, 191, 36, 0.16), transparent 34%), linear-gradient(180deg, rgba(24, 24, 27, 0.94), rgba(9, 9, 11, 0.98))',
                boxShadow: '0 28px 90px -42px rgba(0, 0, 0, 0.95)',
            }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
                    <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, minWidth: 0 }}>
                        <div style={{
                            width: 42,
                            height: 42,
                            borderRadius: 16,
                            display: 'inline-flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            border: '1px solid rgba(251, 191, 36, 0.2)',
                            background: 'rgba(251, 191, 36, 0.10)',
                            color: '#fcd34d',
                            flexShrink: 0,
                        }}>
                            <Flame size={18} />
                        </div>
                        <div style={{ minWidth: 0 }}>
                            <div className="smd-section-title" style={{ marginBottom: 8 }}>监控通知</div>
                            <div className="smd-pool-card-badges">
                                <Badge style={draft.enabled
                                    ? { borderColor: 'rgba(251, 191, 36, 0.2)', background: 'rgba(251, 191, 36, 0.12)', color: '#fde68a' }
                                    : undefined}>
                                    {draft.enabled ? '已开启' : '已关闭'}
                                </Badge>
                                <Badge>Bark {barkStatusText}</Badge>
                                <Badge>BSC</Badge>
                            </div>
                        </div>
                    </div>
                    <div className="smd-filter-group">
                        <button
                            type="button"
                            className={`smd-filter-btn${draft.enabled ? ' active' : ''}`}
                            onClick={() => setDraft((prev) => ({ ...prev, enabled: true }))}
                        >
                            开启
                        </button>
                        <button
                            type="button"
                            className={`smd-filter-btn${!draft.enabled ? ' active' : ''}`}
                            onClick={() => setDraft((prev) => ({ ...prev, enabled: false }))}
                        >
                            关闭
                        </button>
                    </div>
                </div>
                <div className="smd-stats-grid" style={{ marginTop: 16, marginBottom: 0 }}>
                    <StatCard label="通知状态" value={draft.enabled ? '运行中' : '暂停'} />
                    <StatCard label="Bark 状态" value={barkStatusText} />
                    <StatCard label="钱包阈值" value={`${draft.min_wallets || '--'} 个`} />
                    <StatCard label="统计窗口" value={`${draft.window_minutes || '--'} 分钟`} />
                </div>
            </div>

            {!hasInitData ? (
                <div className="smd-inline-error">
                    Web 端需要先登录 Telegram 才能保存提醒配置。Bark Key 继续复用全局配置，不在这里单独设置。
                </div>
            ) : null}
            {error ? <div className="smd-inline-error">{error}</div> : null}
            {!error && savedAt ? (
                <div className="smd-inline-error" style={{ color: '#86efac', borderColor: 'rgba(34,197,94,0.28)', background: 'rgba(34,197,94,0.10)' }}>
                    {savedAt}
                </div>
            ) : null}

            {loading ? <div className="smd-loading">加载中...</div> : (
                <div className="smd-add-form" style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 12, alignItems: 'end' }}>
                    <label style={{ display: 'grid', gap: 6 }}>
                        <span className="muted">钱包数量</span>
                        <input
                            type="number"
                            min="1"
                            step="1"
                            value={draft.min_wallets}
                            onChange={(e) => setDraft((prev) => ({ ...prev, min_wallets: e.target.value }))}
                        />
                    </label>
                    <label style={{ display: 'grid', gap: 6 }}>
                        <span className="muted">统计窗口(分钟)</span>
                        <input
                            type="number"
                            min="1"
                            step="1"
                            value={draft.window_minutes}
                            onChange={(e) => setDraft((prev) => ({ ...prev, window_minutes: e.target.value }))}
                        />
                    </label>
                    <label style={{ display: 'grid', gap: 6 }}>
                        <span className="muted">冷却时间(分钟)</span>
                        <input
                            type="number"
                            min="0"
                            step="1"
                            value={draft.cooldown_minutes}
                            onChange={(e) => setDraft((prev) => ({ ...prev, cooldown_minutes: e.target.value }))}
                        />
                    </label>
                    <button
                        type="button"
                        disabled={saving || !hasInitData}
                        onClick={handleSave}
                        style={{ gridColumn: '1 / -1' }}
                    >
                        {saving ? '保存中...' : '保存监控通知配置'}
                    </button>
                </div>
            )}
        </div>
    );
}

function SettingsPanel({ apiBaseUrl }) {
    const [contracts, setContracts] = useState([]);
    const [loading, setLoading] = useState(true);
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [editingContract, setEditingContract] = useState(null);
    const [showAdd, setShowAdd] = useState(false);
    const [newAddr, setNewAddr] = useState('');
    const [newDesc, setNewDesc] = useState('');

    const loadContracts = useCallback(async () => {
        const d = await fetchSMContracts({ apiBaseUrl });
        setContracts(d?.list || []);
        return d;
    }, [apiBaseUrl]);

    useEffect(() => {
        setLoading(true);
        loadContracts()
            .catch((err) => setActionError(String(err?.message || err || '加载失败')))
            .finally(() => setLoading(false));
    }, [loadContracts]);

    const runAction = useCallback(async (key, action, refresh) => {
        setBusyKey(key);
        setActionError('');
        try {
            await action();
            await refresh();
        } catch (err) {
            setActionError(String(err?.message || err || '操作失败'));
        } finally {
            setBusyKey('');
        }
    }, []);

    const handleAddContract = async () => {
        await runAction('add-contract', async () => {
            const addr = String(newAddr || '').trim();
            if (!isHexAddressValue(addr)) {
                throw new Error('请输入合法的合约地址');
            }
            await addSMContract({ apiBaseUrl, contract_address: addr, description: newDesc });
            setShowAdd(false);
            setNewAddr('');
            setNewDesc('');
        }, loadContracts);
    };

    const confirmAction = async () => {
        if (!confirmState) return;
        const { key, action, refresh } = confirmState;
        setBusyKey(key);
        setActionError('');
        try {
            await action();
            await refresh();
            setConfirmState(null);
        } catch (err) {
            setConfirmState(null);
            setActionError(String(err?.message || err || '操作失败'));
        } finally {
            setBusyKey('');
        }
    };

    const openDeleteConfirm = ({ key, title, description, action, refresh }) => {
        setActionError('');
        setConfirmState({ key, title, description, action, refresh });
    };

    const addBusy = busyKey === 'add-contract';
    const deleteBusy = busyKey.startsWith('contract-delete:');

    return (
        <div>
            <div className="smd-search-row">
                <div className="smd-section-title">合约管理</div>
                <button type="button" onClick={() => setShowAdd(!showAdd)} className="smd-add-btn" style={{ marginLeft: 'auto' }}>
                    <Plus size={14} /> 添加合约
                </button>
            </div>

            {actionError ? <div className="smd-inline-error">{actionError}</div> : null}

            {showAdd && (
                <div className="smd-add-form">
                    <input placeholder="合约地址" value={newAddr} onChange={e => setNewAddr(e.target.value)} />
                    <input className="w-sm" placeholder="描述" value={newDesc} onChange={e => setNewDesc(e.target.value)} />
                    <div className="smd-add-form-hint">只需要填写监控合约地址，添加后会直接扫描发往该地址的交易。</div>
                    <button type="button" disabled={addBusy} onClick={handleAddContract}>
                        {addBusy ? '处理中...' : '添加'}
                    </button>
                </div>
            )}

            {loading ? <div className="smd-loading">加载中...</div> : (
                <div className="smd-table-wrap">
                    <table className="smd-table smd-table--settings">
                        <thead><tr>
                            <th>地址</th>
                            <th>描述</th>
                            <th className="center">状态</th>
                            <th className="right">已扫描至区块</th>
                            <th className="right">操作</th>
                        </tr></thead>
                        <tbody>
                            {contracts.map(c => (
                                <tr key={c.contract_address}>
                                    <td className="mono">{shortAddr(c.contract_address)}</td>
                                    <td className="muted">{c.description || '-'}</td>
                                    <td className="center"><span className={`smd-status-dot ${c.is_active ? 'green' : 'muted'}`}>{c.is_active ? '活跃' : '已暂停'}</span></td>
                                    <td className="right mono muted">{c.last_scanned_block || '未扫描'}</td>
                                    <td className="right">
                                        <div className="smd-action-row" style={{ justifyContent: 'flex-end' }}>
                                            <button
                                                type="button"
                                                className="smd-icon-btn"
                                                disabled={busyKey === `contract-toggle:${c.contract_address}` || busyKey === `contract-delete:${c.contract_address}`}
                                                onClick={() => setEditingContract(c)}
                                            >
                                                <Pencil size={14} />
                                            </button>
                                            <button
                                                type="button"
                                                className="smd-icon-btn"
                                                disabled={busyKey === `contract-toggle:${c.contract_address}` || busyKey === `contract-delete:${c.contract_address}`}
                                                onClick={() => runAction(`contract-toggle:${c.contract_address}`, () => updateSMContract({ apiBaseUrl, address: c.contract_address, updates: { is_active: !c.is_active } }), loadContracts)}
                                            >
                                                {c.is_active ? <Pause size={14} /> : <Play size={14} />}
                                            </button>
                                            <button
                                                type="button"
                                                className="smd-icon-btn danger"
                                                disabled={busyKey === `contract-delete:${c.contract_address}` || busyKey === `contract-toggle:${c.contract_address}`}
                                                onClick={() => openDeleteConfirm({
                                                    key: `contract-delete:${c.contract_address}`,
                                                    title: '删除合约',
                                                    description: `确认删除合约 ${shortAddr(c.contract_address)} 吗？`,
                                                    action: () => deleteSMContract({ apiBaseUrl, address: c.contract_address }),
                                                    refresh: loadContracts,
                                                })}
                                            >
                                                <Trash2 size={14} />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            <ConfirmDialog
                open={Boolean(confirmState)}
                title={confirmState?.title || '确认操作'}
                description={confirmState?.description || ''}
                confirmLabel="删除"
                busy={deleteBusy}
                onCancel={() => { if (!deleteBusy) setConfirmState(null); }}
                onConfirm={confirmAction}
            />
            <EditContractModal
                open={Boolean(editingContract)}
                apiBaseUrl={apiBaseUrl}
                contract={editingContract}
                onClose={() => setEditingContract(null)}
                onSaved={async () => {
                    await loadContracts();
                    setEditingContract(null);
                }}
            />
        </div>
    );
}

function EditWalletModal({ open, apiBaseUrl, wallet, onClose, onSaved }) {
    const [label, setLabel] = useState('');
    const [avatarFile, setAvatarFile] = useState(null);
    const [removeAvatar, setRemoveAvatar] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [avatarPreviewUrl, setAvatarPreviewUrl] = useState('');

    useEffect(() => {
        if (!open || !wallet) return;
        setLabel(String(wallet?.label || ''));
        setAvatarFile(null);
        setRemoveAvatar(false);
        setSaving(false);
        setError('');
    }, [open, wallet]);

    useEffect(() => {
        if (!open || !wallet) {
            setAvatarPreviewUrl('');
            return undefined;
        }
        if (avatarFile) {
            const objectUrl = URL.createObjectURL(avatarFile);
            setAvatarPreviewUrl(objectUrl);
            return () => URL.revokeObjectURL(objectUrl);
        }
        setAvatarPreviewUrl(removeAvatar ? '' : String(wallet?.avatar_url || ''));
        return undefined;
    }, [avatarFile, open, removeAvatar, wallet]);

    const handleAvatarFileChange = useCallback((event) => {
        const nextFile = event.target.files?.[0];
        event.target.value = '';
        if (!nextFile) return;
        if (!['image/png', 'image/jpeg', 'image/webp'].includes(String(nextFile.type || '').toLowerCase())) {
            setError('头像仅支持 PNG、JPG、WEBP。');
            return;
        }
        if (nextFile.size > SMART_MONEY_AVATAR_MAX_BYTES) {
            setError('头像大小不能超过 5MB。');
            return;
        }
        setAvatarFile(nextFile);
        setRemoveAvatar(false);
        setError('');
    }, []);

    const handleSubmit = useCallback(async () => {
        if (!wallet?.address) return;
        setSaving(true);
        setError('');
        try {
            const nextLabel = String(label || '').trim();
            const currentLabel = String(wallet?.label || '').trim();
            const shouldClearAvatar = !avatarFile && removeAvatar && String(wallet?.avatar_url || '').trim();

            if (avatarFile) {
                await uploadSMWalletAvatar({
                    apiBaseUrl,
                    address: wallet.address,
                    file: avatarFile,
                });
            }

            const updates = {};
            if (nextLabel !== currentLabel) {
                updates.label = nextLabel || null;
            }
            if (shouldClearAvatar) {
                updates.avatar_url = null;
            }

            if (Object.keys(updates).length > 0) {
                await updateSMWallet({
                    apiBaseUrl,
                    address: wallet.address,
                    updates,
                });
            }
            await onSaved?.();
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, avatarFile, label, onSaved, removeAvatar, wallet]);

    if (!open || !wallet) return null;

    return (
        <div className="smd-modal-overlay" onClick={saving ? undefined : onClose}>
            <div className="smd-modal" onClick={(e) => e.stopPropagation()}>
                <div className="smd-modal-header">
                    <h3 className="smd-modal-title">编辑钱包</h3>
                    <button type="button" onClick={onClose} disabled={saving} className="smd-modal-close">
                        <X size={18} />
                    </button>
                </div>
                <div style={{ marginBottom: 12 }}>
                    <CompactIdentifier value={wallet.address} label="钱包" />
                </div>
                {error ? <div className="smd-inline-error">{error}</div> : null}
                <div className="smd-modal-form">
                    <div className="smd-wallet-avatar-editor">
                        <WalletAvatar
                            address={wallet.address}
                            color={wallet.color || '#7F77DD'}
                            avatarUrl={avatarPreviewUrl}
                            size={68}
                        />
                        <div className="smd-wallet-avatar-controls">
                            <div className="smd-avatar-upload-row">
                                <label className="smd-avatar-upload-label">
                                    <input
                                        type="file"
                                        accept={SMART_MONEY_AVATAR_ACCEPT}
                                        disabled={saving}
                                        onChange={handleAvatarFileChange}
                                    />
                                    上传头像
                                </label>
                                <button
                                    type="button"
                                    className="smd-avatar-reset-btn"
                                    disabled={saving || (!avatarFile && !String(wallet?.avatar_url || '').trim())}
                                    onClick={() => {
                                        setAvatarFile(null);
                                        setRemoveAvatar(true);
                                        setError('');
                                    }}
                                >
                                    恢复默认
                                </button>
                            </div>
                            <div className="smd-avatar-upload-hint">支持 PNG/JPG/WEBP，大小不超过 5MB。</div>
                            {avatarFile ? <div className="smd-avatar-upload-name">{avatarFile.name}</div> : null}
                        </div>
                    </div>
                    <input placeholder="钱包标签" value={label} onChange={(e) => setLabel(e.target.value)} />
                </div>
                <div className="smd-modal-actions">
                    <button type="button" onClick={onClose} disabled={saving} className="smd-modal-cancel">取消</button>
                    <button type="button" onClick={handleSubmit} disabled={saving} className="smd-modal-submit">
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
}

function EditContractModal({ open, apiBaseUrl, contract, onClose, onSaved }) {
    const [description, setDescription] = useState('');
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');

    useEffect(() => {
        if (!open || !contract) return;
        setDescription(String(contract?.description || ''));
        setSaving(false);
        setError('');
    }, [contract, open]);

    const handleSubmit = useCallback(async () => {
        if (!contract?.contract_address) return;
        setSaving(true);
        setError('');
        try {
            await updateSMContract({
                apiBaseUrl,
                address: contract.contract_address,
                updates: { description: String(description || '').trim() || null },
            });
            await onSaved?.();
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, contract, description, onSaved]);

    if (!open || !contract) return null;

    return (
        <div className="smd-modal-overlay" onClick={saving ? undefined : onClose}>
            <div className="smd-modal" onClick={(e) => e.stopPropagation()}>
                <div className="smd-modal-header">
                    <h3 className="smd-modal-title">编辑合约</h3>
                    <button type="button" onClick={onClose} disabled={saving} className="smd-modal-close">
                        <X size={18} />
                    </button>
                </div>
                <div style={{ marginBottom: 12 }}>
                    <CompactIdentifier value={contract.contract_address} label="合约" />
                </div>
                {error ? <div className="smd-inline-error">{error}</div> : null}
                <div className="smd-modal-form">
                    <textarea
                        placeholder="合约备注"
                        rows={4}
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                    />
                </div>
                <div className="smd-modal-actions">
                    <button type="button" onClick={onClose} disabled={saving} className="smd-modal-cancel">取消</button>
                    <button type="button" onClick={handleSubmit} disabled={saving} className="smd-modal-submit">
                        {saving ? '保存中...' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
}

// ---- Main ----

export default function SmartMoneyDashboard({
    apiBaseUrl,
    initData = '',
    watchedWallets = [],
    watchedWalletSet = new Set(),
    watchToggleMap = {},
    onToggleWatchWallet,
    onSelectPool,
    activePoolAddress = '',
    refreshInterval = 10,
    onOpenPosition,
    isAdmin = false,
}) {
    const [view, setView] = useState('pools');
    const [stats, setStats] = useState(null);
    const [selectedPool, setSelectedPool] = useState(null);
    const [selectedWallet, setSelectedWallet] = useState(null);
    const [showAddModal, setShowAddModal] = useState(false);
    const refreshIntervalMs = useMemo(
        () => getRefreshIntervalMs(refreshInterval),
        [refreshInterval]
    );

    const loadStats = useCallback(() => (
        fetchSMStats({ apiBaseUrl }).then(setStats).catch(() => { })
    ), [apiBaseUrl]);

    useEffect(() => {
        loadStats();
    }, [loadStats]);

    useEffect(() => {
        const timer = setInterval(() => {
            loadStats();
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [loadStats, refreshIntervalMs]);

    useEffect(() => {
        if (!isAdmin && view === 'assets') {
            setView('pools');
        }
    }, [isAdmin, view]);

    const isDetail = selectedPool || selectedWallet;
    const handlePoolCardSelect = useCallback((pool) => {
        const nextPool = { ...pool, chain: resolvePoolChain(pool) };
        if (typeof onSelectPool === 'function') {
            onSelectPool(nextPool);
            return;
        }
        setSelectedPool(nextPool);
        setSelectedWallet(null);
    }, [onSelectPool]);
    const handleOpenPoolDetail = useCallback((pool) => {
        setSelectedPool({ ...pool, chain: resolvePoolChain(pool) });
        setSelectedWallet(null);
    }, []);
    return (
        <SmartMoneyShell
            stats={stats}
            isDetail={Boolean(isDetail)}
            isAdmin={isAdmin}
            view={view}
            onViewChange={setView}
        >
                {selectedPool ? (
                    <PoolDetail
                        apiBaseUrl={apiBaseUrl}
                        pool={selectedPool}
                        onBack={() => setSelectedPool(null)}
                        onSelectWallet={addr => { setSelectedWallet(addr); setView('wallets'); setSelectedPool(null); }}
                        refreshInterval={refreshInterval}
                    />
                ) : selectedWallet ? (
                    <WalletDetail
                        apiBaseUrl={apiBaseUrl}
                        addr={selectedWallet}
                        onBack={() => setSelectedWallet(null)}
                        onSelectPool={p => { setSelectedPool(p); setSelectedWallet(null); }}
                        refreshInterval={refreshInterval}
                        watchedWalletSet={watchedWalletSet}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={onToggleWatchWallet}
                    />
                ) : view === 'pools' ? (
                    <SmartMoneyPoolView
                        apiBaseUrl={apiBaseUrl}
                        onSelect={handlePoolCardSelect}
                        onOpenDetail={handleOpenPoolDetail}
                        activePoolAddress={activePoolAddress}
                        refreshInterval={refreshInterval}
                        onOpenPosition={onOpenPosition}
                    />
                ) : view === 'wallets' ? (
                    <WalletList
                        apiBaseUrl={apiBaseUrl}
                        onSelect={setSelectedWallet}
                        onAdd={() => setShowAddModal(true)}
                        refreshInterval={refreshInterval}
                        watchedWalletSet={watchedWalletSet}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={onToggleWatchWallet}
                    />
                ) : view === 'watch_activity' ? (
                    <WatchActivityPanel
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        watchedWallets={watchedWallets}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={onToggleWatchWallet}
                        onSelectWallet={addr => { setSelectedWallet(addr); setView('wallets'); setSelectedPool(null); }}
                        onSelectPool={p => { setSelectedPool(p); setSelectedWallet(null); }}
                        onOpenWallets={() => setView('wallets')}
                        refreshInterval={refreshInterval}
                    />
                ) : view === 'golden_dog' ? (
                    <GoldenDogPanelContent
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        watchedWallets={watchedWallets}
                        watchedWalletSet={watchedWalletSet}
                        watchToggleMap={watchToggleMap}
                        onToggleWatchWallet={onToggleWatchWallet}
                    />
                ) : view === 'auto_follow' ? (
                    <AutoFollowPanelContent
                        apiBaseUrl={apiBaseUrl}
                        initData={initData}
                        chain="bsc"
                        refreshInterval={refreshInterval}
                        active={view === 'auto_follow'}
                    />
                ) : view === 'assets' && isAdmin ? (
                    <React.Suspense fallback={<div className="panel-loading">正在加载聪明钱资产...</div>}>
                        <LazySmartMoneyAssetsPanel
                            apiBaseUrl={apiBaseUrl}
                            initData={initData}
                            hasInitData={Boolean(String(initData || '').trim())}
                            isAdmin={isAdmin}
                            refreshInterval={refreshInterval}
                        />
                    </React.Suspense>
                ) : (
                    <SettingsPanel apiBaseUrl={apiBaseUrl} />
                )}

                {showAddModal && (
                    <div className="smd-modal-overlay">
                        <div className="smd-modal">
                            <div className="smd-modal-header">
                                <h3 className="smd-modal-title">添加钱包</h3>
                                <button onClick={() => setShowAddModal(false)} className="smd-modal-close"><X size={18} /></button>
                            </div>
                            <AddWalletForm apiBaseUrl={apiBaseUrl} onDone={() => { setShowAddModal(false); }} />
                        </div>
                    </div>
                )}
        </SmartMoneyShell>
    );
}

function AddWalletForm({ apiBaseUrl, onDone }) {
    const [addr, setAddr] = useState('');
    const [label, setLabel] = useState('');
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    return (
        <div className="smd-modal-form">
            <input placeholder="钱包地址 (0x...)" value={addr} onChange={e => setAddr(e.target.value)} />
            <input placeholder="标签（可选）" value={label} onChange={e => setLabel(e.target.value)} />
            {error ? <div className="smd-inline-error">{error}</div> : null}
            <div className="smd-modal-actions">
                <button onClick={onDone} className="smd-modal-cancel">取消</button>
                <button disabled={!addr || saving} className="smd-modal-submit" onClick={async () => {
                    setSaving(true);
                    setError('');
                    try { await addSMWallet({ apiBaseUrl, address: addr, label }); onDone(); } catch (e) { setError(String(e?.message || e)); } finally { setSaving(false); }
                }}>{saving ? '添加中...' : '添加'}</button>
            </div>
        </div>
    );
}

const GOLDEN_DOG_INTENSITY_OPTIONS = [
    { value: 'ring', label: '响铃', description: '普通提醒' },
    { value: 'persistent_ring', label: '持续响铃', description: '持续提醒' },
    { value: 'critical_ring', label: '静音强提醒', description: '静音也响' },
];

const GOLDEN_DOG_INTENSITY_MODE_OPTIONS = [
    { value: 'fixed', label: '固定强度' },
    { value: 'amount_tiers', label: '按金额阶梯' },
];

const GOLDEN_DOG_DEFAULT_AMOUNT_TIERS = [
    { min_amount_usd: '1000', intensity: 'ring' },
    { min_amount_usd: '5000', intensity: 'persistent_ring' },
    { min_amount_usd: '20000', intensity: 'critical_ring' },
];

const GOLDEN_DOG_FEE_RATE_OPTIONS = [
    { value: '', label: '不限' },
    { value: '100', label: '0.0100%' },
    { value: '500', label: '0.0500%' },
    { value: '2500', label: '0.2500%' },
    { value: '3000', label: '0.3000%' },
    { value: '10000', label: '1.0000%' },
];

function createGoldenDogDraft() {
    return {
        wallet_mode: {
            enabled: false,
            min_wallets: '3',
            window_minutes: '10',
            cooldown_minutes: '30',
            min_total_amount_usd: '',
            intensity: 'ring',
            intensity_mode: 'fixed',
            amount_intensity_tiers: cloneGoldenDogDefaultAmountTiers(),
        },
        pool_mode: {
            enabled: false,
            cooldown_minutes: '30',
            min_total_fees: '',
            min_transaction_count: '',
            min_tvl: '',
            min_volume: '',
            min_fee_rate: '',
            min_active_liquidity_ratio: '',
            intensity: 'ring',
        },
    };
}

function cloneGoldenDogDefaultAmountTiers() {
    return GOLDEN_DOG_DEFAULT_AMOUNT_TIERS.map((tier) => ({ ...tier }));
}

function formatGoldenDogDraftValue(value, { emptyWhenZero = false, multiplier = 1 } = {}) {
    const num = Number(value);
    if (!Number.isFinite(num)) return emptyWhenZero ? '' : '0';
    const scaled = Number((num * multiplier).toFixed(4));
    if (emptyWhenZero && Math.abs(scaled) < 0.000001) return '';
    return String(scaled);
}

function mapGoldenDogAmountTiers(value) {
    let rows = value;
    if (typeof rows === 'string' && rows.trim()) {
        try {
            rows = JSON.parse(rows);
        } catch {
            rows = null;
        }
    }
    if (!Array.isArray(rows) || rows.length === 0) {
        return cloneGoldenDogDefaultAmountTiers();
    }
    const mapped = rows.slice(0, 3).map((tier, index) => ({
        min_amount_usd: formatGoldenDogDraftValue(tier?.min_amount_usd, { emptyWhenZero: false }),
        intensity: String(tier?.intensity || GOLDEN_DOG_DEFAULT_AMOUNT_TIERS[index]?.intensity || 'ring'),
    }));
    while (mapped.length < 3) {
        mapped.push({ ...GOLDEN_DOG_DEFAULT_AMOUNT_TIERS[mapped.length] });
    }
    return mapped;
}

function mapGoldenDogConfigToDraft(cfg) {
    const next = createGoldenDogDraft();
    const source = cfg || {};
    next.wallet_mode.enabled = Boolean(source.enabled);
    next.wallet_mode.min_wallets = String(source.min_wallets ?? 3);
    next.wallet_mode.window_minutes = String(source.window_minutes ?? 10);
    next.wallet_mode.cooldown_minutes = String(source.cooldown_minutes ?? 30);
    next.wallet_mode.min_total_amount_usd = formatGoldenDogDraftValue(source.wallet_min_total_amount_usd, { emptyWhenZero: true });
    next.wallet_mode.intensity = String(source.wallet_intensity || 'ring');
    next.wallet_mode.intensity_mode = String(source.wallet_intensity_mode || 'fixed');
    next.wallet_mode.amount_intensity_tiers = mapGoldenDogAmountTiers(source.wallet_amount_intensity_tiers);
    next.pool_mode.enabled = Boolean(source.pool_enabled);
    next.pool_mode.cooldown_minutes = String(source.pool_cooldown_minutes ?? 30);
    next.pool_mode.min_total_fees = formatGoldenDogDraftValue(source.pool_min_total_fees, { emptyWhenZero: true });
    next.pool_mode.min_transaction_count = formatGoldenDogDraftValue(source.pool_min_transaction_count, { emptyWhenZero: true });
    next.pool_mode.min_tvl = formatGoldenDogDraftValue(source.pool_min_tvl, { emptyWhenZero: true });
    next.pool_mode.min_volume = formatGoldenDogDraftValue(source.pool_min_volume, { emptyWhenZero: true });
    next.pool_mode.min_fee_rate = formatGoldenDogDraftValue(source.pool_min_fee_rate, { emptyWhenZero: true });
    next.pool_mode.min_active_liquidity_ratio = formatGoldenDogDraftValue(source.pool_min_active_liquidity_ratio, { emptyWhenZero: true });
    next.pool_mode.intensity = String(source.pool_intensity || 'ring');
    return next;
}

function parseGoldenDogRequiredInt(value, label, { min = 1 } = {}) {
    const num = Number.parseInt(String(value || '').trim(), 10);
    if (!Number.isFinite(num) || num < min) {
        throw new Error(`${label}必须大于等于 ${min}。`);
    }
    return num;
}

function parseGoldenDogOptionalNumber(value, label, { max = Number.MAX_SAFE_INTEGER } = {}) {
    const raw = String(value || '').trim();
    if (!raw) return 0;
    const num = Number(raw);
    if (!Number.isFinite(num) || num < 0) {
        throw new Error(`${label}必须是大于等于 0 的数字。`);
    }
    if (num > max) {
        throw new Error(`${label}不能大于 ${max}。`);
    }
    return num;
}

function parseGoldenDogAmountIntensityTiers(rows, intensityMode) {
    const parsed = (Array.isArray(rows) ? rows : [])
        .map((tier, index) => ({
            min_amount_usd: parseGoldenDogOptionalNumber(tier?.min_amount_usd, `第 ${index + 1} 档金额`),
            intensity: tier?.intensity || 'ring',
        }))
        .filter((tier) => tier.min_amount_usd > 0)
        .sort((a, b) => a.min_amount_usd - b.min_amount_usd);
    if (intensityMode === 'amount_tiers' && parsed.length === 0) {
        throw new Error('按金额阶梯推送至少需要填写一档金额。');
    }
    return parsed;
}

function goldenDogBarkStatusText(status) {
    if (status?.bark_ready) return '已就绪';
    if (status?.bark_configured) return status?.bark_enabled ? '已配置未就绪' : '已配置未开启';
    return '未配置';
}

function goldenDogIntensityLabel(value) {
    return GOLDEN_DOG_INTENSITY_OPTIONS.find((item) => item.value === value)?.label || '响铃';
}

function countGoldenDogPoolThresholds(poolMode) {
    return [
        'min_total_fees',
        'min_transaction_count',
        'min_tvl',
        'min_volume',
        'min_fee_rate',
        'min_active_liquidity_ratio',
    ].reduce((count, key) => count + (String(poolMode?.[key] || '').trim() ? 1 : 0), 0);
}

function goldenDogThresholdText(value, prefix = '', suffix = '') {
    const raw = String(value || '').trim();
    return raw ? `${prefix}${raw}${suffix}` : '--';
}

function goldenDogWalletTestIntensity(walletMode) {
    if (walletMode?.intensity_mode !== 'amount_tiers') {
        return walletMode?.intensity || 'ring';
    }
    const tiers = parseGoldenDogAmountIntensityTiers(walletMode?.amount_intensity_tiers, 'fixed');
    return tiers[tiers.length - 1]?.intensity || walletMode?.intensity || 'ring';
}

function createWatchOpenAlertDraft() {
    return {
        enabled: false,
        bark_enabled: false,
        sound_enabled: false,
    };
}

function mapWatchOpenAlertConfigToDraft(cfg) {
    const source = cfg || {};
    return {
        enabled: Boolean(source.enabled),
        bark_enabled: Boolean(source.bark_enabled),
        sound_enabled: Boolean(source.sound_enabled),
    };
}

function smartMoneyWatchOpenStatusText(draft) {
    if (draft?.enabled) return '运行中';
    return '已暂停';
}

let smartMoneyBeepAudioContext = null;

async function playSmartMoneyBeep() {
    if (typeof window === 'undefined') return false;
    const AudioCtx = window.AudioContext || window.webkitAudioContext;
    if (!AudioCtx) return false;
    if (!smartMoneyBeepAudioContext) {
        smartMoneyBeepAudioContext = new AudioCtx();
    }

    const ctx = smartMoneyBeepAudioContext;
    if (ctx.state === 'suspended') {
        try {
            await ctx.resume();
        } catch {
            return false;
        }
    }

    // 创建一个更悦耳的三音符上升旋律
    const notes = [
        { freq: 523.25, start: 0, duration: 0.12 },      // C5
        { freq: 659.25, start: 0.10, duration: 0.12 },   // E5
        { freq: 783.99, start: 0.20, duration: 0.18 }    // G5
    ];

    const masterGain = ctx.createGain();
    masterGain.gain.setValueAtTime(0.15, ctx.currentTime);
    masterGain.connect(ctx.destination);

    notes.forEach(note => {
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();

        osc.type = 'sine';
        osc.frequency.setValueAtTime(note.freq, ctx.currentTime + note.start);

        // 平滑的音量包络
        gain.gain.setValueAtTime(0.0001, ctx.currentTime + note.start);
        gain.gain.exponentialRampToValueAtTime(1, ctx.currentTime + note.start + 0.02);
        gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + note.start + note.duration);

        osc.connect(gain);
        gain.connect(masterGain);
        osc.start(ctx.currentTime + note.start);
        osc.stop(ctx.currentTime + note.start + note.duration);
    });

    return true;
}

/* ── 自定义下拉选择器 ── */
function CustomSelect({ value, options, onChange, style }) {
    const [open, setOpen] = useState(false);
    const ref = useRef(null);
    const selectedLabel = (options.find(o => String(o.value) === String(value)) || options[0])?.label || '';

    useEffect(() => {
        if (!open) return;
        const handler = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
        document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, [open]);

    return (
        <div ref={ref} style={{ position: 'relative', ...style }}>
            <button
                type="button"
                onClick={() => setOpen(v => !v)}
                style={{
                    width: '100%', borderRadius: 10, border: '1px solid rgba(255,255,255,0.07)',
                    background: 'rgba(0,0,0,0.35)', color: '#e4e4e7', padding: '8px 10px',
                    fontSize: 13, fontFamily: 'inherit', cursor: 'pointer', textAlign: 'left',
                    display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6,
                    transition: 'border-color 0.2s',
                    borderColor: open ? 'rgba(251,191,36,0.3)' : 'rgba(255,255,255,0.07)',
                }}
            >
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{selectedLabel}</span>
                <svg width="10" height="10" viewBox="0 0 10 10" fill="none" style={{ flexShrink: 0, transform: open ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s' }}>
                    <path d="M2 3.5L5 6.5L8 3.5" stroke="#71717a" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
            </button>
            {open && (
                <div style={{
                    position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0,
                    zIndex: 50, borderRadius: 10, padding: 4,
                    border: '1px solid rgba(255,255,255,0.08)',
                    background: 'rgba(15,15,18,0.96)',
                    backdropFilter: 'blur(16px)', WebkitBackdropFilter: 'blur(16px)',
                    boxShadow: '0 12px 40px rgba(0,0,0,0.5)',
                    animation: 'fadeIn 0.15s ease-out',
                }}>
                    {options.map((opt) => {
                        const isSelected = String(opt.value) === String(value);
                        return (
                            <button
                                key={opt.value}
                                type="button"
                                onClick={() => { onChange(opt.value); setOpen(false); }}
                                style={{
                                    width: '100%', border: 'none', borderRadius: 7,
                                    padding: '7px 10px', fontSize: 13, fontFamily: 'inherit',
                                    textAlign: 'left', cursor: 'pointer',
                                    background: isSelected ? 'rgba(251,191,36,0.12)' : 'transparent',
                                    color: isSelected ? '#fcd34d' : '#a1a1aa',
                                    transition: 'background 0.15s, color 0.15s',
                                    display: 'block',
                                }}
                                onMouseEnter={(e) => { if (!isSelected) { e.currentTarget.style.background = 'rgba(255,255,255,0.05)'; e.currentTarget.style.color = '#e4e4e7'; } }}
                                onMouseLeave={(e) => { if (!isSelected) { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.color = '#a1a1aa'; } }}
                            >
                                {opt.label}
                            </button>
                        );
                    })}
                </div>
            )}
        </div>
    );
}

function GoldenDogPanelContent({
    apiBaseUrl,
    initData,
    watchedWallets = [],
    watchedWalletSet = new Set(),
    watchToggleMap = {},
    onToggleWatchWallet,
}) {
    const hasInitData = Boolean(String(initData || '').trim());
    const [loading, setLoading] = useState(hasInitData);
    const [saving, setSaving] = useState(false);
    const [testingMode, setTestingMode] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [status, setStatus] = useState(null);
    const [watchAlertStatus, setWatchAlertStatus] = useState(null);
    const [draft, setDraft] = useState(() => createGoldenDogDraft());
    const [watchAlertDraft, setWatchAlertDraft] = useState(() => createWatchOpenAlertDraft());
    const [activeTab, setActiveTab] = useState('wallet');
    const [lastWatchEventText, setLastWatchEventText] = useState('');

    const barkStatusText = useMemo(() => goldenDogBarkStatusText(status), [status]);
    const watchBarkStatusText = useMemo(() => goldenDogBarkStatusText(watchAlertStatus), [watchAlertStatus]);
    const intensityOptions = useMemo(
        () => (Array.isArray(status?.available_intensities) && status.available_intensities.length > 0
            ? status.available_intensities
            : GOLDEN_DOG_INTENSITY_OPTIONS),
        [status],
    );
    const activePoolThresholdCount = useMemo(
        () => countGoldenDogPoolThresholds(draft.pool_mode),
        [draft.pool_mode],
    );
    const watchedWalletList = useMemo(
        () => Array.from(new Set((Array.isArray(watchedWallets) ? watchedWallets : [])
            .map((item) => normalizeWalletAddress(item))
            .filter(Boolean))).sort(),
        [watchedWallets],
    );

    const applyResponse = useCallback((resp) => {
        setStatus(resp || null);
        setDraft(mapGoldenDogConfigToDraft(resp?.config));
    }, []);
    const applyWatchAlertResponse = useCallback((resp) => {
        setWatchAlertStatus(resp || null);
        setWatchAlertDraft(mapWatchOpenAlertConfigToDraft(resp?.config));
    }, []);

    const loadConfig = useCallback(async () => {
        if (!hasInitData) {
            setLoading(false);
            setStatus(null);
            setWatchAlertStatus(null);
            return;
        }
        setLoading(true);
        setError('');
        try {
            const [goldenDogResp, watchAlertResp] = await Promise.all([
                fetchSMGoldenDogConfig({ apiBaseUrl, initData, chain: 'bsc' }),
                fetchSMWatchOpenAlertConfig({ apiBaseUrl, initData, chain: 'bsc' }),
            ]);
            applyResponse(goldenDogResp);
            applyWatchAlertResponse(watchAlertResp);
        } catch (err) {
            setError(String(err?.message || err || '加载失败'));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, applyResponse, applyWatchAlertResponse, hasInitData, initData]);

    useEffect(() => {
        loadConfig();
    }, [loadConfig]);

    useEffect(() => {
        if (!hasInitData || !watchAlertDraft.enabled || !watchAlertDraft.sound_enabled || !watchedWalletList.length) {
            return undefined;
        }

        const wsUrl = buildSMEventsWsUrl(apiBaseUrl);
        if (!wsUrl) return undefined;

        let cancelled = false;
        let socket;
        try {
            socket = new WebSocket(wsUrl);
        } catch {
            return undefined;
        }

        socket.onmessage = async (messageEvent) => {
            if (cancelled) return;
            if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return;
            try {
                const payload = JSON.parse(messageEvent.data);
                if (payload?.type !== 'lp_event') return;
                const event = payload?.data || {};
                const eventType = String(event?.event_type || '').trim().toLowerCase();
                const walletAddress = normalizeWalletAddress(event?.wallet_address);
                if (eventType !== 'add' || !walletAddress || !watchedWalletSet.has(walletAddress)) {
                    return;
                }
                const walletName = String(event?.wallet_label || '').trim() || tailAddr(walletAddress);
                const pairLabel = getPairLabel({
                    token0_symbol: event?.token0_symbol,
                    token1_symbol: event?.token1_symbol,
                }) || tailAddr(event?.pool_address);
                setLastWatchEventText(`${walletName} 开仓 ${pairLabel}`);
                console.log('[SmartMoney] 检测到开仓事件，准备播放音效:', walletName, pairLabel);
                const played = await playSmartMoneyBeep();
                console.log('[SmartMoney] 音效播放结果:', played ? '成功' : '失败');
            } catch (err) {
                console.error('[SmartMoney] WebSocket消息处理错误:', err);
            }
        };

        return () => {
            cancelled = true;
            try {
                socket?.close();
            } catch {
                // ignore
            }
        };
    }, [apiBaseUrl, hasInitData, watchAlertDraft.enabled, watchAlertDraft.sound_enabled, watchedWalletList, watchedWalletSet]);

    const updateWalletMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            wallet_mode: { ...prev.wallet_mode, [key]: value },
        }));
    }, []);

    const updateWalletAmountTier = useCallback((index, key, value) => {
        setDraft((prev) => {
            const tiers = Array.isArray(prev.wallet_mode.amount_intensity_tiers)
                ? prev.wallet_mode.amount_intensity_tiers.map((tier) => ({ ...tier }))
                : cloneGoldenDogDefaultAmountTiers();
            while (tiers.length < 3) {
                tiers.push({ ...GOLDEN_DOG_DEFAULT_AMOUNT_TIERS[tiers.length] });
            }
            tiers[index] = { ...tiers[index], [key]: value };
            return {
                ...prev,
                wallet_mode: { ...prev.wallet_mode, amount_intensity_tiers: tiers },
            };
        });
    }, []);

    const updatePoolMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            pool_mode: { ...prev.pool_mode, [key]: value },
        }));
    }, []);
    const updateWatchAlertMode = useCallback((key, value) => {
        setWatchAlertDraft((prev) => ({ ...prev, [key]: value }));
    }, []);

    /* ── 精致化的统一样式 ── */
    const fieldInputCss = {
        width: '100%', borderRadius: 10,
        border: '1px solid rgba(255,255,255,0.07)',
        background: 'rgba(0,0,0,0.35)',
        color: '#e4e4e7', padding: '8px 10px',
        outline: 'none', fontSize: 13, fontFamily: 'inherit',
        textAlign: 'center',
        transition: 'border-color 0.2s, box-shadow 0.2s',
        WebkitAppearance: 'none', MozAppearance: 'none', appearance: 'none',
    };
    const fieldLabelCss = { display: 'grid', gap: 5, textAlign: 'center' };
    const fieldLabelTextCss = { fontSize: 11, fontWeight: 500, color: '#a1a1aa', letterSpacing: '0.02em', textTransform: 'uppercase' };

    const pillBtnBase = (isActive, accentRgb) => ({
        borderRadius: 8, border: 'none', padding: '6px 14px',
        fontSize: 12, fontWeight: 600, cursor: 'pointer',
        transition: 'all 0.2s',
        background: isActive ? `rgba(${accentRgb},0.18)` : 'rgba(255,255,255,0.04)',
        color: isActive ? '#f4f4f5' : '#52525b',
        boxShadow: isActive ? `0 0 12px rgba(${accentRgb},0.12)` : 'none',
    });

    const actionBtnCss = {
        borderRadius: 10, border: '1px solid rgba(255,255,255,0.06)',
        background: 'rgba(255,255,255,0.04)', color: '#a1a1aa',
        padding: '6px 14px', fontSize: 12, fontWeight: 600,
        cursor: 'pointer', transition: 'all 0.2s', fontFamily: 'inherit',
    };
    const saveBtnCss = {
        ...actionBtnCss,
        borderColor: 'rgba(251,191,36,0.25)',
        background: 'linear-gradient(135deg, rgba(251,191,36,0.14), rgba(245,158,11,0.08))',
        color: '#fcd34d',
    };

    const miniStatCss = { display: 'flex', alignItems: 'baseline', gap: 6, padding: '6px 0' };
    const miniStatLabel = { fontSize: 11, color: '#a1a1aa', fontWeight: 500 };
    const miniStatValue = { fontSize: 13, color: '#e4e4e7', fontWeight: 600, fontVariantNumeric: 'tabular-nums' };

    const buildSavePayload = useCallback(() => {
        const walletMinWallets = parseGoldenDogRequiredInt(draft.wallet_mode.min_wallets, '钱包数量');
        const walletWindowMinutes = parseGoldenDogRequiredInt(draft.wallet_mode.window_minutes, '统计窗口');
        const walletCooldownMinutes = parseGoldenDogRequiredInt(draft.wallet_mode.cooldown_minutes, '冷却时间', { min: 0 });
        const walletMinTotalAmountUSD = parseGoldenDogOptionalNumber(draft.wallet_mode.min_total_amount_usd, '最低合计金额');
        const walletIntensityMode = draft.wallet_mode.intensity_mode === 'amount_tiers' ? 'amount_tiers' : 'fixed';
        const walletAmountIntensityTiers = walletIntensityMode === 'amount_tiers'
            ? parseGoldenDogAmountIntensityTiers(draft.wallet_mode.amount_intensity_tiers, walletIntensityMode)
            : [];
        const poolCooldownMinutes = parseGoldenDogRequiredInt(draft.pool_mode.cooldown_minutes, '池子模式冷却时间', { min: 0 });
        const poolMinTotalFees = parseGoldenDogOptionalNumber(draft.pool_mode.min_total_fees, '最小手续费');
        const poolMinTransactionCount = parseGoldenDogOptionalNumber(draft.pool_mode.min_transaction_count, '最小交易笔数');
        const poolMinTVL = parseGoldenDogOptionalNumber(draft.pool_mode.min_tvl, '最小 TVL');
        const poolMinVolume = parseGoldenDogOptionalNumber(draft.pool_mode.min_volume, '最小 VOL');
        const poolMinFeeRate = parseGoldenDogOptionalNumber(draft.pool_mode.min_fee_rate, '最小费率');
        const poolMinActiveLiquidityRatioPct = parseGoldenDogOptionalNumber(draft.pool_mode.min_active_liquidity_ratio, '最小活跃费率', { max: 100 });

        const thresholdCount = [
            poolMinTotalFees,
            poolMinTransactionCount,
            poolMinTVL,
            poolMinVolume,
            poolMinFeeRate,
            poolMinActiveLiquidityRatioPct,
        ].filter((value) => Number(value) > 0).length;
        if (draft.pool_mode.enabled && thresholdCount === 0) {
            throw new Error('池子参数模式至少需要填写一个筛选阈值。');
        }

        return {
            wallet_mode: {
                enabled: Boolean(draft.wallet_mode.enabled),
                min_wallets: walletMinWallets,
                window_minutes: walletWindowMinutes,
                cooldown_minutes: walletCooldownMinutes,
                min_total_amount_usd: walletMinTotalAmountUSD,
                intensity: draft.wallet_mode.intensity || 'ring',
                intensity_mode: walletIntensityMode,
                amount_intensity_tiers: walletAmountIntensityTiers,
            },
            pool_mode: {
                enabled: Boolean(draft.pool_mode.enabled),
                cooldown_minutes: poolCooldownMinutes,
                min_total_fees: poolMinTotalFees,
                min_transaction_count: poolMinTransactionCount,
                min_tvl: poolMinTVL,
                min_volume: poolMinVolume,
                min_fee_rate: poolMinFeeRate,
                min_active_liquidity_ratio: poolMinActiveLiquidityRatioPct,
                intensity: draft.pool_mode.intensity || 'ring',
            },
        };
    }, [draft]);

    const handleSave = useCallback(async () => {
        if (!hasInitData) {
            setError('请先登录 WebApp，拿到 initData 后才能保存监控通知。');
            return;
        }
        setSaving(true);
        setError('');
        setNotice('');
        try {
            if (activeTab === 'watch_open') {
                const resp = await saveSMWatchOpenAlertConfig({
                    apiBaseUrl,
                    initData,
                    chain: 'bsc',
                    config: {
                        enabled: Boolean(watchAlertDraft.enabled),
                        bark_enabled: Boolean(watchAlertDraft.bark_enabled),
                        sound_enabled: Boolean(watchAlertDraft.sound_enabled),
                    },
                });
                applyWatchAlertResponse(resp);
                setNotice('特别关注开仓配置已保存');
            } else {
                const resp = await saveSMGoldenDogConfig({
                    apiBaseUrl, initData, chain: 'bsc',
                    config: buildSavePayload(),
                });
                applyResponse(resp);
                setNotice(activeTab === 'pool' ? '池子监控配置已保存' : '金狗通知配置已保存');
            }
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [activeTab, apiBaseUrl, applyResponse, applyWatchAlertResponse, buildSavePayload, hasInitData, initData, watchAlertDraft]);

    const handleTest = useCallback(async (mode) => {
        if (!hasInitData) {
            setError('请先登录 WebApp，拿到 initData 后才能测试监控通知。');
            return;
        }
        setTestingMode(mode);
        setError('');
        setNotice('');
        try {
            if (mode === 'watch_bark') {
                const resp = await testSMWatchOpenAlertConfig({ apiBaseUrl, initData, chain: 'bsc' });
                setNotice(resp?.message || '测试通知已发送');
            } else if (mode === 'watch_sound') {
                const ok = await playSmartMoneyBeep();
                setNotice(ok ? '已播放一声提示音' : '当前环境不支持播放提示音');
            } else {
                const intensity = mode === 'pool' ? draft.pool_mode.intensity : goldenDogWalletTestIntensity(draft.wallet_mode);
                const resp = await testSMGoldenDogConfig({
                    apiBaseUrl, initData, chain: 'bsc', mode, intensity,
                });
                setNotice(resp?.message || '测试通知已发送');
            }
        } catch (err) {
            setError(String(err?.message || err || '测试失败'));
        } finally {
            setTestingMode('');
        }
    }, [apiBaseUrl, draft.pool_mode.intensity, draft.wallet_mode, hasInitData, initData]);

    /* ── 渲染 ── */
    return (
        <div style={{ display: 'grid', gap: 12 }}>
            {/* ─── 紧凑标题行 ─── */}
            <div style={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                gap: 12, flexWrap: 'wrap', padding: '10px 14px',
                borderRadius: 16,
                border: '1px solid rgba(251,191,36,0.12)',
                background: 'linear-gradient(135deg, rgba(251,191,36,0.06) 0%, transparent 60%), rgba(17,17,20,0.85)',
            }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
                    <div style={{
                        width: 34, height: 34, borderRadius: 10,
                        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
                        background: 'rgba(251,191,36,0.10)', border: '1px solid rgba(251,191,36,0.18)',
                        color: '#fcd34d', flexShrink: 0,
                    }}>
                        <Flame size={15} />
                    </div>
                    <div style={{ minWidth: 0 }}>
                        <div style={{ fontSize: 14, fontWeight: 700, color: '#f4f4f5', letterSpacing: '-0.01em' }}>监控通知</div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 3, flexWrap: 'wrap' }}>
                            <span style={{ fontSize: 11, padding: '1px 7px', borderRadius: 6, background: draft.wallet_mode.enabled ? 'rgba(52,211,153,0.12)' : 'rgba(255,255,255,0.04)', color: draft.wallet_mode.enabled ? '#6ee7b7' : '#52525b', fontWeight: 500 }}>
                                金狗 {draft.wallet_mode.enabled ? 'ON' : 'OFF'}
                            </span>
                            <span style={{ fontSize: 11, padding: '1px 7px', borderRadius: 6, background: draft.pool_mode.enabled ? 'rgba(52,211,153,0.12)' : 'rgba(255,255,255,0.04)', color: draft.pool_mode.enabled ? '#6ee7b7' : '#52525b', fontWeight: 500 }}>
                                池子 {draft.pool_mode.enabled ? 'ON' : 'OFF'}
                            </span>
                            <span style={{ fontSize: 11, padding: '1px 7px', borderRadius: 6, background: watchAlertDraft.enabled ? 'rgba(59,130,246,0.12)' : 'rgba(255,255,255,0.04)', color: watchAlertDraft.enabled ? '#93c5fd' : '#52525b', fontWeight: 500 }}>
                                特别关注 {watchAlertDraft.enabled ? 'ON' : 'OFF'}
                            </span>
                            <span style={{ fontSize: 11, padding: '1px 7px', borderRadius: 6, background: 'rgba(255,255,255,0.04)', color: '#71717a', fontWeight: 500 }}>
                                Bark {activeTab === 'watch_open' ? watchBarkStatusText : barkStatusText}
                            </span>
                        </div>
                    </div>
                </div>
                <button type="button" disabled={saving || !hasInitData} onClick={handleSave} style={saveBtnCss}>
                    {saving ? '保存中...' : '保存当前配置'}
                </button>
            </div>

            {/* ─── 消息提示 ─── */}
            {!hasInitData && (
                <div style={{ fontSize: 12, color: '#f87171', padding: '8px 12px', borderRadius: 10, background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.12)' }}>
                    需要先登录 Telegram 才能保存配置
                </div>
            )}
            {error && <div style={{ fontSize: 12, color: '#f87171', padding: '8px 12px', borderRadius: 10, background: 'rgba(239,68,68,0.06)', border: '1px solid rgba(239,68,68,0.12)' }}>{error}</div>}
            {!error && notice && <div style={{ fontSize: 12, color: '#86efac', padding: '8px 12px', borderRadius: 10, background: 'rgba(34,197,94,0.06)', border: '1px solid rgba(34,197,94,0.12)' }}>{notice}</div>}
            {!error && lastWatchEventText && <div style={{ fontSize: 12, color: '#93c5fd', padding: '8px 12px', borderRadius: 10, background: 'rgba(59,130,246,0.08)', border: '1px solid rgba(59,130,246,0.16)' }}>最近提示: {lastWatchEventText}</div>}

            {loading ? (
                <div style={{ textAlign: 'center', padding: 32, color: '#52525b', fontSize: 13 }}>加载中...</div>
            ) : (
                <>
                    {/* ─── Tab 切换（精致药丸） ─── */}
                    <div style={{
                        display: 'flex', gap: 2, padding: 3, borderRadius: 12,
                        background: 'rgba(255,255,255,0.025)', border: '1px solid rgba(255,255,255,0.04)',
                    }}>
                        <button type="button" onClick={() => setActiveTab('wallet')} style={{
                            flex: 1, borderRadius: 10, padding: '9px 0', fontSize: 13, fontWeight: 600,
                            border: 'none', cursor: 'pointer', transition: 'all 0.25s ease',
                            background: activeTab === 'wallet' ? 'rgba(251,191,36,0.12)' : 'transparent',
                            color: activeTab === 'wallet' ? '#fcd34d' : '#52525b',
                            boxShadow: activeTab === 'wallet' ? 'inset 0 1px 0 rgba(251,191,36,0.08)' : 'none',
                        }}>
                            🔥 金狗通知
                        </button>
                        <button type="button" onClick={() => setActiveTab('pool')} style={{
                            flex: 1, borderRadius: 10, padding: '9px 0', fontSize: 13, fontWeight: 600,
                            border: 'none', cursor: 'pointer', transition: 'all 0.25s ease',
                            background: activeTab === 'pool' ? 'rgba(45,212,191,0.12)' : 'transparent',
                            color: activeTab === 'pool' ? '#5eead4' : '#52525b',
                            boxShadow: activeTab === 'pool' ? 'inset 0 1px 0 rgba(45,212,191,0.08)' : 'none',
                        }}>
                            🧠 池子监控
                        </button>
                        <button type="button" onClick={() => setActiveTab('watch_open')} style={{
                            flex: 1, borderRadius: 10, padding: '9px 0', fontSize: 13, fontWeight: 600,
                            border: 'none', cursor: 'pointer', transition: 'all 0.25s ease',
                            background: activeTab === 'watch_open' ? 'rgba(59,130,246,0.12)' : 'transparent',
                            color: activeTab === 'watch_open' ? '#93c5fd' : '#52525b',
                            boxShadow: activeTab === 'watch_open' ? 'inset 0 1px 0 rgba(59,130,246,0.08)' : 'none',
                        }}>
                            🔔 特别关注开仓
                        </button>
                    </div>

                    {/* ─── 金狗通知 Tab ─── */}
                    {activeTab === 'wallet' && (
                        <div style={{
                            borderRadius: 16, padding: 16,
                            border: '1px solid rgba(251,191,36,0.10)',
                            background: 'linear-gradient(160deg, rgba(251,191,36,0.05) 0%, transparent 40%), rgba(15,15,18,0.92)',
                            animation: 'fadeIn 0.25s ease-out',
                        }}>
                            {/* 标题 + 操作行 */}
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap', marginBottom: 14 }}>
                                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                    <div style={{ width: 28, height: 28, borderRadius: 8, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(251,191,36,0.10)', color: '#fde68a', fontSize: 13 }}>
                                        <Flame size={13} />
                                    </div>
                                    <span style={{ fontSize: 14, fontWeight: 700, color: '#e4e4e7' }}>聪明钱聚集</span>
                                    <span style={{ fontSize: 11, color: '#a1a1aa', fontWeight: 400 }}>— 窗口内钱包数达到阈值时推送</span>
                                </div>
                                <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                                    <button type="button" onClick={() => updateWalletMode('enabled', true)} style={pillBtnBase(draft.wallet_mode.enabled, '251,191,36')}>开启</button>
                                    <button type="button" onClick={() => updateWalletMode('enabled', false)} style={pillBtnBase(!draft.wallet_mode.enabled, '161,161,170')}>关闭</button>
                                    <button type="button" disabled={testingMode === 'wallet' || !hasInitData} onClick={() => handleTest('wallet')} style={actionBtnCss}>
                                        {testingMode === 'wallet' ? '⏳' : '🔔'} 测试
                                    </button>
                                </div>
                            </div>

                            {/* 紧凑指标行 */}
                            <div style={{
                                display: 'flex', gap: 0, borderRadius: 10, overflow: 'hidden', marginBottom: 14,
                                border: '1px solid rgba(255,255,255,0.04)', background: 'rgba(0,0,0,0.2)',
                            }}>
                                {[
                                    { l: '触发钱包', v: `${draft.wallet_mode.min_wallets || '--'}个` },
                                    { l: '最低金额', v: goldenDogThresholdText(draft.wallet_mode.min_total_amount_usd, '$') },
                                    { l: '统计窗口', v: `${draft.wallet_mode.window_minutes || '--'}分钟` },
                                    { l: '冷却', v: `${draft.wallet_mode.cooldown_minutes || '--'}分钟` },
                                    { l: '强度', v: draft.wallet_mode.intensity_mode === 'amount_tiers' ? '金额阶梯' : goldenDogIntensityLabel(draft.wallet_mode.intensity) },
                                ].map((s, i) => (
                                    <div key={s.l} style={{
                                        flex: 1, padding: '8px 10px', textAlign: 'center',
                                        borderRight: i < 4 ? '1px solid rgba(255,255,255,0.04)' : 'none',
                                    }}>
                                        <div style={miniStatLabel}>{s.l}</div>
                                        <div style={miniStatValue}>{s.v}</div>
                                    </div>
                                ))}
                            </div>

                            {/* 表单 */}
                            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10 }}>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>钱包数量</span>
                                    <input type="number" min="1" step="1" value={draft.wallet_mode.min_wallets} onChange={(e) => updateWalletMode('min_wallets', e.target.value)} style={fieldInputCss} />
                                </label>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>窗口 (分钟)</span>
                                    <input type="number" min="1" step="1" value={draft.wallet_mode.window_minutes} onChange={(e) => updateWalletMode('window_minutes', e.target.value)} style={fieldInputCss} />
                                </label>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>冷却 (分钟)</span>
                                    <input type="number" min="0" step="1" value={draft.wallet_mode.cooldown_minutes} onChange={(e) => updateWalletMode('cooldown_minutes', e.target.value)} style={fieldInputCss} />
                                </label>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>最低金额 ($)</span>
                                    <input type="number" min="0" step="0.01" placeholder="不限制" value={draft.wallet_mode.min_total_amount_usd} onChange={(e) => updateWalletMode('min_total_amount_usd', e.target.value)} style={fieldInputCss} />
                                </label>
                                <div style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>强度模式</span>
                                    <CustomSelect value={draft.wallet_mode.intensity_mode} options={GOLDEN_DOG_INTENSITY_MODE_OPTIONS} onChange={(v) => updateWalletMode('intensity_mode', v)} />
                                </div>
                                <div style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>固定强度</span>
                                    <CustomSelect value={draft.wallet_mode.intensity} options={intensityOptions} onChange={(v) => updateWalletMode('intensity', v)} />
                                </div>
                            </div>
                            {draft.wallet_mode.intensity_mode === 'amount_tiers' && (
                                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10, marginTop: 10 }}>
                                    {(draft.wallet_mode.amount_intensity_tiers || cloneGoldenDogDefaultAmountTiers()).map((tier, index) => (
                                        <div key={index} style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)', gap: 6, padding: 8, borderRadius: 10, background: 'rgba(0,0,0,0.22)', border: '1px solid rgba(255,255,255,0.05)' }}>
                                            <label style={{ ...fieldLabelCss, gap: 4 }}>
                                                <span style={fieldLabelTextCss}>第 {index + 1} 档 ($)</span>
                                                <input type="number" min="0" step="0.01" value={tier.min_amount_usd} onChange={(e) => updateWalletAmountTier(index, 'min_amount_usd', e.target.value)} style={fieldInputCss} />
                                            </label>
                                            <div style={{ ...fieldLabelCss, gap: 4 }}>
                                                <span style={fieldLabelTextCss}>强度</span>
                                                <CustomSelect value={tier.intensity} options={intensityOptions} onChange={(v) => updateWalletAmountTier(index, 'intensity', v)} />
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}

                    {/* ─── 池子监控 Tab ─── */}
                    {activeTab === 'pool' && (
                        <div style={{
                            borderRadius: 16, padding: 16,
                            border: '1px solid rgba(45,212,191,0.10)',
                            background: 'linear-gradient(160deg, rgba(45,212,191,0.05) 0%, transparent 40%), rgba(15,15,18,0.92)',
                            animation: 'fadeIn 0.25s ease-out',
                        }}>
                            {/* 标题 + 操作行 */}
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap', marginBottom: 14 }}>
                                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                    <div style={{ width: 28, height: 28, borderRadius: 8, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(45,212,191,0.10)', color: '#99f6e4', fontSize: 13 }}>
                                        <Brain size={13} />
                                    </div>
                                    <span style={{ fontSize: 14, fontWeight: 700, color: '#e4e4e7' }}>PoolM 池子参数</span>
                                    <span style={{ fontSize: 11, color: '#a1a1aa', fontWeight: 400 }}>— AND 条件筛选，留空不参与匹配</span>
                                </div>
                                <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                                    <button type="button" onClick={() => updatePoolMode('enabled', true)} style={pillBtnBase(draft.pool_mode.enabled, '45,212,191')}>开启</button>
                                    <button type="button" onClick={() => updatePoolMode('enabled', false)} style={pillBtnBase(!draft.pool_mode.enabled, '161,161,170')}>关闭</button>
                                    <button type="button" disabled={testingMode === 'pool' || !hasInitData} onClick={() => handleTest('pool')} style={actionBtnCss}>
                                        {testingMode === 'pool' ? '⏳' : '🔔'} 测试
                                    </button>
                                </div>
                            </div>

                            {/* 紧凑指标行：5 个筛选条件 */}
                            <div style={{
                                display: 'flex', gap: 0, borderRadius: 10, overflow: 'hidden', marginBottom: 14,
                                border: '1px solid rgba(255,255,255,0.04)', background: 'rgba(0,0,0,0.2)',
                            }}>
                                {[
                                    { l: 'TVL', v: goldenDogThresholdText(draft.pool_mode.min_tvl, '$') },
                                    { l: 'VOL', v: goldenDogThresholdText(draft.pool_mode.min_volume, '$') },
                                    { l: '笔数', v: goldenDogThresholdText(draft.pool_mode.min_transaction_count) },
                                    { l: '费率', v: goldenDogThresholdText(draft.pool_mode.min_fee_rate, '', '%') },
                                    { l: '活跃', v: goldenDogThresholdText(draft.pool_mode.min_active_liquidity_ratio, '', '%') },
                                ].map((s, i) => (
                                    <div key={s.l} style={{
                                        flex: 1, padding: '8px 10px', textAlign: 'center',
                                        borderRight: i < 4 ? '1px solid rgba(255,255,255,0.04)' : 'none',
                                    }}>
                                        <div style={miniStatLabel}>{s.l}</div>
                                        <div style={miniStatValue}>{s.v}</div>
                                    </div>
                                ))}
                            </div>

                            {/* 筛选条件表单（5列） */}
                            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 10, marginBottom: 10 }}>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>TVL ($)</span>
                                    <input type="number" min="0" step="0.01" placeholder="不限制" value={draft.pool_mode.min_tvl} onChange={(e) => updatePoolMode('min_tvl', e.target.value)} style={fieldInputCss} />
                                </label>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>VOL ($)</span>
                                    <input type="number" min="0" step="0.01" placeholder="不限制" value={draft.pool_mode.min_volume} onChange={(e) => updatePoolMode('min_volume', e.target.value)} style={fieldInputCss} />
                                </label>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>交易笔数</span>
                                    <input type="number" min="0" step="1" placeholder="不限制" value={draft.pool_mode.min_transaction_count} onChange={(e) => updatePoolMode('min_transaction_count', e.target.value)} style={fieldInputCss} />
                                </label>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>费率 (%)</span>
                                    <input type="number" min="0" step="0.01" placeholder="如1即1%" value={draft.pool_mode.min_fee_rate} onChange={(e) => updatePoolMode('min_fee_rate', e.target.value)} style={fieldInputCss} />
                                </label>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>活跃费率 (%)</span>
                                    <input type="number" min="0" max="100" step="0.1" placeholder="不限制" value={draft.pool_mode.min_active_liquidity_ratio} onChange={(e) => updatePoolMode('min_active_liquidity_ratio', e.target.value)} style={fieldInputCss} />
                                </label>
                            </div>
                            {/* 冷却 + 通知强度 */}
                            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 10 }}>
                                <label style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>冷却 (分钟)</span>
                                    <input type="number" min="0" step="1" value={draft.pool_mode.cooldown_minutes} onChange={(e) => updatePoolMode('cooldown_minutes', e.target.value)} style={fieldInputCss} />
                                </label>
                                <div style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>通知强度</span>
                                    <CustomSelect value={draft.pool_mode.intensity} options={intensityOptions} onChange={(v) => updatePoolMode('intensity', v)} />
                                </div>
                            </div>
                        </div>
                    )}

                    {activeTab === 'watch_open' && (
                        <div style={{
                            borderRadius: 16,
                            padding: 16,
                            border: '1px solid rgba(59,130,246,0.12)',
                            background: 'linear-gradient(160deg, rgba(59,130,246,0.06) 0%, transparent 40%), rgba(15,15,18,0.92)',
                        }}>
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap', marginBottom: 14 }}>
                                <div style={{ display: 'grid', gap: 4 }}>
                                    <div style={{ fontSize: 14, fontWeight: 700, color: '#e4e4e7' }}>特别关注开仓</div>
                                    <div style={{ fontSize: 11, color: '#a1a1aa' }}>特别关注的钱包一旦开仓，可发 Bark，页面前台可播放一声“滴”。</div>
                                </div>
                                <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
                                    <button type="button" onClick={() => updateWatchAlertMode('enabled', true)} style={pillBtnBase(watchAlertDraft.enabled, '59,130,246')}>开启</button>
                                    <button type="button" onClick={() => updateWatchAlertMode('enabled', false)} style={pillBtnBase(!watchAlertDraft.enabled, '161,161,170')}>关闭</button>
                                    <button type="button" disabled={testingMode === 'watch_bark' || !hasInitData} onClick={() => handleTest('watch_bark')} style={actionBtnCss}>
                                        {testingMode === 'watch_bark' ? '测试中...' : '测试 Bark'}
                                    </button>
                                    <button type="button" disabled={testingMode === 'watch_sound'} onClick={() => handleTest('watch_sound')} style={actionBtnCss}>
                                        {testingMode === 'watch_sound' ? '试听中...' : '试听提示音'}
                                    </button>
                                </div>
                            </div>

                            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10, marginBottom: 14 }}>
                                <div style={{ padding: '10px 12px', borderRadius: 12, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
                                    <div style={miniStatLabel}>通知状态</div>
                                    <div style={{ ...miniStatValue, marginTop: 4 }}>{smartMoneyWatchOpenStatusText(watchAlertDraft)}</div>
                                </div>
                                <div style={{ padding: '10px 12px', borderRadius: 12, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
                                    <div style={miniStatLabel}>Bark 状态</div>
                                    <div style={{ ...miniStatValue, marginTop: 4 }}>{watchBarkStatusText}</div>
                                </div>
                                <div style={{ padding: '10px 12px', borderRadius: 12, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
                                    <div style={miniStatLabel}>特别关注钱包</div>
                                    <div style={{ ...miniStatValue, marginTop: 4 }}>{watchedWalletList.length} 个</div>
                                </div>
                                <div style={{ padding: '10px 12px', borderRadius: 12, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.04)' }}>
                                    <div style={miniStatLabel}>前台音效</div>
                                    <div style={{ ...miniStatValue, marginTop: 4 }}>{watchAlertDraft.sound_enabled ? '已开启' : '已关闭'}</div>
                                </div>
                            </div>

                            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10, marginBottom: 14 }}>
                                <button
                                    type="button"
                                    onClick={() => updateWatchAlertMode('bark_enabled', !watchAlertDraft.bark_enabled)}
                                    style={{
                                        ...actionBtnCss,
                                        padding: '10px 12px',
                                        color: watchAlertDraft.bark_enabled ? '#93c5fd' : '#a1a1aa',
                                        borderColor: watchAlertDraft.bark_enabled ? 'rgba(59,130,246,0.26)' : 'rgba(255,255,255,0.06)',
                                        background: watchAlertDraft.bark_enabled ? 'rgba(59,130,246,0.12)' : 'rgba(255,255,255,0.04)',
                                    }}
                                >
                                    Bark 推送 {watchAlertDraft.bark_enabled ? '已开' : '已关'}
                                </button>
                                <button
                                    type="button"
                                    onClick={() => updateWatchAlertMode('sound_enabled', !watchAlertDraft.sound_enabled)}
                                    style={{
                                        ...actionBtnCss,
                                        padding: '10px 12px',
                                        color: watchAlertDraft.sound_enabled ? '#93c5fd' : '#a1a1aa',
                                        borderColor: watchAlertDraft.sound_enabled ? 'rgba(59,130,246,0.26)' : 'rgba(255,255,255,0.06)',
                                        background: watchAlertDraft.sound_enabled ? 'rgba(59,130,246,0.12)' : 'rgba(255,255,255,0.04)',
                                    }}
                                >
                                    前台提示音 {watchAlertDraft.sound_enabled ? '已开' : '已关'}
                                </button>
                                <div style={{ padding: '10px 12px', borderRadius: 10, border: '1px solid rgba(255,255,255,0.06)', background: 'rgba(255,255,255,0.03)', color: '#a1a1aa', fontSize: 12 }}>
                                    Bark Key / Server / Group 继续复用全局配置。
                                </div>
                            </div>

                            <div style={{ display: 'grid', gap: 8 }}>
                                <div style={{ fontSize: 12, fontWeight: 600, color: '#dbeafe' }}>特别关注列表</div>
                                {watchedWalletList.length ? watchedWalletList.map((walletAddress) => (
                                    <div
                                        key={walletAddress}
                                        style={{
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'space-between',
                                            gap: 10,
                                            padding: '10px 12px',
                                            borderRadius: 12,
                                            border: '1px solid rgba(255,255,255,0.05)',
                                            background: 'rgba(255,255,255,0.03)',
                                        }}
                                    >
                                        <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
                                            <img
                                                src={resolveWalletAvatarSrc(walletAddress)}
                                                alt=""
                                                style={{ width: 36, height: 36, borderRadius: '50%', flexShrink: 0 }}
                                            />
                                            <div style={{ display: 'grid', gap: 4, minWidth: 0 }}>
                                                <span style={{ fontSize: 13, color: '#e4e4e7', fontWeight: 600 }}>尾号 {tailAddr(walletAddress)}</span>
                                                <span style={{ fontSize: 11, color: '#a1a1aa' }}>{shortAddr(walletAddress)}</span>
                                            </div>
                                        </div>
                                        <button
                                            type="button"
                                            disabled={Boolean(watchToggleMap[walletAddress])}
                                            onClick={() => onToggleWatchWallet?.(walletAddress)}
                                            style={{
                                                ...actionBtnCss,
                                                color: '#fda4af',
                                                borderColor: 'rgba(244,63,94,0.24)',
                                                background: 'rgba(244,63,94,0.08)',
                                            }}
                                        >
                                            {watchToggleMap[walletAddress] ? '处理中...' : '移除'}
                                        </button>
                                    </div>
                                )) : (
                                    <div style={{ padding: '14px 16px', borderRadius: 12, border: '1px dashed rgba(255,255,255,0.08)', color: '#71717a', background: 'rgba(255,255,255,0.02)' }}>
                                        暂无特别关注钱包。可在 K 线聪明钱标记弹层或钱包视图里点心形按钮加入。
                                    </div>
                                )}
                            </div>
                        </div>
                    )}
                </>
            )}
        </div>
    );
}

// ============================================================
//  自动跟单 Panel
// ============================================================

const AUTO_FOLLOW_DRAFT_INITIAL = Object.freeze({
    id: 0,
    target_wallet_address: '',
    target_wallet_addresses: [''],
    execution_wallet_id: '',
    execution_wallet_address: '',
    trigger_mode: 'any',
    trigger_min_wallets: '2',
    trigger_window_seconds: '300',
    enabled: true,
    amount_mode: 'fixed',
    fixed_amount_usdt: '',
    ratio_percent: '100',
    delay_mode: 'immediate',
    delay_seconds: '0',
    follow_close: true,
});

function normalizeAutoFollowWalletList(config) {
    const source = Array.isArray(config?.target_wallet_addresses) && config.target_wallet_addresses.length > 0
        ? config.target_wallet_addresses
        : [config?.target_wallet_address || ''];
    const wallets = source.map((value) => String(value || '').trim()).filter(Boolean);
    return wallets.length ? wallets : [''];
}

function parseAutoFollowWalletInputs(values) {
    const seen = new Set();
    const wallets = [];
    (Array.isArray(values) ? values : []).forEach((value) => {
        const raw = String(value || '').trim();
        if (!raw) return;
        if (!/^0x[a-fA-F0-9]{40}$/.test(raw)) {
            throw new Error('请填写正确的钱包地址 (0x 开头 42 位)');
        }
        const address = `0x${raw.slice(2).toLowerCase()}`;
        if (!seen.has(address)) {
            seen.add(address);
            wallets.push(address);
        }
    });
    if (wallets.length === 0) {
        throw new Error('至少填写一个跟单钱包地址');
    }
    return wallets;
}

function autoFollowTriggerText(config) {
    const wallets = normalizeAutoFollowWalletList(config).filter(Boolean);
    if (String(config?.trigger_mode || 'any') === 'threshold') {
        return `${Number(config?.trigger_min_wallets || 2)} / ${wallets.length} 钱包 · ${Number(config?.trigger_window_seconds || 300)}s`;
    }
    return wallets.length > 1 ? `任意 1 / ${wallets.length} 钱包` : '单钱包触发';
}

function findAutoFollowExecutionWallet(wallets, id, address) {
    const walletId = Number(id) || 0;
    const addr = normalizeWalletAddress(address);
    if (!Array.isArray(wallets)) return null;
    return wallets.find((wallet) => {
        if (walletId > 0 && Number(wallet?.id) === walletId) return true;
        return addr && normalizeWalletAddress(wallet?.address) === addr;
    }) || null;
}

function formatAutoFollowExecutionWallet(value, wallets) {
    const wallet = findAutoFollowExecutionWallet(wallets, value?.execution_wallet_id, value?.execution_wallet_address);
    if (wallet) {
        const name = String(wallet.name || '').trim();
        const addr = normalizeWalletAddress(wallet.address);
        return name ? `${name} · ${shortAddr(addr)}` : shortAddr(addr);
    }
    return shortAddr(normalizeWalletAddress(value?.execution_wallet_address)) || '未设置';
}

function ensureAutoFollowDraftExecutionWallet(draft, wallets) {
    if (Number(draft?.execution_wallet_id) > 0) return draft;
    const source = Array.isArray(wallets) ? wallets : [];
    const wallet = source.find((item) => item?.is_default) || source[0];
    if (!wallet) return draft;
    return {
        ...draft,
        execution_wallet_id: String(wallet.id),
        execution_wallet_address: normalizeWalletAddress(wallet.address),
    };
}

function createAutoFollowDraft(config) {
    if (!config) return { ...AUTO_FOLLOW_DRAFT_INITIAL };
    const ratioPct = Number(config.ratio || 0) * 100;
    const wallets = normalizeAutoFollowWalletList(config);
    return {
        id: Number(config.id) || 0,
        target_wallet_address: String(config.target_wallet_address || ''),
        target_wallet_addresses: wallets,
        execution_wallet_id: config.execution_wallet_id ? String(config.execution_wallet_id) : '',
        execution_wallet_address: normalizeWalletAddress(config.execution_wallet_address),
        trigger_mode: config.trigger_mode === 'threshold' ? 'threshold' : 'any',
        trigger_min_wallets: config.trigger_min_wallets != null ? String(config.trigger_min_wallets) : '2',
        trigger_window_seconds: config.trigger_window_seconds != null ? String(config.trigger_window_seconds) : '300',
        enabled: Boolean(config.enabled),
        amount_mode: config.amount_mode === 'ratio' ? 'ratio' : 'fixed',
        fixed_amount_usdt: config.fixed_amount_usdt != null ? String(config.fixed_amount_usdt) : '',
        ratio_percent: Number.isFinite(ratioPct) ? String(ratioPct) : '100',
        delay_mode: config.delay_mode === 'fixed_delay' ? 'fixed_delay' : 'immediate',
        delay_seconds: config.delay_seconds != null ? String(config.delay_seconds) : '0',
        follow_close: Boolean(config.follow_close),
    };
}

function autoFollowDraftReducer(state, action) {
    switch (action.type) {
        case 'reset':
            return ensureAutoFollowDraftExecutionWallet(
                action.payload ? createAutoFollowDraft(action.payload) : { ...AUTO_FOLLOW_DRAFT_INITIAL },
                action.wallets
            );
        case 'set':
            return { ...state, ...action.payload };
        case 'ensureExecutionWallet': {
            return ensureAutoFollowDraftExecutionWallet(state, action.payload);
        }
        default:
            return state;
    }
}

function normalizeAutoFollowDraft(draft) {
    const wallets = parseAutoFollowWalletInputs(draft.target_wallet_addresses);
    const executionWalletID = Number(draft.execution_wallet_id);
    if (!Number.isFinite(executionWalletID) || executionWalletID <= 0) {
        throw new Error('请选择执行钱包');
    }
    const executionWalletAddress = normalizeWalletAddress(draft.execution_wallet_address);
    const amountMode = draft.amount_mode === 'ratio' ? 'ratio' : 'fixed';
    let fixedAmount = 0;
    let ratio = 1;
    if (amountMode === 'fixed') {
        fixedAmount = Number(draft.fixed_amount_usdt);
        if (!Number.isFinite(fixedAmount) || fixedAmount <= 0) {
            throw new Error('固定金额必须大于 0');
        }
    } else {
        const pct = Number(draft.ratio_percent);
        if (!Number.isFinite(pct) || pct <= 0) {
            throw new Error('跟单比例必须大于 0');
        }
        ratio = pct / 100;
    }
    const delayMode = draft.delay_mode === 'fixed_delay' ? 'fixed_delay' : 'immediate';
    let delaySeconds = 0;
    if (delayMode === 'fixed_delay') {
        delaySeconds = Math.max(0, Math.round(Number(draft.delay_seconds) || 0));
    }
    const triggerMode = draft.trigger_mode === 'threshold' ? 'threshold' : 'any';
    let triggerMinWallets = 1;
    let triggerWindowSeconds = 300;
    if (triggerMode === 'threshold') {
        triggerMinWallets = Math.round(Number(draft.trigger_min_wallets));
        triggerWindowSeconds = Math.round(Number(draft.trigger_window_seconds));
        if (!Number.isFinite(triggerMinWallets) || triggerMinWallets < 2) {
            throw new Error('触发钱包数至少为 2');
        }
        if (triggerMinWallets > wallets.length) {
            throw new Error('触发钱包数不能超过目标钱包数量');
        }
        if (!Number.isFinite(triggerWindowSeconds) || triggerWindowSeconds <= 0 || triggerWindowSeconds > 86400) {
            throw new Error('统计窗口必须在 1 到 86400 秒之间');
        }
    }
    return {
        id: Number(draft.id) || 0,
        target_wallet_address: wallets[0],
        target_wallet_addresses: wallets,
        execution_wallet_id: executionWalletID,
        execution_wallet_address: executionWalletAddress,
        trigger_mode: triggerMode,
        trigger_min_wallets: triggerMinWallets,
        trigger_window_seconds: triggerWindowSeconds,
        enabled: Boolean(draft.enabled),
        amount_mode: amountMode,
        fixed_amount_usdt: amountMode === 'fixed' ? fixedAmount : 0,
        ratio: amountMode === 'ratio' ? ratio : 1,
        delay_mode: delayMode,
        delay_seconds: delaySeconds,
        follow_close: Boolean(draft.follow_close),
    };
}

function autoFollowStatusInfo(status) {
    const s = String(status || '').toLowerCase();
    switch (s) {
        case 'success': return { label: '成功', cls: 'af-status-success', Icon: CheckCircle2 };
        case 'failed': return { label: '失败', cls: 'af-status-failed', Icon: XCircle };
        case 'running': return { label: '执行中', cls: 'af-status-running', Icon: Activity };
        case 'skipped': return { label: '跳过', cls: 'af-status-skipped', Icon: AlertCircle };
        case 'pending': return { label: '等待', cls: 'af-status-pending', Icon: Clock };
        default: return { label: s || '未知', cls: 'af-status-pending', Icon: Clock };
    }
}

function formatJobTime(value) {
    if (!value) return '--';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '--';
    const now = Date.now();
    const diff = now - date.getTime();
    if (diff >= 0 && diff < 60_000) return '刚刚';
    if (diff >= 0 && diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
    if (diff >= 0 && diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`;
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function aggregateAutoFollowStats(configs, jobs) {
    const total = configs.length;
    const running = configs.reduce((acc, c) => acc + (c?.enabled ? 1 : 0), 0);
    const finished = jobs.filter((j) => ['success', 'failed', 'skipped'].includes(String(j.status || '').toLowerCase()));
    const succeeded = jobs.filter((j) => String(j.status || '').toLowerCase() === 'success');
    const successRate = finished.length > 0 ? Math.round((succeeded.length / finished.length) * 100) : 0;
    const totalUsdt = jobs.reduce((acc, j) => acc + (Number(j.amount_usdt) || 0), 0);
    const openCount = jobs.filter((j) => j.action === 'open').length;
    const closeCount = jobs.filter((j) => j.action === 'close').length;
    return { total, running, successRate, totalUsdt, openCount, closeCount, finished: finished.length, jobs: jobs.length };
}

function AutoFollowSummaryBar({ stats }) {
    return (
        <div className="af-summary-grid">
            <div className="af-summary-cell">
                <div className="af-summary-label">配置数</div>
                <div className="af-summary-value">{stats.total}</div>
                <div className="af-summary-sub">{stats.running} 运行中</div>
            </div>
            <div className="af-summary-cell">
                <div className="af-summary-label">近 30 笔成功率</div>
                <div className={`af-summary-value${stats.successRate >= 80 ? ' pos' : stats.successRate > 0 && stats.successRate < 60 ? ' neg' : ''}`}>
                    {stats.finished > 0 ? `${stats.successRate}%` : '--'}
                </div>
                <div className="af-summary-sub">{stats.finished} / {stats.jobs} 已完成</div>
            </div>
            <div className="af-summary-cell">
                <div className="af-summary-label">近 30 笔金额</div>
                <div className="af-summary-value">{formatUsd(stats.totalUsdt)}</div>
                <div className="af-summary-sub">累计跟单</div>
            </div>
            <div className="af-summary-cell">
                <div className="af-summary-label">动作分布</div>
                <div className="af-summary-value af-summary-value--mini">
                    <span className="af-pill af-pill--open">开 {stats.openCount}</span>
                    <span className="af-pill af-pill--close">撤 {stats.closeCount}</span>
                </div>
                <div className="af-summary-sub">近 30 笔</div>
            </div>
        </div>
    );
}

function AutoFollowForm({ draft, dispatch, saving, hasInitData, executionWallets, walletsLoading, onSubmit, onReset }) {
    const editing = Number(draft.id) > 0;
    const isFixed = draft.amount_mode === 'fixed';
    const isImmediate = draft.delay_mode === 'immediate';
    const wallets = Array.isArray(draft.target_wallet_addresses) && draft.target_wallet_addresses.length > 0
        ? draft.target_wallet_addresses
        : [''];
    const isThreshold = draft.trigger_mode === 'threshold';
    const updateWallet = (index, value) => {
        const next = wallets.map((wallet, i) => (i === index ? value : wallet));
        dispatch({ type: 'set', payload: { target_wallet_addresses: next, target_wallet_address: next[0] || '' } });
    };
    const removeWallet = (index) => {
        const next = wallets.filter((_, i) => i !== index);
        const finalWallets = next.length ? next : [''];
        dispatch({ type: 'set', payload: { target_wallet_addresses: finalWallets, target_wallet_address: finalWallets[0] || '' } });
    };
    const addWallet = () => {
        dispatch({ type: 'set', payload: { target_wallet_addresses: [...wallets, ''] } });
    };
    const updateExecutionWallet = (value) => {
        const wallet = findAutoFollowExecutionWallet(executionWallets, value, '');
        dispatch({
            type: 'set',
            payload: {
                execution_wallet_id: wallet ? String(wallet.id) : '',
                execution_wallet_address: wallet ? normalizeWalletAddress(wallet.address) : '',
            },
        });
    };
    return (
        <div className="af-form">
            <div className="af-form-row">
                <label className="af-field-label">执行钱包</label>
                {walletsLoading ? (
                    <div className="af-wallet-loading">加载钱包中…</div>
                ) : (
                    <div className="af-exec-wallet-list" role="radiogroup" aria-label="执行钱包">
                        {executionWallets.map((wallet) => {
                            const active = Number(draft.execution_wallet_id) === Number(wallet.id);
                            const addr = normalizeWalletAddress(wallet.address);
                            return (
                                <button
                                    key={wallet.id}
                                    type="button"
                                    className={`af-exec-wallet-option${active ? ' active' : ''}`}
                                    onClick={() => updateExecutionWallet(wallet.id)}
                                    aria-pressed={active}
                                >
                                    <Wallet size={13} />
                                    <span>
                                        <strong>{String(wallet.name || '').trim() || `钱包 #${wallet.id}`}</strong>
                                        <small>{shortAddr(addr)}{wallet.is_default ? ' · 默认' : ''}</small>
                                    </span>
                                </button>
                            );
                        })}
                    </div>
                )}
            </div>

            <div className="af-form-row">
                <div className="af-wallet-head">
                    <label className="af-field-label">目标钱包地址</label>
                    <button type="button" className="af-inline-btn" onClick={addWallet}>
                        <Plus size={12} /> 添加钱包
                    </button>
                </div>
                <div className="af-wallet-list">
                    {wallets.map((wallet, index) => (
                        <div className="af-wallet-row" key={index}>
                            <input
                                type="text"
                                className="af-input af-input--mono"
                                placeholder="0x... 42 位 EVM 地址"
                                value={wallet}
                                onChange={(e) => updateWallet(index, e.target.value)}
                                spellCheck={false}
                                autoComplete="off"
                            />
                            <button
                                type="button"
                                className="af-icon-btn af-icon-btn--danger"
                                onClick={() => removeWallet(index)}
                                disabled={wallets.length === 1}
                                title="移除钱包"
                            >
                                <X size={14} />
                            </button>
                        </div>
                    ))}
                </div>
            </div>

            <div className="af-form-row">
                <label className="af-field-label">触发规则</label>
                <div className="af-segmented">
                    <button
                        type="button"
                        className={`af-segmented-btn${!isThreshold ? ' active' : ''}`}
                        onClick={() => dispatch({ type: 'set', payload: { trigger_mode: 'any' } })}
                    >
                        <Zap size={13} /> 任意钱包
                    </button>
                    <button
                        type="button"
                        className={`af-segmented-btn${isThreshold ? ' active' : ''}`}
                        onClick={() => dispatch({ type: 'set', payload: { trigger_mode: 'threshold' } })}
                    >
                        <Users size={13} /> 多钱包确认
                    </button>
                </div>
                {isThreshold && (
                    <div className="af-form-row af-form-row--split">
                        <div className="af-input-wrap">
                            <input
                                type="number"
                                min="2"
                                step="1"
                                className="af-input"
                                placeholder="触发数量"
                                value={draft.trigger_min_wallets}
                                onChange={(e) => dispatch({ type: 'set', payload: { trigger_min_wallets: e.target.value } })}
                            />
                            <span className="af-input-suffix">个</span>
                        </div>
                        <div className="af-input-wrap">
                            <input
                                type="number"
                                min="1"
                                step="1"
                                className="af-input"
                                placeholder="统计窗口"
                                value={draft.trigger_window_seconds}
                                onChange={(e) => dispatch({ type: 'set', payload: { trigger_window_seconds: e.target.value } })}
                            />
                            <span className="af-input-suffix">秒</span>
                        </div>
                    </div>
                )}
            </div>

            <div className="af-form-row af-form-row--split">
                <div className="af-field">
                    <label className="af-field-label">状态</label>
                    <div className="af-segmented">
                        <button
                            type="button"
                            className={`af-segmented-btn${draft.enabled ? ' active' : ''}`}
                            onClick={() => dispatch({ type: 'set', payload: { enabled: true } })}
                        >
                            <Play size={13} /> 开启
                        </button>
                        <button
                            type="button"
                            className={`af-segmented-btn${!draft.enabled ? ' active' : ''}`}
                            onClick={() => dispatch({ type: 'set', payload: { enabled: false } })}
                        >
                            <Pause size={13} /> 暂停
                        </button>
                    </div>
                </div>
                <div className="af-field">
                    <label className="af-field-label">撤仓跟单</label>
                    <div className="af-segmented">
                        <button
                            type="button"
                            className={`af-segmented-btn${draft.follow_close ? ' active' : ''}`}
                            onClick={() => dispatch({ type: 'set', payload: { follow_close: true } })}
                        >
                            <Check size={13} /> 跟单
                        </button>
                        <button
                            type="button"
                            className={`af-segmented-btn${!draft.follow_close ? ' active' : ''}`}
                            onClick={() => dispatch({ type: 'set', payload: { follow_close: false } })}
                        >
                            <X size={13} /> 忽略
                        </button>
                    </div>
                </div>
            </div>

            <div className="af-form-row">
                <label className="af-field-label">金额模式</label>
                <div className="af-segmented">
                    <button
                        type="button"
                        className={`af-segmented-btn${isFixed ? ' active' : ''}`}
                        onClick={() => dispatch({ type: 'set', payload: { amount_mode: 'fixed' } })}
                    >
                        <DollarSign size={13} /> 固定金额
                    </button>
                    <button
                        type="button"
                        className={`af-segmented-btn${!isFixed ? ' active' : ''}`}
                        onClick={() => dispatch({ type: 'set', payload: { amount_mode: 'ratio' } })}
                    >
                        <Percent size={13} /> 按比例
                    </button>
                </div>
                {isFixed ? (
                    <div className="af-input-wrap">
                        <input
                            type="number"
                            min="0"
                            step="0.01"
                            className="af-input"
                            placeholder="例如 100"
                            value={draft.fixed_amount_usdt}
                            onChange={(e) => dispatch({ type: 'set', payload: { fixed_amount_usdt: e.target.value } })}
                        />
                        <span className="af-input-suffix">USDT</span>
                    </div>
                ) : (
                    <div className="af-input-wrap">
                        <input
                            type="number"
                            min="0"
                            step="1"
                            className="af-input"
                            placeholder="例如 50"
                            value={draft.ratio_percent}
                            onChange={(e) => dispatch({ type: 'set', payload: { ratio_percent: e.target.value } })}
                        />
                        <span className="af-input-suffix">%</span>
                    </div>
                )}
            </div>

            <div className="af-form-row">
                <label className="af-field-label">跟单时机</label>
                <div className="af-segmented">
                    <button
                        type="button"
                        className={`af-segmented-btn${isImmediate ? ' active' : ''}`}
                        onClick={() => dispatch({ type: 'set', payload: { delay_mode: 'immediate', delay_seconds: '0' } })}
                    >
                        <Zap size={13} /> 立即
                    </button>
                    <button
                        type="button"
                        className={`af-segmented-btn${!isImmediate ? ' active' : ''}`}
                        onClick={() => dispatch({ type: 'set', payload: { delay_mode: 'fixed_delay' } })}
                    >
                        <Clock size={13} /> 延时
                    </button>
                </div>
                {!isImmediate && (
                    <div className="af-input-wrap">
                        <input
                            type="number"
                            min="0"
                            step="1"
                            className="af-input"
                            placeholder="延时秒数"
                            value={draft.delay_seconds}
                            onChange={(e) => dispatch({ type: 'set', payload: { delay_seconds: e.target.value } })}
                        />
                        <span className="af-input-suffix">秒</span>
                    </div>
                )}
            </div>

            <div className="af-form-actions">
                {editing && (
                    <button type="button" className="af-btn af-btn--ghost" onClick={onReset} disabled={saving}>
                        新建配置
                    </button>
                )}
                <button
                    type="button"
                    className="af-btn af-btn--primary"
                    onClick={onSubmit}
                    disabled={saving || !hasInitData || executionWallets.length === 0}
                >
                    {saving ? '保存中…' : editing ? '保存修改' : '新增配置'}
                </button>
            </div>
        </div>
    );
}

function AutoFollowConfigCard({ config, executionWallets, busy, onEdit, onToggle, onDelete }) {
    const amountText = config.amount_mode === 'ratio'
        ? `${Math.round(Number(config.ratio || 0) * 100)}% 仓位`
        : `${formatUsd(config.fixed_amount_usdt)} 固定`;
    const delayText = config.delay_mode === 'fixed_delay' ? `延时 ${config.delay_seconds}s` : '立即跟单';
    const wallets = normalizeAutoFollowWalletList(config).filter(Boolean);
    return (
        <div className={`af-config-card${config.enabled ? ' active' : ''}`}>
            <div className="af-config-head">
                <div className="af-config-addr">
                    <span className="af-config-dot" />
                    <span className="af-config-addr-text">
                        {wallets.length > 1 ? `${shortAddr(wallets[0])} +${wallets.length - 1}` : shortAddr(config.target_wallet_address)}
                    </span>
                    <span className={`af-pill ${config.enabled ? 'af-pill--on' : 'af-pill--off'}`}>
                        {config.enabled ? '运行中' : '已暂停'}
                    </span>
                </div>
                <div className="af-config-actions">
                    <button type="button" className="af-icon-btn" onClick={() => onEdit(config)} title="编辑">
                        <Pencil size={14} />
                    </button>
                    <button
                        type="button"
                        className="af-icon-btn"
                        onClick={() => onToggle(config)}
                        title={config.enabled ? '暂停' : '开启'}
                        disabled={busy}
                    >
                        {config.enabled ? <Pause size={14} /> : <Play size={14} />}
                    </button>
                    <button
                        type="button"
                        className="af-icon-btn af-icon-btn--danger"
                        onClick={() => onDelete(config)}
                        title="删除"
                        disabled={busy}
                    >
                        <Trash2 size={14} />
                    </button>
                </div>
            </div>
            <div className="af-config-meta">
                <span className="af-meta-tag"><Users size={11} />{autoFollowTriggerText(config)}</span>
                <span className="af-meta-tag"><Wallet size={11} />{formatAutoFollowExecutionWallet(config, executionWallets)}</span>
                <span className="af-meta-tag"><DollarSign size={11} />{amountText}</span>
                <span className="af-meta-tag"><Clock size={11} />{delayText}</span>
                <span className="af-meta-tag">
                    {config.follow_close ? <Check size={11} /> : <X size={11} />}
                    撤仓{config.follow_close ? '跟单' : '忽略'}
                </span>
            </div>
        </div>
    );
}

function AutoFollowJobCard({ job, executionWallets }) {
    const info = autoFollowStatusInfo(job.status);
    const StatusIcon = info.Icon;
    const isOpen = job.action === 'open';
    const amount = Number(job.amount_usdt) > 0 ? formatUsd(job.amount_usdt) : '—';
    const triggerWallets = Array.isArray(job.trigger_wallet_addresses) ? job.trigger_wallet_addresses.filter(Boolean) : [];
    return (
        <div className="af-job-card">
            <div className={`af-job-stripe ${info.cls}`} />
            <div className="af-job-body">
                <div className="af-job-row1">
                    <span className={`af-job-status ${info.cls}`}>
                        <StatusIcon size={12} />
                        {info.label}
                    </span>
                    <span className={`af-job-action${isOpen ? ' open' : ' close'}`}>
                        {isOpen ? '开仓' : '撤仓'}
                    </span>
                    <span className="af-job-amount">{amount}</span>
                </div>
                <div className="af-job-row2">
                    <span className="af-job-addr">
                        {triggerWallets.length > 1 ? `${triggerWallets.length} 钱包触发` : shortAddr(triggerWallets[0] || job.target_wallet_address)}
                    </span>
                    <span className="af-job-time">{formatAutoFollowExecutionWallet(job, executionWallets)}</span>
                    <span className="af-job-time">{formatJobTime(job.scheduled_at)}</span>
                    <span className="af-job-id">#{job.id}</span>
                </div>
                {job.error_message ? (
                    <div className="af-job-error" title={job.error_message}>{job.error_message}</div>
                ) : null}
            </div>
        </div>
    );
}

function AutoFollowPanelContent({ apiBaseUrl, initData, chain = 'bsc', refreshInterval = 10, active = true }) {
    const hasInitData = Boolean(String(initData || '').trim());
    const [configs, setConfigs] = useState([]);
    const [jobs, setJobs] = useState([]);
    const [executionWallets, setExecutionWallets] = useState([]);
    const [draft, dispatch] = useReducer(autoFollowDraftReducer, undefined, () => createAutoFollowDraft());
    const [activeTab, setActiveTab] = useState('configure');
    const [loading, setLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [confirmTarget, setConfirmTarget] = useState(null);

    const refreshMs = useMemo(() => {
        const sec = Number(refreshInterval);
        if (!Number.isFinite(sec) || sec <= 0) return 10_000;
        return Math.max(5_000, Math.round(sec * 1000));
    }, [refreshInterval]);

    const load = useCallback(async (signal) => {
        if (!hasInitData) {
            setConfigs([]);
            setJobs([]);
            setExecutionWallets([]);
            return;
        }
        setLoading(true);
        setError('');
        try {
            const resp = await fetchSMAutoFollow({ apiBaseUrl, initData, chain, signal });
            const wallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
            setExecutionWallets(wallets);
            setConfigs(Array.isArray(resp?.configs) ? resp.configs : []);
            setJobs(Array.isArray(resp?.jobs) ? resp.jobs : []);
            dispatch({ type: 'ensureExecutionWallet', payload: wallets });
        } catch (err) {
            if (err?.name === 'AbortError') return;
            setError(String(err?.message || err || '加载失败'));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, chain, hasInitData, initData]);

    useEffect(() => {
        if (!active) return undefined;
        const ctrl = new AbortController();
        load(ctrl.signal);
        const timer = setInterval(() => {
            const c = new AbortController();
            load(c.signal);
        }, refreshMs);
        return () => {
            ctrl.abort();
            clearInterval(timer);
        };
    }, [active, load, refreshMs]);

    const stats = useMemo(() => aggregateAutoFollowStats(configs, jobs), [configs, jobs]);

    const handleReset = useCallback(() => {
        dispatch({ type: 'reset', wallets: executionWallets });
    }, [executionWallets]);

    const handleEdit = useCallback((config) => {
        dispatch({ type: 'reset', payload: config });
        setNotice('');
        setError('');
        setActiveTab('configure');
    }, []);

    const handleSubmit = useCallback(async () => {
        if (!hasInitData) {
            setError('缺少 Telegram initData，无法保存');
            return;
        }
        if (executionWallets.length === 0) {
            setError('没有可用执行钱包');
            return;
        }
        setSaving(true);
        setError('');
        setNotice('');
        try {
            const config = normalizeAutoFollowDraft(draft);
            await saveSMAutoFollowConfig({ apiBaseUrl, initData, chain, config });
            setNotice(draft.id ? '配置已更新' : '配置已新增');
            dispatch({ type: 'reset', wallets: executionWallets });
            await load();
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, chain, draft, executionWallets, hasInitData, initData, load]);

    const handleToggle = useCallback(async (config) => {
        if (!config || !hasInitData) return;
        setSaving(true);
        setError('');
        setNotice('');
        try {
            await saveSMAutoFollowConfig({
                apiBaseUrl,
                initData,
                chain,
                config: {
                    id: config.id,
                    chain: config.chain,
                    target_wallet_address: config.target_wallet_address,
                    target_wallet_addresses: normalizeAutoFollowWalletList(config).filter(Boolean),
                    execution_wallet_id: Number(config.execution_wallet_id || 0),
                    execution_wallet_address: config.execution_wallet_address || '',
                    trigger_mode: config.trigger_mode || 'any',
                    trigger_min_wallets: Number(config.trigger_min_wallets || 1),
                    trigger_window_seconds: Number(config.trigger_window_seconds || 300),
                    enabled: !config.enabled,
                    amount_mode: config.amount_mode,
                    fixed_amount_usdt: config.fixed_amount_usdt,
                    ratio: config.ratio,
                    delay_mode: config.delay_mode,
                    delay_seconds: config.delay_seconds,
                    follow_close: config.follow_close,
                },
            });
            setNotice(config.enabled ? '已暂停' : '已开启');
            await load();
        } catch (err) {
            setError(String(err?.message || err || '更新失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, chain, hasInitData, initData, load]);

    const handleDelete = useCallback(async () => {
        if (!confirmTarget || !hasInitData) return;
        setSaving(true);
        setError('');
        setNotice('');
        try {
            await deleteSMAutoFollowConfig({ apiBaseUrl, initData, chain, id: confirmTarget.id });
            setNotice('配置已删除');
            setConfirmTarget(null);
            if (draft.id === confirmTarget.id) {
                dispatch({ type: 'reset', wallets: executionWallets });
            }
            await load();
        } catch (err) {
            setError(String(err?.message || err || '删除失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, chain, confirmTarget, draft.id, executionWallets, hasInitData, initData, load]);

    return (
        <div className="af-panel">
            {!hasInitData && (
                <div className="af-banner af-banner--warn">
                    <AlertCircle size={14} />
                    缺少 Telegram initData，请通过 Telegram 入口打开以启用自动跟单。
                </div>
            )}
            {error && (
                <div className="af-banner af-banner--err">
                    <XCircle size={14} />
                    {error}
                </div>
            )}
            {notice && !error && (
                <div className="af-banner af-banner--ok">
                    <CheckCircle2 size={14} />
                    {notice}
                </div>
            )}

            <AutoFollowSummaryBar stats={stats} />

            <div className="af-tabs" role="tablist" aria-label="自动跟单">
                {[
                    { key: 'configure', label: Number(draft.id) > 0 ? '编辑任务' : '配置任务', Icon: Settings },
                    { key: 'configs', label: '我的跟单', Icon: Users, count: configs.length },
                    { key: 'jobs', label: '最近任务', Icon: Activity, count: jobs.length },
                ].map(({ key, label, Icon, count }) => (
                    <button
                        key={key}
                        type="button"
                        role="tab"
                        aria-selected={activeTab === key}
                        className={`af-tab-btn${activeTab === key ? ' active' : ''}`}
                        onClick={() => setActiveTab(key)}
                    >
                        <Icon size={14} />
                        <span>{label}</span>
                        {typeof count === 'number' ? <em>{count}</em> : null}
                    </button>
                ))}
            </div>

            {activeTab === 'configure' ? (
                <section className="af-section">
                    <header className="af-section-head">
                        <h3 className="af-section-title">
                            {Number(draft.id) > 0 ? '编辑跟单配置' : '新增跟单配置'}
                        </h3>
                        <span className="af-section-hint">
                            {Number(draft.id) > 0 ? `正在编辑 #${draft.id}` : 'BSC 链 · 监听目标钱包的 LP 动作'}
                        </span>
                    </header>
                    <AutoFollowForm
                        draft={draft}
                        dispatch={dispatch}
                        saving={saving}
                        hasInitData={hasInitData}
                        executionWallets={executionWallets}
                        walletsLoading={loading && executionWallets.length === 0}
                        onSubmit={handleSubmit}
                        onReset={handleReset}
                    />
                </section>
            ) : null}

            {activeTab === 'configs' ? (
                <section className="af-section">
                    <header className="af-section-head">
                        <h3 className="af-section-title">我的跟单 ({configs.length})</h3>
                        <span className="af-section-hint">{stats.running} 个运行中</span>
                    </header>
                    {loading && configs.length === 0 ? (
                        <div className="af-empty">加载中…</div>
                    ) : configs.length === 0 ? (
                        <div className="af-empty">暂无跟单配置，可到“配置任务”新增。</div>
                    ) : (
                        <div className="af-config-list">
                            {configs.map((c) => (
                                <AutoFollowConfigCard
                                    key={c.id}
                                    config={c}
                                    executionWallets={executionWallets}
                                    busy={saving}
                                    onEdit={handleEdit}
                                    onToggle={handleToggle}
                                    onDelete={setConfirmTarget}
                                />
                            ))}
                        </div>
                    )}
                </section>
            ) : null}

            {activeTab === 'jobs' ? (
                <section className="af-section">
                    <header className="af-section-head">
                        <h3 className="af-section-title">最近任务</h3>
                        <span className="af-section-hint">近 {jobs.length} 条</span>
                    </header>
                    {jobs.length === 0 ? (
                        <div className="af-empty">还没有执行记录，跟单生效后会在这里出现。</div>
                    ) : (
                        <div className="af-job-list">
                            {jobs.slice(0, 12).map((j) => (
                                <AutoFollowJobCard key={j.id} job={j} executionWallets={executionWallets} />
                            ))}
                        </div>
                    )}
                </section>
            ) : null}

            <ConfirmDialog
                open={Boolean(confirmTarget)}
                title="删除跟单配置"
                description={confirmTarget ? `确认删除对钱包 ${shortAddr(confirmTarget.target_wallet_address)} 的跟单配置？删除后将不再跟单该钱包。` : ''}
                confirmLabel="删除"
                busy={saving}
                onConfirm={handleDelete}
                onCancel={() => setConfirmTarget(null)}
            />
        </div>
    );
}
