/**
 * SmartMoneyAssetsPage — 聪明钱资产概览/排行榜/钱包详情
 * 从原 AssetManagementPage 的 smart_money_assets tab 抽取，
 * 现作为 SmartMoneyPage "聪明钱资产" 视图的内容。
 */
import React, { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, Medal, RefreshCw, Search, Trophy } from 'lucide-react';
import {
    fetchAdminSmartMoneyLeaderboard,
    fetchAdminSmartMoneyOverview,
    fetchAdminSmartMoneyWallet,
} from '../lib/api';
import { getBrandTheme } from '../lib/brand';
import { resolveSMAvatarAssetUrl } from '../lib/smartMoneyApi';
import MiniChart from './MiniChart.jsx';
import NumberFlowValue from './NumberFlowValue.jsx';

const AVATAR_URLS = Object.entries(
    import.meta.glob('../icon/avatar_*.png', { eager: true, import: 'default' })
).sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true })).map(([, src]) => src);

const SMART_MONEY_WINDOWS = [1, 7, 30];
const CHINA_TIME_ZONE = 'Asia/Shanghai';
const LEADERBOARD_METRICS = [
    { key: 'pnl', label: '收益额' },
    { key: 'yield_rate', label: '收益率' },
    { key: 'participation', label: '参与次数' },
];
const SM_PAGE_SIZE = 10;

const usdFmt = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 });
function formatUsd(v) { const n = Number(v || 0); return Number.isFinite(n) ? usdFmt.format(n) : '$--'; }
function formatUsdCompact(v) {
    const n = Number(v || 0); if (!Number.isFinite(n)) return '$--';
    const a = Math.abs(n);
    if (a >= 1e6) return `$${(n/1e6).toFixed(a>=1e7?0:1).replace(/\.0$/,'')}M`;
    if (a >= 1e3) return `$${(n/1e3).toFixed(a>=1e4?0:1).replace(/\.0$/,'')}K`;
    if (a >= 100) return `$${n.toFixed(0)}`;
    if (a >= 10) return `$${n.toFixed(1).replace(/\.0$/,'')}`;
    return `$${n.toFixed(2).replace(/0+$/,'').replace(/\.$/,'')}`;
}
function formatPct(v, d=2) { const n=Number(v||0); return Number.isFinite(n)?`${(n*100).toFixed(d).replace(/\.?0+$/,'')}%`:'--'; }
function formatChain(cid) { return Number(cid) === 8453 ? 'Base' : 'BSC'; }
function walletKey(w) { return `${Number(w?.chain_id||0)}:${String(w?.address||'').toLowerCase()}`; }
function walletLabel(w) { const l=String(w?.label||'').trim(); if(l) return l; const a=String(w?.address||'').trim(); return a?`${a.slice(0,6)}...${a.slice(-4)}`:'--'; }
function walletSourceLabel(w) {
    const s=String(w?.source??w?.wallet_source??'').trim();
    if(s==='manual') return '手动添加';
    if(s==='contract_interaction') return '合约发现';
    return s||'未标记来源';
}
function walletSourceContractLabel(w) {
    const a=String(w?.source_contract??w?.wallet_source_contract??'').trim();
    if(!/^0x[0-9a-fA-F]{40}$/.test(a)) return '';
    return `来源合约 ${a.slice(0,6)}...${a.slice(-4)}`;
}
function errorText(e) { return String(e?.message||e||'').trim(); }
function isIgnorableSmartMoneyDataError(err) { const m=errorText(err).toLowerCase(); return m.includes("unknown column 'open_lp_usd'")||m.includes("unknown column `open_lp_usd`"); }
function chinaDateParts(v=new Date()) {
    const d=v instanceof Date?v:new Date(v); if(Number.isNaN(d.getTime())) return null;
    const parts=new Intl.DateTimeFormat('en-CA',{timeZone:CHINA_TIME_ZONE,year:'numeric',month:'2-digit',day:'2-digit'}).formatToParts(d);
    const m={}; parts.forEach(p=>{if(p.type!=='literal')m[p.type]=p.value;}); return m.year&&m.month&&m.day?m:null;
}
function formatChinaDay(v=new Date()) { const p=chinaDateParts(v); return p?`${p.year}-${p.month}-${p.day}`:''; }
function monthStart(v=new Date()) { const d=v instanceof Date?v:new Date(v); return Number.isNaN(d.getTime())?new Date():new Date(d.getFullYear(),d.getMonth(),1); }
function calendarHistoryDaysForMonth(v=new Date()) {
    const start=monthStart(v);
    const now=new Date();
    const todayStart=new Date(now.getFullYear(),now.getMonth(),now.getDate());
    const diffDays=Math.ceil((todayStart.getTime()-start.getTime())/86400000)+32;
    return Math.max(30,Math.min(365,diffDays));
}

