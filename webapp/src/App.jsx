import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertTriangle,
  BrainCircuit,
  BriefcaseBusiness,
  CandlestickChart,
  Check,
  Copy,
  Flame,
  GripVertical,
  Link2,
  LogOut,
  Maximize,
  Minimize,
  Search,
  Settings,
  SlidersHorizontal,
} from 'lucide-react';
import {
  checkLoginCode,
  deleteTask,
  fetchHotPools,
  fetchRealtimePositions,
  fetchSmartMoneyOverview,
  fetchSmartMoneyPoolAdds,
  generateLoginCode,
  openPosition as apiOpenPosition,
  setTaskPaused,
  stopTask,
  updateTaskRange,
} from './api';
import { WEBAPP_CONFIG } from './config';
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
  compactPrice,
  formatNumber,
  formatPct,
  formatPriceDisplay,
  formatUsd,
  formatUsdCompact,
  normalizePoolAddress,
  normalizeWidgetSelection,
  pickNonStableTokenAddress,
  shortAddress,
} from './utils';

const STORAGE = {
  initData: 'tglp_web_init_data',
  loginUser: 'tglp_web_login_user',
  chain: 'tglp_web_chain',
  widgets: 'tglp_web_widgets',
  sort: 'tglp_web_hot_pools_sort',
  refreshInterval: 'tglp_web_refresh_interval',
};

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

