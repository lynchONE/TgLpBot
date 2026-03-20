import React, { useState, useEffect, useCallback, useMemo } from 'react';
import {
    Eye, Wallet, Settings, Search, Plus, ExternalLink, X, Check,
    ChevronRight, ChevronDown, ChevronLeft, Pause, Play, Trash2, Copy, Brain, Flame,
} from 'lucide-react';
import {
    fetchSMPools, fetchSMPoolStats, fetchSMPositions, fetchSMWallets,
    fetchSMStats, addSMWallet, updateSMWallet, deleteSMWallet,
    fetchSMContracts, addSMContract, updateSMContract, deleteSMContract,
    fetchSMGoldenDogConfig, saveSMGoldenDogConfig,
} from '../smartMoneyApi';
import uniswapLogo from '../img/uniswap.svg';
import pancakeLogo from '../img/pancake.svg';

const PROTOCOL_MAP = {
    pancake_v3: { version: 'V3', icon: pancakeLogo, color: '#d1884f' },
    uniswap_v3: { version: 'V3', icon: uniswapLogo, color: '#ff007a' },
    uniswap_v4: { version: 'V4', icon: uniswapLogo, color: '#ff007a' },
};
const PROTOCOL_LABELS = Object.fromEntries(Object.entries(PROTOCOL_MAP).map(([k, v]) => [k, v.version]));
fu const SMART_MONEY_PROTOCOL_OPTIONS = [
    { value: 'pancake_v3', label: 'PancakeSwap V3' },
    { value: 'uniswap_v3', label: 'Uniswap V3' },
    { value: 'uniswap_v4', label: 'Uniswap V4' },
];
const DEFAULT_SMART_MONEY_PROTOCOL = SMART_MONEY_PROTOCOL_OPTIONS[0].value;

const WALLET_AVATAR_ICONS = Object.entries(
    import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
)
    .sort(([pathA], [pathB]) => pathA.localeCompare(pathB, undefined, { numeric: true }))
    .map(([, src]) => src);

function walletAvatarIdx(addr) {
    if (!WALLET_AVATAR_ICONS.length || !addr || addr.length < 6) return 0;
    return parseInt(addr.slice(-4), 16) % WALLET_AVATAR_ICONS.length;
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
    const map = { 100: '0.01%', 500: '0.05%', 2500: '0.25%', 3000: '0.3%', 10000: '1%' };
    return map[fee] || `${(fee / 10000).toFixed(2)}%`;
}

