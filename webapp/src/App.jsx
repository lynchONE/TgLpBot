import React, { useCallback, useEffect, useMemo, useState } from 'react';
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
  exchangeTelegramLogin,
  fetchHotPools,
  fetchPoolOHLCV,
  fetchRealtimePositions,
  fetchSmartMoneyOverview,
} from './api';
import { WEBAPP_CONFIG } from './config';
import KlineChart from './components/KlineChart';
import PanelShell, { EmptyState, MetricCard } from './components/PanelShell';
import telegramLogo from './img/telegram.svg';
import {
  DEFAULT_WIDGETS,
  WIDGETS,
  buildGmgnUrl,
  formatNumber,
  formatPct,
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

let telegramScriptPromise = null;

function ensureTelegramScript() {
  if (typeof window === 'undefined') {
    return Promise.reject(new Error('浏览器环境不可用'));
  }
  if (window.Telegram?.Login?.auth) return Promise.resolve();
  if (telegramScriptPromise) return telegramScriptPromise;

  telegramScriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector('script[data-tg-login-script="1"]');
    if (existing) {
      existing.addEventListener('load', () => resolve(), { once: true });
      existing.addEventListener('error', () => reject(new Error('加载 Telegram 登录脚本失败')), {
        once: true,
      });
      return;
    }

    const script = document.createElement('script');
    script.async = true;
    script.src = 'https://telegram.org/js/telegram-widget.js?22';
    script.dataset.tgLoginScript = '1';
    script.onload = () => resolve();
    script.onerror = () => reject(new Error('加载 Telegram 登录脚本失败'));
    document.body.appendChild(script);
  });

  return telegramScriptPromise;
}

function telegramAuthPopup(botId) {
  return ensureTelegramScript().then(
    () =>
      new Promise((resolve, reject) => {
        if (!window.Telegram?.Login?.auth) {
          reject(new Error('当前环境不支持 Telegram 登录，请刷新页面重试'));
          return;
        }

        window.Telegram.Login.auth(
          {
            bot_id: String(botId),
            request_access: true,
          },
          (data) => {
            if (!data) {
              reject(new Error('已取消登录或二维码未确认'));
              return;
            }
            resolve(data);
          }
        );
      })
  );
}

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

  const handleLoginAuth = useCallback(
    async (authUser) => {
      const resp = await exchangeTelegramLogin({
        apiBaseUrl,
        id: authUser?.id,
        first_name: authUser?.first_name,
        last_name: authUser?.last_name,
        username: authUser?.username,
        photo_url: authUser?.photo_url,
        auth_date: authUser?.auth_date,
        hash: authUser?.hash,
      });
      const nextInitData = String(resp?.initData || '').trim();
      if (!nextInitData) throw new Error('后端未返回 initData');
      setInitData(nextInitData);
      setLoginUser(resp?.user || null);
    },
    [apiBaseUrl]
  );

  const startTelegramLogin = useCallback(async () => {
    const botId = String(WEBAPP_CONFIG.telegramBotId || '').trim();
    if (!botId) {
      setLoginError('缺少 VITE_TELEGRAM_BOT_ID 配置');
      return;
    }

    setLoginBusy(true);
    setLoginError('');
    try {
      const authUser = await telegramAuthPopup(botId);
      await handleLoginAuth(authUser);
    } catch (e) {
      setLoginError(String(e?.message || e));
    } finally {
      setLoginBusy(false);
    }
  }, [handleLoginAuth]);

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

  const summary = positions?.summary || {};
  const smartSummary = smart?.summary || {};

  const panelMap = {
    hot_pools: (
      <PanelShell
        title="热门池子"
        subtitle="支持搜索与排序"
        icon={Flame}
        actions={
          <select
            value={hotSort}
            onChange={(e) => setHotSort(e.target.value)}
            className="surface-input slim"
          >
            <option value="fees">Fees</option>
            <option value="fee_rate">Fee Rate</option>
            <option value="volume">Volume</option>
          </select>
        }
      >
        <div className="search-row">
          <Search size={14} />
          <input value={keyword} onChange={(e) => setKeyword(e.target.value)} placeholder="搜索交易对/地址" />
        </div>

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
              const gmgn = buildGmgnUrl({ ...pool, chain }, chain);

              return (
                <div
                  key={`${pool?.protocol_version || ''}:${addr || idx}`}
                  className={`data-row clickable ${selected ? 'selected' : ''}`}
                  onClick={() => selectPool({ ...pool, chain }, chain)}
                >
                  <div className="row-main">
                    <div className="row-title">{pool?.trading_pair || '--'}</div>
                    <div className="row-subtitle">{shortAddress(addr || pool?.pool_address || '')}</div>
                  </div>
                  <div className="row-metrics">
                    <span>{formatUsdCompact(pool?.total_fees_24h ?? pool?.total_fees)}</span>
                    <span>{formatUsdCompact(pool?.total_volume_24h ?? pool?.total_volume)}</span>
                    <span>{formatPct(pool?.fee_rate, 3)}</span>
                  </div>
                  <button
                    type="button"
                    className="mini-link"
                    disabled={!gmgn}
                    onClick={(e) => {
                      e.stopPropagation();
                      openExternal(gmgn);
                    }}
                  >
                    GMGN
                  </button>
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
            sortedPositions.slice(0, 50).map((p, idx) => (
              <div
                key={String(p?.position_id || idx)}
                className="data-row clickable"
                onClick={() =>
                  selectPool(
                    {
                      pool_id: p?.pool_id,
                      pool_address: p?.pool_id,
                      trading_pair: p?.title,
                      chain: p?.chain || chain,
                    },
                    p?.chain || chain
                  )
                }
              >
                <div className="row-main">
                  <div className="row-title">{p?.title || shortAddress(p?.pool_id || '')}</div>
                  <div className="row-subtitle">
                    {String(p?.version || '').toUpperCase()} · {p?.in_range ? 'In Range' : 'Out of Range'}
                  </div>
                </div>
                <div className="row-metrics">
                  <span>{formatUsdCompact(p?.totals?.total_usd || 0)}</span>
                  <span className={Number(p?.absolute_pnl_usd || 0) >= 0 ? 'pnl-positive' : 'pnl-negative'}>
                    {formatUsdCompact(p?.absolute_pnl_usd || 0)}
                  </span>
                </div>
              </div>
            ))
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
          <p>右上角 Telegram 图标点击后唤起二维码登录。模块支持拖拽重排。</p>
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
          ) : (
            <button
              type="button"
              className="telegram-icon-btn"
              onClick={startTelegramLogin}
              disabled={loginBusy}
              title="Telegram 扫码登录"
              aria-label="Telegram 扫码登录"
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

        <div className="config-grid compact">
          <label className="field small">
            <span>Chain</span>
            <select className="surface-input" value={chain} onChange={(e) => setChain(e.target.value)}>
              <option value="bsc">BSC</option>
              <option value="base">Base</option>
            </select>
          </label>
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
            <span>请点击右上角 Telegram 图标，扫码完成登录后再查看数据。</span>
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
              e.dataTransfer.setData('text/plain', widget.key);
            }}
            onDragOver={(e) => {
              if (!draggingKey || draggingKey === widget.key) return;
              e.preventDefault();
              setDragOverKey(widget.key);
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
              <span>拖拽重排</span>
            </div>
            {panelMap[widget.key]}
          </div>
        ))}
      </main>
    </div>
  );
}
