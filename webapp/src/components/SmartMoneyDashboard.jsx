import React, { useState, useEffect, useCallback, useMemo } from 'react';
import {
    Eye, Wallet, Settings, Search, Plus, ExternalLink, X, Check,
    ChevronRight, ChevronDown, Pause, Play, Trash2, Copy, Brain,
} from 'lucide-react';
import {
    fetchSMPools, fetchSMPoolStats, fetchSMPositions, fetchSMWallets,
    fetchSMStats, addSMWallet, updateSMWallet, deleteSMWallet,
    fetchSMContracts, addSMContract, updateSMContract, deleteSMContract,
} from '../smartMoneyApi';
import uniswapLogo from '../img/uniswap.svg';
import pancakeLogo from '../img/pancake.svg';

const PROTOCOL_MAP = {
    pancake_v3: { version: 'V3', icon: pancakeLogo, color: '#d1884f' },
    uniswap_v3: { version: 'V3', icon: uniswapLogo, color: '#ff007a' },
    uniswap_v4: { version: 'V4', icon: uniswapLogo, color: '#ff007a' },
};
const PROTOCOL_LABELS = Object.fromEntries(Object.entries(PROTOCOL_MAP).map(([k, v]) => [k, v.version]));

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

function WalletDot({ color, size = 18, label }) {
    return (
        <span
            className="smd-wallet-dot"
            style={{ backgroundColor: color, width: size, height: size, fontSize: size * 0.5 }}
        >
            {(label || '?')[0].toUpperCase()}
        </span>
    );
}

