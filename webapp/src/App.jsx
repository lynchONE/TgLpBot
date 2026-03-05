import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertTriangle,
  BrainCircuit,
  BriefcaseBusiness,
  CandlestickChart,
  Flame,
  GripVertical,
  Link2,
  LogOut,
  RefreshCw,
  Search,
  SlidersHorizontal,
} from 'lucide-react';
import {
  checkLoginCode,
  deleteTask,
  fetchHotPools,
  fetchPoolOHLCV,
  fetchRealtimePositions,
  fetchSmartMoneyOverview,
  generateLoginCode,
  openPosition as apiOpenPosition,
  setTaskPaused,
  stopTask,
  updateTaskRange,
} from './api';
import { WEBAPP_CONFIG } from './config';
import KlineChart from './components/KlineChart';
import PanelShell, { EmptyState, MetricCard } from './components/PanelShell';
import OpenPositionModal from './components/OpenPositionModal';
import TaskActionMenu from './components/TaskActionMenu';
import NumberFlowValue from './components/NumberFlowValue';
import telegramLogo from './img/telegram.svg';
import uniswapLogo from './img/uniswap.svg';
import pancakeLogo from './img/pancake.svg';
import bnbLogo from './img/bnb.svg';
import baseLogo from './img/base.svg';
import {
  DEFAULT_WIDGETS,
  WIDGETS,
  buildGmgnUrl,
  formatNumber,
  formatPct,
  formatPriceDisplay,
  formatUsd,
  formatUsdCompact,
  normalizePoolAddress,
  normalizeWidgetSelection,
  shortAddress,
} from './utils';

const STORAGE = {
  initData: 'tglp_web_init_data',
  loginUser: 'tglp_web_login_user',
  chain: 'tglp_web_chain',
  widgets: 'tglp_web_widgets',
  sort: 'tglp_web_hot_pools_sort',
  kline: 'tglp_web_kline_preset',
};

const KLINE_PRESETS = [
  { key: '1m', label: '1m', timeframe: 'minute', aggregate: 1, limit: 300 },
  { key: '5m', label: '5m', timeframe: 'minute', aggregate: 5, limit: 260 },
  { key: '15m', label: '15m', timeframe: 'minute', aggregate: 15, limit: 220 },
  { key: '1h', label: '1h', timeframe: 'hour', aggregate: 1, limit: 200 },
];

function storageGet(key) {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function storageSet(key, value) {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // ignore
  }
}

function storageRemove(key) {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // ignore
  }
}

function normalizeChain(value) {
  const chain = String(value || '').trim().toLowerCase();
  return chain === 'base' ? 'base' : 'bsc';
}

function moduleLayoutClass(count) {
  if (count <= 1) return 'layout-1';
  if (count === 2) return 'layout-2';
  if (count === 3) return 'layout-3';
  return 'layout-4';
}

function reorderList(list, fromKey, toKey) {
  const rows = [...list];
  const from = rows.indexOf(fromKey);
  const to = rows.indexOf(toKey);
  if (from < 0 || to < 0 || from === to) return rows;
  const [item] = rows.splice(from, 1);
  rows.splice(to, 0, item);
  return rows;
}

function parseLoginUser(raw) {
  if (!raw) return null;
  try {
    const user = JSON.parse(raw);
    return user && typeof user === 'object' ? user : null;
  } catch {
    return null;
  }
}

function openExternal(url) {
  if (!url) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}

function getDexIcon(factoryName) {
  const name = String(factoryName || '').toLowerCase();
  if (name.includes('uniswap')) {
    const m = name.match(/v(\d+)/i);
    return { src: uniswapLogo, label: m ? `V${m[1]}` : '', color: '#ff007a' };
  }
  if (name.includes('pancake') || name.includes('pcs')) {
    const m = name.match(/v(\d+)/i);
    return { src: pancakeLogo, label: m ? `V${m[1]}` : '', color: '#d1884f' };
  }
  return null;
}

