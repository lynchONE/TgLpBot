import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import {
    Eye, Wallet, Settings, Search, Plus, ExternalLink, X, Check,
    ChevronRight, ChevronDown, Pause, Play, Trash2, Edit3, Copy, Filter,
} from 'lucide-react';
import {
    fetchSMPools, fetchSMPoolStats, fetchSMPositions, fetchSMWallets,
    fetchSMStats, addSMWallet, updateSMWallet, deleteSMWallet,
    fetchSMContracts, addSMContract, updateSMContract, deleteSMContract,
} from '../lib/smartMoneyApi';
import { getBrandTheme } from '../lib/brand';
import uniswapIcon from '../image/uniswap.svg';
import pancakeIcon from '../image/pancake.svg';

const PROTOCOL_MAP = {
    pancake_v3: { version: 'V3', icon: pancakeIcon, color: '#d1884f' },
    uniswap_v3: { version: 'V3', icon: uniswapIcon, color: '#ff007a' },
    uniswap_v4: { version: 'V4', icon: uniswapIcon, color: '#ff007a' },
};
const PROTOCOL_LABELS = Object.fromEntries(Object.entries(PROTOCOL_MAP).map(([k, v]) => [k, v.version]));

const STATUS_COLORS = {
    open: 'text-green-400',
    closed: 'text-zinc-500',
};

function getBrandLinkClass(brand) {
    return brand?.key === 'emerald'
        ? 'text-emerald-400 hover:text-emerald-300'
        : 'text-[#bcff2f] hover:text-[#dfff8b]';
}

function getBrandHoverTextClass(brand) {
    return brand?.key === 'emerald'
        ? 'hover:text-emerald-400'
        : 'hover:text-[#bcff2f]';
}

function getBrandFocusRingClass(brand) {
    return brand?.key === 'emerald'
        ? 'focus:ring-emerald-500'
        : 'focus:ring-[#bcff2f]';
}

function shortAddr(addr) {
    if (!addr || addr.length < 10) return addr || '';
    return addr.slice(0, 6) + '...' + addr.slice(-4);
}