/* ─── Pill ─── */
function Pill({ active, brand, onClick, children }) {
    return (
        <button type="button" onClick={onClick} className={`inline-flex items-center rounded-full px-2.5 py-1 text-[10px] font-semibold ring-1 transition active:scale-95 ${active ? brand.softButtonClass : 'bg-zinc-100 text-zinc-500 ring-zinc-200 hover:bg-zinc-50 dark:bg-white/[0.04] dark:text-white/50 dark:ring-white/[0.06] dark:hover:bg-white/[0.07]'}`}>{children}</button>
    );
}

function SmSearchInput({ value, onChange, placeholder }) {
    return (
        <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-zinc-400 dark:text-white/30" />
            <input type="text" value={value} onChange={e=>onChange(e.target.value)} placeholder={placeholder} className="w-full rounded-xl border border-zinc-200 bg-zinc-50/80 py-2 pl-8 pr-3 text-[11px] text-zinc-700 placeholder-zinc-400 outline-none ring-0 transition focus:border-zinc-300 focus:ring-1 focus:ring-zinc-300 dark:border-white/[0.06] dark:bg-white/[0.03] dark:text-white/80 dark:placeholder-white/25 dark:focus:border-white/10 dark:focus:ring-white/10" />
        </div>
    );
}

function SmPagination({ page, totalPages, onPageChange }) {
    if (totalPages <= 1) return null;
    return (
        <div className="flex items-center justify-center gap-3 pt-2">
            <button type="button" disabled={page<=0} onClick={()=>onPageChange(page-1)} className="inline-flex items-center rounded-lg px-2 py-1 text-[10px] font-medium text-zinc-500 ring-1 ring-zinc-200 transition enabled:hover:bg-zinc-100 enabled:active:scale-95 disabled:opacity-30 dark:text-white/50 dark:ring-white/[0.06] dark:enabled:hover:bg-white/[0.06]">上一页</button>
            <span className="text-[10px] tabular-nums text-zinc-400 dark:text-white/35">{page+1} / {totalPages}</span>
            <button type="button" disabled={page>=totalPages-1} onClick={()=>onPageChange(page+1)} className="inline-flex items-center rounded-lg px-2 py-1 text-[10px] font-medium text-zinc-500 ring-1 ring-zinc-200 transition enabled:hover:bg-zinc-100 enabled:active:scale-95 disabled:opacity-30 dark:text-white/50 dark:ring-white/[0.06] dark:enabled:hover:bg-white/[0.06]">下一页</button>
        </div>
    );
}

function Card({ children, className = '' }) {
    return <div className={`rounded-2xl border border-zinc-200/80 bg-white p-3 dark:border-white/5 dark:bg-[#131518] ${className}`.trim()}>{children}</div>;
}

function StatBlock({ label, value, tone = 'default' }) {
    const ring = tone==='accent'?'ring-emerald-500/20 dark:ring-emerald-400/25':tone==='warn'?'ring-amber-500/20 dark:ring-amber-400/25':'ring-zinc-200 dark:ring-white/[0.06]';
    const bg = tone==='accent'?'bg-emerald-500/[0.06] dark:bg-emerald-500/[0.08]':tone==='warn'?'bg-amber-500/[0.06] dark:bg-amber-500/[0.08]':'bg-zinc-50 dark:bg-white/[0.03]';
    return (
        <div className={`rounded-xl ${bg} ring-1 ${ring} px-3 py-2.5`}>
            <div className="text-[9px] font-medium uppercase tracking-wide text-zinc-400 dark:text-white/35">{label}</div>
            <div className="mt-1 text-base font-extrabold tabular-nums text-zinc-900 dark:text-white/95 leading-none"><NumberFlowValue value={value} formatter={()=>value} /></div>
        </div>
    );
}

function Empty({ text }) {
    return <div className="flex items-center justify-center rounded-xl border border-dashed border-zinc-200 bg-zinc-50/50 px-4 py-6 text-[11px] text-zinc-400 dark:border-white/[0.06] dark:bg-white/[0.02] dark:text-white/30">{text}</div>;
}

function walletAvatarUrl(address) {
    if (!AVATAR_URLS.length) return '';
    const hex = String(address||'').toLowerCase();
    let hash = 0;
    for (let i=0;i<hex.length;i++) hash=((hash<<5)-hash+hex.charCodeAt(i))|0;
    return AVATAR_URLS[Math.abs(hash)%AVATAR_URLS.length]||AVATAR_URLS[0]||'';
}

function WalletAvatar({ address, avatarUrl, size = 28, className = '' }) {
    const fallbackSrc = useMemo(()=>walletAvatarUrl(address),[address]);
    const preferredSrc = resolveSMAvatarAssetUrl(avatarUrl)||fallbackSrc;
    const [src, setSrc] = useState(preferredSrc);
    useEffect(()=>{setSrc(preferredSrc);},[preferredSrc]);
    if (!src) return null;
    return <img src={src} alt="" width={size} height={size} className={`shrink-0 rounded-lg object-cover ${className}`.trim()} style={{width:size,height:size}} onError={()=>{if(src!==fallbackSrc) setSrc(fallbackSrc);}} />;
}

