import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { BrainCircuit, Factory, Flame, RefreshCw } from 'lucide-react';
import {
  executeCreatePool,
  fetchHotPools,
  fetchSmartMoneyOverview,
  fetchWallets,
  previewCreatePool,
} from '../api';
import PanelShell, { EmptyState } from './PanelShell';
import { normalizeHexAddress, shortAddress } from '../utils';

const PROTOCOL_OPTIONS = [
  { key: 'univ3', label: 'Uniswap V3' },
  { key: 'univ4', label: 'Uniswap V4' },
  { key: 'pcsv3', label: 'Pancake V3' },
];

const MODE_OPTIONS = [
  { key: 'create_and_seed', label: '建池+首注' },
  { key: 'create_only', label: '仅建池' },
];

const SOURCE_TABS = [
  { key: 'manual', label: '手动' },
  { key: 'hot', label: '热门池子' },
  { key: 'smart', label: '聪明钱' },
];

const STABLE_SYMBOLS = new Set(['USDT', 'USDC', 'BUSD', 'DAI', 'FDUSD', 'USD']);
const PRICE_RE = /^1\s+([A-Za-z0-9._-]+)\s*=\s*([0-9.]+)\s*(USD|USDT|USDC|BUSD)$/i;

function parsePairSymbols(pair) {
  const parts = String(pair || '')
    .split('/')
    .map((item) => String(item || '').trim())
    .filter(Boolean);
  return [parts[0] || '', parts[1] || ''];
}

function normalizeSymbol(value) {
  return String(value || '').trim().toUpperCase();
}

function isStableSymbol(symbol) {
  return STABLE_SYMBOLS.has(normalizeSymbol(symbol));
}

function feeTierFromPct(value) {
  const n = Number(value ?? 0);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * 10000);
}

function supportedFeeTiers(protocol) {
  switch (String(protocol || '').trim()) {
    case 'pcsv3':
      return [100, 500, 2500, 10000];
    case 'univ4':
    case 'univ3':
    default:
      return [100, 500, 3000, 10000];
  }
}

function supportsFeeTier(protocol, feeTier) {
  return supportedFeeTiers(protocol).includes(Number(feeTier || 0));
}

function defaultFeeTier(protocol) {
  return supportedFeeTiers(protocol)[1] || supportedFeeTiers(protocol)[0] || 500;
}

function formatFeeTier(feeTier) {
  const n = Number(feeTier || 0);
  if (!Number.isFinite(n) || n <= 0) return '--';
  return `${(n / 10000).toFixed(n >= 10000 ? 2 : 2).replace(/\.?0+$/, '')}%`;
}

function buildSourcePricePrefill(token0Symbol, token1Symbol, priceDisplay) {
  const match = String(priceDisplay || '').trim().match(PRICE_RE);
  if (!match) return '';
  const quotedSymbol = normalizeSymbol(match[1]);
  const priceValue = Number(match[2]);
  if (!Number.isFinite(priceValue) || priceValue <= 0) return '';

  const sym0 = normalizeSymbol(token0Symbol);
  const sym1 = normalizeSymbol(token1Symbol);
  const token0Stable = isStableSymbol(sym0);
  const token1Stable = isStableSymbol(sym1);

  if (quotedSymbol === sym0 && token1Stable) return String(priceValue);
  if (quotedSymbol === sym1 && token0Stable) return String(1 / priceValue);
  return '';
}

function buildHotSources(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map((row, index) => {
      const token0Address = normalizeHexAddress(row?.token0_address);
      const token1Address = normalizeHexAddress(row?.token1_address);
      if (!token0Address || !token1Address) return null;
      const [pair0, pair1] = parsePairSymbols(row?.trading_pair);
      const token0Symbol = String(row?.token0_symbol || pair0 || '').trim();
      const token1Symbol = String(row?.token1_symbol || pair1 || '').trim();
      const feeTier = feeTierFromPct(row?.fee_percentage);
      return {
        key: `hot:${normalizeHexAddress(row?.pool_address) || index}`,
        label: String(row?.trading_pair || `${token0Symbol}/${token1Symbol}` || '--').trim(),
        subtitle: String(row?.factory_name || row?.dex || '热门池子').trim(),
        token0Address,
        token1Address,
        token0Symbol,
        token1Symbol,
        feeTier,
        priceHint: String(row?.price_display || '').trim(),
        pricePrefill: buildSourcePricePrefill(token0Symbol, token1Symbol, row?.price_display),
      };
    })
    .filter(Boolean)
    .slice(0, 10);
}