export default function App() {
  const apiBaseUrl = WEBAPP_CONFIG.apiBaseUrl;

  const [initData, setInitData] = useState(() => String(storageGet(STORAGE.initData) || '').trim());
  const [loginUser, setLoginUser] = useState(() => parseLoginUser(storageGet(STORAGE.loginUser)));

  const [chain, setChain] = useState(() =>
    normalizeChain(storageGet(STORAGE.chain) || WEBAPP_CONFIG.defaultChain)
  );
  const [widgets, setWidgets] = useState(() => {
    const raw = storageGet(STORAGE.widgets);
    if (!raw) return [...DEFAULT_WIDGETS];
    try {
      return normalizeWidgetSelection(JSON.parse(raw));
    } catch {
      return [...DEFAULT_WIDGETS];
    }
  });
  const [hotSort, setHotSort] = useState(() => {
    const raw = String(storageGet(STORAGE.sort) || '').toLowerCase();
    return raw === 'fee_rate' || raw === 'volume' || raw === 'fees' ? raw : 'fees';
  });
  const [klinePresetKey, setKlinePresetKey] = useState(() => {
    const raw = String(storageGet(STORAGE.kline) || '5m');
    return KLINE_PRESETS.some((x) => x.key === raw) ? raw : '5m';
  });

  const [keyword, setKeyword] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [hotPools, setHotPools] = useState([]);
  const [hotPoolsLoading, setHotPoolsLoading] = useState(false);
  const [hotPoolsError, setHotPoolsError] = useState('');
  const [hotPoolsUpdatedAt, setHotPoolsUpdatedAt] = useState('');

  const [positions, setPositions] = useState(null);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [positionsError, setPositionsError] = useState('');

  const [smart, setSmart] = useState(null);
  const [smartLoading, setSmartLoading] = useState(false);
  const [smartError, setSmartError] = useState('');

  const [selectedPool, setSelectedPool] = useState(null);
  const [candles, setCandles] = useState([]);
  const [klineLoading, setKlineLoading] = useState(false);
  const [klineError, setKlineError] = useState('');
  const [klineUpdatedAt, setKlineUpdatedAt] = useState('');

  const [refreshing, setRefreshing] = useState(false);
  const [loginBusy, setLoginBusy] = useState(false);
  const [loginError, setLoginError] = useState('');

  const [draggingKey, setDraggingKey] = useState('');
  const [dragOverKey, setDragOverKey] = useState('');

  const hasInitData = Boolean(initData);
  const activeWidgets = useMemo(() => WIDGETS.filter((x) => widgets.includes(x.key)), [widgets]);
  const layoutClass = moduleLayoutClass(activeWidgets.length);

  const klinePreset = useMemo(
    () => KLINE_PRESETS.find((x) => x.key === klinePresetKey) || KLINE_PRESETS[1],
    [klinePresetKey]
  );

  const selectedPoolAddress = useMemo(
    () => normalizePoolAddress(selectedPool?.pool_address || selectedPool?.pool_id),
    [selectedPool]
  );
  const selectedPoolGmgnUrl = useMemo(() => buildGmgnUrl(selectedPool, chain), [selectedPool, chain]);
  const latestCandle = useMemo(() => (candles.length ? candles[candles.length - 1] : null), [candles]);

  const filteredHotPools = useMemo(() => {
    const q = String(keyword || '').trim().toLowerCase();
    if (!q) return hotPools;
    return hotPools.filter((x) => {
      const pair = String(x?.trading_pair || '').toLowerCase();
      const addr = String(x?.pool_address || '').toLowerCase();
      return pair.includes(q) || addr.includes(q);
    });
  }, [hotPools, keyword]);

  const sortedPositions = useMemo(() => {
    const rows = Array.isArray(positions?.positions) ? positions.positions : [];
    return [...rows].sort(
      (a, b) => Number(b?.totals?.total_usd || 0) - Number(a?.totals?.total_usd || 0)
    );
  }, [positions]);

  const smartPools = useMemo(() => {
    const rows = Array.isArray(smart?.pools) ? smart.pools : [];
    return [...rows].sort((a, b) => Number(b?.added_liquidity || 0) - Number(a?.added_liquidity || 0));
  }, [smart]);

  const smartWallets = useMemo(() => {
    const rows = Array.isArray(smart?.wallets_24h) ? smart.wallets_24h : [];
    return [...rows].sort((a, b) => Number(b?.pnl_usdt_24h || 0) - Number(a?.pnl_usdt_24h || 0));
  }, [smart]);

  useEffect(() => {
    storageSet(STORAGE.initData, initData);
    storageSet(STORAGE.chain, chain);
    storageSet(STORAGE.widgets, JSON.stringify(widgets));
    storageSet(STORAGE.sort, hotSort);
    storageSet(STORAGE.kline, klinePresetKey);

    if (loginUser) {
      storageSet(STORAGE.loginUser, JSON.stringify(loginUser));
    } else {
      storageRemove(STORAGE.loginUser);
    }
  }, [chain, hotSort, initData, klinePresetKey, loginUser, widgets]);

  const selectPool = useCallback(
    (pool, fallbackChain) => {
      const normalized = normalizePoolAddress(pool?.pool_address || pool?.pool_id);
      const next = {
        ...pool,
        chain: String(pool?.chain || fallbackChain || chain || 'bsc').toLowerCase(),
        pool_address: normalized || pool?.pool_address || pool?.pool_id,
      };
      setSelectedPool(next);
    },
    [chain]
  );

  const loadHotPools = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setHotPools([]);
        setHotPoolsError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      setHotPoolsLoading(true);
      setHotPoolsError('');
      try {
        const resp = await fetchHotPools({
          apiBaseUrl,
          initData,
          chain,
          sort: hotSort,
          timeframeMinutes: 5,
          limit: 60,
          signal,
        });
        setHotPools(Array.isArray(resp?.data) ? resp.data : []);
        setHotPoolsUpdatedAt(resp?.updated_at || new Date().toISOString());
      } catch (e) {
        if (e?.name !== 'AbortError') setHotPoolsError(String(e?.message || e));
      } finally {
        setHotPoolsLoading(false);
      }
    },
    [apiBaseUrl, chain, hasInitData, hotSort, initData]
  );

  const loadPositions = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setPositions(null);
        setPositionsError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      setPositionsLoading(true);
      setPositionsError('');
      try {
        setPositions(await fetchRealtimePositions({ apiBaseUrl, initData, signal }));
      } catch (e) {
        if (e?.name !== 'AbortError') setPositionsError(String(e?.message || e));
      } finally {
        setPositionsLoading(false);
      }
    },
    [apiBaseUrl, hasInitData, initData]
  );

  const loadSmart = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setSmart(null);
        setSmartError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      setSmartLoading(true);
      setSmartError('');
      try {
        setSmart(
          await fetchSmartMoneyOverview({
            apiBaseUrl,
            initData,
            chain,
            poolLimit: 24,
            walletLimit: 20,
            poolsWindowHours: 2,
            pnlWindowHours: 24,
            signal,
          })
        );
      } catch (e) {
        if (e?.name !== 'AbortError') setSmartError(String(e?.message || e));
      } finally {
        setSmartLoading(false);
      }
    },
    [apiBaseUrl, chain, hasInitData, initData]
  );

  const loadKline = useCallback(
    async (signal) => {
      if (!hasInitData) {
        setCandles([]);
        setKlineError('请先点击右上角 Telegram 图标扫码登录。');
        return;
      }
      if (!selectedPoolAddress) {
        setCandles([]);
        setKlineError('');
        return;
      }

      setKlineLoading(true);
      setKlineError('');
      try {
        const resp = await fetchPoolOHLCV({
          apiBaseUrl,
          initData,
          chain: selectedPool?.chain || chain,
          poolAddress: selectedPoolAddress,
          timeframe: klinePreset.timeframe,
          aggregate: klinePreset.aggregate,
          limit: klinePreset.limit,
          signal,
        });
        setCandles(Array.isArray(resp?.candles) ? resp.candles : []);
        setKlineUpdatedAt(resp?.updated_at || new Date().toISOString());
      } catch (e) {
        if (e?.name !== 'AbortError') setKlineError(String(e?.message || e));
      } finally {
        setKlineLoading(false);
      }
    },
    [
      apiBaseUrl,
      chain,
      hasInitData,
      initData,
      klinePreset.aggregate,
      klinePreset.limit,
      klinePreset.timeframe,
      selectedPool?.chain,
      selectedPoolAddress,
    ]
  );

  useEffect(() => {
    const ctrl = new AbortController();
    loadHotPools(ctrl.signal);
    loadPositions(ctrl.signal);
    loadSmart(ctrl.signal);
    return () => ctrl.abort();
  }, [loadHotPools, loadPositions, loadSmart]);

  useEffect(() => {
    if (!selectedPoolAddress || !hasInitData) return;
    const ctrl = new AbortController();
    loadKline(ctrl.signal);
    return () => ctrl.abort();
  }, [hasInitData, loadKline, selectedPoolAddress]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadHotPools(), 15_000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadHotPools]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadPositions(), 10_000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadPositions]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadSmart(), 45_000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadSmart]);

  useEffect(() => {
    if (!hasInitData || !selectedPoolAddress) return undefined;
    const timer = window.setInterval(() => loadKline(), 30_000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadKline, selectedPoolAddress]);

  useEffect(() => {
    if (!hotPools.length) return;
    setSelectedPool((prev) => {
      const prevAddr = normalizePoolAddress(prev?.pool_address || prev?.pool_id);
      if (!prevAddr) return { ...hotPools[0], chain };
      const matched = hotPools.find(
        (row) => normalizePoolAddress(row?.pool_address || row?.pool_id) === prevAddr
      );
      return matched ? { ...matched, chain } : prev;
    });
  }, [chain, hotPools]);

  const [loginCode, setLoginCode] = useState('');
  const [loginCodeExpiry, setLoginCodeExpiry] = useState(0);
  const pollRef = useRef(null);

  const handleLoginFromResp = useCallback((resp) => {
    const nextInitData = String(resp?.initData || '').trim();
    if (!nextInitData) throw new Error('后端未返回 initData');
    setInitData(nextInitData);
    setLoginUser(resp?.user || null);
    setLoginCode('');
  }, []);

  const startCodeLogin = useCallback(async () => {
    setLoginBusy(true);
    setLoginError('');
    setLoginCode('');
    try {
      const resp = await generateLoginCode({ apiBaseUrl });
      if (!resp?.code) throw new Error('生成验证码失败');
      setLoginCode(resp.code);
      setLoginCodeExpiry(Date.now() + (resp.expires_in || 300) * 1000);
    } catch (e) {
      setLoginError(String(e?.message || e));
    } finally {
      setLoginBusy(false);
    }
  }, [apiBaseUrl]);

  // Poll for code confirmation
  useEffect(() => {
    if (!loginCode || hasInitData) {
      if (pollRef.current) clearInterval(pollRef.current);
      return;
    }

    const poll = async () => {
      if (Date.now() > loginCodeExpiry) {
        setLoginCode('');
        setLoginError('验证码已过期，请重新获取');
        if (pollRef.current) clearInterval(pollRef.current);
        return;
      }
      try {
        const resp = await checkLoginCode({ apiBaseUrl, code: loginCode });
        if (resp?.ok && resp?.initData) {
          handleLoginFromResp(resp);
          if (pollRef.current) clearInterval(pollRef.current);
        } else if (resp?.status === 'expired') {
          setLoginCode('');
          setLoginError('验证码已过期，请重新获取');
          if (pollRef.current) clearInterval(pollRef.current);
        }
      } catch {
        // ignore poll errors
      }
    };

    pollRef.current = setInterval(poll, 2000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [apiBaseUrl, handleLoginFromResp, hasInitData, loginCode, loginCodeExpiry]);

  const logout = useCallback(() => {
    setInitData('');
    setLoginUser(null);
    storageRemove(STORAGE.initData);
    storageRemove(STORAGE.loginUser);
    setHotPools([]);
    setPositions(null);
    setSmart(null);
    setCandles([]);
  }, []);

  const refreshAll = useCallback(async () => {
    if (!hasInitData) return;
    setRefreshing(true);
    await Promise.allSettled([loadHotPools(), loadPositions(), loadSmart(), loadKline()]);
    setRefreshing(false);
  }, [hasInitData, loadHotPools, loadKline, loadPositions, loadSmart]);

  const toggleWidget = useCallback((key) => {
    setWidgets((prev) => {
      const exists = prev.includes(key);
      if (exists && prev.length === 1) return prev;
      if (exists) return prev.filter((x) => x !== key);
      return [...prev, key];
    });
  }, []);

  const [openPosPool, setOpenPosPool] = useState(null);
  const [openPosBusy, setOpenPosBusy] = useState(false);
  const [taskActionPos, setTaskActionPos] = useState(null);

  const handleOpenPosition = useCallback(async (params) => {
    setOpenPosBusy(true);
    try {
      await apiOpenPosition({ apiBaseUrl, initData, ...params });
      setOpenPosPool(null);
      loadPositions();
    } catch (e) {
      alert(String(e?.message || e));
    } finally {
      setOpenPosBusy(false);
    }
  }, [apiBaseUrl, initData, loadPositions]);

  const handleTaskPause = useCallback(async (taskId, paused) => {
    await setTaskPaused({ apiBaseUrl, initData, taskId, paused });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleTaskStop = useCallback(async (taskId) => {
    await stopTask({ apiBaseUrl, initData, taskId });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleTaskDelete = useCallback(async (taskId) => {
    await deleteTask({ apiBaseUrl, initData, taskId });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const handleTaskEditRange = useCallback(async (taskId, rl, ru, amt) => {
    await updateTaskRange({ apiBaseUrl, initData, taskId, rangeLowerPct: rl, rangeUpperPct: ru, amountUSDT: amt });
    loadPositions();
  }, [apiBaseUrl, initData, loadPositions]);

  const copyAddr = useCallback((addr) => {
    navigator.clipboard?.writeText(addr).catch(() => {});
  }, []);

  const summary = positions?.summary || {};
  const smartSummary = smart?.summary || {};

  const panelMap = {
    hot_pools: (
      <PanelShell
        title="热门池子"
        subtitle="支持搜索与排序"
        icon={Flame}
      >
        <div className="sort-tabs">
          {[{ key: 'fees', label: 'Fees' }, { key: 'fee_rate', label: 'Fee Rate' }, { key: 'volume', label: 'Volume' }].map((item) => (
            <button
              type="button"
              key={item.key}
              className={`sort-tab ${hotSort === item.key ? 'active' : ''}`}
              onClick={() => setHotSort(item.key)}
            >
              {item.label}
            </button>
          ))}
          <button type="button" className={`sort-tab search-toggle ${searchOpen ? 'active' : ''}`} onClick={() => setSearchOpen((v) => !v)}>
            <Search size={12} />
          </button>
        </div>

        {searchOpen && (
          <div className="search-row">
            <Search size={14} />
            <input value={keyword} onChange={(e) => setKeyword(e.target.value)} placeholder="搜索交易对/地址" autoFocus />
          </div>
        )}

        {hotPoolsError ? <div className="error-text">{hotPoolsError}</div> : null}

        <div className="data-list">
          {hotPoolsLoading && filteredHotPools.length === 0 ? (
            <EmptyState text="正在加载热门池子..." />
          ) : filteredHotPools.length === 0 ? (
            <EmptyState text="暂无可展示的池子数据" />
          ) : (
            filteredHotPools.slice(0, 60).map((pool, idx) => {
              const addr = normalizePoolAddress(pool?.pool_address || '');
              const selected = selectedPoolAddress && addr === selectedPoolAddress;
              const feePct = Number(pool?.fee_percentage || 0);
              const feeRate = Number(pool?.fee_rate || 0);
              const volume = Number(pool?.total_volume || 0);
              const totalFees = Number(pool?.total_fees || 0);
              const tvl = Number(pool?.current_pool_value || 0);
              const txCount = Number(pool?.transaction_count || 0);
              const priceDisplay = String(pool?.price_display || '');
              const factoryName = String(pool?.factory_name || pool?.dex || '');
              const userPosUsd = Number(pool?.userPositionUsd || 0);
              const pair = String(pool?.trading_pair || '--');
              const pairInitials = pair.split(/[\/\-]/).map((s) => s.trim().charAt(0).toUpperCase()).join('').slice(0, 2);
              const dex = getDexIcon(factoryName);

              return (
                <div
                  key={`${pool?.protocol_version || ''}:${addr || idx}`}
                  className={`pool-row ${selected ? 'selected' : ''}`}
                  onClick={() => selectPool({ ...pool, chain }, chain)}
                >
                  {/* Avatar */}
                  <div className="pool-avatar" style={dex ? { borderColor: dex.color + '60' } : undefined}>
                    {dex ? <img src={dex.src} alt="" /> : <span>{pairInitials}</span>}
                  </div>

                  {/* Info block */}
                  <div className="pool-info">
                    <div className="pool-name-line">
                      <span className="pool-name">{pair}</span>
                      <button type="button" className="copy-tiny" onClick={(e) => { e.stopPropagation(); copyAddr(addr); }} title="复制地址">
                        <svg viewBox="0 0 24 24" fill="currentColor" width="11" height="11"><path d="M16 1H4a2 2 0 00-2 2v14h2V3h12V1zm3 4H8a2 2 0 00-2 2v14a2 2 0 002 2h11a2 2 0 002-2V7a2 2 0 00-2-2zm0 16H8V7h11v14z"/></svg>
                      </button>
                      {feePct > 0 && <span className="tag tag-blue"><NumberFlowValue value={feePct} formatter={(v) => `${Number(v).toFixed(2).replace(/\.?0+$/, '')}%`} /></span>}
                      {dex?.label && <span className="tag tag-dex">{dex.label}</span>}
                      {userPosUsd > 0 && <span className="tag tag-purple">持仓</span>}
                    </div>
                    <div className="pool-meta-line">
                      <span className="meta-cyan">Vol <b><NumberFlowValue value={volume} formatter={(v) => formatUsdCompact(v)} /></b></span>
                      <span className="dot-sep" />
                      <span className="meta-cyan">TVL <b><NumberFlowValue value={tvl} formatter={(v) => formatUsdCompact(v)} /></b></span>
                      <span className="dot-sep" />
                      <span className="meta-orange"><NumberFlowValue value={txCount} formatter={(v) => `${Number(v || 0).toLocaleString()}笔`} /></span>
                      {feeRate > 0 && (<><span className="dot-sep" /><span className="meta-accent">Fee/TVL <b><NumberFlowValue value={feeRate} formatter={(v) => `${Number(v).toFixed(3)}%`} /></b></span></>)}
                    </div>
                  </div>

                  {/* Values block */}
                  <div className="pool-values">
                    <div className="pool-main-val">
                      <NumberFlowValue
                        value={hotSort === 'volume' ? volume : hotSort === 'fee_rate' ? feeRate : totalFees}
                        formatter={(v) => hotSort === 'fee_rate' ? (Number(v) > 0 ? `${Number(v).toFixed(3)}%` : '--') : formatUsdCompact(v)}
                      />
                    </div>
                    {priceDisplay ? (
                      <div className={`pool-sub-val ${priceDisplay.includes('↑') || priceDisplay.includes('+') ? 'up' : priceDisplay.includes('↓') || priceDisplay.includes('-') ? 'down' : ''}`} title={priceDisplay}>
                        <NumberFlowValue value={priceDisplay} formatter={() => formatPriceDisplay(priceDisplay)} />
                      </div>
                    ) : feeRate > 0 && hotSort !== 'fee_rate' ? (
                      <div className="pool-sub-val purple"><NumberFlowValue value={feeRate} formatter={(v) => `${Number(v).toFixed(3)}%`} /></div>
                    ) : null}
                  </div>

                  {/* Action */}
                  <button type="button" className="pool-buy-btn" onClick={(e) => { e.stopPropagation(); setOpenPosPool({ ...pool, chain }); }}>开仓</button>
                </div>
              );
            })
          )}
        </div>

        <div className="panel-footnote">
          更新时间: {hotPoolsUpdatedAt ? new Date(hotPoolsUpdatedAt).toLocaleTimeString() : '--'}
        </div>
      </PanelShell>
    ),

    gmgn_kline: (
      <PanelShell
        title="GMGN K线"
        subtitle={selectedPool?.trading_pair || '请选择池子'}
        icon={CandlestickChart}
        actions={
          <div className="inline-actions">
            {KLINE_PRESETS.map((x) => (
              <button
                type="button"
                key={x.key}
                className={`ghost-chip ${klinePresetKey === x.key ? 'active' : ''}`}
                onClick={() => setKlinePresetKey(x.key)}
              >
                {x.label}
              </button>
            ))}
            <button
              type="button"
              className="icon-link"
              disabled={!selectedPoolGmgnUrl}
              onClick={() => openExternal(selectedPoolGmgnUrl)}
            >
              <Link2 size={14} />
            </button>
          </div>
        }
      >
        {!selectedPoolAddress ? (
          <EmptyState text="点选池子后自动加载 K 线" />
        ) : (
          <>
            <div className="selected-pool-bar">
              <span>{shortAddress(selectedPoolAddress, 10, 8)}</span>
              <span>{String(selectedPool?.chain || chain).toUpperCase()}</span>
              <span>{klineLoading ? '加载中...' : '已连接'}</span>
            </div>

            {klineError ? <div className="error-text">{klineError}</div> : null}

            <div className="kline-wrap">
              {klineLoading && candles.length === 0 ? (
                <EmptyState text="正在加载 K 线..." />
              ) : candles.length === 0 ? (
                <EmptyState text="暂无 K 线数据" />
              ) : (
                <KlineChart candles={candles} />
              )}
            </div>

            <div className="kline-stats">
              <MetricCard label="Close" value={latestCandle ? formatNumber(latestCandle.c, 8) : '--'} />
              <MetricCard label="Volume" value={latestCandle ? formatNumber(latestCandle.v, 2) : '--'} />
              <MetricCard
                label="Updated"
                value={klineUpdatedAt ? new Date(klineUpdatedAt).toLocaleTimeString() : '--'}
              />
            </div>
          </>
        )}
      </PanelShell>
    ),

    positions: (
      <PanelShell
        title="仓位"
        subtitle={positions?.wallet?.address ? shortAddress(positions.wallet.address, 8, 6) : '钱包未连接'}
        icon={BriefcaseBusiness}
      >
        {positionsError ? <div className="error-text">{positionsError}</div> : null}

        <div className="summary-grid">
          <MetricCard label="总资产" value={formatUsd(summary?.total_usd)} tone="strong" />
          <MetricCard label="钱包" value={formatUsd(summary?.wallet_usd)} />
          <MetricCard label="仓位" value={formatUsd(summary?.position_usd)} />
          <MetricCard label="手续费" value={formatUsd(summary?.fee_usd)} />
        </div>

        <div className="data-list">
          {positionsLoading && sortedPositions.length === 0 ? (
            <EmptyState text="正在加载仓位..." />
          ) : sortedPositions.length === 0 ? (
            <EmptyState text="暂无仓位数据" />
          ) : (
            sortedPositions.slice(0, 50).map((p, idx) => {
              const taskId = Number(p?.task_id || 0);
              const statusLabel = String(p?.status_label || '运行中');
              const pnl = Number(p?.absolute_pnl_usd || 0);
              const hasPnl = Boolean(p?.has_pnl) || Number.isFinite(pnl) && pnl !== 0;
              const totalVal = Number(p?.current_value_usd || p?.totals?.total_usd || 0);
              const inRange = Boolean(p?.in_range);
              const token0 = p?.token_rows?.[0];
              const token1 = p?.token_rows?.[1];
              const taskRangeLo = Number(p?.task_range_lower_pct);
              const taskRangeUp = Number(p?.task_range_upper_pct);
              const taskAmount = Number(p?.task_amount_usdt);

              const statusClass = statusLabel.includes('错误') ? 'st-error' :
                statusLabel.includes('暂停') || statusLabel.includes('停止') || statusLabel.includes('撤出') ? 'st-warn' :
                statusLabel.includes('等待') ? 'st-wait' : 'st-ok';

              return (
                <div key={String(p?.position_id || idx)} className="pos-card">
                  <div className="pos-card-header">
                    <div className="pos-card-left"
                      onClick={() => selectPool({ pool_id: p?.pool_id, pool_address: p?.pool_id, trading_pair: p?.title, chain: p?.chain || chain }, p?.chain || chain)}>
                      <div className="pos-pair-row">
                        <span className="pos-pair-name">{p?.title || shortAddress(p?.pool_id || '')}</span>
                        {p?.tick_spacing && (
                          <span className="badge badge-fee">{
                            { 1: '0.01%', 10: '0.05%', 50: '0.25%', 60: '0.30%', 100: '0.50%', 200: '1%' }[Number(p.tick_spacing)] || ''
                          }</span>
                        )}
                      </div>
                      <div className="pos-status-row">
                        <span className={`status-pill ${statusClass}`}>
                          <span className="status-dot" />
                          {statusLabel}
                        </span>
                        {taskId > 0 && <span className="pos-task-id">#{taskId}</span>}
                        <span className={`range-pill ${inRange ? 'in' : 'out'}`}>{inRange ? 'In Range' : 'Out'}</span>
                      </div>
                    </div>
                    <div className="pos-card-right-block">
                      <div className="pos-total">{formatUsd(totalVal)}</div>
                      {hasPnl && (
                        <div className={`pos-pnl ${pnl >= 0 ? 'positive' : 'negative'}`}>
                          {pnl >= 0 ? '+' : ''}{formatNumber(pnl, 2)}
                        </div>
                      )}
                      {taskId > 0 && (
                        <button type="button" className="icon-btn-tiny" onClick={(e) => { e.stopPropagation(); setTaskActionPos(p); }} title="任务操作">
                          <svg viewBox="0 0 24 24" fill="currentColor" width="14" height="14"><path d="M12 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4zm0 7a2 2 0 110-4 2 2 0 010 4z"/></svg>
                        </button>
                      )}
                    </div>
                  </div>

                  {(token0 || token1) && (
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
                    </div>
                  )}

                  {Number.isFinite(taskRangeLo) && taskRangeLo > 0 && (
                    <div className="pos-range-info">
                      <span>范围: {Math.abs(taskRangeLo - taskRangeUp) < 0.01
                        ? `±${((taskRangeLo + taskRangeUp) / 2).toFixed(2)}%`
                        : `下 ${taskRangeLo.toFixed(2)}% / 上 ${taskRangeUp.toFixed(2)}%`}
                      </span>
                      {Number.isFinite(taskAmount) && taskAmount > 0 && <span> | ${taskAmount.toFixed(2)}</span>}
                    </div>
                  )}
                </div>
              );
            })
          )}
        </div>
      </PanelShell>
    ),

    smart_money: (
      <PanelShell title="聪明钱" subtitle="2h 池子 + 24h 钱包表现" icon={BrainCircuit}>
        {smartError ? <div className="error-text">{smartError}</div> : null}

        <div className="summary-grid">
          <MetricCard label="池子数" value={formatNumber(smartSummary?.pool_count || 0)} />
          <MetricCard label="钱包数" value={formatNumber(smartSummary?.wallet_count || 0)} />
          <MetricCard label="24h PnL" value={formatUsd(smartSummary?.total_pnl_usdt_24h)} tone="strong" />
          <MetricCard label="24h 事件" value={formatNumber(smartSummary?.total_events_24h || 0)} />
        </div>

        <div className="split-list">
          <div className="split-col">
            <div className="list-title">池子热度</div>
            <div className="data-list compact">
              {smartLoading && smartPools.length === 0 ? (
                <EmptyState text="正在加载聪明钱池子..." />
              ) : smartPools.length === 0 ? (
                <EmptyState text="暂无池子数据" />
              ) : (
                smartPools.slice(0, 24).map((pool, idx) => (
                  <div
                    key={`${pool?.pool_version || ''}:${pool?.pool_id || idx}`}
                    className="data-row compact clickable"
                    onClick={() =>
                      selectPool(
                        {
                          pool_id: pool?.pool_id,
                          pool_address: pool?.pool_id,
                          trading_pair: pool?.pair,
                          chain,
                        },
                        chain
                      )
                    }
                  >
                    <div className="row-main">
                      <div className="row-title">{pool?.pair || shortAddress(pool?.pool_id || '')}</div>
                      <div className="row-subtitle">{shortAddress(pool?.pool_id || '')}</div>
                    </div>
                    <div className="row-metrics">
                      <span>{formatNumber(pool?.wallet_count || 0)}</span>
                      <span>{formatUsdCompact(pool?.added_liquidity || 0)}</span>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="split-col">
            <div className="list-title">钱包榜单</div>
            <div className="data-list compact">
              {smartWallets.length === 0 ? (
                <EmptyState text="暂无钱包榜单数据" />
              ) : (
                smartWallets.slice(0, 20).map((wallet, idx) => {
                  const pnl = Number(wallet?.pnl_usdt_24h || 0);
                  return (
                    <div key={wallet?.wallet_address || idx} className="data-row compact static">
                      <div className="row-main">
                        <div className="row-title">{shortAddress(wallet?.wallet_address || '', 8, 6)}</div>
                        <div className="row-subtitle">事件 {formatNumber(wallet?.event_count_24h || 0)}</div>
                      </div>
                      <div className={pnl >= 0 ? 'wallet-pnl positive' : 'wallet-pnl negative'}>
                        {formatUsdCompact(pnl)}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>
      </PanelShell>
    ),
  };

  return (
    <div className="app-shell">
      <div className="bg-orb orb-a" />
      <div className="bg-orb orb-b" />
      <div className="bg-grid" />

      <header className="top-bar">
        <div className="title-block">
          <div className="eyebrow">TGLPBOT WEB TERMINAL</div>
          <h1>多模块交易工作台</h1>
          <p>点击右上角 Telegram 图标获取验证码，在 Bot 中发送即可登录。模块支持拖拽重排。</p>
        </div>

        <div className="top-actions">
          <button type="button" className="primary-btn" onClick={refreshAll} disabled={refreshing || !hasInitData}>
            <RefreshCw size={14} className={refreshing ? 'spin' : ''} />
            刷新数据
          </button>

          {loginUser ? (
            <div className="user-chip">
              {loginUser?.photo_url ? (
                <img src={loginUser.photo_url} alt="avatar" className="user-avatar" />
              ) : (
                <div className="user-avatar fallback">{String(loginUser?.first_name || '?').slice(0, 1)}</div>
              )}
              <div className="user-meta">
                <div className="user-name">{loginUser?.first_name || 'Telegram User'}</div>
                <div className="user-sub">@{loginUser?.username || 'unknown'}</div>
              </div>
              <button type="button" className="logout-btn" onClick={logout}>
                <LogOut size={13} />
                退出
              </button>
            </div>
          ) : loginCode ? (
            <div className="login-code-box">
              <div className="login-code-label">验证码</div>
              <div className="login-code-value">{loginCode}</div>
              <div className="login-code-hint">
                在 Telegram 中发送: <code>/weblogin {loginCode}</code>
              </div>
              <button type="button" className="ghost-chip" onClick={() => { setLoginCode(''); setLoginError(''); }}>
                取消
              </button>
            </div>
          ) : (
            <button
              type="button"
              className="telegram-icon-btn"
              onClick={startCodeLogin}
              disabled={loginBusy}
              title="获取登录验证码"
              aria-label="获取登录验证码"
            >
              <img src={telegramLogo} alt="Telegram" />
            </button>
          )}
        </div>
      </header>

      {loginError ? <div className="error-text top-error">{loginError}</div> : null}

      <section className="config-panel">
        <div className="config-head">
          <SlidersHorizontal size={14} />
          <span>布局与链设置</span>
        </div>

        <div className="chain-toggles">
          <button type="button" className={`chain-btn ${chain === 'bsc' ? 'active' : ''}`} onClick={() => setChain('bsc')}>
            <img src={bnbLogo} alt="BSC" className="chain-icon" />
            <span>BSC</span>
          </button>
          <button type="button" className={`chain-btn ${chain === 'base' ? 'active' : ''}`} onClick={() => setChain('base')}>
            <img src={baseLogo} alt="Base" className="chain-icon" />
            <span>Base</span>
          </button>
        </div>

        <div className="widget-toggles">
          {WIDGETS.map((item) => (
            <button
              type="button"
              key={item.key}
              className={`toggle-chip ${widgets.includes(item.key) ? 'active' : ''}`}
              onClick={() => toggleWidget(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>

        {!hasInitData ? (
          <div className="warning-box">
            <AlertTriangle size={14} />
            <span>请点击右上角 Telegram 图标获取验证码，在 Bot 中发送 /weblogin 验证码 完成登录。</span>
          </div>
        ) : null}
      </section>

      <main className={`workbench ${layoutClass}`}>
        {activeWidgets.map((widget) => (
          <div
            key={widget.key}
            className={`module-slot module-${widget.key} ${
              draggingKey === widget.key ? 'dragging' : ''
            } ${dragOverKey === widget.key ? 'drop-target' : ''}`}
            draggable
            onDragStart={(e) => {
              setDraggingKey(widget.key);
              e.dataTransfer.effectAllowed = 'move';
              e.dataTransfer.setData('text/plain', widget.key);
            }}
            onDragOver={(e) => {
              e.preventDefault();
              e.dataTransfer.dropEffect = 'move';
              if (draggingKey && draggingKey !== widget.key) {
                setDragOverKey(widget.key);
              }
            }}
            onDrop={(e) => {
              e.preventDefault();
              const from = e.dataTransfer.getData('text/plain') || draggingKey;
              if (from && from !== widget.key) {
                setWidgets((prev) => reorderList(prev, from, widget.key));
              }
              setDraggingKey('');
              setDragOverKey('');
            }}
            onDragEnd={() => {
              setDraggingKey('');
              setDragOverKey('');
            }}
          >
            <div className="drag-hint">
              <GripVertical size={12} />
            </div>
            {panelMap[widget.key]}
          </div>
        ))}
      </main>

      {openPosPool && (
        <OpenPositionModal
          pool={openPosPool}
          chain={openPosPool?.chain || chain}
          onSubmit={handleOpenPosition}
          onClose={() => setOpenPosPool(null)}
          busy={openPosBusy}
        />
      )}

      {taskActionPos && (
        <TaskActionMenu
          position={taskActionPos}
          onPause={handleTaskPause}
          onStop={handleTaskStop}
          onDelete={handleTaskDelete}
          onEditRange={handleTaskEditRange}
          onClose={() => setTaskActionPos(null)}
        />
      )}
    </div>
  );
}