function RankBadge({ rank }) {
    if (rank===1) return <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-yellow-400 to-amber-500 shadow-sm shadow-amber-500/30"><Trophy className="h-3.5 w-3.5 text-white" /></span>;
    if (rank===2) return <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-slate-300 to-slate-400 shadow-sm shadow-slate-400/30"><Medal className="h-3.5 w-3.5 text-white" /></span>;
    if (rank===3) return <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-amber-600 to-amber-700 shadow-sm shadow-amber-700/30"><Medal className="h-3.5 w-3.5 text-white" /></span>;
    return <span className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-zinc-100 text-[11px] font-bold tabular-nums text-zinc-500 dark:bg-white/[0.06] dark:text-white/50">{rank}</span>;
}

/* ─── PnL Calendar ─── */
const PNL_CAL_WEEKDAYS = ['一','二','三','四','五','六','日'];
function PnLCalendar({ data, loading=false, viewDate, onMonthChange }) {
    const currentViewDate=viewDate instanceof Date?viewDate:new Date();
    const changeMonth=useCallback((delta)=>{
        const next=new Date(currentViewDate.getFullYear(),currentViewDate.getMonth()+delta,1);
        if(typeof onMonthChange==='function') onMonthChange(next);
    },[currentViewDate,onMonthChange]);
    const year=currentViewDate.getFullYear(), month=currentViewDate.getMonth();
    const daysInMonth=new Date(year,month+1,0).getDate();
    const firstDayJS=new Date(year,month,1).getDay();
    const startOffset=firstDayJS===0?6:firstDayJS-1;
    const pnlMap=useMemo(()=>{const m={};if(Array.isArray(data))data.forEach(d=>{if(d.day)m[d.day]=d;});return m;},[data]);
    const monthLabel=new Intl.DateTimeFormat('en-US',{timeZone:CHINA_TIME_ZONE,year:'numeric',month:'short'}).format(new Date(Date.UTC(year,month,1,12,0,0)));
    const todayStr=formatChinaDay();
    if(loading) return <div className="animate-pulse rounded-lg bg-zinc-200 dark:bg-zinc-700" style={{height:200}} />;

    const cells=[];
    for(let i=0;i<startOffset;i++) cells.push(<div key={`e-${i}`} className="rounded-md bg-zinc-100/50 dark:bg-white/[0.02]" style={{minHeight:32}} />);
    for(let day=1;day<=daysInMonth;day++){
        const dateStr=`${year}-${String(month+1).padStart(2,'0')}-${String(day).padStart(2,'0')}`;
        const entry=pnlMap[dateStr]; const pnl=entry?Number(entry.realized_pnl_usd||0):null;
        const isToday=dateStr===todayStr; const isFuture=dateStr>todayStr;
        const dayTone=isToday?'text-emerald-700 dark:text-emerald-300':isFuture?'text-zinc-300 dark:text-white/15':'text-zinc-400 dark:text-white/30';
        const valTone=pnl!==null?(pnl>=0?'text-emerald-600 dark:text-emerald-400':'text-red-500 dark:text-red-400'):'text-transparent';
        cells.push(
            <div key={day} className={`rounded-md px-1 py-1 ${isToday?'bg-emerald-500/15 ring-1 ring-emerald-500/30':isFuture?'bg-zinc-100/30 dark:bg-white/[0.015]':'bg-zinc-100/50 dark:bg-white/[0.03]'}`} style={{minHeight:38}}>
                <div className={`text-[9px] leading-none ${dayTone}`}>{day}</div>
                <div className={`flex min-h-[20px] items-center justify-center px-0.5 text-center text-[10px] font-semibold leading-tight tabular-nums ${valTone}`}>{pnl!==null?`${pnl>=0?'+':''}${formatUsdCompact(pnl)}`:'0'}</div>
            </div>
        );
    }
    const rem=(startOffset+daysInMonth)%7;
    if(rem>0) for(let i=0;i<7-rem;i++) cells.push(<div key={`t-${i}`} className="rounded-md bg-zinc-100/30 dark:bg-white/[0.015]" style={{minHeight:32}} />);

    return (
        <div>
            <div className="flex items-center justify-between mb-2">
                <span className="text-[13px] font-bold text-zinc-900 dark:text-white/90">{monthLabel}</span>
                <div className="flex items-center gap-0.5">
                    <button type="button" onClick={()=>changeMonth(-1)} className="p-1 rounded-md hover:bg-zinc-200 dark:hover:bg-white/10 text-zinc-500 dark:text-white/40"><ChevronLeft size={14} /></button>
                    <button type="button" onClick={()=>changeMonth(1)} className="p-1 rounded-md hover:bg-zinc-200 dark:hover:bg-white/10 text-zinc-500 dark:text-white/40"><ChevronRight size={14} /></button>
                </div>
            </div>
            <div className="grid grid-cols-7 gap-1">
                {PNL_CAL_WEEKDAYS.map(d=><div key={d} className="text-center text-[9px] text-zinc-400 dark:text-white/20 pb-0.5">{d}</div>)}
                {cells}
            </div>
        </div>
    );
}