function formatUSDCompact(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '—';
    const abs = Math.abs(num);
    if (abs >= 1000000) return `$${(num / 1000000).toFixed(abs >= 10000000 ? 0 : 1).replace(/\.0$/, '')}M`;
    if (abs >= 1000) return `$${(num / 1000).toFixed(abs >= 10000 ? 0 : 1).replace(/\.0$/, '')}K`;
    if (abs >= 100) return `$${num.toFixed(0)}`;
    if (abs >= 10) return `$${num.toFixed(1).replace(/\.0$/, '')}`;
    return `$${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatRangePercent(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '—';
    if (num >= 100) return `±${Math.round(num)}%`;
    if (num >= 10) return `±${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `±${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
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

function WalletAvatar({ address, color, size = 18 }) {
    const iconSrc = WALLET_AVATAR_ICONS[walletAvatarIdx(address)] || WALLET_AVATAR_ICONS[0];
    return (
        <span className="smd-wallet-avatar" style={{ borderColor: color, width: size, height: size }}>
            <img src={iconSrc} alt="" className="smd-wallet-avatar-icon" />
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

function WalletIdentity({ address, color, label, size = 16, onClick, showCopy = false }) {
    const content = (
        <>
            <WalletAvatar address={address} color={color} size={size} />
            <span className="smd-wallet-info-name">{label && label !== address ? label : tailAddr(address)}</span>
            {showCopy ? <CopyTinyBtn text={address} /> : null}
        </>
    );

    if (typeof onClick === 'function') {
        return (
            <button className="smd-wallet-info smd-wallet-btn" onClick={onClick}>
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
            <div className={`smd-stat-value${color === 'red' ? ' red' : ''}`}>{value ?? '—'}</div>
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

// ---- Pages ----

function PoolList({ apiBaseUrl, onSelect, onOpenDetail, activePoolAddress = '' }) {
    const [pools, setPools] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [proto, setProto] = useState('all');
    const normalizedActivePoolAddress = useMemo(
        () => normalizePoolSelectionId(activePoolAddress),
        [activePoolAddress]
    );

    useEffect(() => {
        setLoading(true);
        fetchSMPools({ apiBaseUrl }).then(d => setPools(d?.list || [])).catch(() => {}).finally(() => setLoading(false));
    }, [apiBaseUrl]);

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
        ? `${position.price_lower} – ${position.price_upper}`
        : '—';

    return (
        <div className="smd-range-cell">
            <div className={`smd-range-main mono muted${isClosed ? ' is-closed' : ''}`}>
                {rangeText}
            </div>
            <div className="smd-range-sub">{formatRangePercent(position.range_percent)}</div>
        </div>
    );
}

function PoolDetail({ apiBaseUrl, pool, onBack, onSelectWallet }) {
    const [positions, setPositions] = useState([]);
    const [stats, setStats] = useState(null);
    const [status, setStatus] = useState('open');
    const [loading, setLoading] = useState(true);
    const poolIdentifier = getPoolIdentifier(pool);

    useEffect(() => { fetchSMPoolStats({ apiBaseUrl, poolAddress: pool.pool_address }).then(setStats).catch(() => {}); }, [apiBaseUrl, pool.pool_address]);
    useEffect(() => {
        setLoading(true);
        fetchSMPositions({ apiBaseUrl, pool: pool.pool_address, status, size: 100 }).then(d => setPositions(d?.list || [])).catch(() => {}).finally(() => setLoading(false));
    }, [apiBaseUrl, pool.pool_address, status]);

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
                            <a
                                href={`https://bscscan.com/address/${pool.pool_address}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="smd-link"
                            >
                                查看池子 <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} />
                            </a>
                        </div>
                    </div>
                </div>
            </div>

            {stats && (
                <div className="smd-stats-grid smd-stats-grid--pool">
                    <StatCard label="当前价格" value={stats.current_price || '—'} />
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
                <div className="smd-empty">{status === 'open' ? '全部已关闭，切换到"全部"查看' : '暂无仓位'}</div>
            ) : (
                <div className="smd-pos-list">
                    {positions.map(pos => (
                        <div key={pos.id} className={`smd-pos-card${pos.status === 'closed' ? ' closed' : ''}`}>
                            <div className="smd-pos-card-top">
                                <WalletIdentity
                                    address={pos.wallet_address}
                                    color={pos.wallet_color}
                                    label={pos.wallet_label || pos.wallet_address}
                                    size={28}
                                    onClick={() => onSelectWallet(pos.wallet_address)}
                                />
                                <div className="smd-pos-card-top-right">
                                    <span className="smd-pos-card-amount">{formatUSDCompact(pos.position_amount_usd)}</span>
                                    <Badge cls={pos.status === 'open' ? 'status-open' : 'status-closed'}>
                                        {pos.status === 'open' ? '持仓中' : '已关闭'}
                                    </Badge>
                                </div>
                            </div>
                            <div className="smd-pos-card-range smd-pos-card-range--detail">
                                <span className={`smd-pos-card-prices${pos.status === 'closed' ? ' is-closed' : ''}`}>
                                    {pos.price_lower && pos.price_upper ? `${pos.price_lower} – ${pos.price_upper}` : '—'}
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
                                    >
                                        查看交易 <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} />
                                    </a>
                                ) : null}
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function WalletList({ apiBaseUrl, onSelect, onAdd }) {
    const [wallets, setWallets] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);

    const load = useCallback(() => {
        setLoading(true);
        fetchSMWallets({ apiBaseUrl, size: 100 }).then(d => setWallets(d?.list || [])).catch(() => {}).finally(() => setLoading(false));
    }, [apiBaseUrl]);
    useEffect(() => { load(); }, [load]);

    const filtered = useMemo(() => {
        if (!search) return wallets;
        const q = search.toLowerCase();
        return wallets.filter(w => w.address.toLowerCase().includes(q) || (w.label && w.label.toLowerCase().includes(q)));
    }, [wallets, search]);

    const runAction = async (key, action) => {
        setBusyKey(key);
        setActionError('');
        try {
            await action();
            await load();
        } catch (err) {
            setActionError(String(err?.message || err || '操作失败'));
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
            setActionError(String(err?.message || err || '操作失败'));
        } finally {
            setBusyKey('');
        }
    };

    return (
        <div>
            <div className="smd-search-row">
                <div className="smd-search-input">
                    <Search size={14} />
                    <input placeholder="搜索钱包..." value={search} onChange={e => setSearch(e.target.value)} />
                </div>
                <button onClick={onAdd} className="smd-add-btn">
                    <Plus size={14} /> 添加钱包
                </button>
            </div>
            {actionError ? <div className="smd-inline-error">{actionError}</div> : null}
            {loading ? <div className="smd-loading">加载中...</div> : filtered.length === 0 ? (
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
                    {filtered.map(w => (
                        <tr key={w.address} className="clickable" onClick={() => onSelect(w.address)}>
                            <td>
                                <WalletIdentity address={w.address} color={w.color} label={w.label || w.address} size={20} showCopy />
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
            <ConfirmDialog
                open={Boolean(confirmState)}
                title={confirmState?.title || '确认操作'}
                description={confirmState?.description || ''}
                confirmLabel="删除"
                busy={busyKey.startsWith('wallet-delete:')}
                onCancel={() => { if (!busyKey.startsWith('wallet-delete:')) setConfirmState(null); }}
                onConfirm={confirmDelete}
            />
        </div>
    );
}

function WalletDetail({ apiBaseUrl, addr, onBack, onSelectPool }) {
    const [positions, setPositions] = useState([]);
    const [info, setInfo] = useState(null);
    const [status, setStatus] = useState('open');
    const [loading, setLoading] = useState(true);

    useEffect(() => { fetchSMStats({ apiBaseUrl, address: addr }).then(setInfo).catch(() => {}); }, [apiBaseUrl, addr]);
    useEffect(() => {
        setLoading(true);
        fetchSMPositions({ apiBaseUrl, wallet: addr, status, size: 100 }).then(d => setPositions(d?.list || [])).catch(() => {}).finally(() => setLoading(false));
    }, [apiBaseUrl, addr, status]);

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
                        <WalletAvatar address={addr} color={info.color || '#7F77DD'} size={72} />
                        <div className="smd-detail-copy">
                            <h3 className="smd-detail-title">{info.label || `钱包 ${tailAddr(addr)}`}</h3>
                            <CompactIdentifier value={addr} label="钱包" />
                        </div>
                    </div>
                    <div className="smd-stats-grid">
                        <StatCard label="持仓笔数" value={info.open_position_count} />
                        <StatCard label="活跃池子" value={info.active_pool_count} />
                        <StatCard label="总添加次数" value={info.total_add_count} />
                        <StatCard label="总移除次数" value={info.total_remove_count} />
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
            ) : groups.map(g => (
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
                        {g.positions.map(pos => (
                            <div key={pos.id} className={`smd-pos-card smd-pos-card--compact${pos.status === 'closed' ? ' closed' : ''}`}>
                                <span className="smd-pos-card-amount">{formatUSDCompact(pos.position_amount_usd)}</span>
                                <span className={`smd-pos-card-prices${pos.status === 'closed' ? ' is-closed' : ''}`}>
                                    {pos.price_lower && pos.price_upper ? `${pos.price_lower} – ${pos.price_upper}` : '—'}
                                </span>
                                {pos.range_percent > 0 && <span className="smd-pos-card-pct">{formatRangePercent(pos.range_percent)}</span>}
                            </div>
                        ))}
                    </div>
                </div>
            ))}
        </div>
    );
}

function GoldenDogPanel({ apiBaseUrl, initData }) {
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
            setError('请先登录 WebApp，拿到 initData 后才能保存金狗通知。');
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
            <div className="smd-search-row">
                <div className="smd-section-title">金狗通知</div>
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

            <div className="smd-stats-grid" style={{ marginBottom: 16 }}>
                <StatCard label="Bark 状态" value={barkStatusText} />
                <StatCard label="钱包阈值" value={`${draft.min_wallets || '--'} 个`} />
                <StatCard label="冷却时间" value={`${draft.cooldown_minutes || '--'} 分钟`} />
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

            <div className="smd-detail-card" style={{ marginBottom: 16 }}>
                <div className="muted" style={{ lineHeight: 1.7 }}>
                    当同一个交易对在统计窗口内达到设定的钱包数量时，后端会按交易对聚合发送 Bark。
                    同一波信号跨多个池子和 fee tier 只按交易对判断，不再重复拆分。
                </div>
                <div className="muted" style={{ marginTop: 10, lineHeight: 1.7 }}>
                    Bark Server / Key / Group 继续复用全局配置。Web 端这里只负责开关、阈值和冷却时间。
                </div>
            </div>

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
                        {saving ? '保存中...' : '保存金狗通知配置'}
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
    const [showAdd, setShowAdd] = useState(false);
    const [newAddr, setNewAddr] = useState('');
    const [newProto, setNewProto] = useState(DEFAULT_SMART_MONEY_PROTOCOL);
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
            if (!SMART_MONEY_PROTOCOL_OPTIONS.some((option) => option.value === newProto)) {
                throw new Error('请选择支持的协议');
            }
            await addSMContract({ apiBaseUrl, contract_address: addr, protocol: newProto, description: newDesc });
            setShowAdd(false);
            setNewAddr('');
            setNewProto(DEFAULT_SMART_MONEY_PROTOCOL);
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
                    <>
                        <select className="w-md" value={newProto} onChange={e => setNewProto(e.target.value)}>
                            {SMART_MONEY_PROTOCOL_OPTIONS.map((option) => (
                                <option key={option.value} value={option.value}>
                                    {option.label}
                                </option>
                            ))}
                        </select>
                        <input className="w-sm" placeholder="描述" value={newDesc} onChange={e => setNewDesc(e.target.value)} />
                    </>
                    <div className="smd-add-form-hint">仅支持 PancakeSwap V3、Uniswap V3、Uniswap V4</div>
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
                        <th>协议</th>
                        <th>描述</th>
                        <th className="center">状态</th>
                        <th className="right">已扫描至区块</th>
                        <th className="right">操作</th>
                    </tr></thead>
                    <tbody>
                    {contracts.map(c => (
                        <tr key={c.contract_address}>
                            <td className="mono">{shortAddr(c.contract_address)}</td>
                            <td className="muted">{c.protocol}</td>
                            <td className="muted">{c.description || '—'}</td>
                            <td className="center"><span className={`smd-status-dot ${c.is_active ? 'green' : 'muted'}`}>{c.is_active ? '活跃' : '已暂停'}</span></td>
                            <td className="right mono muted">{c.last_scanned_block || '未扫描'}</td>
                            <td className="right">
                                <div className="smd-action-row" style={{ justifyContent: 'flex-end' }}>
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
        </div>
    );
}

// ---- Main ----

export default function SmartMoneyDashboard({ apiBaseUrl, initData = '', onSelectPool, activePoolAddress = '' }) {
    const [view, setView] = useState('pools');
    const [stats, setStats] = useState(null);
    const [selectedPool, setSelectedPool] = useState(null);
    const [selectedWallet, setSelectedWallet] = useState(null);
    const [showAddModal, setShowAddModal] = useState(false);

    useEffect(() => {
        fetchSMStats({ apiBaseUrl }).then(setStats).catch(() => {});
        const t = setInterval(() => fetchSMStats({ apiBaseUrl }).then(setStats).catch(() => {}), 30000);
        return () => clearInterval(t);
    }, [apiBaseUrl]);

    const isDetail = selectedPool || selectedWallet;
    const handlePoolCardSelect = useCallback((pool) => {
        if (typeof onSelectPool === 'function') {
            onSelectPool(pool);
            return;
        }
        setSelectedPool(pool);
        setSelectedWallet(null);
    }, [onSelectPool]);
    const handleOpenPoolDetail = useCallback((pool) => {
        setSelectedPool(pool);
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
                            <Flame size={16} /> 金狗通知
                        </button>
                    </div>
                )}

                {selectedPool ? (
                    <PoolDetail apiBaseUrl={apiBaseUrl} pool={selectedPool} onBack={() => setSelectedPool(null)} onSelectWallet={addr => { setSelectedWallet(addr); setView('wallets'); setSelectedPool(null); }} />
                ) : selectedWallet ? (
                    <WalletDetail apiBaseUrl={apiBaseUrl} addr={selectedWallet} onBack={() => setSelectedWallet(null)} onSelectPool={p => { setSelectedPool(p); setSelectedWallet(null); }} />
                ) : view === 'pools' ? (
                    <PoolList
                        apiBaseUrl={apiBaseUrl}
                        onSelect={handlePoolCardSelect}
                        onOpenDetail={handleOpenPoolDetail}
                        activePoolAddress={activePoolAddress}
                    />
                ) : view === 'wallets' ? (
                    <WalletList apiBaseUrl={apiBaseUrl} onSelect={setSelectedWallet} onAdd={() => setShowAddModal(true)} />
                ) : view === 'golden_dog' ? (
                    <GoldenDogPanel apiBaseUrl={apiBaseUrl} initData={initData} />
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