function Badge({ children, cls = '' }) {
    return <span className={`smd-badge ${cls}`}>{children}</span>;
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
    const curPct = currentPrice ? pct(parseFloat(currentPrice)) : null;
    const walletIdx = {};

    return (
        <div className="smd-price-chart">
            <div className="smd-price-chart-area" style={{ minHeight: valid.length * 14 + 30 }}>
                {curPct !== null && (
                    <div className="smd-price-cur" style={{ left: `${curPct}%` }}>
                        <div className="smd-price-cur-label">{currentPrice}</div>
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
                            left: `${l}%`, width: `${w}%`, top: i * 14,
                            backgroundColor: color, opacity: inRange ? op : 0.35,
                        }} title={`${shortAddr(p.wallet_address)}: ${p.price_lower} - ${p.price_upper}`} />
                    );
                })}
                <div className="smd-price-axis">
                    {Array.from({ length: 6 }, (_, i) => (
                        <span key={i}>{(minP + ((maxP - minP) / 5) * i).toPrecision(4)}</span>
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

function PoolList({ apiBaseUrl, onSelect }) {
    const [pools, setPools] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState('');
    const [proto, setProto] = useState('all');

    useEffect(() => {
        setLoading(true);
        fetchSMPools({ apiBaseUrl }).then(d => setPools(d?.list || [])).catch(() => {}).finally(() => setLoading(false));
    }, [apiBaseUrl]);

    const filtered = useMemo(() => {
        let l = pools;
        if (search) { const q = search.toLowerCase(); l = l.filter(p => `${p.token0_symbol}/${p.token1_symbol}`.toLowerCase().includes(q) || p.pool_address.toLowerCase().includes(q)); }
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
                <table className="smd-table">
                    <thead>
                    <tr>
                        <th>池子</th>
                        <th>协议</th>
                        <th className="right">钱包数</th>
                        <th className="right">仓位数</th>
                        <th className="right">最近活跃</th>
                    </tr>
                    </thead>
                    <tbody>
                    {filtered.map(p => (
                        <tr key={p.pool_address} className="clickable" onClick={() => onSelect(p)}>
                            <td>
                                <span style={{ fontWeight: 500 }}>{p.token0_symbol}/{p.token1_symbol}</span>
                                {p.fee_tier && <Badge cls="fee" style={{ marginLeft: 6 }}>{formatFeeTier(p.fee_tier)}</Badge>}
                            </td>
                            <td><ProtocolBadge protocol={p.protocol} /></td>
                            <td className="right">{p.wallet_count}</td>
                            <td className="right">{p.open_position_count}</td>
                            <td className="right">
                                    <span className={`smd-status-dot ${p.latest_event_at && (Date.now() - new Date(p.latest_event_at).getTime()) < 120000 ? 'green' : 'muted'}`}>
                                        {relativeTime(p.latest_event_at)}
                                    </span>
                            </td>
                        </tr>
                    ))}
                    </tbody>
                </table>
            )}
        </div>
    );
}

function PoolDetail({ apiBaseUrl, pool, onBack, onSelectWallet }) {
    const [positions, setPositions] = useState([]);
    const [stats, setStats] = useState(null);
    const [status, setStatus] = useState('open');
    const [loading, setLoading] = useState(true);

    useEffect(() => { fetchSMPoolStats({ apiBaseUrl, poolAddress: pool.pool_address }).then(setStats).catch(() => {}); }, [apiBaseUrl, pool.pool_address]);
    useEffect(() => {
        setLoading(true);
        fetchSMPositions({ apiBaseUrl, pool: pool.pool_address, status, size: 100 }).then(d => setPositions(d?.list || [])).catch(() => {}).finally(() => setLoading(false));
    }, [apiBaseUrl, pool.pool_address, status]);

    return (
        <div>
            <button onClick={onBack} className="smd-back-btn">&larr; 返回池子列表</button>
            <div className="smd-detail-header">
                <h3 className="smd-detail-title">{pool.token0_symbol}/{pool.token1_symbol}</h3>
                <ProtocolBadge protocol={pool.protocol} />
                {pool.fee_tier && <Badge cls="fee">{formatFeeTier(pool.fee_tier)}</Badge>}
                <span className="smd-detail-addr">{pool.pool_address}</span>
                <CopyBtn text={pool.pool_address} />
                <a href={`https://bscscan.com/address/${pool.pool_address}`} target="_blank" rel="noopener noreferrer" className="smd-link">
                    BSCScan <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} />
                </a>
            </div>

            {stats && (
                <div className="smd-stats-grid" style={{ gridTemplateColumns: 'repeat(5, 1fr)' }}>
                    <StatCard label="当前价格" value={stats.current_price || '—'} />
                    <StatCard label="24h 涨跌" value={stats.price_change_24h ? `${stats.price_change_24h}%` : '—'} color={stats.price_change_24h < 0 ? 'red' : undefined} />
                    <StatCard label="钱包数" value={stats.wallet_count} />
                    <StatCard label="持仓中" value={stats.open_position_count} />
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
                <table className="smd-table">
                    <thead>
                    <tr>
                        <th>钱包</th>
                        <th>价格区间</th>
                        <th className="center">区间内</th>
                        <th>开仓时间</th>
                        <th>NFT ID</th>
                        <th className="center">状态</th>
                        <th className="right">交易</th>
                    </tr>
                    </thead>
                    <tbody>
                    {positions.map(pos => (
                        <tr key={pos.id} className={pos.status === 'closed' ? 'dim' : ''}>
                            <td>
                                <button className="smd-wallet-info" style={{ background: 'none', border: 'none', cursor: 'pointer' }} onClick={() => onSelectWallet(pos.wallet_address)}>
                                    <WalletDot color={pos.wallet_color} size={16} label={pos.wallet_label || pos.wallet_address} />
                                    <span className="smd-wallet-info-name">{pos.wallet_label || shortAddr(pos.wallet_address)}</span>
                                </button>
                            </td>
                            <td className={`mono muted${pos.status === 'closed' ? '' : ''}`} style={pos.status === 'closed' ? { textDecoration: 'line-through' } : {}}>
                                {pos.price_lower || '—'} – {pos.price_upper || '—'}
                            </td>
                            <td className="center">
                                {pos.status === 'open' ? <span className="smd-status-dot green">在区间内</span> : <span className="smd-status-dot muted">—</span>}
                            </td>
                            <td className="muted">{pos.opened_at ? new Date(pos.opened_at).toLocaleDateString() : '—'}</td>
                            <td className="mono muted">#{pos.nft_token_id}</td>
                            <td className="center">
                                <span className={`smd-status-dot ${pos.status === 'open' ? 'green' : 'muted'}`}>{pos.status === 'open' ? '持仓中' : '已关闭'}</span>
                            </td>
                            <td className="right">
                                <a href={pos.bscscan_url} target="_blank" rel="noopener noreferrer" className="smd-link">
                                    {shortAddr(pos.open_tx_hash)} <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} />
                                </a>
                            </td>
                        </tr>
                    ))}
                    </tbody>
                </table>
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
                <table className="smd-table">
                    <thead>
                    <tr>
                        <th>钱包</th>
                        <th>来源</th>
                        <th className="right">持仓</th>
                        <th className="right">池子</th>
                        <th className="right">最近活跃</th>
                        <th className="center">状态</th>
                        <th className="right">操作</th>
                    </tr>
                    </thead>
                    <tbody>
                    {filtered.map(w => (
                        <tr key={w.address} className="clickable" onClick={() => onSelect(w.address)}>
                            <td>
                                <div className="smd-wallet-info">
                                    <WalletDot color={w.color} size={20} label={w.label || w.address} />
                                    <div>
                                        <div>{w.label || shortAddr(w.address)}</div>
                                        <div className="smd-detail-addr" style={{ marginLeft: 0 }}>{shortAddr(w.address)}</div>
                                    </div>
                                </div>
                            </td>
                            <td>
                                <Badge cls={w.source === 'manual' ? 'source-manual' : 'source-contract'}>
                                    {w.source === 'manual' ? '手动' : '合约'}
                                </Badge>
                            </td>
                            <td className="right">{w.open_position_count}</td>
                            <td className="right">{w.active_pool_count}</td>
                            <td className="right muted">{relativeTime(w.last_active_at)}</td>
                            <td className="center">
                                    <span className={`smd-status-dot ${w.is_active ? 'green' : 'muted'}`}>
                                        {w.is_active ? '监控中' : '已暂停'}
                                    </span>
                            </td>
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
            if (!m[p.pool_address]) m[p.pool_address] = { pool_address: p.pool_address, token0_symbol: p.token0_symbol, token1_symbol: p.token1_symbol, fee_tier: p.fee_tier, protocol: p.protocol, positions: [], hasOpen: false };
            m[p.pool_address].positions.push(p);
            if (p.status === 'open') m[p.pool_address].hasOpen = true;
        });
        return Object.values(m).sort((a, b) => (a.hasOpen ? -1 : 1) - (b.hasOpen ? -1 : 1));
    }, [positions]);

    return (
        <div>
            <button onClick={onBack} className="smd-back-btn">&larr; 返回钱包列表</button>
            {info && (
                <div style={{ marginBottom: 16 }}>
                    <div className="smd-detail-header">
                        <WalletDot color={info.color || '#7F77DD'} size={36} label={info.label || addr} />
                        <div>
                            <h3 className="smd-detail-title">{info.label || '未命名钱包'}</h3>
                            <span className="smd-detail-addr">{addr}</span>
                            <CopyBtn text={addr} />
                        </div>
                    </div>
                    <div className="smd-stats-grid">
                        <StatCard label="持仓中" value={info.open_position_count} />
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
                            <span className="smd-pool-group-pair">{g.token0_symbol}/{g.token1_symbol}</span>
                            {g.fee_tier && <Badge cls="fee">{formatFeeTier(g.fee_tier)}</Badge>}
                            <ProtocolBadge protocol={g.protocol} />
                            <span className="smd-pool-group-count">{g.positions.length} 个仓位</span>
                        </div>
                        <button className="smd-link" onClick={() => onSelectPool({
                            pool_address: g.pool_address, token0_symbol: g.token0_symbol, token1_symbol: g.token1_symbol, fee_tier: g.fee_tier, protocol: g.protocol,
                        })}>池子详情 <ExternalLink size={10} style={{ display: 'inline', verticalAlign: 'middle' }} /></button>
                    </div>
                    <table className="smd-table">
                        <thead><tr>
                            <th>价格区间</th>
                            <th className="center">状态</th>
                            <th>NFT</th>
                            <th className="right">交易</th>
                        </tr></thead>
                        <tbody>
                        {g.positions.map(pos => (
                            <tr key={pos.id} className={pos.status === 'closed' ? 'dim' : ''}>
                                <td className="mono muted" style={pos.status === 'closed' ? { textDecoration: 'line-through' } : {}}>
                                    {pos.price_lower || '—'} – {pos.price_upper || '—'}
                                </td>
                                <td className="center">
                                    <span className={`smd-status-dot ${pos.status === 'open' ? 'green' : 'muted'}`}>{pos.status}</span>
                                </td>
                                <td className="mono muted">#{pos.nft_token_id}</td>
                                <td className="right">
                                    <a href={pos.bscscan_url} target="_blank" rel="noopener noreferrer" className="smd-link">
                                        <ExternalLink size={10} />
                                    </a>
                                </td>
                            </tr>
                        ))}
                        </tbody>
                    </table>
                </div>
            ))}
        </div>
    );
}

function SettingsPanel({ apiBaseUrl }) {
    const [tab, setTab] = useState('wallets');
    const [wallets, setWallets] = useState([]);
    const [contracts, setContracts] = useState([]);
    const [loading, setLoading] = useState(true);
    const [busyKey, setBusyKey] = useState('');
    const [actionError, setActionError] = useState('');
    const [confirmState, setConfirmState] = useState(null);
    const [showAdd, setShowAdd] = useState(false);
    const [newAddr, setNewAddr] = useState('');
    const [newLabel, setNewLabel] = useState('');
    const [newProto, setNewProto] = useState('');
    const [newDesc, setNewDesc] = useState('');

    const loadWallets = useCallback(async () => {
        const d = await fetchSMWallets({ apiBaseUrl, size: 100 });
        setWallets(d?.list || []);
        return d;
    }, [apiBaseUrl]);
    const loadContracts = useCallback(async () => {
        const d = await fetchSMContracts({ apiBaseUrl });
        setContracts(d?.list || []);
        return d;
    }, [apiBaseUrl]);

    useEffect(() => {
        setLoading(true);
        Promise.all([loadWallets(), loadContracts()])
            .catch((err) => setActionError(String(err?.message || err || '加载失败')))
            .finally(() => setLoading(false));
    }, [loadWallets, loadContracts]);

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

    const handleAddWallet = async () => {
        await runAction('add-wallet', async () => {
            await addSMWallet({ apiBaseUrl, address: newAddr, label: newLabel });
            setShowAdd(false);
            setNewAddr('');
            setNewLabel('');
        }, loadWallets);
    };
    const handleAddContract = async () => {
        await runAction('add-contract', async () => {
            await addSMContract({ apiBaseUrl, contract_address: newAddr, protocol: newProto, description: newDesc });
            setShowAdd(false);
            setNewAddr('');
            setNewProto('');
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

    const addBusy = busyKey === 'add-wallet' || busyKey === 'add-contract';
    const deleteBusy = busyKey.startsWith('wallet-delete:') || busyKey.startsWith('contract-delete:');

    return (
        <div>
            <div className="smd-search-row">
                <div className="smd-filter-group">
                    {[['wallets', '钱包'], ['contracts', '合约']].map(([t, label]) => (
                        <button type="button" key={t} className={`smd-filter-btn${tab === t ? ' active' : ''}`} onClick={() => { setTab(t); setShowAdd(false); setActionError(''); }}>
                            {label}
                        </button>
                    ))}
                </div>
                <button type="button" onClick={() => setShowAdd(!showAdd)} className="smd-add-btn" style={{ marginLeft: 'auto' }}>
                    <Plus size={14} /> 添加{tab === 'wallets' ? '钱包' : '合约'}
                </button>
            </div>

            {actionError ? <div className="smd-inline-error">{actionError}</div> : null}

            {showAdd && (
                <div className="smd-add-form">
                    <input placeholder={tab === 'wallets' ? '0x...' : '合约地址'} value={newAddr} onChange={e => setNewAddr(e.target.value)} />
                    {tab === 'wallets' ? (
                        <input className="w-sm" placeholder="标签" value={newLabel} onChange={e => setNewLabel(e.target.value)} />
                    ) : (<>
                        <input className="w-md" placeholder="协议" value={newProto} onChange={e => setNewProto(e.target.value)} />
                        <input className="w-sm" placeholder="描述" value={newDesc} onChange={e => setNewDesc(e.target.value)} />
                    </>)}
                    <button type="button" disabled={addBusy} onClick={tab === 'wallets' ? handleAddWallet : handleAddContract}>
                        {addBusy ? '处理中...' : '添加'}
                    </button>
                </div>
            )}

            {loading ? <div className="smd-loading">加载中...</div> : tab === 'wallets' ? (
                <table className="smd-table">
                    <thead><tr>
                        <th>地址</th>
                        <th>标签</th>
                        <th>来源</th>
                        <th className="center">状态</th>
                        <th className="right">操作</th>
                    </tr></thead>
                    <tbody>
                    {wallets.map(w => (
                        <tr key={w.address}>
                            <td className="mono">{shortAddr(w.address)}</td>
                            <td className="muted">{w.label || '—'}</td>
                            <td><Badge cls={w.source === 'manual' ? 'source-manual' : 'source-contract'}>{w.source}</Badge></td>
                            <td className="center"><span className={`smd-status-dot ${w.is_active ? 'green' : 'muted'}`}>{w.is_active ? '活跃' : '已暂停'}</span></td>
                            <td className="right">
                                <div className="smd-action-row" style={{ justifyContent: 'flex-end' }}>
                                    <button type="button" className="smd-icon-btn" disabled={busyKey === `wallet-toggle:${w.address}` || busyKey === `wallet-delete:${w.address}`} onClick={() => runAction(`wallet-toggle:${w.address}`, () => updateSMWallet({ apiBaseUrl, address: w.address, updates: { is_active: !w.is_active } }), loadWallets)}>
                                        {w.is_active ? <Pause size={14} /> : <Play size={14} />}
                                    </button>
                                    <button
                                        type="button"
                                        className="smd-icon-btn danger"
                                        disabled={busyKey === `wallet-delete:${w.address}` || busyKey === `wallet-toggle:${w.address}`}
                                        onClick={() => openDeleteConfirm({
                                            key: `wallet-delete:${w.address}`,
                                            title: '删除钱包',
                                            description: `确认删除钱包 ${shortAddr(w.address)} 吗？`,
                                            action: () => deleteSMWallet({ apiBaseUrl, address: w.address }),
                                            refresh: loadWallets,
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
            ) : (
                <table className="smd-table">
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

export default function SmartMoneyDashboard({ apiBaseUrl }) {
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
                        <StatCard label="持仓中" value={stats.open_position_count} />
                        <StatCard label="今日关闭" value={stats.closed_today_count} color="red" />
                    </div>
                )}

                {!isDetail && (
                    <div className="smd-tabs">
                        {[
                            { key: 'pools', label: '池子视图', icon: Eye },
                            { key: 'wallets', label: '钱包视图', icon: Wallet },
                            { key: 'settings', label: '设置', icon: Settings },
                        ].map(({ key, label, icon: Icon }) => (
                            <button key={key} className={`smd-tab${view === key ? ' active' : ''}`} onClick={() => setView(key)}>
                                <Icon size={16} /> {label}
                            </button>
                        ))}
                    </div>
                )}

                {selectedPool ? (
                    <PoolDetail apiBaseUrl={apiBaseUrl} pool={selectedPool} onBack={() => setSelectedPool(null)} onSelectWallet={addr => { setSelectedWallet(addr); setView('wallets'); setSelectedPool(null); }} />
                ) : selectedWallet ? (
                    <WalletDetail apiBaseUrl={apiBaseUrl} addr={selectedWallet} onBack={() => setSelectedWallet(null)} onSelectPool={p => { setSelectedPool(p); setSelectedWallet(null); }} />
                ) : view === 'pools' ? (
                    <PoolList apiBaseUrl={apiBaseUrl} onSelect={setSelectedPool} />
                ) : view === 'wallets' ? (
                    <WalletList apiBaseUrl={apiBaseUrl} onSelect={setSelectedWallet} onAdd={() => setShowAddModal(true)} />
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
