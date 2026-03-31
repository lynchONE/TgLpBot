import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import {
    Eye, Wallet, Settings, Search, Plus, ExternalLink, X, Check,
    ChevronRight, ChevronDown, ChevronLeft, Pause, Play, Trash2, Copy, Brain, Flame, Pencil,
} from 'lucide-react';
import {
    fetchSMPools, fetchSMPoolStats, fetchSMPositionDetail, fetchSMPositions, fetchSMWallets,
    fetchSMStats, addSMWallet, updateSMWallet, deleteSMWallet,
    fetchSMContracts, addSMContract, updateSMContract, deleteSMContract,
    uploadSMWalletAvatar, resolveSMAvatarAssetUrl,
    fetchSMGoldenDogConfig, saveSMGoldenDogConfig, testSMGoldenDogConfig,
} from '../smartMoneyApi';
import { buildGmgnUrl, compactPrice, computePriceRange, formatDuration, formatUsd, shortAddress } from '../utils';
import uniswapLogo from '../img/uniswap.svg';
import pancakeLogo from '../img/pancake.svg';
import flashIcon from '../img/flash.svg';
import gmgnIcon from '../img/gmgn.svg';

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

function formatRangeDrift(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num < 0) return '--';
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

function buildRangeStatusSummary(rangeState) {
    if (!rangeState) return null;
    if (rangeState.inRange) {
        return { text: '区间内', tone: 'positive' };
    }
    if (rangeState.outOfRange?.direction) {
        const direction = rangeState.outOfRange.direction === 'above' ? '高于区间' : '低于区间';
        return { text: `${direction} ${formatRangeDrift(rangeState.outOfRange.pct)}`, tone: 'negative' };
    }
    if (rangeState.inRange === false) {
        return { text: '已离开区间', tone: 'negative' };
    }
    return null;
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
const POSITION_PREVIEW_STALE_MS = 30000;
const POSITION_PREVIEW_BATCH_SIZE = 4;
const POSITION_LIST_PAGE_SIZE = 6;
const WALLET_LIST_PAGE_SIZE = 10;

function getPositionSelectionKey(position) {
    const positionRef = String(position?.position_ref || '').trim();
    if (positionRef) return positionRef;
    const id = String(position?.id || '').trim();
    if (id) return id;
    const wallet = String(position?.wallet_address || '').trim().toLowerCase();
    const pool = String(position?.pool_address || '').trim().toLowerCase();
    const nft = String(position?.nft_token_id || '').trim();
    return [wallet, pool, nft].filter(Boolean).join(':');
}

function useSmartMoneyPositionPreviewMap(apiBaseUrl, positions) {
    const [previewMap, setPreviewMap] = useState({});
    const previewRef = useRef(previewMap);

    useEffect(() => {
        previewRef.current = previewMap;
    }, [previewMap]);

    useEffect(() => {
        const rows = Array.isArray(positions) ? positions : [];
        if (rows.length === 0) return undefined;

        const now = Date.now();
        const pending = rows.filter((position) => {
            const key = getPositionSelectionKey(position);
            if (!key) return false;
            const cached = previewRef.current[key];
            return !cached || now - Number(cached.fetchedAt || 0) >= POSITION_PREVIEW_STALE_MS;
        });
        if (pending.length === 0) return undefined;

        let cancelled = false;

        const loadPreview = async (position) => {
            const key = getPositionSelectionKey(position);
            if (!key) return;
            try {
                const data = await fetchSMPositionDetail({
                    apiBaseUrl,
                    positionRef: position.position_ref,
                    positionId: position.id,
                });
                if (cancelled) return;
                setPreviewMap((prev) => ({
                    ...prev,
                    [key]: {
                        fetchedAt: Date.now(),
                        currentValueUsd: Number.isFinite(Number(data?.current_value_usd))
                            ? Number(data.current_value_usd)
                            : Number(data?.totals?.position_usd || 0) + Number(data?.totals?.fee_usd || 0),
                        feeUsd: Number(data?.totals?.fee_usd ?? 0),
                        netInvestedUsd: Number(data?.net_invested_usd ?? position?.position_amount_usd ?? 0),
                        rangeStatus: buildRangeStatusSummary(
                            computePriceRange(data) || (data?.in_range === undefined ? null : { inRange: Boolean(data.in_range) })
                        ),
                        runningSince: String(data?.running_since || position?.opened_at || '').trim(),
                    },
                }));
            } catch (error) {
                if (cancelled) return;
                setPreviewMap((prev) => ({
                    ...prev,
                    [key]: {
                        ...(prev[key] || {}),
                        fetchedAt: Date.now(),
                        runningSince: String(prev[key]?.runningSince || position?.opened_at || '').trim(),
                    },
                }));
            }
        };

        (async () => {
            for (let index = 0; index < pending.length && !cancelled; index += POSITION_PREVIEW_BATCH_SIZE) {
                const batch = pending.slice(index, index + POSITION_PREVIEW_BATCH_SIZE);
                await Promise.all(batch.map((position) => loadPreview(position)));
            }
        })();

        return () => {
            cancelled = true;
        };
    }, [apiBaseUrl, positions]);

    return previewMap;
}

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
    return Math.max(Math.round(seconds), 10) * 1000;
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

function WalletIdentity({ address, color, label, avatarUrl, size = 16, onClick, showCopy = false }) {
    const content = (
        <>
            <WalletAvatar address={address} color={color} avatarUrl={avatarUrl} size={size} />
            <span className="smd-wallet-info-name">{label && label !== address ? label : tailAddr(address)}</span>
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

function StatCard({ label, value, color }) {
    return (
        <div className="smd-stat-card">
            <div className="smd-stat-label">{label}</div>
            <div className={`smd-stat-value${color === 'red' ? ' red' : ''}`}>{value ?? '--'}</div>
        </div>
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
                    if (!a[p.wallet_address]) a[p.wallet_address] = { color: p.wallet_color, label: p.wallet_label };
                    return a;
                }, {})).map(([addr, { color, label }]) => (
                    <span key={addr}>
                        <span className="smd-legend-dot" style={{ backgroundColor: color }} />
                        {label || shortAddr(addr)}
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
    const feeValue = Number(preview?.feeUsd);
    const feeText = Number.isFinite(feeValue) ? formatUsd(preview.feeUsd) : '--';
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
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [proto, setProto] = useState('all');
    const normalizedActivePoolAddress = useMemo(
        () => normalizePoolSelectionId(activePoolAddress),
        [activePoolAddress]
    );
    const refreshIntervalMs = useMemo(
        () => getRefreshIntervalMs(refreshInterval),
        [refreshInterval]
    );

    const loadPools = useCallback((silent = false) => {
        if (!silent) setLoading(true);
        return fetchSMPools({ apiBaseUrl })
            .then((d) => setPools(d?.list || []))
            .catch(() => { })
            .finally(() => {
                if (!silent) setLoading(false);
            });
    }, [apiBaseUrl]);

    useEffect(() => {
        loadPools();
    }, [loadPools]);

    useEffect(() => {
        const timer = setInterval(() => {
            loadPools(true);
        }, refreshIntervalMs);
        return () => clearInterval(timer);
    }, [loadPools, refreshIntervalMs]);

    const filtered = useMemo(() => {
        let l = pools;
        if (search) {
            const q = search.toLowerCase();
            l = l.filter((p) => getPairLabel(p).toLowerCase().includes(q) || getPoolIdentifier(p).toLowerCase().includes(q));
        }
        if (proto !== 'all') l = l.filter(p => p.protocol === proto);
        return l;
    }, [pools, search, proto]);

    return (
        <div>
            <div className="smd-search-row">
                <div className="smd-search-input">
                    <Search size={14} />
                    <input placeholder="搜索池子..." value={search} onChange={e => setSearch(e.target.value)} />
                </div>
                <div className="smd-filter-group">
                    {['all', 'pancake_v3', 'uniswap_v3', 'uniswap_v4'].map(p => {
                        const info = PROTOCOL_MAP[p];
                        return (
                            <button key={p} className={`smd-filter-btn${proto === p ? ' active' : ''}`} onClick={() => setProto(p)}>
                                {info && <img src={info.icon} alt="" className="smd-proto-img" />}
                                {p === 'all' ? '全部' : info?.version || p}
                            </button>
                        );
                    })}
                </div>
            </div>
            {loading ? <div className="smd-loading">加载中...</div> : filtered.length === 0 ? (
                <div className="smd-empty">暂无活跃仓位的池子</div>
            ) : (
                <div className="smd-pool-cards">
                    {filtered.map((p) => {
                        const isActive = normalizedActivePoolAddress && normalizePoolSelectionId(p) === normalizedActivePoolAddress;
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
            )}
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
                                            size={28}
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

function WalletList({ apiBaseUrl, onSelect, onAdd, refreshInterval = 10 }) {
    const [wallets, setWallets] = useState([]);
    const [walletsTotal, setWalletsTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [page, setPage] = useState(1);
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [editingWallet, setEditingWallet] = useState(null);
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
                                <th className="right">余额</th>
                                <th className="right">持仓</th>
                                <th className="right">池子</th>
                                <th className="right">操作</th>
                            </tr>
                        </thead>
                        <tbody>
                            {wallets.map(w => (
                                <tr key={w.address} className="clickable" onClick={() => onSelect(w.address)}>
                                    <td>
                                        <WalletIdentity address={w.address} color={w.color} label={w.label || w.address} avatarUrl={w.avatar_url} size={20} showCopy />
                                    </td>
                                    <td className="center">
                                        <span className={`smd-status-dot ${w.is_active ? 'green' : 'muted'}`}>
                                            {w.is_active ? '监控中' : '已暂停'}
                                        </span>
                                    </td>
                                    <td className="right">{formatWalletBalance(w.wallet_balance_usd)}</td>
                                    <td className="right">{w.open_position_count}</td>
                                    <td className="right">{w.active_pool_count}</td>
                                    <td className="right">
                                        <div className="smd-action-row" style={{ justifyContent: 'flex-end' }}>
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
                                                    description: `确认删除钱包 ${shortAddr(w.address)} 吗？`,
                                                    action: () => deleteSMWallet({ apiBaseUrl, address: w.address }),
                                                });
                                            }}><Trash2 size={14} /></button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
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

function WalletDetail({ apiBaseUrl, addr, onBack, onSelectPool, refreshInterval = 10 }) {
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
                                <button className="smd-link" onClick={() => onSelectPool({
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
    onSelectPool,
    activePoolAddress = '',
    refreshInterval = 10,
    onOpenPosition,
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
    const monitorSummary = useMemo(() => {
        const activeWallets = stats?.monitored_wallet_count ?? 0;
        const activeContracts = stats?.active_contract_count ?? 0;
        const watcherEnabled = Boolean(stats?.watcher_enabled);
        const contractMonitorEnabled = Boolean(stats?.crawler_enabled);
        const monitorEnabled = Boolean(stats?.monitor_enabled);

        if (!monitorEnabled) {
            return {
                enabled: false,
                label: '监控未开启',
                detail: '后端 Smart Money 服务未启动',
            };
        }

        const channels = [];
        if (watcherEnabled) channels.push(`LP 监听 ${activeWallets} 钱包`);
        if (contractMonitorEnabled) channels.push(activeContracts > 0 ? `合约监控 ${activeContracts} 个` : '合约监控待配置');

        return {
            enabled: true,
            label: '监控已开启',
            detail: channels.length ? channels.join(' / ') : 'Smart Money 服务运行中',
        };
    }, [stats]);

    return (
        <section className="panel-shell">
            <header className="panel-header">
                <div className="panel-title-wrap">
                    <div className="panel-icon-wrap"><Brain size={16} /></div>
                    <div className="panel-title-texts">
                        <h2>聪明钱监控</h2>
                        {!isDetail && <p>跟踪聪明钱 LP 仓位动态</p>}
                    </div>
                </div>
            </header>
            <div className="panel-body">
                {stats && !isDetail && (
                    <div className={`smd-monitor-banner${monitorSummary.enabled ? '' : ' off'}`}>
                        <div className="smd-monitor-pill">
                            <span className="smd-monitor-dot" />
                            {monitorSummary.label}
                        </div>
                        <div className="smd-monitor-detail">{monitorSummary.detail}</div>
                    </div>
                )}

                {stats && !isDetail && (
                    <div className="smd-stats-grid">
                        <StatCard label="活跃池子" value={stats.active_pool_count} />
                        <StatCard label="监控钱包" value={stats.monitored_wallet_count} />
                        <StatCard label="持仓笔数" value={stats.open_position_count} />
                        <StatCard label="今日关闭" value={stats.closed_today_count} color="red" />
                    </div>
                )}

                {!isDetail && (
                    <div className="smd-tabs">
                        {[
                            { key: 'pools', label: '池子视图', icon: Eye },
                            { key: 'wallets', label: '钱包视图', icon: Wallet },
                            { key: 'settings', label: '合约视图', icon: Settings },
                        ].map(({ key, label, icon: Icon }) => (
                            <button key={key} className={`smd-tab${view === key ? ' active' : ''}`} onClick={() => setView(key)}>
                                <Icon size={16} /> {label}
                            </button>
                        ))}
                        <button
                            key="golden_dog"
                            className={`smd-tab${view === 'golden_dog' ? ' active' : ''}`}
                            onClick={() => setView('golden_dog')}
                        >
                            <Flame size={16} /> 监控通知
                        </button>
                    </div>
                )}

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
                    />
                ) : view === 'pools' ? (
                    <PoolList
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
                    />
                ) : view === 'golden_dog' ? (
                    <GoldenDogPanelContent apiBaseUrl={apiBaseUrl} initData={initData} />
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
            </div>
        </section>
    );
}

function AddWalletForm({ apiBaseUrl, onDone }) {
    const [addr, setAddr] = useState('');
    const [label, setLabel] = useState('');
    const [saving, setSaving] = useState(false);
    return (
        <div className="smd-modal-form">
            <input placeholder="钱包地址 (0x...)" value={addr} onChange={e => setAddr(e.target.value)} />
            <input placeholder="标签（可选）" value={label} onChange={e => setLabel(e.target.value)} />
            <div className="smd-modal-actions">
                <button onClick={onDone} className="smd-modal-cancel">取消</button>
                <button disabled={!addr || saving} className="smd-modal-submit" onClick={async () => {
                    setSaving(true);
                    try { await addSMWallet({ apiBaseUrl, address: addr, label }); onDone(); } catch (e) { alert(e.message); } finally { setSaving(false); }
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
            intensity: 'ring',
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

function formatGoldenDogDraftValue(value, { emptyWhenZero = false, multiplier = 1 } = {}) {
    const num = Number(value);
    if (!Number.isFinite(num)) return emptyWhenZero ? '' : '0';
    const scaled = Number((num * multiplier).toFixed(4));
    if (emptyWhenZero && Math.abs(scaled) < 0.000001) return '';
    return String(scaled);
}

function mapGoldenDogConfigToDraft(cfg) {
    const next = createGoldenDogDraft();
    const source = cfg || {};
    next.wallet_mode.enabled = Boolean(source.enabled);
    next.wallet_mode.min_wallets = String(source.min_wallets ?? 3);
    next.wallet_mode.window_minutes = String(source.window_minutes ?? 10);
    next.wallet_mode.cooldown_minutes = String(source.cooldown_minutes ?? 30);
    next.wallet_mode.intensity = String(source.wallet_intensity || 'ring');
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

function GoldenDogPanelContent({ apiBaseUrl, initData }) {
    const hasInitData = Boolean(String(initData || '').trim());
    const [loading, setLoading] = useState(hasInitData);
    const [saving, setSaving] = useState(false);
    const [testingMode, setTestingMode] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [status, setStatus] = useState(null);
    const [draft, setDraft] = useState(() => createGoldenDogDraft());
    const [activeTab, setActiveTab] = useState('wallet');

    const barkStatusText = useMemo(() => goldenDogBarkStatusText(status), [status]);
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

    const applyResponse = useCallback((resp) => {
        setStatus(resp || null);
        setDraft(mapGoldenDogConfigToDraft(resp?.config));
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

    const updateWalletMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            wallet_mode: { ...prev.wallet_mode, [key]: value },
        }));
    }, []);

    const updatePoolMode = useCallback((key, value) => {
        setDraft((prev) => ({
            ...prev,
            pool_mode: { ...prev.pool_mode, [key]: value },
        }));
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
                intensity: draft.wallet_mode.intensity || 'ring',
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
            const resp = await saveSMGoldenDogConfig({
                apiBaseUrl, initData, chain: 'bsc',
                config: buildSavePayload(),
            });
            applyResponse(resp);
            setNotice('配置已保存');
        } catch (err) {
            setError(String(err?.message || err || '保存失败'));
        } finally {
            setSaving(false);
        }
    }, [apiBaseUrl, applyResponse, buildSavePayload, hasInitData, initData]);

    const handleTest = useCallback(async (mode) => {
        if (!hasInitData) {
            setError('请先登录 WebApp，拿到 initData 后才能测试监控通知。');
            return;
        }
        setTestingMode(mode);
        setError('');
        setNotice('');
        try {
            const intensity = mode === 'pool' ? draft.pool_mode.intensity : draft.wallet_mode.intensity;
            const resp = await testSMGoldenDogConfig({
                apiBaseUrl, initData, chain: 'bsc', mode, intensity,
            });
            setNotice(resp?.message || '测试通知已发送');
        } catch (err) {
            setError(String(err?.message || err || '测试失败'));
        } finally {
            setTestingMode('');
        }
    }, [apiBaseUrl, draft.pool_mode.intensity, draft.wallet_mode.intensity, hasInitData, initData]);

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
                                钱包 {draft.wallet_mode.enabled ? 'ON' : 'OFF'}
                            </span>
                            <span style={{ fontSize: 11, padding: '1px 7px', borderRadius: 6, background: draft.pool_mode.enabled ? 'rgba(52,211,153,0.12)' : 'rgba(255,255,255,0.04)', color: draft.pool_mode.enabled ? '#6ee7b7' : '#52525b', fontWeight: 500 }}>
                                池子 {draft.pool_mode.enabled ? 'ON' : 'OFF'}
                            </span>
                            <span style={{ fontSize: 11, padding: '1px 7px', borderRadius: 6, background: 'rgba(255,255,255,0.04)', color: '#71717a', fontWeight: 500 }}>
                                Bark {barkStatusText}
                            </span>
                        </div>
                    </div>
                </div>
                <button type="button" disabled={saving || !hasInitData} onClick={handleSave} style={saveBtnCss}>
                    {saving ? '保存中...' : '💾 保存配置'}
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
                                    { l: '统计窗口', v: `${draft.wallet_mode.window_minutes || '--'}分钟` },
                                    { l: '冷却', v: `${draft.wallet_mode.cooldown_minutes || '--'}分钟` },
                                    { l: '强度', v: goldenDogIntensityLabel(draft.wallet_mode.intensity) },
                                ].map((s, i) => (
                                    <div key={s.l} style={{
                                        flex: 1, padding: '8px 10px', textAlign: 'center',
                                        borderRight: i < 3 ? '1px solid rgba(255,255,255,0.04)' : 'none',
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
                                <div style={fieldLabelCss}>
                                    <span style={fieldLabelTextCss}>通知强度</span>
                                    <CustomSelect value={draft.wallet_mode.intensity} options={intensityOptions} onChange={(v) => updateWalletMode('intensity', v)} />
                                </div>
                            </div>
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
                </>
            )}
        </div>
    );
}