function buildDexScreenerEmbedUrl(pool, chainName) {
  if (!pool) return '';
  const c = String(pool?.chain || chainName || 'bsc').toLowerCase() === 'base' ? 'base' : 'bsc';
  const factory = String(pool?.factory_name || '').toLowerCase();
  const isV4 = factory.includes('v4');
  // V4 pools: DEXScreener doesn't recognise pool ID, use non-stable token address instead
  const addr = isV4
    ? pickNonStableTokenAddress(pool)
    : normalizePoolAddress(pool?.pool_address || pool?.pool_id);
  if (!addr) return '';
  return `https://dexscreener.com/${c}/${addr}?embed=1&theme=dark&trades=1&info=0&interval=1&chartType=price`;
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

  const [refreshing, setRefreshing] = useState(false);
  const [loginBusy, setLoginBusy] = useState(false);
  const [loginError, setLoginError] = useState('');
  const [workMode, setWorkMode] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [refreshInterval, setRefreshInterval] = useState(() => {
    const saved = Number(storageGet(STORAGE.refreshInterval));
    return saved >= 10 ? saved : 10;
  });
  const [draggingKey, setDraggingKey] = useState('');
  const [dragOverKey, setDragOverKey] = useState('');

  const hasInitData = Boolean(initData);
  const activeWidgets = useMemo(() => {
    const map = Object.fromEntries(WIDGETS.map((w) => [w.key, w]));
    return widgets.map((k) => map[k]).filter(Boolean);
  }, [widgets]);
  const layoutClass = moduleLayoutClass(activeWidgets.length);
  const workLayoutClass = workMode ? `work-mode layout-work-${Math.min(activeWidgets.length, 4)}` : layoutClass;

  const selectedPoolAddress = useMemo(
    () => normalizePoolAddress(selectedPool?.pool_address || selectedPool?.pool_id),
    [selectedPool]
  );
  const selectedPoolGmgnUrl = useMemo(() => buildGmgnUrl(selectedPool, chain), [selectedPool, chain]);
  const selectedPoolEmbedUrl = useMemo(
    () => buildDexScreenerEmbedUrl(selectedPool, chain),
    [selectedPool, chain]
  );

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
    storageSet(STORAGE.refreshInterval, String(refreshInterval));

    if (loginUser) {
      storageSet(STORAGE.loginUser, JSON.stringify(loginUser));
    } else {
      storageRemove(STORAGE.loginUser);
    }
  }, [chain, hotSort, initData, loginUser, refreshInterval, widgets]);

  useEffect(() => {
    if (!workMode) return;
    const handler = (e) => { if (e.key === 'Escape') setWorkMode(false); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [workMode]);

  useEffect(() => {
    if (!showSettings) return;
    const handler = (e) => {
      if (!e.target.closest('.settings-wrap')) setShowSettings(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [showSettings]);

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

  useEffect(() => {
    const ctrl = new AbortController();
    loadHotPools(ctrl.signal);
    loadPositions(ctrl.signal);
    loadSmart(ctrl.signal);
    return () => ctrl.abort();
  }, [loadHotPools, loadPositions, loadSmart]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadHotPools(), refreshInterval * 1000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadHotPools, refreshInterval]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadPositions(), refreshInterval * 1000);
    return () => window.clearInterval(timer);
  }, [hasInitData, loadPositions, refreshInterval]);

  useEffect(() => {
    if (!hasInitData) return undefined;
    const timer = window.setInterval(() => loadSmart(), Math.max(refreshInterval * 1000, 30_000));
    return () => window.clearInterval(timer);
  }, [hasInitData, loadSmart, refreshInterval]);

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
  }, []);

  const refreshAll = useCallback(async () => {
    if (!hasInitData) return;
    setRefreshing(true);
    await Promise.allSettled([loadHotPools(), loadPositions(), loadSmart()]);
    setRefreshing(false);
  }, [hasInitData, loadHotPools, loadPositions, loadSmart]);

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

  // Pool adds preview data: { "v3:0x1234": { status, wallets, totalUsd, error } }
  const [poolAddsMap, setPoolAddsMap] = useState({});

  // Auto-load pool adds for top pools when smart data changes
  useEffect(() => {
    if (!smartPools.length || !hasInitData) return;
    const ctrl = new AbortController();
    const toLoad = smartPools.slice(0, 12);
    toLoad.forEach((pool) => {
      const key = `${pool?.pool_version || ''}:${pool?.pool_id || ''}`;
      // skip if already loaded or loading
      if (poolAddsMap[key]?.status === 'success' || poolAddsMap[key]?.status === 'loading') return;
      setPoolAddsMap((prev) => ({ ...prev, [key]: { status: 'loading', wallets: [], totalUsd: 0, error: '' } }));
      fetchSmartMoneyPoolAdds({
        apiBaseUrl,
        initData,
        chain,
        poolVersion: pool?.pool_version,
        poolId: pool?.pool_id,
        windowHours: 2,
        limit: 20,
        signal: ctrl.signal,
      })
        .then((res) => {
          const wallets = Array.isArray(res?.wallets) ? res.wallets : [];
          const totalUsd = wallets.reduce((s, w) => s + Number(w?.total_usd || 0), 0);
          setPoolAddsMap((prev) => ({
            ...prev,
            [key]: { status: 'success', wallets, totalUsd, error: '' },
          }));
        })
        .catch((e) => {
          if (e?.name === 'AbortError') return;
          setPoolAddsMap((prev) => ({
            ...prev,
            [key]: { status: 'error', wallets: [], totalUsd: 0, error: String(e?.message || e) },
          }));
        });
    });
    return () => ctrl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [smartPools, hasInitData, apiBaseUrl, initData, chain]);

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
        title="K线行情"
        subtitle={selectedPool?.trading_pair || '请选择池子'}
        icon={CandlestickChart}
      >
        {!selectedPoolAddress ? (
          <EmptyState text="点选池子后自动加载 K 线" />
        ) : (
          <>
            <div className="dex-embed-wrap">
              <iframe
                key={selectedPoolEmbedUrl}
                src={selectedPoolEmbedUrl}
                className="dex-embed-iframe"
                title="DEXScreener Chart"
                sandbox="allow-scripts allow-same-origin allow-popups"
                referrerPolicy="no-referrer"
              />
            </div>

            <div className="kline-ext-links">
              {selectedPoolGmgnUrl && (
                <button type="button" className="kline-ext-btn gmgn" onClick={() => openExternal(selectedPoolGmgnUrl)}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" width="13" height="13"><path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
                  GMGN 查看 KOL·活动·交易者
                </button>
              )}
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
      <PanelShell
        title="聪明钱"
        subtitle={`${smartPools.length} 个池子被监控钱包加 LP`}
        icon={BrainCircuit}
      >
        {smartError ? <div className="error-text">{smartError}</div> : null}

        <div className="data-list compact">
          {smartLoading && smartPools.length === 0 ? (
            <EmptyState text="正在加载聪明钱数据..." />
          ) : smartPools.length === 0 ? (
            <EmptyState text="暂无监控钱包加 LP 数据" />
          ) : (
            smartPools.slice(0, 20).map((pool, idx) => {
              const poolKey = `${pool?.pool_version || ''}:${pool?.pool_id || ''}`;
              const adds = poolAddsMap[poolKey];
              const wallets = adds?.wallets || [];
              const totalUsd = adds?.totalUsd || Number(pool?.added_liquidity || 0);
              const version = String(pool?.pool_version || '').toUpperCase();
              const feePct = Number(pool?.fee_pct || 0);

              return (
                <div
                  key={poolKey || idx}
                  className="sm-pool-card"
                  onClick={() =>
                    selectPool(
                      {
                        pool_id: pool?.pool_id,
                        pool_address: pool?.pool_id,
                        trading_pair: pool?.pair,
                        factory_name: pool?.factory_name,
                        chain,
                      },
                      chain
                    )
                  }
                >
                  <div className="sm-pool-header">
                    <div className="sm-pool-left">
                      <span className="sm-rank">#{idx + 1}</span>
                      <span className="sm-pair">{pool?.pair || shortAddress(pool?.pool_id || '')}</span>
                      {version ? <span className="tag">{version}</span> : null}
                      {feePct > 0 ? <span className="tag tag-blue">{formatPct(feePct)}</span> : null}
                    </div>
                    <div className="sm-pool-right">
                      {totalUsd > 0 ? (
                        <span className="sm-total-usd">${formatNumber(Math.round(totalUsd))}</span>
                      ) : null}
                      <span className="sm-wallet-count">
                        {formatNumber(pool?.wallet_count || 0)} 钱包
                      </span>
                    </div>
                  </div>

                  {adds?.status === 'loading' && wallets.length === 0 ? (
                    <div className="sm-wallet-loading">加载钱包明细...</div>
                  ) : null}

                  {wallets.length > 0 ? (
                    <div className="sm-wallet-list">
                      {wallets.slice(0, 5).map((w, wi) => {
                        const addr = String(w?.wallet_address || '').trim();
                        const usd = Number(w?.total_usd ?? 0);
                        const priceLower = Number(w?.price_lower ?? 0);
                        const priceUpper = Number(w?.price_upper ?? 0);
                        const quote = String(w?.price_quote || '').trim();
                        const hasRange =
                          Number.isFinite(priceLower) &&
                          priceLower > 0 &&
                          Number.isFinite(priceUpper) &&
                          priceUpper > 0;
                        const rangePct = hasRange
                          ? (Math.abs(priceUpper - priceLower) / ((priceUpper + priceLower) / 2)) * 100
                          : 0;

                        return (
                          <div key={addr || wi} className="sm-wallet-row">
                            <div className="sm-wallet-addr">{shortAddress(addr, 6, 4)}</div>
                            <div className="sm-wallet-stats">
                              <span className="sm-wallet-usd">${formatNumber(Math.round(usd))}</span>
                              {hasRange ? (
                                <span className="sm-wallet-range-pct">{formatPct(rangePct, 1)}</span>
                              ) : null}
                            </div>
                            {hasRange ? (
                              <div className="sm-wallet-range">
                                <span className="sm-range-label">区间</span>
                                <span className="sm-range-val">{compactPrice(priceLower)}</span>
                                <span className="sm-range-arrow">&rarr;</span>
                                <span className="sm-range-val">{compactPrice(priceUpper)}</span>
                                {quote ? <span className="sm-range-quote">{quote}</span> : null}
                              </div>
                            ) : null}
                          </div>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              );
            })
          )}
        </div>
      </PanelShell>
    ),
  };

  return (
    <div className={`app-shell ${workMode ? 'work-mode-shell' : ''}`}>
      <div className="bg-orb orb-a" />
      <div className="bg-orb orb-b" />
      <div className="bg-grid" />

      {workMode ? (
        <div className="work-mode-bar">
          <button type="button" className="work-mode-exit-btn" onClick={() => setWorkMode(false)}>
            <Minimize size={14} />
            退出工作模式
          </button>
        </div>
      ) : (
        <>
          <header className="top-bar">
            <div className="title-block">
              <div className="eyebrow">lynchL</div>
              <h1>LP交易工作台</h1>
            </div>

            <div className="top-actions">
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
              <div className="settings-wrap">
                <button type="button" className="settings-btn" onClick={() => setShowSettings((v) => !v)}>
                  <Settings size={15} />
                </button>
                {showSettings && (
                  <div className="settings-popover">
                    <div className="settings-row">
                      <span className="settings-label">数据刷新间隔</span>
                      <div className="settings-input-wrap">
                        <input
                          type="number"
                          className="settings-input"
                          min={10}
                          value={refreshInterval}
                          onChange={(e) => {
                            const v = Math.max(10, Math.round(Number(e.target.value) || 10));
                            setRefreshInterval(v);
                          }}
                        />
                        <span className="settings-unit">秒</span>
                      </div>
                    </div>
                    <div className="settings-hint">最低 10 秒，当前每 {refreshInterval} 秒刷新</div>
                  </div>
                )}
              </div>
              <button type="button" className="logout-btn" onClick={logout}>
                <LogOut size={13} />
                退出
              </button>
            </div>
          ) : loginCode ? (
            <div className="login-code-box">
              <div className="login-code-top">
                <div className="login-code-badge">验证码</div>
                <div className="login-code-value">{loginCode}</div>
              </div>
              <div className="login-code-cmd-row">
                <code className="login-code-cmd">/weblogin {loginCode}</code>
                <button
                  type="button"
                  className="login-copy-btn"
                  onClick={(e) => {
                    navigator.clipboard.writeText(`/weblogin ${loginCode}`);
                    const btn = e.currentTarget;
                    btn.classList.add('copied');
                    setTimeout(() => btn.classList.remove('copied'), 1500);
                  }}
                >
                  <Copy size={12} className="copy-icon" />
                  <Check size={12} className="check-icon" />
                </button>
              </div>
              <div className="login-code-hint">在 Telegram Bot 中发送上方指令完成登录</div>
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
          <button type="button" className="work-mode-btn" onClick={() => setWorkMode(true)}>
            <Maximize size={13} />
            工作模式
          </button>
        </div>

        {!hasInitData ? (
          <div className="warning-box">
            <AlertTriangle size={14} />
            <span>请点击右上角 Telegram 图标获取验证码，在 Bot 中发送 /weblogin 验证码 完成登录。</span>
          </div>
        ) : null}
      </section>
      </>
      )}

      <main className={`workbench ${workLayoutClass}`}>
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