function buildSmartSources(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map((row, index) => {
      const token0Address = normalizeHexAddress(row?.token0);
      const token1Address = normalizeHexAddress(row?.token1);
      if (!token0Address || !token1Address) return null;
      const [pair0, pair1] = parsePairSymbols(row?.pair);
      return {
        key: `smart:${normalizeHexAddress(row?.pool_id) || index}`,
        label: String(row?.pair || '--').trim(),
        subtitle: `${String(row?.exchange || 'Smart Money').trim()} ${String(row?.pool_version || '').trim().toUpperCase()}`.trim(),
        token0Address,
        token1Address,
        token0Symbol: String(row?.token0_symbol || pair0 || '').trim(),
        token1Symbol: String(row?.token1_symbol || pair1 || '').trim(),
        feeTier: feeTierFromPct(row?.fee_pct),
        priceHint: '可作为币对与费率参考来源',
        pricePrefill: '',
      };
    })
    .filter(Boolean)
    .slice(0, 10);
}

function fieldErrorText(resp) {
  if (!resp) return '';
  if (Array.isArray(resp?.warnings) && resp.warnings.length > 0) return String(resp.warnings[0]);
  return '';
}

export default function CreatePoolPanel({ apiBaseUrl, initData, hasInitData }) {
  const [wallets, setWallets] = useState([]);
  const [walletsLoading, setWalletsLoading] = useState(false);
  const [walletsError, setWalletsError] = useState('');
  const [hotSources, setHotSources] = useState([]);
  const [smartSources, setSmartSources] = useState([]);
  const [sourcesLoading, setSourcesLoading] = useState(false);
  const [sourcesError, setSourcesError] = useState('');

  const [protocol, setProtocol] = useState('univ3');
  const [mode, setMode] = useState('create_and_seed');
  const [walletId, setWalletId] = useState(0);
  const [sourceTab, setSourceTab] = useState('manual');
  const [selectedSourceKey, setSelectedSourceKey] = useState('');
  const [tokenAAddress, setTokenAAddress] = useState('');
  const [tokenBAddress, setTokenBAddress] = useState('');
  const [feeTier, setFeeTier] = useState(defaultFeeTier('univ3'));
  const [initialPrice, setInitialPrice] = useState('');
  const [amountA, setAmountA] = useState('');
  const [amountB, setAmountB] = useState('');
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [executeLoading, setExecuteLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState(null);

  const sourceOptions = useMemo(() => ({
    hot: hotSources,
    smart: smartSources,
  }), [hotSources, smartSources]);

  const selectedSource = useMemo(() => {
    const list = sourceOptions[sourceTab] || [];
    return list.find((item) => item.key === selectedSourceKey) || null;
  }, [selectedSourceKey, sourceOptions, sourceTab]);

  const protocolFees = useMemo(() => supportedFeeTiers(protocol), [protocol]);
  const selectedWalletId = useMemo(() => {
    const numeric = Number(walletId || 0);
    if (Number.isFinite(numeric) && numeric > 0 && wallets.some((item) => Number(item?.id || 0) === numeric)) {
      return numeric;
    }
    const def = wallets.find((item) => item?.is_default);
    return Number(def?.id || wallets?.[0]?.id || 0);
  }, [walletId, wallets]);

  useEffect(() => {
    if (supportsFeeTier(protocol, feeTier)) return;
    setFeeTier(defaultFeeTier(protocol));
  }, [feeTier, protocol]);

  useEffect(() => {
    if (!hasInitData) {
      setWallets([]);
      setHotSources([]);
      setSmartSources([]);
      return undefined;
    }

    const controller = new AbortController();
    setWalletsLoading(true);
    setWalletsError('');
    fetchWallets({ apiBaseUrl, initData, chain: 'bsc', signal: controller.signal })
      .then((resp) => {
        const nextWallets = Array.isArray(resp?.wallets) ? resp.wallets : [];
        setWallets(nextWallets);
        const def = nextWallets.find((item) => item?.is_default);
        setWalletId((prev) => (Number(prev || 0) > 0 ? prev : Number(def?.id || nextWallets?.[0]?.id || 0)));
      })
      .catch((err) => {
        if (err?.name !== 'AbortError') {
          setWalletsError(String(err?.message || err));
          setWallets([]);
        }
      })
      .finally(() => setWalletsLoading(false));

    return () => controller.abort();
  }, [apiBaseUrl, hasInitData, initData]);

  const reloadSources = useCallback(() => {
    if (!hasInitData) return;
    const controller = new AbortController();
    setSourcesLoading(true);
    setSourcesError('');
    Promise.all([
      fetchHotPools({
        apiBaseUrl,
        initData,
        chain: 'bsc',
        sort: 'fees',
        timeframeMinutes: 5,
        limit: 24,
        signal: controller.signal,
      }),
      fetchSmartMoneyOverview({
        apiBaseUrl,
        initData,
        chain: 'bsc',
        poolLimit: 16,
        walletLimit: 8,
        poolsWindowHours: 2,
        pnlWindowHours: 2,
        signal: controller.signal,
      }),
    ])
      .then(([hotResp, smartResp]) => {
        setHotSources(buildHotSources(hotResp?.data));
        setSmartSources(buildSmartSources(smartResp?.pools));
      })
      .catch((err) => {
        if (err?.name !== 'AbortError') setSourcesError(String(err?.message || err));
      })
      .finally(() => setSourcesLoading(false));
    return () => controller.abort();
  }, [apiBaseUrl, hasInitData, initData]);

  useEffect(() => {
    const cleanup = reloadSources();
    return () => {
      if (typeof cleanup === 'function') cleanup();
    };
  }, [reloadSources]);

  const applySource = useCallback((source, tabKey) => {
    if (!source) return;
    setSourceTab(tabKey);
    setSelectedSourceKey(source.key);
    setTokenAAddress(source.token0Address || '');
    setTokenBAddress(source.token1Address || '');
    if (supportsFeeTier(protocol, source.feeTier)) {
      setFeeTier(source.feeTier);
    }
    setInitialPrice(source.pricePrefill || '');
    setPreview(null);
    setResult(null);
    setError('');
  }, [protocol]);

  const createPayload = useCallback(() => ({
    apiBaseUrl,
    initData,
    chain: 'bsc',
    protocol,
    walletId: selectedWalletId || undefined,
    tokenAAddress,
    tokenBAddress,
    feeTier: Number(feeTier || 0),
    initialPrice,
    mode,
    rangeMode: 'full_range',
    amountA,
    amountB,
  }), [amountA, amountB, apiBaseUrl, feeTier, initData, initialPrice, mode, protocol, selectedWalletId, tokenAAddress, tokenBAddress]);

  const handlePreview = useCallback(async () => {
    if (!hasInitData) {
      setError('请先完成 Telegram 登录。');
      return;
    }
    setPreviewLoading(true);
    setError('');
    setResult(null);
    try {
      const resp = await previewCreatePool(createPayload());
      setPreview(resp);
      if (!String(initialPrice || '').trim() && String(resp?.suggested_initial_price || '').trim()) {
        setInitialPrice(String(resp.suggested_initial_price).trim());
      }
    } catch (err) {
      setError(String(err?.message || err));
      setPreview(null);
    } finally {
      setPreviewLoading(false);
    }
  }, [createPayload, hasInitData, initialPrice]);

  const handleExecute = useCallback(async () => {
    if (!hasInitData) {
      setError('请先完成 Telegram 登录。');
      return;
    }
    setExecuteLoading(true);
    setError('');
    setResult(null);
    try {
      const resp = await executeCreatePool(createPayload());
      setResult(resp);
      setPreview(null);
    } catch (err) {
      setError(String(err?.message || err));
    } finally {
      setExecuteLoading(false);
    }
  }, [createPayload, hasInitData]);

  const activeSources = sourceOptions[sourceTab] || [];

  return (
    <PanelShell
      title="创建池子"
      subtitle="BSC · Uniswap V3 / V4 / Pancake V3 · 可选择热门池子或聪明钱作为基础数据来源"
      icon={Factory}
      actions={(
        <button
          type="button"
          className="ghost-chip"
          onClick={reloadSources}
          disabled={!hasInitData || sourcesLoading}
        >
          <RefreshCw size={13} className={sourcesLoading ? 'spin' : ''} />
          刷新来源
        </button>
      )}
    >
      {!hasInitData ? (
        <EmptyState text="请先完成 Telegram 登录后再创建池子。" />
      ) : (
        <div className="create-pool-panel">
          <div className="create-pool-toolbar">
            <div className="create-pool-chip-group">
              {PROTOCOL_OPTIONS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  className={`create-pool-chip ${protocol === item.key ? 'active' : ''}`}
                  onClick={() => setProtocol(item.key)}
                >
                  {item.label}
                </button>
              ))}
            </div>
            <div className="create-pool-chip-group">
              {MODE_OPTIONS.map((item) => (
                <button
                  key={item.key}
                  type="button"
                  className={`create-pool-chip ${mode === item.key ? 'active' : ''}`}
                  onClick={() => setMode(item.key)}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>

          <div className="create-pool-source-card">
            <div className="create-pool-section-head">
              <span>基础数据来源</span>
              <span className="create-pool-section-sub">只做预填，不在热门池子/聪明钱列表里新增入口</span>
            </div>
            <div className="create-pool-chip-group">
              {SOURCE_TABS.map((tab) => (
                <button
                  key={tab.key}
                  type="button"
                  className={`create-pool-chip ${sourceTab === tab.key ? 'active' : ''}`}
                  onClick={() => setSourceTab(tab.key)}
                >
                  {tab.key === 'hot' ? <Flame size={13} /> : null}
                  {tab.key === 'smart' ? <BrainCircuit size={13} /> : null}
                  {tab.label}
                </button>
              ))}
            </div>

            {sourceTab === 'manual' ? (
              <div className="create-pool-source-empty">手动模式下你可以直接输入目标币对地址。</div>
            ) : sourcesLoading && activeSources.length === 0 ? (
              <div className="create-pool-source-empty">正在加载 BSC 来源池子...</div>
            ) : activeSources.length === 0 ? (
              <div className="create-pool-source-empty">{sourcesError || '暂无可用来源数据'}</div>
            ) : (
              <div className="create-pool-source-list">
                {activeSources.map((source) => (
                  <button
                    key={source.key}
                    type="button"
                    className={`create-pool-source-item ${selectedSourceKey === source.key ? 'active' : ''}`}
                    onClick={() => applySource(source, sourceTab)}
                  >
                    <div className="create-pool-source-main">
                      <div className="create-pool-source-title">{source.label}</div>
                      <div className="create-pool-source-subtitle">{source.subtitle}</div>
                    </div>
                    <div className="create-pool-source-side">
                      <span className="create-pool-badge">{formatFeeTier(source.feeTier)}</span>
                    </div>
                  </button>
                ))}
              </div>
            )}

            {selectedSource ? (
              <div className="create-pool-source-hint">
                来源提示: {selectedSource.label} · {selectedSource.subtitle}
                {selectedSource.priceHint ? ` · ${selectedSource.priceHint}` : ''}
              </div>
            ) : null}
          </div>

          <div className="create-pool-grid">
            <label className="modal-field">
              <span>钱包</span>
              <select value={selectedWalletId || ''} onChange={(e) => setWalletId(Number(e.target.value || 0))}>
                {wallets.map((item) => (
                  <option key={item.id} value={item.id}>
                    {(item.name || shortAddress(item.address, 6, 4))} · {shortAddress(item.address, 6, 4)}
                  </option>
                ))}
              </select>
              {walletsLoading ? <small className="create-pool-inline-hint">正在读取 BSC 钱包...</small> : null}
              {walletsError ? <small className="error-text">{walletsError}</small> : null}
            </label>

            <label className="modal-field">
              <span>费率档位</span>
              <select value={feeTier} onChange={(e) => setFeeTier(Number(e.target.value || 0))}>
                {protocolFees.map((item) => (
                  <option key={item} value={item}>
                    {formatFeeTier(item)} ({item})
                  </option>
                ))}
              </select>
            </label>

            <label className="modal-field">
              <span>Token A 地址</span>
              <input
                type="text"
                value={tokenAAddress}
                onChange={(e) => setTokenAAddress(e.target.value)}
                placeholder="0x..."
              />
            </label>

            <label className="modal-field">
              <span>Token B 地址</span>
              <input
                type="text"
                value={tokenBAddress}
                onChange={(e) => setTokenBAddress(e.target.value)}
                placeholder="0x..."
              />
            </label>

            <label className="modal-field create-pool-span-2">
              <span>初始价格</span>
              <input
                type="text"
                value={initialPrice}
                onChange={(e) => setInitialPrice(e.target.value)}
                placeholder="1 TokenA = X TokenB"
              />
              <small className="create-pool-inline-hint">
                如果留空，后端会先尝试用两边 token 的 USD 价格自动推导。
              </small>
            </label>

            {mode === 'create_and_seed' ? (
              <>
                <label className="modal-field">
                  <span>Token A 数量</span>
                  <input type="text" value={amountA} onChange={(e) => setAmountA(e.target.value)} placeholder="例如 1000" />
                </label>
                <label className="modal-field">
                  <span>Token B 数量</span>
                  <input type="text" value={amountB} onChange={(e) => setAmountB(e.target.value)} placeholder="例如 0.5" />
                </label>
              </>
            ) : (
              <div className="create-pool-mode-note create-pool-span-2">
                当前模式为仅建池，只会初始化池子价格，不会立刻打首仓。
              </div>
            )}
          </div>

          <div className="create-pool-summary">
            <div className="create-pool-summary-item">
              <span>范围模式</span>
              <strong>Full Range</strong>
            </div>
            <div className="create-pool-summary-item">
              <span>目标链</span>
              <strong>BSC</strong>
            </div>
            <div className="create-pool-summary-item">
              <span>执行钱包</span>
              <strong>{selectedWalletId ? `#${selectedWalletId}` : '--'}</strong>
            </div>
          </div>

          {error ? <div className="error-text">{error}</div> : null}

          <div className="modal-actions create-pool-actions">
            <button type="button" className="ghost-chip" onClick={handlePreview} disabled={previewLoading || executeLoading}>
              {previewLoading ? '预览中...' : '预览参数'}
            </button>
            <button type="button" className="accent-btn" onClick={handleExecute} disabled={executeLoading || previewLoading}>
              {executeLoading ? '执行中...' : mode === 'create_only' ? '创建池子' : '一键创建并首注'}
            </button>
          </div>

          {preview ? (
            <div className={`create-pool-preview ${preview.ready_to_execute ? 'ready' : ''}`}>
              <div className="create-pool-section-head">
                <span>预览结果</span>
                <span className="create-pool-section-sub">
                  {preview.pool_exists ? '目标协议下已存在同币对同费率池子' : preview.ready_to_execute ? '参数已满足执行条件' : '仍有参数需要补齐'}
                </span>
              </div>
              <div className="create-pool-preview-grid">
                <div className="create-pool-preview-item">
                  <span>Token 排序</span>
                  <strong>{preview.token0?.symbol || '--'} / {preview.token1?.symbol || '--'}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>初始价格</span>
                  <strong>{preview.initial_price || '--'}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>Tick 区间</span>
                  <strong>{preview.tick_lower} / {preview.tick_upper}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>TickSpacing</span>
                  <strong>{preview.tick_spacing || '--'}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>预测 PoolId</span>
                  <strong>{preview.predicted_pool_id ? shortAddress(preview.predicted_pool_id, 10, 8) : '--'}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>估算流动性</span>
                  <strong>{preview.estimated_liquidity || '--'}</strong>
                </div>
              </div>
              {fieldErrorText(preview) ? <div className="create-pool-inline-hint">{fieldErrorText(preview)}</div> : null}
              {Array.isArray(preview?.warnings) && preview.warnings.length > 0 ? (
                <div className="create-pool-warnings">
                  {preview.warnings.map((item, index) => (
                    <div key={`${item}-${index}`} className="create-pool-warning">
                      {item}
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}

          {result ? (
            <div className="create-pool-result">
              <div className="create-pool-section-head">
                <span>执行结果</span>
                <span className="create-pool-section-sub">{result.status || 'ok'}</span>
              </div>
              <div className="create-pool-preview-grid">
                <div className="create-pool-preview-item">
                  <span>Pool</span>
                  <strong>{result.pool_address ? shortAddress(result.pool_address, 10, 8) : shortAddress(result.pool_id, 10, 8)}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>TokenId</span>
                  <strong>{result.token_id || '--'}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>Liquidity</span>
                  <strong>{result.liquidity || '--'}</strong>
                </div>
                <div className="create-pool-preview-item">
                  <span>主交易</span>
                  <strong>{shortAddress(result.tx_hash, 10, 8)}</strong>
                </div>
              </div>
              {Array.isArray(result?.explorer_urls) && result.explorer_urls.length > 0 ? (
                <div className="create-pool-links">
                  {result.explorer_urls.map((item, index) => (
                    <a key={`${item}-${index}`} href={item} target="_blank" rel="noreferrer" className="create-pool-link">
                      查看交易 #{index + 1}
                    </a>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      )}
    </PanelShell>
  );
}