function formatFeeTier(fee) {
    if (!fee) return '';
    const map = { 100: '0.01%', 500: '0.05%', 2500: '0.25%', 3000: '0.3%', 10000: '1%' };
    return map[fee] || `${(fee / 10000).toFixed(2)}%`;
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

function CopyButton({ text }) {
    const [copied, setCopied] = useState(false);
    return (
        <button
            className="text-zinc-500 hover:text-zinc-300 ml-1"
            onClick={(e) => {
                e.stopPropagation();
                navigator.clipboard.writeText(text);
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
            }}
        >
            {copied ? <Check size={12} /> : <Copy size={12} />}
        </button>
    );
}

function WalletDot({ color, size = 18, label }) {
    const letter = label ? label[0].toUpperCase() : '?';
    return (
        <span
            className="inline-flex items-center justify-center rounded-full text-white font-bold shrink-0"
            style={{ backgroundColor: color, width: size, height: size, fontSize: size * 0.5 }}
        >
            {letter}
        </span>
    );
}

function ProtocolBadge({ protocol }) {
    const info = PROTOCOL_MAP[protocol];
    return (
        <span className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-zinc-700 text-zinc-300"
              style={info ? { borderLeft: `2px solid ${info.color}` } : undefined}>
            {info && <img src={info.icon} alt="" className="w-3.5 h-3.5 rounded-full" />}
            {info?.version || protocol}
        </span>
    );
}

function FeeBadge({ fee }) {
    if (!fee) return null;
    return (
        <span className="text-[10px] px-1.5 py-0.5 rounded bg-zinc-700/60 text-zinc-400">
            {formatFeeTier(fee)}
        </span>
    );
}

function StatCard({ label, value, color }) {
    return (
        <div className="bg-zinc-800/60 rounded-lg p-3 flex-1 min-w-0">
            <div className="text-[11px] text-zinc-500 mb-1">{label}</div>
            <div className={`text-lg font-semibold ${color || 'text-zinc-100'}`}>{value ?? '—'}</div>
        </div>
    );
}

// ============ PriceRangeChart ============
function PriceRangeChart({ positions, currentPrice }) {
    if (!positions || positions.length === 0) return null;

    const validPositions = positions.filter(p => p.price_lower && p.price_upper);
    if (validPositions.length === 0) return null;

    const allPrices = validPositions.flatMap(p => [parseFloat(p.price_lower), parseFloat(p.price_upper)]);
    let minP = Math.min(...allPrices);
    let maxP = Math.max(...allPrices);
    const padding = (maxP - minP) * 0.1 || 1;
    minP -= padding;
    maxP += padding;
    if (minP < 0) minP = 0;

    const priceToPct = (price) => Math.max(0, Math.min(100, ((price - minP) / (maxP - minP)) * 100));

    const currentPct = currentPrice ? priceToPct(parseFloat(currentPrice)) : null;

    const walletCounts = {};
    validPositions.forEach(p => {
        walletCounts[p.wallet_address] = (walletCounts[p.wallet_address] || 0) + 1;
    });
    const walletIndices = {};

    return (
        <div className="bg-zinc-800/40 rounded-lg p-3 mb-4 overflow-x-auto">
            <div className="relative min-w-[300px]" style={{ minHeight: validPositions.length * 14 + 30 }}>
                {/* Current price line */}
                {currentPct !== null && (
                    <div
                        className="absolute top-0 bottom-6 w-px bg-yellow-400/60 z-10"
                        style={{ left: `${currentPct}%` }}
                    >
                        <div className="absolute -top-4 -translate-x-1/2 text-[9px] text-yellow-400 whitespace-nowrap">
                            {currentPrice}
                        </div>
                    </div>
                )}

                {/* Position bars */}
                {validPositions.map((p, i) => {
                    const left = priceToPct(parseFloat(p.price_lower));
                    const right = priceToPct(parseFloat(p.price_upper));
                    const width = Math.max(right - left, 0.5);
                    const color = p.wallet_color || '#7F77DD';

                    const walletKey = p.wallet_address;
                    walletIndices[walletKey] = (walletIndices[walletKey] || 0) + 1;
                    const idx = walletIndices[walletKey];
                    const opacity = idx === 1 ? 0.85 : idx === 2 ? 0.6 : 0.4;

                    const inRange = currentPrice &&
                        parseFloat(p.price_lower) <= parseFloat(currentPrice) &&
                        parseFloat(currentPrice) <= parseFloat(p.price_upper);

                    return (
                        <div
                            key={p.id || i}
                            className="absolute h-[10px] rounded-[3px]"
                            style={{
                                left: `${left}%`,
                                width: `${width}%`,
                                top: i * 14,
                                backgroundColor: color,
                                opacity: inRange ? opacity : 0.35,
                            }}
                            title={`${shortAddr(p.wallet_address)}: ${p.price_lower} - ${p.price_upper}`}
                        />
                    );
                })}

                {/* X-axis labels */}
                <div className="absolute bottom-0 left-0 right-0 flex justify-between text-[9px] text-zinc-600">
                    {Array.from({ length: 5 }, (_, i) => {
                        const price = minP + ((maxP - minP) / 4) * i;
                        return <span key={i}>{price.toPrecision(4)}</span>;
                    })}
                </div>
            </div>

            {/* Legend */}
            <div className="flex flex-wrap gap-2 mt-3 text-[10px]">
                {Object.entries(
                    validPositions.reduce((acc, p) => {
                        if (!acc[p.wallet_address]) {
                            acc[p.wallet_address] = { color: p.wallet_color, label: p.wallet_label };
                        }
                        return acc;
                    }, {})
                ).map(([addr, { color, label }]) => (
                    <span key={addr} className="flex items-center gap-1 text-zinc-400">
                        <span className="inline-block w-2 h-2 rounded-full" style={{ backgroundColor: color }} />
                        {label || shortAddr(addr)}
                    </span>
                ))}
            </div>
        </div>
    );
}

// ============ PAGES ============

function PoolListPage({ apiBaseUrl, onSelectPool, stats, brand }) {
    const [pools, setPools] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [protocolFilter, setProtocolFilter] = useState('all');

    const load = useCallback((silent = false) => {
        if (!silent) {
            setLoading(true);
        }
        fetchSMPools({ apiBaseUrl })
            .then(d => setPools(d?.list || []))
            .catch(() => {})
            .finally(() => {
                if (!silent) {
                    setLoading(false);
                }
            });
    }, [apiBaseUrl]);

    useEffect(() => { load(); }, [load]);
    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, 10000);
        return () => clearInterval(timer);
    }, [load]);

    const filtered = useMemo(() => {
        let list = pools;
        if (search) {
            const q = search.toLowerCase();
            list = list.filter(p =>
                (p.token0_symbol + '/' + p.token1_symbol).toLowerCase().includes(q) ||
                p.pool_address.toLowerCase().includes(q)
            );
        }
        if (protocolFilter !== 'all') {
            list = list.filter(p => p.protocol === protocolFilter);
        }
        return list;
    }, [pools, search, protocolFilter]);

    return (
        <div>
            {/* Search & filter */}
            <div className="flex gap-2 mb-3">
                <div className="flex-1 relative">
                    <Search size={14} className="absolute left-2 top-1/2 -translate-y-1/2 text-zinc-500" />
                    <input
                        className="w-full bg-zinc-800 rounded-lg pl-7 pr-3 py-2 text-sm text-zinc-200 outline-none focus:ring-1 focus:ring-zinc-600"
                        placeholder="搜索池子..."
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                    />
                </div>
            </div>
            <div className="flex gap-1.5 mb-3 overflow-x-auto text-[11px]">
                {['all', 'pancake_v3', 'uniswap_v3', 'uniswap_v4'].map(p => {
                    const info = PROTOCOL_MAP[p];
                    return (
                        <button
                            key={p}
                            className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full whitespace-nowrap ${protocolFilter === p
                                ? brand.softButtonClass
                                : 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700'
                            }`}
                            onClick={() => setProtocolFilter(p)}
                        >
                            {info && <img src={info.icon} alt="" className="w-3.5 h-3.5 rounded-full" />}
                            {p === 'all' ? '全部' : info?.version || p}
                        </button>
                    );
                })}
            </div>

            {loading ? (
                <div className="text-center text-zinc-500 py-8">加载中...</div>
            ) : filtered.length === 0 ? (
                <div className="text-center text-zinc-500 py-8">
                    暂无活跃仓位的池子。请先添加钱包开始监控。
                </div>
            ) : (
                <div className="space-y-2">
                    {filtered.map(pool => (
                        <div
                            key={pool.pool_address}
                            className="bg-zinc-800/60 rounded-lg p-3 cursor-pointer hover:bg-zinc-800 transition-colors"
                            onClick={() => onSelectPool(pool)}
                        >
                            <div className="flex items-center justify-between">
                                <div className="flex items-center gap-2">
                                    <span className="font-medium text-zinc-100">
                                        {pool.token0_symbol}/{pool.token1_symbol}
                                    </span>
                                    <FeeBadge fee={pool.fee_tier} />
                                    <ProtocolBadge protocol={pool.protocol} />
                                </div>
                                <ChevronRight size={16} className="text-zinc-600" />
                            </div>
                            <div className="flex items-center gap-4 mt-1.5 text-[11px] text-zinc-500">
                                <span>{pool.open_position_count} 个仓位</span>
                                <span>{pool.wallet_count} 个钱包</span>
                                <span className={
                                    pool.latest_event_at && (Date.now() - new Date(pool.latest_event_at).getTime()) < 120000
                                        ? 'text-green-400'
                                        : ''
                                }>
                                    {relativeTime(pool.latest_event_at)}
                                </span>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function PoolDetailPage({ apiBaseUrl, pool, onBack, onSelectWallet, brand }) {
    const [positions, setPositions] = useState([]);
    const [poolStats, setPoolStats] = useState(null);
    const [status, setStatus] = useState('open');
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        fetchSMPoolStats({ apiBaseUrl, poolAddress: pool.pool_address }).then(setPoolStats).catch(() => {});
    }, [apiBaseUrl, pool.pool_address]);

    useEffect(() => {
        setLoading(true);
        fetchSMPositions({ apiBaseUrl, pool: pool.pool_address, status, size: 100 })
            .then(d => setPositions(d?.list || []))
            .catch(() => {})
            .finally(() => setLoading(false));
    }, [apiBaseUrl, pool.pool_address, status]);

    return (
        <div>
            <button onClick={onBack} className="text-zinc-500 text-sm mb-3 hover:text-zinc-300">
                &larr; 返回池子列表
            </button>
            <div className="bg-zinc-800/60 rounded-lg p-4 mb-4">
                <div className="flex items-center gap-2 mb-2">
                    <span className="text-lg font-medium text-zinc-100">
                        {pool.token0_symbol}/{pool.token1_symbol}
                    </span>
                    <FeeBadge fee={pool.fee_tier} />
                    <ProtocolBadge protocol={pool.protocol} />
                </div>
                <div className="flex items-center gap-1 text-[11px] text-zinc-500">
                    <span className="font-mono">{pool.pool_address}</span>
                    <CopyButton text={pool.pool_address} />
                    <a
                        href={`https://bscscan.com/address/${pool.pool_address}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className={`${getBrandLinkClass(brand)} ml-1`}
                    >
                        <ExternalLink size={11} />
                    </a>
                </div>
            </div>

            {/* Stats */}
            {poolStats && (
                <div className="grid grid-cols-2 gap-2 mb-4">
                    <StatCard label="持仓中" value={poolStats.open_position_count} />
                    <StatCard label="钱包数" value={poolStats.wallet_count} />
                    <StatCard label="今日关闭" value={poolStats.closed_today_count} color="text-red-400" />
                    <StatCard label="当前价格" value={poolStats.current_price || '—'} />
                </div>
            )}

            {/* Price Range Chart */}
            <PriceRangeChart
                positions={positions}
                currentPrice={poolStats?.current_price}
            />

            {/* Toggle */}
            <div className="flex items-center justify-between mb-3">
                <span className="text-sm text-zinc-300">仓位</span>
                <div className="flex gap-1 text-[11px]">
                    {['open', 'all'].map(s => (
                        <button
                            key={s}
                            className={`px-2.5 py-1 rounded ${status === s ? brand.softButtonClass : 'bg-zinc-800 text-zinc-400'}`}
                            onClick={() => setStatus(s)}
                        >
                            {s === 'open' ? '持仓中' : '全部'}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? (
                <div className="text-center text-zinc-500 py-4">加载中...</div>
            ) : positions.length === 0 ? (
                <div className="text-center text-zinc-500 py-4">
                    {status === 'open'
                        ? "该池子所有仓位已关闭。切换到「全部」查看历史记录。"
                        : '暂无仓位数据。'}
                </div>
            ) : (
                <div className="space-y-2">
                    {positions.map(pos => (
                        <div
                            key={pos.id}
                            className={`bg-zinc-800/60 rounded-lg p-3 ${pos.status === 'closed' ? 'opacity-65' : ''}`}
                        >
                            <div className="flex items-center gap-2 mb-1.5">
                                <WalletDot
                                    color={pos.wallet_color}
                                    size={18}
                                    label={pos.wallet_label || pos.wallet_address}
                                />
                                <button
                                    className={`text-sm text-zinc-300 font-mono ${getBrandHoverTextClass(brand)}`}
                                    onClick={() => onSelectWallet(pos.wallet_address)}
                                >
                                    {pos.wallet_label || shortAddr(pos.wallet_address)}
                                </button>
                                <span className={`text-[10px] ${STATUS_COLORS[pos.status]}`}>
                                    {pos.status === 'open' ? '持仓中' : '已关闭'}
                                </span>
                            </div>
                            <div className="flex items-center justify-between text-[11px]">
                                <span className={`font-mono text-zinc-400 ${pos.status === 'closed' ? 'line-through' : ''}`}>
                                    {pos.price_lower || '—'} — {pos.price_upper || '—'}
                                </span>
                                <div className="flex items-center gap-2 text-zinc-500">
                                    <span className="font-mono text-zinc-600">#{pos.nft_token_id}</span>
                                    <a
                                        href={pos.bscscan_url}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        className={getBrandLinkClass(brand)}
                                    >
                                        <ExternalLink size={11} />
                                    </a>
                                </div>
                            </div>
                            {/* Progress bar */}
                            {pos.status === 'open' && pos.price_lower && pos.price_upper && (
                                <div className="mt-1.5 h-1 bg-zinc-700 rounded-full overflow-hidden">
                                    <div
                                        className="h-full rounded-full"
                                        style={{
                                            backgroundColor: pos.wallet_color,
                                            opacity: 0.6,
                                            width: '50%', // Placeholder - would use current price
                                        }}
                                    />
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function WalletListPage({ apiBaseUrl, onSelectWallet, onAddWallet, brand }) {
    const [wallets, setWallets] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [sourceFilter, setSourceFilter] = useState('all');

    const load = useCallback((silent = false) => {
        if (!silent) {
            setLoading(true);
        }
        fetchSMWallets({ apiBaseUrl, size: 100 })
            .then(d => setWallets(d?.list || []))
            .catch(() => {})
            .finally(() => {
                if (!silent) {
                    setLoading(false);
                }
            });
    }, [apiBaseUrl]);

    useEffect(() => { load(); }, [load]);

    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, 10000);
        return () => clearInterval(timer);
    }, [load]);

    const filtered = useMemo(() => {
        let list = wallets;
        if (search) {
            const q = search.toLowerCase();
            list = list.filter(w =>
                w.address.toLowerCase().includes(q) ||
                (w.label && w.label.toLowerCase().includes(q))
            );
        }
        if (sourceFilter !== 'all') {
            if (sourceFilter === 'active') list = list.filter(w => w.is_active);
            else if (sourceFilter === 'paused') list = list.filter(w => !w.is_active);
            else list = list.filter(w => w.source === sourceFilter);
        }
        return list;
    }, [wallets, search, sourceFilter]);

    const handleToggle = async (wallet) => {
        try {
            await updateSMWallet({ apiBaseUrl, address: wallet.address, updates: { is_active: !wallet.is_active } });
            load();
        } catch (err) {
            alert(err.message);
        }
    };

    return (
        <div>
            <div className="flex gap-2 mb-3">
                <div className="flex-1 relative">
                    <Search size={14} className="absolute left-2 top-1/2 -translate-y-1/2 text-zinc-500" />
                    <input
                        className="w-full bg-zinc-800 rounded-lg pl-7 pr-3 py-2 text-sm text-zinc-200 outline-none focus:ring-1 focus:ring-zinc-600"
                        placeholder="搜索钱包..."
                        value={search}
                        onChange={e => setSearch(e.target.value)}
                    />
                </div>
                <button
                    onClick={onAddWallet}
                    className={`${brand.solidButtonClass} ${brand.solidRingClass} rounded-lg px-3 py-2 text-sm flex items-center gap-1`}
                >
                    <Plus size={14} /> 添加
                </button>
            </div>

            <div className="flex gap-1.5 mb-3 overflow-x-auto text-[11px]">
                {['all', 'manual', 'contract_interaction', 'active', 'paused'].map(f => (
                    <button
                        key={f}
                        className={`px-2.5 py-1 rounded-full whitespace-nowrap ${sourceFilter === f
                            ? brand.softButtonClass
                            : 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700'
                        }`}
                        onClick={() => setSourceFilter(f)}
                    >
                        {f === 'all' ? '全部' : f === 'contract_interaction' ? '合约' : f === 'manual' ? '手动' : f === 'active' ? '监控中' : f === 'paused' ? '已暂停' : f}
                    </button>
                ))}
            </div>

            {loading ? (
                <div className="text-center text-zinc-500 py-8">加载中...</div>
            ) : filtered.length === 0 ? (
                <div className="text-center text-zinc-500 py-8">
                    暂无监控钱包。点击「+ 添加」开始。
                </div>
            ) : (
                <div className="space-y-2">
                    {filtered.map(w => (
                        <div
                            key={w.address}
                            className="bg-zinc-800/60 rounded-lg p-3 cursor-pointer hover:bg-zinc-800 transition-colors"
                            onClick={() => onSelectWallet(w.address)}
                        >
                            <div className="flex items-center justify-between">
                                <div className="flex items-center gap-2">
                                    <WalletDot color={w.color} size={20} label={w.label || w.address} />
                                    <div>
                                        <div className="text-sm text-zinc-200">{w.label || shortAddr(w.address)}</div>
                                        <div className="text-[10px] font-mono text-zinc-500">{shortAddr(w.address)}</div>
                                    </div>
                                </div>
                                <div className="flex items-center gap-2">
                                    <span className={`text-[10px] ${w.is_active ? 'text-green-400' : 'text-zinc-600'}`}>
                                        {w.is_active ? '监控中' : '已暂停'}
                                    </span>
                                    <button
                                        className="text-zinc-500 hover:text-zinc-300"
                                        onClick={e => { e.stopPropagation(); handleToggle(w); }}
                                    >
                                        {w.is_active ? <Pause size={14} /> : <Play size={14} />}
                                    </button>
                                </div>
                            </div>
                            <div className="flex gap-3 mt-1.5 text-[11px] text-zinc-500">
                                <span>{w.open_position_count} 持仓</span>
                                <span>{w.active_pool_count} 池子</span>
                                <span>{w.source === 'manual' ? '手动' : '合约'}</span>
                                {w.last_active_at && <span>{relativeTime(w.last_active_at)}</span>}
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function WalletDetailPage({ apiBaseUrl, walletAddress, onBack, onSelectPool, brand }) {
    const [positions, setPositions] = useState([]);
    const [walletInfo, setWalletInfo] = useState(null);
    const [status, setStatus] = useState('open');
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        fetchSMStats({ apiBaseUrl, address: walletAddress }).then(setWalletInfo).catch(() => {});
    }, [apiBaseUrl, walletAddress]);

    useEffect(() => {
        setLoading(true);
        fetchSMPositions({ apiBaseUrl, wallet: walletAddress, status, size: 100 })
            .then(d => setPositions(d?.list || []))
            .catch(() => {})
            .finally(() => setLoading(false));
    }, [apiBaseUrl, walletAddress, status]);

    // Group positions by pool
    const poolGroups = useMemo(() => {
        const groups = {};
        (positions || []).forEach(p => {
            if (!groups[p.pool_address]) {
                groups[p.pool_address] = {
                    pool_address: p.pool_address,
                    token0_symbol: p.token0_symbol,
                    token1_symbol: p.token1_symbol,
                    fee_tier: p.fee_tier,
                    protocol: p.protocol,
                    positions: [],
                    hasOpen: false,
                };
            }
            groups[p.pool_address].positions.push(p);
            if (p.status === 'open') groups[p.pool_address].hasOpen = true;
        });
        return Object.values(groups).sort((a, b) => {
            if (a.hasOpen && !b.hasOpen) return -1;
            if (!a.hasOpen && b.hasOpen) return 1;
            return 0;
        });
    }, [positions]);

    return (
        <div>
            <button onClick={onBack} className="text-zinc-500 text-sm mb-3 hover:text-zinc-300">
                &larr; 返回钱包列表
            </button>

            {walletInfo && (
                <div className="bg-zinc-800/60 rounded-lg p-4 mb-4">
                    <div className="flex items-center gap-2 mb-2">
                        <WalletDot
                            color={walletInfo.color || '#7F77DD'}
                            size={32}
                            label={walletInfo.label || walletAddress}
                        />
                        <div>
                            <div className="text-lg text-zinc-100">{walletInfo.label || '未命名钱包'}</div>
                            <div className="flex items-center gap-1 text-[11px] text-zinc-500 font-mono">
                                {walletAddress}
                                <CopyButton text={walletAddress} />
                            </div>
                        </div>
                    </div>
                    <div className="flex gap-2 mt-2 text-[10px]">
                        <span className={`px-2 py-0.5 rounded ${walletInfo.source === 'manual' ? brand.selectionClass : 'bg-zinc-700 text-zinc-400'}`}>
                            {walletInfo.source === 'manual' ? '手动' : '合约'}
                        </span>
                        <span className={`px-2 py-0.5 rounded ${walletInfo.is_active ? 'bg-green-600/20 text-green-400' : 'bg-zinc-700 text-zinc-500'}`}>
                            {walletInfo.is_active ? '监控中' : '已暂停'}
                        </span>
                        <span className="px-2 py-0.5 rounded bg-zinc-700 text-zinc-400">BSC</span>
                    </div>

                    <div className="grid grid-cols-2 gap-2 mt-3">
                        <StatCard label="持仓中" value={walletInfo.open_position_count} />
                        <StatCard label="活跃池子" value={walletInfo.active_pool_count} />
                        <StatCard label="总添加" value={walletInfo.total_add_count} />
                        <StatCard label="总移除" value={walletInfo.total_remove_count} />
                    </div>
                </div>
            )}

            {/* Toggle */}
            <div className="flex items-center justify-between mb-3">
                <span className="text-sm text-zinc-300">仓位</span>
                <div className="flex gap-1 text-[11px]">
                    {['open', 'all'].map(s => (
                        <button
                            key={s}
                            className={`px-2.5 py-1 rounded ${status === s ? brand.softButtonClass : 'bg-zinc-800 text-zinc-400'}`}
                            onClick={() => setStatus(s)}
                        >
                            {s === 'open' ? '持仓中' : '全部'}
                        </button>
                    ))}
                </div>
            </div>

            {loading ? (
                <div className="text-center text-zinc-500 py-4">加载中...</div>
            ) : poolGroups.length === 0 ? (
                <div className="text-center text-zinc-500 py-4">
                    暂未检测到该钱包的 LP 活动。
                </div>
            ) : (
                <div className="space-y-3">
                    {poolGroups.map(group => (
                        <PoolGroupCard
                            key={group.pool_address}
                            group={group}
                            onSelectPool={() => onSelectPool({
                                pool_address: group.pool_address,
                                token0_symbol: group.token0_symbol,
                                token1_symbol: group.token1_symbol,
                                fee_tier: group.fee_tier,
                                protocol: group.protocol,
                            })}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

function PoolGroupCard({ group, onSelectPool }) {
    const [collapsed, setCollapsed] = useState(!group.hasOpen);
    const openCount = group.positions.filter(p => p.status === 'open').length;
    const closedCount = group.positions.filter(p => p.status === 'closed').length;

    return (
        <div className={`bg-zinc-800/60 rounded-lg overflow-hidden ${!group.hasOpen ? 'opacity-65' : ''}`}>
            <div
                className="flex items-center justify-between p-3 cursor-pointer hover:bg-zinc-800/80"
                onClick={() => setCollapsed(!collapsed)}
            >
                <div className="flex items-center gap-2">
                    {collapsed ? <ChevronRight size={14} className="text-zinc-500" /> : <ChevronDown size={14} className="text-zinc-500" />}
                    <span className="text-sm font-medium text-zinc-200">
                        {group.token0_symbol}/{group.token1_symbol}
                    </span>
                    <FeeBadge fee={group.fee_tier} />
                    <ProtocolBadge protocol={group.protocol} />
                </div>
                <div className="flex items-center gap-2">
                    <span className="text-[11px] text-zinc-500">
                        {group.hasOpen ? `${openCount} 个仓位` : `${closedCount} 已关闭`}
                    </span>
                    <button
                        className={`${getBrandLinkClass(brand)} text-[11px]`}
                        onClick={e => { e.stopPropagation(); onSelectPool(); }}
                    >
                        池子详情 <ExternalLink size={10} className="inline" />
                    </button>
                </div>
            </div>
            {!collapsed && (
                <div className="px-3 pb-3 space-y-1.5">
                    {group.positions.map(pos => (
                        <div key={pos.id} className={`flex items-center justify-between text-[11px] py-1 ${pos.status === 'closed' ? 'opacity-65' : ''}`}>
                            <span className={`font-mono text-zinc-400 ${pos.status === 'closed' ? 'line-through' : ''}`}>
                                {pos.price_lower || '—'} — {pos.price_upper || '—'}
                            </span>
                            <div className="flex items-center gap-2 text-zinc-500">
                                <span className={STATUS_COLORS[pos.status]}>
                                    {pos.status === 'open' ? '持仓中' : '已关闭'}
                                </span>
                                <span className="font-mono">#{pos.nft_token_id}</span>
                                <a href={pos.bscscan_url} target="_blank" rel="noopener noreferrer" className={getBrandLinkClass(brand)}>
                                    <ExternalLink size={10} />
                                </a>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function SettingsPage({ apiBaseUrl, brand }) {
    const [tab, setTab] = useState('wallets');

    return (
        <div>
            <div className="flex gap-1 mb-4 text-[12px]">
                {['wallets', 'contracts'].map(t => (
                    <button
                        key={t}
                        className={`px-3 py-1.5 rounded ${tab === t ? brand.softButtonClass : 'bg-zinc-800 text-zinc-400'}`}
                        onClick={() => setTab(t)}
                    >
                        {t === 'wallets' ? '钱包' : '合约'}
                    </button>
                ))}
            </div>
            {tab === 'wallets' ? (
                <WalletSettingsTab apiBaseUrl={apiBaseUrl} brand={brand} />
            ) : (
                <ContractSettingsTab apiBaseUrl={apiBaseUrl} brand={brand} />
            )}
        </div>
    );
}

function ContractSettingsPage({ apiBaseUrl, brand }) {
    return <ContractSettingsTab apiBaseUrl={apiBaseUrl} brand={brand} />;
}

function WalletSettingsTab({ apiBaseUrl, brand }) {
    const [wallets, setWallets] = useState([]);
    const [loading, setLoading] = useState(true);
    const [showAdd, setShowAdd] = useState(false);
    const [newAddr, setNewAddr] = useState('');
    const [newLabel, setNewLabel] = useState('');
    const [saving, setSaving] = useState(false);

    const load = useCallback(() => {
        setLoading(true);
        fetchSMWallets({ apiBaseUrl, size: 100 })
            .then(d => setWallets(d?.list || []))
            .catch(() => {})
            .finally(() => setLoading(false));
    }, [apiBaseUrl]);

    useEffect(() => { load(); }, [load]);

    const handleAdd = async () => {
        setSaving(true);
        try {
            await addSMWallet({ apiBaseUrl, address: newAddr, label: newLabel });
            setShowAdd(false);
            setNewAddr('');
            setNewLabel('');
            load();
        } catch (err) {
            alert(err.message);
        } finally {
            setSaving(false);
        }
    };

    const handleToggle = async (w) => {
        try {
            await updateSMWallet({ apiBaseUrl, address: w.address, updates: { is_active: !w.is_active } });
            load();
        } catch (err) {
            alert(err.message);
        }
    };

    const handleDelete = async (w) => {
        if (!confirm(`确认删除钱包 ${shortAddr(w.address)}？`)) return;
        try {
            await deleteSMWallet({ apiBaseUrl, address: w.address });
            load();
        } catch (err) {
            alert(err.message);
        }
    };

    return (
        <div>
            <div className="flex justify-between items-center mb-3">
                <span className="text-sm text-zinc-300">监控钱包</span>
                <button
                    onClick={() => setShowAdd(!showAdd)}
                    className={`${brand.solidButtonClass} ${brand.solidRingClass} rounded-lg px-3 py-1.5 text-xs flex items-center gap-1`}
                >
                    <Plus size={12} /> 添加钱包
                </button>
            </div>

            {showAdd && (
                <div className="bg-zinc-800 rounded-lg p-3 mb-3 space-y-2">
                    <input
                        className="w-full bg-zinc-900 rounded px-3 py-2 text-sm text-zinc-200 outline-none"
                        placeholder="钱包地址 (0x...)"
                        value={newAddr}
                        onChange={e => setNewAddr(e.target.value)}
                    />
                    <input
                        className="w-full bg-zinc-900 rounded px-3 py-2 text-sm text-zinc-200 outline-none"
                        placeholder="标签（可选）"
                        value={newLabel}
                        onChange={e => setNewLabel(e.target.value)}
                    />
                    <div className="flex gap-2 justify-end">
                        <button onClick={() => setShowAdd(false)} className="text-xs text-zinc-400 hover:text-zinc-300 px-3 py-1.5">取消</button>
                        <button onClick={handleAdd} disabled={saving} className={`text-xs ${brand.solidButtonClass} rounded px-3 py-1.5 disabled:opacity-50`}>
                            {saving ? '保存中...' : '添加'}
                        </button>
                    </div>
                </div>
            )}

            {loading ? (
                <div className="text-center text-zinc-500 py-4">加载中...</div>
            ) : wallets.length === 0 ? (
                <div className="text-center text-zinc-500 py-4">暂无钱包。</div>
            ) : (
                <div className="space-y-2">
                    {wallets.map(w => (
                        <div key={w.address} className="bg-zinc-800/60 rounded-lg p-3 flex items-center justify-between">
                            <div>
                                <div className="text-sm text-zinc-200">{w.label || shortAddr(w.address)}</div>
                                <div className="text-[10px] text-zinc-500 font-mono">{shortAddr(w.address)}</div>
                                <div className="flex gap-1 mt-1 text-[10px]">
                                    <span className={`px-1.5 py-0.5 rounded ${w.source === 'manual' ? brand.selectionClass : 'bg-zinc-700 text-zinc-400'}`}>
                                        {w.source === 'manual' ? '手动' : '合约'}
                                    </span>
                                    <span className={`px-1.5 py-0.5 rounded ${w.is_active ? 'bg-green-600/20 text-green-400' : 'bg-zinc-700 text-zinc-500'}`}>
                                        {w.is_active ? '监控中' : '已暂停'}
                                    </span>
                                </div>
                            </div>
                            <div className="flex gap-1">
                                <button onClick={() => handleToggle(w)} className="p-1.5 text-zinc-500 hover:text-zinc-300">
                                    {w.is_active ? <Pause size={14} /> : <Play size={14} />}
                                </button>
                                <button onClick={() => handleDelete(w)} className="p-1.5 text-zinc-500 hover:text-red-400">
                                    <Trash2 size={14} />
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function ContractSettingsTab({ apiBaseUrl, brand }) {
    const [contracts, setContracts] = useState([]);
    const [loading, setLoading] = useState(true);
    const [showAdd, setShowAdd] = useState(false);
    const [newAddr, setNewAddr] = useState('');
    const [newProtocol, setNewProtocol] = useState('');
    const [newDesc, setNewDesc] = useState('');
    const [saving, setSaving] = useState(false);

    const load = useCallback((silent = false) => {
        if (!silent) {
            setLoading(true);
        }
        fetchSMContracts({ apiBaseUrl })
            .then(d => setContracts(d?.list || []))
            .catch(() => {})
            .finally(() => {
                if (!silent) {
                    setLoading(false);
                }
            });
    }, [apiBaseUrl]);

    useEffect(() => { load(); }, [load]);
    useEffect(() => {
        const timer = setInterval(() => {
            load(true);
        }, 10000);
        return () => clearInterval(timer);
    }, [load]);

    const handleAdd = async () => {
        setSaving(true);
        try {
            await addSMContract({ apiBaseUrl, contract_address: newAddr, protocol: newProtocol, description: newDesc });
            setShowAdd(false);
            setNewAddr('');
            setNewProtocol('');
            setNewDesc('');
            load();
        } catch (err) {
            alert(err.message);
        } finally {
            setSaving(false);
        }
    };

    const handleToggle = async (c) => {
        try {
            await updateSMContract({ apiBaseUrl, address: c.contract_address, updates: { is_active: !c.is_active } });
            load();
        } catch (err) {
            alert(err.message);
        }
    };

    const handleDelete = async (c) => {
        if (!confirm(`确认删除合约 ${shortAddr(c.contract_address)}？`)) return;
        try {
            await deleteSMContract({ apiBaseUrl, address: c.contract_address });
            load();
        } catch (err) {
            alert(err.message);
        }
    };

    return (
        <div>
            <div className="flex justify-between items-center mb-3">
                <span className="text-sm text-zinc-300">监控合约</span>
                <button
                    onClick={() => setShowAdd(!showAdd)}
                    className={`${brand.solidButtonClass} ${brand.solidRingClass} rounded-lg px-3 py-1.5 text-xs flex items-center gap-1`}
                >
                    <Plus size={12} /> 添加合约
                </button>
            </div>

            {showAdd && (
                <div className="bg-zinc-800 rounded-lg p-3 mb-3 space-y-2">
                    <input
                        className="w-full bg-zinc-900 rounded px-3 py-2 text-sm text-zinc-200 outline-none"
                        placeholder="合约地址 (0x...)"
                        value={newAddr}
                        onChange={e => setNewAddr(e.target.value)}
                    />
                    <input
                        className="w-full bg-zinc-900 rounded px-3 py-2 text-sm text-zinc-200 outline-none"
                        placeholder="协议名称"
                        value={newProtocol}
                        onChange={e => setNewProtocol(e.target.value)}
                    />
                    <textarea
                        className="w-full bg-zinc-900 rounded px-3 py-2 text-sm text-zinc-200 outline-none resize-none"
                        placeholder="描述（可选）"
                        rows={2}
                        value={newDesc}
                        onChange={e => setNewDesc(e.target.value)}
                    />
                    <div className="flex gap-2 justify-end">
                        <button onClick={() => setShowAdd(false)} className="text-xs text-zinc-400 hover:text-zinc-300 px-3 py-1.5">取消</button>
                        <button onClick={handleAdd} disabled={saving} className={`text-xs ${brand.solidButtonClass} rounded px-3 py-1.5 disabled:opacity-50`}>
                            {saving ? '保存中...' : '添加'}
                        </button>
                    </div>
                </div>
            )}

            {loading ? (
                <div className="text-center text-zinc-500 py-4">加载中...</div>
            ) : contracts.length === 0 ? (
                <div className="text-center text-zinc-500 py-4">暂无配置合约。</div>
            ) : (
                <div className="space-y-2">
                    {contracts.map(c => (
                        <div key={c.contract_address} className="bg-zinc-800/60 rounded-lg p-3 flex items-center justify-between">
                            <div>
                                <div className="text-sm text-zinc-200">{c.protocol}</div>
                                <div className="text-[10px] text-zinc-500 font-mono">{shortAddr(c.contract_address)}</div>
                                {c.description && <div className="text-[10px] text-zinc-600 mt-0.5">{c.description}</div>}
                                <div className="text-[9px] text-zinc-600 mt-0.5">已扫描至区块 {c.last_scanned_block || '未扫描'}</div>
                            </div>
                            <div className="flex gap-1">
                                <button onClick={() => handleToggle(c)} className="p-1.5 text-zinc-500 hover:text-zinc-300">
                                    {c.is_active ? <Pause size={14} /> : <Play size={14} />}
                                </button>
                                <button onClick={() => handleDelete(c)} className="p-1.5 text-zinc-500 hover:text-red-400">
                                    <Trash2 size={14} />
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

// ============ Add Wallet Modal ============
function AddWalletModal({ apiBaseUrl, onClose, onAdded, brand }) {
    const [address, setAddress] = useState('');
    const [label, setLabel] = useState('');
    const [saving, setSaving] = useState(false);

    const handleSubmit = async () => {
        setSaving(true);
        try {
            await addSMWallet({ apiBaseUrl, address, label });
            onAdded?.();
            onClose();
        } catch (err) {
            alert(err.message);
        } finally {
            setSaving(false);
        }
    };

    return (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
            <div className="bg-zinc-900 rounded-xl p-5 w-full max-w-md">
                <div className="flex items-center justify-between mb-4">
                    <h3 className="text-lg font-medium text-zinc-100">添加钱包</h3>
                    <button onClick={onClose} className="text-zinc-500 hover:text-zinc-300"><X size={18} /></button>
                </div>
                <div className="space-y-3">
                    <input
                        className={`w-full bg-zinc-800 rounded-lg px-3 py-2.5 text-sm text-zinc-200 outline-none focus:ring-1 ${getBrandFocusRingClass(brand)}`}
                        placeholder="钱包地址 (0x...)"
                        value={address}
                        onChange={e => setAddress(e.target.value)}
                    />
                    <input
                        className={`w-full bg-zinc-800 rounded-lg px-3 py-2.5 text-sm text-zinc-200 outline-none focus:ring-1 ${getBrandFocusRingClass(brand)}`}
                        placeholder="标签（可选）"
                        value={label}
                        onChange={e => setLabel(e.target.value)}
                    />
                </div>
                <div className="flex gap-2 mt-4 justify-end">
                    <button onClick={onClose} className="text-sm text-zinc-400 hover:text-zinc-300 px-4 py-2">取消</button>
                    <button
                        onClick={handleSubmit}
                        disabled={!address || saving}
                        className={`text-sm ${brand.solidButtonClass} rounded-lg px-4 py-2 disabled:opacity-50`}
                    >
                        {saving ? '添加中...' : '添加钱包'}
                    </button>
                </div>
            </div>
        </div>
    );
}

// ============ MAIN COMPONENT ============
export default function SmartMoneyPage({ apiBaseUrl, accentTheme = 'lime' }) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);
    const [view, setView] = useState('pools'); // pools | wallets | settings
    const [stats, setStats] = useState(null);
    const [selectedPool, setSelectedPool] = useState(null);
    const [selectedWallet, setSelectedWallet] = useState(null);
    const [showAddModal, setShowAddModal] = useState(false);

    useEffect(() => {
        fetchSMStats({ apiBaseUrl }).then(setStats).catch(() => {});
        const interval = setInterval(() => {
            fetchSMStats({ apiBaseUrl }).then(setStats).catch(() => {});
        }, 30000);
        return () => clearInterval(interval);
    }, [apiBaseUrl]);

    const handleSelectPool = useCallback((pool) => {
        setSelectedPool(pool);
    }, []);

    const handleSelectWallet = useCallback((addr) => {
        setSelectedWallet(addr);
        setView('wallets');
    }, []);

    const handleBack = useCallback(() => {
        setSelectedPool(null);
        setSelectedWallet(null);
    }, []);

    // Breadcrumb navigation
    const isDetailView = selectedPool || selectedWallet;
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
        if (watcherEnabled) channels.push(`LP 监控 ${activeWallets} 钱包`);
        if (contractMonitorEnabled) channels.push(activeContracts > 0 ? `合约监控 ${activeContracts} 个` : '合约监控待配置');

        return {
            enabled: true,
            label: '监控已开启',
            detail: channels.length ? channels.join(' / ') : 'Smart Money 服务运行中',
        };
    }, [stats]);

    return (
        <div className="max-w-3xl mx-auto">
            {/* Header */}
            <div className="flex items-center justify-between mb-4">
                <h2 className="text-base font-semibold text-zinc-100">聪明钱</h2>
            </div>

            {stats && !isDetailView && (
                <div className="flex justify-end mb-3">
                    <div className="flex flex-col items-end gap-1 text-right">
                        <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[11px] ${monitorSummary.enabled ? brand.softButtonClass : 'bg-zinc-800 text-zinc-300'}`}>
                            <span className={`inline-block h-2 w-2 rounded-full ${monitorSummary.enabled ? brand.dotClass : 'bg-zinc-500'}`} />
                            {monitorSummary.label}
                        </span>
                        <span className="text-[10px] text-zinc-500">{monitorSummary.detail}</span>
                    </div>
                </div>
            )}

            {/* Stats bar */}
            {stats && !isDetailView && (
                <div className="grid grid-cols-4 gap-2 mb-4">
                    <StatCard label="活跃池子" value={stats.active_pool_count} />
                    <StatCard label="钱包数" value={stats.monitored_wallet_count} />
                    <StatCard label="持仓中" value={stats.open_position_count} />
                    <StatCard label="今日关闭" value={stats.closed_today_count} color="text-red-400" />
                </div>
            )}

            {/* Nav tabs */}
            {!isDetailView && (
                <div className="flex gap-1 mb-4">
                    {[
                        { key: 'pools', label: '池子', icon: Eye },
                        { key: 'wallets', label: '钱包', icon: Wallet },
                        { key: 'settings', label: '设置', icon: Settings },
                    ].map(({ key, label, icon: Icon }) => (
                        <button
                            key={key}
                            className={`flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm ${view === key
                                ? brand.softButtonClass
                                : 'bg-zinc-800 text-zinc-400 hover:bg-zinc-700'
                            }`}
                            onClick={() => setView(key)}
                        >
                            <Icon size={14} /> {key === 'settings' ? '合约视图' : label}
                        </button>
                    ))}
                </div>
            )}

            {/* Content */}
            {selectedPool ? (
                <PoolDetailPage
                    apiBaseUrl={apiBaseUrl}
                    pool={selectedPool}
                    onBack={handleBack}
                    onSelectWallet={handleSelectWallet}
                    brand={brand}
                />
            ) : selectedWallet ? (
                <WalletDetailPage
                    apiBaseUrl={apiBaseUrl}
                    walletAddress={selectedWallet}
                    onBack={handleBack}
                    onSelectPool={handleSelectPool}
                    brand={brand}
                />
            ) : view === 'pools' ? (
                <PoolListPage
                    apiBaseUrl={apiBaseUrl}
                    onSelectPool={handleSelectPool}
                    stats={stats}
                    brand={brand}
                />
            ) : view === 'wallets' ? (
                <WalletListPage
                    apiBaseUrl={apiBaseUrl}
                    onSelectWallet={(addr) => setSelectedWallet(addr)}
                    onAddWallet={() => setShowAddModal(true)}
                    brand={brand}
                />
            ) : (
                <ContractSettingsPage apiBaseUrl={apiBaseUrl} brand={brand} />
            )}

            {/* Add wallet modal */}
            {showAddModal && (
                <AddWalletModal
                    apiBaseUrl={apiBaseUrl}
                    onClose={() => setShowAddModal(false)}
                    brand={brand}
                    onAdded={() => {
                        // Refresh wallet list
                    }}
                />
            )}
        </div>
    );
}