/* ═══════════════════════════════════════════════════════ */
export default function SmartMoneyAssetsPage({
    apiBaseUrl, initData, hasInitData, isAdmin = false,
    tick, pollIntervalSec = 15, accentTheme = 'lime', onNotice,
}) {
    const brand = useMemo(() => getBrandTheme(accentTheme), [accentTheme]);

    const [smartMoneyDays, setSmartMoneyDays] = useState(7);
    const [leaderboardMetric, setLeaderboardMetric] = useState('pnl');
    const [smartMoneyOverview, setSmartMoneyOverview] = useState(null);
    const [smartMoneyWallet, setSmartMoneyWallet] = useState(null);
    const [smartMoneyLeaderboard, setSmartMoneyLeaderboard] = useState(null);
    const [smartMoneyLoading, setSmartMoneyLoading] = useState(false);
    const [smartMoneyRefreshing, setSmartMoneyRefreshing] = useState(false);
    const [smartMoneyError, setSmartMoneyError] = useState('');
    const [selectedWalletId, setSelectedWalletId] = useState('');
    const [selectedWalletMeta, setSelectedWalletMeta] = useState(null);
    const [smSubTab, setSmSubTab] = useState('wallets');
    const [smWalletSearch, setSmWalletSearch] = useState('');
    const [smWalletPage, setSmWalletPage] = useState(0);
    const [smLeaderSearch, setSmLeaderSearch] = useState('');
    const [smLeaderPage, setSmLeaderPage] = useState(0);
    const [smDrillWalletId, setSmDrillWalletId] = useState('');
    const [smDetailCalendarMonth, setSmDetailCalendarMonth] = useState(()=>monthStart(new Date()));

    const hasSmartMoneyData = Boolean(smartMoneyOverview || smartMoneyLeaderboard || smartMoneyWallet);
    const hasSmartMoneyDataRef = useRef(false);
    useEffect(() => { hasSmartMoneyDataRef.current = hasSmartMoneyData; }, [hasSmartMoneyData]);

    const selectSmartMoneyWallet = useCallback((wallet, { openDetail = false } = {}) => {
        if (!wallet) return;
        const next = { address: wallet.address, chain_id: wallet.chain_id, label: wallet.label, avatar_url: wallet.avatar_url, source: wallet.source, source_contract: wallet.source_contract, assets: wallet.assets, active_pool_count: wallet.active_pool_count, today_event_count: wallet.today_event_count, last_active_at: wallet.last_active_at };
        const id = walletKey(next);
        setSelectedWalletId(id);
        setSelectedWalletMeta(next);
        if (openDetail) { setSmSubTab('wallets'); setSmDrillWalletId(id); setSmDetailCalendarMonth(monthStart(new Date())); }
    }, []);

    const applySmartMoneyWalletRows = useCallback((wallets) => {
        if (!Array.isArray(wallets)) return;
        const current = wallets.find(i => walletKey(i) === selectedWalletId);
        if (current) setSelectedWalletMeta(prev => ({ ...(prev || {}), ...current }));
        else if (!selectedWalletId && wallets[0]) selectSmartMoneyWallet(wallets[0]);
        else if (!wallets.length && !smDrillWalletId) { setSelectedWalletId(''); setSelectedWalletMeta(null); setSmartMoneyWallet(null); }
    }, [selectSmartMoneyWallet, selectedWalletId, smDrillWalletId]);

    const mergeSmartMoneyOverview = useCallback((patch) => {
        if (!patch) return;
        setSmartMoneyOverview(current => ({ ...(current || {}), ...patch }));
    }, []);

    const loadSmartMoneySummary = useCallback(async ({ forceRefresh = false } = {}) => {
        if (!hasInitData || !isAdmin) return;
        try {
            const summary = await fetchAdminSmartMoneyOverview({ apiBaseUrl, initData, days: smartMoneyDays, section: 'summary', forceRefresh });
            startTransition(() => { mergeSmartMoneyOverview(summary || {}); });
        } catch (err) { if (!isIgnorableSmartMoneyDataError(err)) setSmartMoneyError(errorText(err)); }
    }, [apiBaseUrl, hasInitData, initData, isAdmin, mergeSmartMoneyOverview, smartMoneyDays]);

    const loadSmartMoneyWallets = useCallback(async ({ forceRefresh = false } = {}) => {
        if (!hasInitData || !isAdmin) return;
        try {
            const overview = await fetchAdminSmartMoneyOverview({ apiBaseUrl, initData, days: smartMoneyDays, page: smWalletPage + 1, pageSize: SM_PAGE_SIZE, keyword: smWalletSearch, section: 'wallets', forceRefresh });
            const wallets = Array.isArray(overview?.wallets) ? overview.wallets : [];
            startTransition(() => { mergeSmartMoneyOverview(overview || {}); });
            applySmartMoneyWalletRows(wallets);
        } catch (err) { if (!isIgnorableSmartMoneyDataError(err)) setSmartMoneyError(errorText(err)); }
    }, [apiBaseUrl, applySmartMoneyWalletRows, hasInitData, initData, isAdmin, mergeSmartMoneyOverview, smWalletPage, smWalletSearch, smartMoneyDays]);

    const loadSmartMoneyLeaderboard = useCallback(async ({ forceRefresh = false } = {}) => {
        if (!hasInitData || !isAdmin) return;
        try {
            const leaderboard = await fetchAdminSmartMoneyLeaderboard({ apiBaseUrl, initData, days: 1, metric: leaderboardMetric, page: smLeaderPage + 1, pageSize: SM_PAGE_SIZE, keyword: smLeaderSearch, forceRefresh });
            startTransition(() => { setSmartMoneyLeaderboard(leaderboard || null); });
        } catch (err) { if (!isIgnorableSmartMoneyDataError(err)) setSmartMoneyError(errorText(err)); }
    }, [apiBaseUrl, hasInitData, initData, isAdmin, leaderboardMetric, smLeaderPage, smLeaderSearch]);

    const loadSmartMoney = useCallback(async ({ forceRefresh = false } = {}) => {
        if (!hasInitData || !isAdmin) return;
        if (hasSmartMoneyDataRef.current) setSmartMoneyRefreshing(true); else setSmartMoneyLoading(true);
        setSmartMoneyError('');
        try {
            await Promise.allSettled([
                loadSmartMoneySummary({ forceRefresh }),
                loadSmartMoneyWallets({ forceRefresh }),
                loadSmartMoneyLeaderboard({ forceRefresh }),
            ]);
        } finally { setSmartMoneyLoading(false); setSmartMoneyRefreshing(false); }
    }, [hasInitData, isAdmin, loadSmartMoneyLeaderboard, loadSmartMoneySummary, loadSmartMoneyWallets]);

    useEffect(() => {
        loadSmartMoneySummary();
    }, [loadSmartMoneySummary]);

    useEffect(() => {
        loadSmartMoneyWallets();
    }, [loadSmartMoneyWallets]);

    useEffect(() => {
        loadSmartMoneyLeaderboard();
    }, [loadSmartMoneyLeaderboard]);

    useEffect(() => {
        if (!hasInitData || !isAdmin) return undefined;
        const timer = setInterval(() => loadSmartMoney(), Math.max(60, Number(pollIntervalSec || 15)) * 1000);
        return () => clearInterval(timer);
    }, [hasInitData, isAdmin, loadSmartMoney, pollIntervalSec]);

    const selectedWallet = useMemo(() => {
        const wallets = Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : [];
        return wallets.find(i => walletKey(i) === selectedWalletId) || selectedWalletMeta || null;
    }, [selectedWalletId, selectedWalletMeta, smartMoneyOverview]);

    const loadSmartMoneyWallet = useCallback(async ({ wallet, forceRefresh = false } = {}) => {
        if (!wallet || !hasInitData || !isAdmin) return;
        const detailDays = Math.max(smartMoneyDays, calendarHistoryDaysForMonth(smDetailCalendarMonth));
        try {
            const detail = await fetchAdminSmartMoneyWallet({ apiBaseUrl, initData, address: wallet.address, chainId: wallet.chain_id, days: detailDays, forceRefresh });
            startTransition(() => { setSmartMoneyWallet(detail || null); });
            setSmartMoneyError('');
        } catch (err) { if (!isIgnorableSmartMoneyDataError(err)) setSmartMoneyError(errorText(err)); }
    }, [apiBaseUrl, hasInitData, initData, isAdmin, smDetailCalendarMonth, smartMoneyDays]);

    useEffect(() => {
        if (smSubTab !== 'wallets' || !smDrillWalletId || !selectedWallet || !hasInitData || !isAdmin) return undefined;
        let disposed = false;
        const run = async (fr = false) => { await loadSmartMoneyWallet({ wallet: selectedWallet, forceRefresh: fr }); if (disposed) return; };
        run();
        const timer = setInterval(() => run(), Math.max(60, Number(pollIntervalSec || 15)) * 1000);
        return () => { disposed = true; clearInterval(timer); };
    }, [hasInitData, isAdmin, loadSmartMoneyWallet, pollIntervalSec, selectedWallet, smDrillWalletId, smSubTab]);

    const smartMoneyRows = useMemo(
        () => (Array.isArray(smartMoneyWallet?.history) ? smartMoneyWallet.history.map(i => ({ close: Number(i?.total_usd || 0) })) : []),
        [smartMoneyWallet],
    );
    const smartMoneyPnlCalData = useMemo(() => {
        const h = Array.isArray(smartMoneyWallet?.history) ? [...smartMoneyWallet.history].sort((a,b) => a.day.localeCompare(b.day)) : [];
        return h.map(i => ({ day: i.day, realized_pnl_usd: Number(i.estimated_realized_pnl_usd || 0), has_transfer_in: Boolean(i.has_transfer_in), has_transfer_out: Boolean(i.has_transfer_out), transfer_in_count: Number(i.transfer_in_count||0), transfer_out_count: Number(i.transfer_out_count||0), transfer_total_count: Number(i.transfer_total_count||0), transfer_in_usd: Number(i.transfer_in_usd||0), transfer_out_usd: Number(i.transfer_out_usd||0), transfer_net_usd: Number(i.transfer_net_usd||0) }));
    }, [smartMoneyWallet?.history]);

    const overviewWallets = useMemo(() => (Array.isArray(smartMoneyOverview?.wallets) ? smartMoneyOverview.wallets : []), [smartMoneyOverview]);
    const walletTotal = Math.max(0, Number(smartMoneyOverview?.wallet_total || 0) || overviewWallets.length);
    const walletTotalPages = Math.max(1, Number(smartMoneyOverview?.wallet_total_pages || 0) || 1);
    const pagedWallets = overviewWallets;

    useEffect(() => { if (smWalletPage > walletTotalPages - 1) setSmWalletPage(Math.max(walletTotalPages - 1, 0)); }, [smWalletPage, walletTotalPages]);
    const leaderboardRows = useMemo(() => (Array.isArray(smartMoneyLeaderboard?.list) ? smartMoneyLeaderboard.list : []), [smartMoneyLeaderboard]);
    const leaderTotalPages = Math.max(1, Number(smartMoneyLeaderboard?.total_pages || 0) || 1);
    useEffect(() => { if (smLeaderPage > leaderTotalPages - 1) setSmLeaderPage(Math.max(leaderTotalPages - 1, 0)); }, [leaderTotalPages, smLeaderPage]);
    useEffect(() => { setSmLeaderPage(0); }, [leaderboardMetric]);

    const isLoading = smartMoneyLoading || smartMoneyRefreshing;

    return (
        <div className="flex flex-col gap-3">
            {/* refresh header */}
            <div className="flex items-center justify-between gap-2">
                <span className="text-[13px] font-bold text-zinc-100">聪明钱资产</span>
                <button type="button" onClick={() => loadSmartMoney({ forceRefresh: true })} disabled={isLoading} className="inline-flex h-7 w-7 items-center justify-center rounded-lg border border-white/10 bg-white/5 text-white/50 transition active:scale-95 disabled:opacity-40">
                    <RefreshCw className={`h-3.5 w-3.5 ${isLoading ? 'animate-spin' : ''}`} />
                </button>
            </div>

            {smartMoneyError && <div className="rounded-xl border border-red-500/20 bg-red-500/[0.06] px-3 py-2.5 text-[11px] font-medium text-red-300 ring-1 ring-red-500/15">{smartMoneyError}</div>}

            <div className="flex flex-wrap gap-1.5">
                {SMART_MONEY_WINDOWS.map(d => <Pill key={d} active={smartMoneyDays === d} brand={brand} onClick={() => setSmartMoneyDays(d)}>{d === 1 ? '昨日' : `${d}D`}</Pill>)}
            </div>

            <div className="grid grid-cols-2 gap-2">
                <StatBlock label="总资产" value={formatUsd(smartMoneyOverview?.summary?.total_usd)} tone="accent" />
                <StatBlock label="原生币" value={formatUsd(smartMoneyOverview?.summary?.native_usd)} />
                <StatBlock label="稳定币" value={formatUsd(smartMoneyOverview?.summary?.stable_usd)} />
                <StatBlock label="代币持仓" value={formatUsd(smartMoneyOverview?.summary?.tracked_token_usd)} />
                <StatBlock label="Open LP" value={formatUsd(smartMoneyOverview?.summary?.open_lp_usd)} />
                <StatBlock label="代币种类" value={`${Number(smartMoneyOverview?.summary?.tracked_token_count || 0)} 个`} />
            </div>

            {/* sub-tab pills */}
            <div className="flex gap-1.5">
                <Pill active={smSubTab === 'wallets'} brand={brand} onClick={() => { setSmSubTab('wallets'); setSmDrillWalletId(''); }}>钱包总览</Pill>
                <Pill active={smSubTab === 'leaderboard'} brand={brand} onClick={() => { setSmSubTab('leaderboard'); setSmDrillWalletId(''); }}>排行榜</Pill>
            </div>

            {/* wallets list */}
            {smSubTab === 'wallets' && !smDrillWalletId && (
                <Card>
                    <div className="flex items-center justify-between gap-2">
                        <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">钱包总览</span>
                        <span className="inline-flex items-center rounded-full bg-zinc-100 px-2 py-0.5 text-[10px] font-semibold text-zinc-500 ring-1 ring-zinc-200 dark:bg-white/[0.04] dark:text-white/50 dark:ring-white/[0.06]">{walletTotal} 个</span>
                    </div>
                    <div className="mt-2"><SmSearchInput value={smWalletSearch} onChange={v => { setSmWalletSearch(v); setSmWalletPage(0); }} placeholder="搜索地址或标签" /></div>
                    <div className="mt-2.5 flex flex-col gap-2">
                        {pagedWallets.length > 0 ? pagedWallets.map(wallet => {
                            const selected = walletKey(wallet) === selectedWalletId;
                            const assets = wallet.assets || {};
                            const total = Number(assets.total_usd || 0);
                            const nativePct = total > 0 ? (Number(assets.native_usd||0)/total*100) : 0;
                            const stablePct = total > 0 ? (Number(assets.stable_usd||0)/total*100) : 0;
                            const tokenPct = total > 0 ? (Number(assets.tracked_token_usd||0)/total*100) : 0;
                            const lpPct = total > 0 ? (Number(assets.open_lp_usd||0)/total*100) : 0;
                            return (
                                <button key={walletKey(wallet)} type="button" onClick={() => selectSmartMoneyWallet(wallet, { openDetail: true })}
                                    className={`flex w-full flex-col gap-2 rounded-xl border px-3 py-2.5 text-left transition active:scale-[0.98] ${selected ? `${brand.selectionClass} dark:text-white` : 'border-zinc-100 bg-zinc-50/60 text-zinc-700 hover:bg-white dark:border-white/[0.04] dark:bg-[#0d0f12] dark:text-white/75 dark:hover:bg-white/[0.06]'}`}>
                                    <div className="flex items-center justify-between gap-3 w-full">
                                        <div className="flex items-center gap-2.5 min-w-0">
                                            <WalletAvatar address={wallet.address} avatarUrl={wallet.avatar_url} size={28} />
                                            <div className="min-w-0">
                                                <div className="truncate text-[12px] font-semibold">{walletLabel(wallet)}</div>
                                                <div className="mt-0.5 text-[10px] opacity-60">
                                                    {formatChain(wallet.chain_id)} · {walletSourceLabel(wallet)} · {Number(wallet.today_event_count||0)} 事件 · {Number(wallet.active_pool_count||0)} 池
                                                    {walletSourceContractLabel(wallet) ? ` · ${walletSourceContractLabel(wallet)}` : ''}
                                                </div>
                                            </div>
                                        </div>
                                        <div className="flex items-center gap-1.5 shrink-0">
                                            <span className="text-[13px] font-bold tabular-nums">{formatUsdCompact(wallet.assets?.total_usd)}</span>
                                            <ChevronRight className="h-3 w-3 opacity-40" />
                                        </div>
                                    </div>
                                    {total > 0 && (
                                        <div className="w-full">
                                            <div className="h-1.5 w-full rounded-full overflow-hidden bg-zinc-200/60 dark:bg-white/[0.06]">
                                                <div className="flex h-full">
                                                    {nativePct > 0 && <div className="h-full" style={{width:`${nativePct}%`,backgroundColor:'#0ea5e9'}} />}
                                                    {stablePct > 0 && <div className="h-full" style={{width:`${stablePct}%`,backgroundColor:'#10b981'}} />}
                                                    {tokenPct > 0 && <div className="h-full" style={{width:`${tokenPct}%`,backgroundColor:'#8b5cf6'}} />}
                                                    {lpPct > 0 && <div className="h-full" style={{width:`${lpPct}%`,backgroundColor:'#f59e0b'}} />}
                                                </div>
                                            </div>
                                        </div>
                                    )}
                                </button>
                            );
                        }) : <Empty text={smartMoneyLoading ? '加载中...' : '暂无钱包数据'} />}
                    </div>
                    <SmPagination page={smWalletPage} totalPages={walletTotalPages} onPageChange={setSmWalletPage} />
                </Card>
            )}

            {/* wallet drill-in detail */}
            {smSubTab === 'wallets' && smDrillWalletId && (
                <Card>
                    <button type="button" onClick={() => setSmDrillWalletId('')} className="inline-flex items-center gap-1 text-[11px] font-medium text-zinc-500 hover:text-zinc-700 dark:text-white/40 dark:hover:text-white/70 transition mb-2">
                        <ChevronLeft className="h-3.5 w-3.5" />返回列表
                    </button>
                    {selectedWallet && smartMoneyWallet ? (
                        <div className="flex flex-col gap-2.5">
                            <div className="flex items-center gap-3 rounded-xl bg-emerald-500/[0.06] ring-1 ring-emerald-500/20 dark:bg-emerald-500/[0.08] dark:ring-emerald-400/25 px-3 py-2.5">
                                <WalletAvatar address={selectedWallet.address} avatarUrl={selectedWallet.avatar_url || smartMoneyWallet.wallet?.avatar_url} size={36} />
                                <div className="flex-1 min-w-0">
                                    <div className="text-[12px] font-bold text-zinc-900 dark:text-white/95 truncate">{walletLabel(selectedWallet)}</div>
                                    <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                        {formatChain(selectedWallet.chain_id)} · {walletSourceLabel(smartMoneyWallet.wallet || selectedWallet)} · 总资产 <span className="font-bold text-zinc-900 dark:text-white/90">{formatUsdCompact(smartMoneyWallet.wallet?.assets?.total_usd)}</span>
                                        {walletSourceContractLabel(smartMoneyWallet.wallet || selectedWallet) ? ` · ${walletSourceContractLabel(smartMoneyWallet.wallet || selectedWallet)}` : ''}
                                    </div>
                                </div>
                            </div>
                            <div className="grid grid-cols-3 gap-1.5">
                                <StatBlock label="今日收益" value={formatUsd(smartMoneyWallet.today?.estimated_realized_pnl_usd)} tone={Number(smartMoneyWallet.today?.estimated_realized_pnl_usd||0)>=0?'accent':'warn'} />
                                <StatBlock label="加仓次数" value={`${Number(smartMoneyWallet.today?.add_count||0)} 次`} />
                                <StatBlock label="撤仓次数" value={`${Number(smartMoneyWallet.today?.remove_count||0)} 次`} />
                            </div>
                            {smartMoneyRows.length > 1 && <MiniChart data={smartMoneyRows} color="#10b981" height={120} />}
                            <PnLCalendar data={smartMoneyPnlCalData} loading={smartMoneyLoading} viewDate={smDetailCalendarMonth} onMonthChange={setSmDetailCalendarMonth} />
                        </div>
                    ) : <Empty text={smartMoneyLoading ? '加载钱包详情中...' : '暂无数据'} />}
                </Card>
            )}

            {/* leaderboard */}
            {smSubTab === 'leaderboard' && (
                <Card>
                    <div className="flex items-center justify-between gap-2">
                        <span className="text-[12px] font-bold text-zinc-900 dark:text-white/90">排行榜 (昨日)</span>
                        <div className="flex gap-1">
                            {LEADERBOARD_METRICS.map(m => <Pill key={m.key} active={leaderboardMetric === m.key} brand={brand} onClick={() => setLeaderboardMetric(m.key)}>{m.label}</Pill>)}
                        </div>
                    </div>
                    <div className="mt-2"><SmSearchInput value={smLeaderSearch} onChange={v => { setSmLeaderSearch(v); setSmLeaderPage(0); }} placeholder="搜索地址或标签" /></div>
                    <div className="mt-2.5 flex flex-col gap-2">
                        {leaderboardRows.length > 0 ? leaderboardRows.map((item, idx) => {
                            const rank = smLeaderPage * SM_PAGE_SIZE + idx + 1;
                            return (
                                <button key={`${item.address}:${item.chain_id}`} type="button"
                                    onClick={() => selectSmartMoneyWallet(item, { openDetail: true })}
                                    className="flex w-full items-center gap-2.5 rounded-xl border border-zinc-100 bg-zinc-50/60 px-3 py-2.5 text-left transition active:scale-[0.98] hover:bg-white dark:border-white/[0.04] dark:bg-[#0d0f12] dark:text-white/75 dark:hover:bg-white/[0.06]">
                                    <RankBadge rank={rank} />
                                    <WalletAvatar address={item.address} avatarUrl={item.avatar_url} size={28} />
                                    <div className="flex-1 min-w-0">
                                        <div className="truncate text-[12px] font-semibold text-zinc-900 dark:text-white/90">{walletLabel(item)}</div>
                                        <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40">
                                            {formatChain(item.chain_id)} · {walletSourceLabel(item)} · {Number(item.participation_count||0)} 笔
                                            {walletSourceContractLabel(item) ? ` · ${walletSourceContractLabel(item)}` : ''}
                                        </div>
                                    </div>
                                    <div className="text-right shrink-0">
                                        {(() => {
                                            const pnl = Number(item.estimated_realized_pnl_usd || 0);
                                            const yieldRate = Number(item.yield_rate || 0);
                                            const participationCount = Number(item.participation_count || 0);
                                            const primaryClass = leaderboardMetric === 'participation'
                                                ? 'text-zinc-700 dark:text-white/80'
                                                : ((leaderboardMetric === 'yield_rate' ? yieldRate : pnl) >= 0
                                                    ? 'text-emerald-600 dark:text-emerald-300'
                                                    : 'text-red-600 dark:text-red-300');
                                            let primaryText = `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}`;
                                            let secondaryText = formatPct(yieldRate);
                                            if (leaderboardMetric === 'yield_rate') {
                                                primaryText = formatPct(yieldRate);
                                                secondaryText = `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}`;
                                            } else if (leaderboardMetric === 'participation') {
                                                primaryText = `${participationCount} 次`;
                                                secondaryText = `${pnl >= 0 ? '+' : ''}${formatUsdCompact(pnl)}`;
                                            }
                                            return (
                                                <>
                                                    <div className={`text-[12px] font-bold tabular-nums ${primaryClass}`}>
                                                        {primaryText}
                                                    </div>
                                                    <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40 tabular-nums">{secondaryText}</div>
                                                </>
                                            );
                                        })()}
                                    </div>
                                </button>
                            );
                        }) : <Empty text={smartMoneyLoading ? '加载中...' : '暂无排行榜数据'} />}
                    </div>
                    <SmPagination page={smLeaderPage} totalPages={leaderTotalPages} onPageChange={setSmLeaderPage} />
                </Card>
            )}
        </div>
    );
}


