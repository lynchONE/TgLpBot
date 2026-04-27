import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Activity,
  RefreshCw,
  Settings2,
  Shield,
} from 'lucide-react';
import {
  addAdminPoolDataSource,
  addAdminRPCEndpoint,
  checkAdminPoolDataSource,
  checkAdminRPCEndpoint,
  deleteAdminPoolDataSource,
  deleteAdminRPCEndpoint,
  disableAdminPoolDataSource,
  disableAdminRPCEndpointNextMonth,
  enableAdminPoolDataSource,
  enableAdminRPCEndpoint,
  fetchAdminActiveTasks,
  fetchAdminOnlineUsers,
  fetchAdminPoolDataSources,
  fetchAdminPrivateZap,
  fetchAdminRPCPool,
  fetchAdminRealtimePositions,
  fetchSystemConfig,
  invalidateAdminPrivateZap,
  renameAdminRPCEndpoint,
  switchAdminPoolDataSource,
  switchAdminRPCEndpoint,
  updateSystemConfig,
} from '../api';
import { formatUsd, shortAddress } from '../utils';
import PanelShell, { EmptyState, MetricCard } from './PanelShell';

function errorText(err) {
  return String(err?.message || err || '').trim();
}

function formatDateTime(value) {
  if (!value) return '--';
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString();
}

function formatUserLabel(user) {
  if (!user) return '--';
  const first = String(user.first_name || '').trim();
  const last = String(user.last_name || '').trim();
  const username = String(user.username || '').trim();
  const fullName = [first, last].filter(Boolean).join(' ');
  if (fullName && username) return `${fullName} (@${username})`;
  if (fullName) return fullName;
  if (username) return `@${username}`;
  return `用户 ${user.user_id || '--'}`;
}

function formatTaskPair(task) {
  const token0 = String(task?.token0_symbol || '').trim();
  const token1 = String(task?.token1_symbol || '').trim();
  if (token0 && token1) return `${token0}/${token1}`;
  const title = String(task?.title || '').trim();
  return title || '--';
}

function formatPositionPair(position) {
  const title = String(position?.title || '').trim();
  if (title) return title;
  const token0 = String(position?.token_rows?.[0]?.symbol || '').trim();
  const token1 = String(position?.token_rows?.[1]?.symbol || '').trim();
  if (token0 && token1) return `${token0}/${token1}`;
  return '--';
}

function formatStatus(status) {
  switch (String(status || '').trim().toLowerCase()) {
    case 'opening':
      return '开仓中';
    case 'running':
      return '运行中';
    case 'waiting':
      return '等待中';
    case 'stopping':
      return '退出中';
    case 'stopped':
      return '已停止';
    case 'error':
      return '错误';
    default:
      return status || '--';
  }
}

function statusClass(status) {
  switch (String(status || '').trim().toLowerCase()) {
    case 'running':
      return 'am-badge am-badge-ok';
    case 'opening':
    case 'waiting':
    case 'error':
      return 'am-badge am-badge-warn';
    default:
      return 'am-badge';
  }
}

function formatChain(chain) {
  return String(chain || '').trim().toLowerCase() === 'base' ? 'Base' : 'BSC';
}

function formatTransport(transport) {
  return String(transport || '').trim().toLowerCase() === 'ws' ? 'WS' : 'HTTP';
}

function endpointDisplayName(endpoint) {
  const name = String(endpoint?.name || '').trim();
  if (name) return name;
  const url = String(endpoint?.url || '').trim();
  if (!url) return '--';
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

function isUnavailable(endpoint) {
  return String(endpoint?.status || '').trim().toLowerCase() === 'unavailable';
}

function formatEndpointUrl(endpoint) {
  const masked = String(endpoint?.url_masked || '').trim();
  const raw = String(endpoint?.url || '').trim();
  return masked || raw || '--';
}

function formatPrivateZapKind(kind) {
  const value = String(kind || '').trim().toLowerCase();
  if (value === 'atomic_increase_zap') return 'Atomic Increase Zap';
  if (value === 'zap_simple') return 'Zap Simple';
  return kind || '--';
}

function formatPoolDataSourceType(type) {
  const value = String(type || '').trim().toLowerCase();
  if (value === 'poolm_top_fees') return 'PoolM';
  if (value === 'market_pools') return 'Market Pools';
  return type || '--';
}

function poolSourceDisplayName(source) {
  const name = String(source?.name || '').trim();
  if (name) return name;
  const url = String(source?.base_url || '').trim();
  if (!url) return '--';
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

function formatPoolSourceUrl(source) {
  const masked = String(source?.base_url_masked || '').trim();
  const raw = String(source?.base_url || '').trim();
  const path = String(source?.path_template || '').trim();
  return `${masked || raw || '--'}${path}`;
}

function splitCSV(raw) {
  return String(raw || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

function formatCoverage(source) {
  const coverage = source?.last_field_coverage;
  if (!coverage || typeof coverage !== 'object') return '';
  const poolCount = Number(coverage.pool_count || 0);
  if (!poolCount) return '';
  const parts = [`池子 ${poolCount}`];
  if (Number(coverage.missing_tvl_count || 0) > 0) {
    parts.push(`TVL 缺 ${coverage.missing_tvl_count}`);
  }
  if (Number(coverage.missing_active_liquidity_usd_count || 0) > 0) {
    parts.push(`活跃流动性缺 ${coverage.missing_active_liquidity_usd_count}`);
  }
  if (Number(coverage.missing_token0_count || 0) > 0 || Number(coverage.missing_token1_count || 0) > 0) {
    parts.push(`Token 缺 ${Number(coverage.missing_token0_count || 0) + Number(coverage.missing_token1_count || 0)}`);
  }
  if (Number(coverage.v4_pool_id_fallback_count || 0) > 0) {
    parts.push(`v4 poolId ${coverage.v4_pool_id_fallback_count}`);
  }
  return parts.join(' / ');
}

function RPCEndpointRow({
  endpoint,
  onRename,
  onCheck,
  onSwitch,
  onDisable,
  onEnable,
  onDelete,
}) {
  const [renameValue, setRenameValue] = useState(String(endpoint?.name || '').trim());
  const available = !isUnavailable(endpoint);

  useEffect(() => {
    setRenameValue(String(endpoint?.name || '').trim());
  }, [endpoint?.id, endpoint?.name]);

  return (
    <div className="am-list-item am-list-item-wrap">
      <div style={{ minWidth: 0, flex: 1 }}>
        <div className="am-item-title">{endpointDisplayName(endpoint)}</div>
        <div className="am-item-sub">
          {formatEndpointUrl(endpoint)} / 延迟 {Number(endpoint?.last_latency_ms || 0) > 0 ? `${Number(endpoint.last_latency_ms)}ms` : '--'}
        </div>
        <div className="am-actions" style={{ justifyContent: 'flex-start' }}>
          <span className={available ? 'am-badge am-badge-ok' : 'am-badge am-badge-warn'}>
            {available ? '可用' : '不可用'}
          </span>
          {endpoint?.is_current ? <span className="am-badge">当前节点</span> : null}
          {endpoint?.disabled_until ? (
            <span className="am-badge am-badge-warn">禁用至 {formatDateTime(endpoint.disabled_until)}</span>
          ) : null}
        </div>
        <div className="am-rename">
          <span>名称</span>
          <input
            value={renameValue}
            onChange={(event) => setRenameValue(event.target.value)}
            placeholder="节点名称"
          />
          <button
            type="button"
            className="am-action-btn"
            onClick={() => onRename?.(endpoint, renameValue)}
          >
            改名
          </button>
        </div>
      </div>

      <div className="am-btn-group">
        <button type="button" className="am-action-btn" onClick={() => onCheck?.(endpoint)}>检测</button>
        {!endpoint?.is_current && available ? (
          <button type="button" className="am-action-btn" onClick={() => onSwitch?.(endpoint)}>切换</button>
        ) : null}
        {available ? (
          <button type="button" className="am-action-btn" onClick={() => onDisable?.(endpoint)}>禁用到下月</button>
        ) : (
          <button type="button" className="am-action-btn" onClick={() => onEnable?.(endpoint)}>启用</button>
        )}
        <button type="button" className="am-action-btn" onClick={() => onDelete?.(endpoint)}>删除</button>
      </div>
    </div>
  );
}

function PoolDataSourceRow({
  source,
  onCheck,
  onSwitch,
  onDisable,
  onEnable,
  onDelete,
}) {
  const enabled = Boolean(source?.is_enabled);
  const current = Boolean(source?.is_current);
  const coverage = formatCoverage(source);

  return (
    <div className="am-list-item am-list-item-wrap">
      <div style={{ minWidth: 0, flex: 1 }}>
        <div className="am-item-title">{poolSourceDisplayName(source)}</div>
        <div className="am-item-sub">
          {formatPoolSourceUrl(source)} / {formatPoolDataSourceType(source?.source_type)} / {Number(source?.timeframe_minutes || 5)}m / limit {Number(source?.limit || 100)}
        </div>
        <div className="am-actions" style={{ justifyContent: 'flex-start' }}>
          <span className={enabled ? 'am-badge am-badge-ok' : 'am-badge am-badge-warn'}>
            {enabled ? '启用' : '停用'}
          </span>
          {current ? <span className="am-badge">当前来源</span> : null}
          {source?.is_env_fallback ? <span className="am-badge">ENV 兜底</span> : null}
          {Number(source?.last_latency_ms || 0) > 0 ? (
            <span className="am-badge">延迟 {Number(source.last_latency_ms)}ms</span>
          ) : null}
        </div>
        <div className="am-item-sub">
          检测 {formatDateTime(source?.last_checked_at)} / 成功 {formatDateTime(source?.last_success_at)}
          {coverage ? ` / ${coverage}` : ''}
        </div>
        {source?.last_error ? <div className="am-error">{source.last_error}</div> : null}
      </div>

      <div className="am-btn-group">
        <button type="button" className="am-action-btn" onClick={() => onCheck?.(source)}>检测</button>
        <button
          type="button"
          className="am-action-btn"
          disabled={!enabled || current}
          onClick={() => onSwitch?.(source)}
        >
          切换
        </button>
        {enabled ? (
          <button type="button" className="am-action-btn" onClick={() => onDisable?.(source)}>停用</button>
        ) : (
          <button type="button" className="am-action-btn" onClick={() => onEnable?.(source)}>启用</button>
        )}
        <button type="button" className="am-action-btn" onClick={() => onDelete?.(source)}>删除</button>
      </div>
    </div>
  );
}

export default function AdminPanel({
  apiBaseUrl,
  initData,
  hasInitData,
  isAdmin = false,
  refreshInterval = 10,
}) {
  const [activeTab, setActiveTab] = useState('operations');
  const [notice, setNotice] = useState('');

  const [onlineUsers, setOnlineUsers] = useState([]);
  const [onlineLoading, setOnlineLoading] = useState(false);
  const [onlineError, setOnlineError] = useState('');

  const [activeTasks, setActiveTasks] = useState([]);
  const [taskLoading, setTaskLoading] = useState(false);
  const [taskError, setTaskError] = useState('');

  const [selectedUser, setSelectedUser] = useState(null);
  const [userPositions, setUserPositions] = useState(null);
  const [positionsLoading, setPositionsLoading] = useState(false);
  const [positionsError, setPositionsError] = useState('');

  const [systemConfig, setSystemConfig] = useState(null);
  const [systemDefaults, setSystemDefaults] = useState(null);
  const [systemSizingDefaults, setSystemSizingDefaults] = useState(null);
  const [systemDraft, setSystemDraft] = useState({
    zap_price_deviation_max_percent: '',
    zap_min_pool_liquidity_usd: '',
    open_position_target_share_min: '',
    open_position_target_share_max: '',
    open_position_risk_cap_usd: '',
    open_position_risk_cap_ratio: '',
  });
  const [systemLoading, setSystemLoading] = useState(false);
  const [systemSaving, setSystemSaving] = useState(false);
  const [systemError, setSystemError] = useState('');

  const [rpcData, setRpcData] = useState(null);
  const [rpcLoading, setRpcLoading] = useState(false);
  const [rpcError, setRpcError] = useState('');
  const [rpcAdding, setRpcAdding] = useState(false);
  const [rpcAddDraft, setRpcAddDraft] = useState({
    chain: 'bsc',
    transport: 'http',
    url: '',
    name: '',
    setCurrent: false,
  });

  const [poolSourceData, setPoolSourceData] = useState(null);
  const [poolSourceLoading, setPoolSourceLoading] = useState(false);
  const [poolSourceError, setPoolSourceError] = useState('');
  const [poolSourceAdding, setPoolSourceAdding] = useState(false);
  const [poolSourceAddDraft, setPoolSourceAddDraft] = useState({
    sourceType: 'market_pools',
    chain: 'bsc',
    baseUrl: 'http://localhost:8080',
    pathTemplate: '/api/market/pools',
    timeframeMinutes: 5,
    limit: 100,
    protocols: 'v3,v4',
    dexes: 'PancakeswapV3,UniswapV3,UniswapV4',
    name: '',
    setCurrent: false,
  });

  const [privateZapData, setPrivateZapData] = useState(null);
  const [privateZapLoading, setPrivateZapLoading] = useState(false);
  const [privateZapError, setPrivateZapError] = useState('');
  const [invalidatingKey, setInvalidatingKey] = useState('');

  const userPositionsList = useMemo(
    () => (Array.isArray(userPositions?.positions) ? userPositions.positions : []),
    [userPositions?.positions]
  );
  const isReady = hasInitData && isAdmin;

  const showNotice = useCallback((message) => {
    setNotice(message);
    setTimeout(() => setNotice(''), 3000);
  }, []);

  const loadOnlineUsers = useCallback(async () => {
    if (!isReady) return;
    setOnlineLoading(true);
    setOnlineError('');
    try {
      const response = await fetchAdminOnlineUsers({ apiBaseUrl, initData });
      setOnlineUsers(Array.isArray(response?.users) ? response.users : []);
    } catch (err) {
      setOnlineError(errorText(err));
    } finally {
      setOnlineLoading(false);
    }
  }, [apiBaseUrl, initData, isReady]);

  const loadActiveTasks = useCallback(async () => {
    if (!isReady) return;
    setTaskLoading(true);
    setTaskError('');
    try {
      const response = await fetchAdminActiveTasks({ apiBaseUrl, initData });
      setActiveTasks(Array.isArray(response?.tasks) ? response.tasks : []);
    } catch (err) {
      setTaskError(errorText(err));
    } finally {
      setTaskLoading(false);
    }
  }, [apiBaseUrl, initData, isReady]);

  const loadUserPositions = useCallback(async (userId) => {
    if (!isReady || !userId) return;
    setPositionsLoading(true);
    setPositionsError('');
    try {
      const response = await fetchAdminRealtimePositions({ apiBaseUrl, initData, userId });
      setUserPositions(response || null);
    } catch (err) {
      setPositionsError(errorText(err));
    } finally {
      setPositionsLoading(false);
    }
  }, [apiBaseUrl, initData, isReady]);

  const loadSystemConfig = useCallback(async () => {
    if (!isReady) return;
    setSystemLoading(true);
    setSystemError('');
    try {
      const response = await fetchSystemConfig({ apiBaseUrl, initData });
      setSystemConfig(response?.config || null);
      setSystemDefaults(response?.zap_safety_defaults || null);
      setSystemSizingDefaults(response?.open_position_sizing_defaults || null);
      setSystemDraft({
        zap_price_deviation_max_percent: String(response?.config?.zap_price_deviation_max_percent ?? ''),
        zap_min_pool_liquidity_usd: String(response?.config?.zap_min_pool_liquidity_usd ?? ''),
        open_position_target_share_min: String(response?.config?.open_position_target_share_min ?? ''),
        open_position_target_share_max: String(response?.config?.open_position_target_share_max ?? ''),
        open_position_risk_cap_usd: String(response?.config?.open_position_risk_cap_usd ?? ''),
        open_position_risk_cap_ratio: String(response?.config?.open_position_risk_cap_ratio ?? ''),
      });
    } catch (err) {
      setSystemError(errorText(err));
    } finally {
      setSystemLoading(false);
    }
  }, [apiBaseUrl, initData, isReady]);

  const loadRPCPool = useCallback(async () => {
    if (!isReady) return;
    setRpcLoading(true);
    setRpcError('');
    try {
      const response = await fetchAdminRPCPool({ apiBaseUrl, initData });
      setRpcData(response || null);
    } catch (err) {
      setRpcError(errorText(err));
    } finally {
      setRpcLoading(false);
    }
  }, [apiBaseUrl, initData, isReady]);

  const loadPoolDataSources = useCallback(async () => {
    if (!isReady) return;
    setPoolSourceLoading(true);
    setPoolSourceError('');
    try {
      const response = await fetchAdminPoolDataSources({ apiBaseUrl, initData });
      setPoolSourceData(response || null);
    } catch (err) {
      setPoolSourceError(errorText(err));
    } finally {
      setPoolSourceLoading(false);
    }
  }, [apiBaseUrl, initData, isReady]);

  const loadPrivateZap = useCallback(async () => {
    if (!isReady) return;
    setPrivateZapLoading(true);
    setPrivateZapError('');
    try {
      const response = await fetchAdminPrivateZap({ apiBaseUrl, initData });
      setPrivateZapData(response || null);
    } catch (err) {
      setPrivateZapError(errorText(err));
    } finally {
      setPrivateZapLoading(false);
    }
  }, [apiBaseUrl, initData, isReady]);

  useEffect(() => {
    if (activeTab === 'operations') {
      loadOnlineUsers();
      loadActiveTasks();
    } else if (activeTab === 'system') {
      loadSystemConfig();
      loadRPCPool();
      loadPoolDataSources();
      loadPrivateZap();
    }
  }, [activeTab, loadActiveTasks, loadOnlineUsers, loadPoolDataSources, loadPrivateZap, loadRPCPool, loadSystemConfig]);

  useEffect(() => {
    if (!isReady || activeTab !== 'operations') return undefined;
    const timer = setInterval(() => {
      loadOnlineUsers();
      loadActiveTasks();
      if (selectedUser?.user_id) loadUserPositions(selectedUser.user_id);
    }, Math.max(10, Number(refreshInterval || 10)) * 1000);
    return () => clearInterval(timer);
  }, [activeTab, isReady, loadActiveTasks, loadOnlineUsers, loadUserPositions, refreshInterval, selectedUser?.user_id]);

  useEffect(() => {
    if (activeTab === 'operations' && selectedUser?.user_id) {
      loadUserPositions(selectedUser.user_id);
    }
  }, [activeTab, loadUserPositions, selectedUser?.user_id]);

  const refreshCurrentTab = useCallback(() => {
    if (activeTab === 'operations') {
      loadOnlineUsers();
      loadActiveTasks();
      if (selectedUser?.user_id) loadUserPositions(selectedUser.user_id);
      return;
    }
    loadSystemConfig();
    loadRPCPool();
    loadPoolDataSources();
    loadPrivateZap();
  }, [
    activeTab,
    loadActiveTasks,
    loadOnlineUsers,
    loadPoolDataSources,
    loadPrivateZap,
    loadRPCPool,
    loadSystemConfig,
    loadUserPositions,
    selectedUser?.user_id,
  ]);

  const handleSaveSystemConfig = useCallback(async () => {
    if (!isReady) return;
    setSystemSaving(true);
    setSystemError('');
    try {
      const parseNumber = (value) => {
        const number = Number(value);
        return Number.isFinite(number) ? number : 0;
      };
      const response = await updateSystemConfig({
        apiBaseUrl,
        initData,
        config: {
          zap_price_deviation_max_percent: parseNumber(systemDraft.zap_price_deviation_max_percent),
          zap_min_pool_liquidity_usd: parseNumber(systemDraft.zap_min_pool_liquidity_usd),
          open_position_target_share_min: parseNumber(systemDraft.open_position_target_share_min),
          open_position_target_share_max: parseNumber(systemDraft.open_position_target_share_max),
          open_position_risk_cap_usd: parseNumber(systemDraft.open_position_risk_cap_usd),
          open_position_risk_cap_ratio: parseNumber(systemDraft.open_position_risk_cap_ratio),
        },
      });
      setSystemConfig(response?.config || null);
      setSystemDefaults(response?.zap_safety_defaults || null);
      setSystemSizingDefaults(response?.open_position_sizing_defaults || null);
      showNotice('系统配置已保存');
    } catch (err) {
      setSystemError(errorText(err));
    } finally {
      setSystemSaving(false);
    }
  }, [apiBaseUrl, initData, isReady, showNotice, systemDraft]);

  const runRPCAction = useCallback(async (runner, successMessage) => {
    try {
      await runner();
      await loadRPCPool();
      if (successMessage) showNotice(successMessage);
    } catch (err) {
      setRpcError(errorText(err));
    }
  }, [loadRPCPool, showNotice]);

  const handleAddRPC = useCallback(async () => {
    if (!isReady) return;
    const url = String(rpcAddDraft.url || '').trim();
    if (!url) {
      setRpcError('请先填写 RPC URL');
      return;
    }
    setRpcAdding(true);
    setRpcError('');
    try {
      await addAdminRPCEndpoint({
        apiBaseUrl,
        initData,
        chain: rpcAddDraft.chain,
        transport: rpcAddDraft.transport,
        name: String(rpcAddDraft.name || '').trim(),
        url,
        setCurrent: Boolean(rpcAddDraft.setCurrent),
      });
      setRpcAddDraft((prev) => ({ ...prev, url: '', name: '', setCurrent: false }));
      await loadRPCPool();
      showNotice('RPC 节点已添加');
    } catch (err) {
      setRpcError(errorText(err));
    } finally {
      setRpcAdding(false);
    }
  }, [apiBaseUrl, initData, isReady, loadRPCPool, rpcAddDraft, showNotice]);

  const updatePoolSourceDraft = useCallback((key, value) => {
    setPoolSourceAddDraft((prev) => {
      const next = { ...prev, [key]: value };
      if (key === 'sourceType') {
        if (value === 'poolm_top_fees') {
          next.pathTemplate = '';
          next.protocols = '';
          next.dexes = 'pcsv3,univ3,univ4';
          if (!String(prev.baseUrl || '').trim() || prev.baseUrl === 'http://localhost:8080') {
            next.baseUrl = 'https://mapi.poolm.xyz';
          }
        } else {
          next.pathTemplate = '/api/market/pools';
          next.protocols = 'v3,v4';
          next.dexes = 'PancakeswapV3,UniswapV3,UniswapV4';
          if (!String(prev.baseUrl || '').trim() || prev.baseUrl === 'https://mapi.poolm.xyz') {
            next.baseUrl = 'http://localhost:8080';
          }
        }
      }
      return next;
    });
  }, []);

  const runPoolSourceAction = useCallback(async (runner, successMessage) => {
    try {
      await runner();
      await loadPoolDataSources();
      if (successMessage) showNotice(successMessage);
    } catch (err) {
      setPoolSourceError(errorText(err));
    }
  }, [loadPoolDataSources, showNotice]);

  const handleAddPoolSource = useCallback(async () => {
    if (!isReady) return;
    const baseUrl = String(poolSourceAddDraft.baseUrl || '').trim();
    if (!baseUrl) {
      setPoolSourceError('请先填写 Base URL');
      return;
    }
    setPoolSourceAdding(true);
    setPoolSourceError('');
    try {
      await addAdminPoolDataSource({
        apiBaseUrl,
        initData,
        name: String(poolSourceAddDraft.name || '').trim(),
        sourceType: poolSourceAddDraft.sourceType,
        chain: poolSourceAddDraft.chain,
        timeframeMinutes: Number(poolSourceAddDraft.timeframeMinutes) || 5,
        limit: Number(poolSourceAddDraft.limit) || 100,
        baseUrl,
        pathTemplate: String(poolSourceAddDraft.pathTemplate || '').trim(),
        protocols: splitCSV(poolSourceAddDraft.protocols),
        dexes: splitCSV(poolSourceAddDraft.dexes),
        setCurrent: Boolean(poolSourceAddDraft.setCurrent),
      });
      setPoolSourceAddDraft((prev) => ({ ...prev, name: '', setCurrent: false }));
      await loadPoolDataSources();
      showNotice('池子数据源已添加');
    } catch (err) {
      setPoolSourceError(errorText(err));
    } finally {
      setPoolSourceAdding(false);
    }
  }, [apiBaseUrl, initData, isReady, loadPoolDataSources, poolSourceAddDraft, showNotice]);

  const handleInvalidatePrivateZap = useCallback(async (chain, kind) => {
    if (!isReady) return;
    if (typeof window !== 'undefined') {
      const confirmed = window.confirm(`确认清理 ${formatChain(chain)} / ${formatPrivateZapKind(kind)} 绑定吗？`);
      if (!confirmed) return;
    }
    const busyKey = `${chain}:${kind}`;
    setInvalidatingKey(busyKey);
    setPrivateZapError('');
    try {
      await invalidateAdminPrivateZap({ apiBaseUrl, initData, chain, kind });
      await loadPrivateZap();
      showNotice(`${formatChain(chain)} ${formatPrivateZapKind(kind)} 已失效化`);
    } catch (err) {
      setPrivateZapError(errorText(err));
    } finally {
      setInvalidatingKey('');
    }
  }, [apiBaseUrl, initData, isReady, loadPrivateZap, showNotice]);

  const rpcGroups = useMemo(
    () => (Array.isArray(rpcData?.groups) ? rpcData.groups : []),
    [rpcData?.groups]
  );
  const poolSourceGroups = useMemo(
    () => (Array.isArray(poolSourceData?.groups) ? poolSourceData.groups : []),
    [poolSourceData?.groups]
  );
  const rpcChains = useMemo(() => {
    const grouped = new Map();
    rpcGroups.forEach((group) => {
      const chain = formatChain(group?.chain);
      const rows = grouped.get(chain) || [];
      rows.push(group);
      grouped.set(chain, rows);
    });
    return Array.from(grouped.entries());
  }, [rpcGroups]);
  const privateZapChains = Array.isArray(privateZapData?.chains) ? privateZapData.chains : [];
  const privateZapKinds = Array.isArray(privateZapData?.kinds) && privateZapData.kinds.length > 0
    ? privateZapData.kinds
    : ['zap_simple', 'atomic_increase_zap'];

  return (
    <PanelShell
      title="管理员"
      subtitle={activeTab === 'operations' ? '运行管理' : '系统'}
      icon={Shield}
      actions={<button type="button" className="panel-action-btn" onClick={refreshCurrentTab}>刷新</button>}
    >
      {!hasInitData ? <EmptyState text="请先完成 Telegram 登录后再访问管理员模块。" /> : null}
      {hasInitData && !isAdmin ? <EmptyState text="当前账号没有管理员权限。" /> : null}
      {isReady ? (
        <div className="am-stack">
          {notice ? <div className="panel-success">{notice}</div> : null}

          <div className="am-actions">
            <button
              type="button"
              className={`am-tab-btn ${activeTab === 'operations' ? 'active' : ''}`}
              onClick={() => setActiveTab('operations')}
            >
              <Activity size={12} />
              运行管理
            </button>
            <button
              type="button"
              className={`am-tab-btn ${activeTab === 'system' ? 'active' : ''}`}
              onClick={() => setActiveTab('system')}
            >
              <Settings2 size={12} />
              系统
            </button>
          </div>

          {activeTab === 'operations' ? (
            <>
              <div className="am-metric-row">
                <MetricCard label="在线用户" value={String(onlineUsers.length)} tone="strong" />
                <MetricCard label="活跃任务" value={String(activeTasks.length)} />
                <MetricCard label="当前用户" value={selectedUser?.user_id ? `#${selectedUser.user_id}` : '--'} />
                <MetricCard label="用户仓位" value={String(userPositionsList.length)} />
              </div>

              <div className="am-two-col">
                <div className="am-card">
                  <div className="am-card-header">
                    <div className="am-card-title">在线用户</div>
                    <span className="am-item-sub">{onlineLoading ? '加载中...' : `${onlineUsers.length} 个`}</span>
                  </div>
                  {onlineError ? <div className="am-error">{onlineError}</div> : null}
                  <div className="am-list">
                    {onlineUsers.length > 0 ? onlineUsers.map((user) => (
                      <button
                        type="button"
                        key={user.user_id || user.telegram_id}
                        className={`am-list-item am-list-btn ${Number(user?.user_id) === Number(selectedUser?.user_id) ? 'selected' : ''}`}
                        onClick={() => {
                          setSelectedUser(user);
                          setUserPositions(null);
                        }}
                      >
                        <div style={{ minWidth: 0 }}>
                          <div className="am-item-title">{formatUserLabel(user)}</div>
                          <div className="am-item-sub">
                            TG {user?.telegram_id || '--'} / 更新时间 {formatDateTime(user?.updated_at)}
                          </div>
                        </div>
                        <div className="am-list-end">
                          <strong>{Number(user?.total_tasks || 0)}</strong>
                        </div>
                      </button>
                    )) : <EmptyState text={onlineLoading ? '正在加载在线用户...' : '暂无在线用户'} />}
                  </div>
                </div>

                <div className="am-card">
                  <div className="am-card-header">
                    <div className="am-card-title">活跃任务</div>
                    <span className="am-item-sub">{taskLoading ? '加载中...' : `${activeTasks.length} 个`}</span>
                  </div>
                  {taskError ? <div className="am-error">{taskError}</div> : null}
                  <div className="am-list">
                    {activeTasks.length > 0 ? activeTasks.map((task) => (
                      <button
                        type="button"
                        key={task.task_id || `${task.user_id}:${task.pool_id}`}
                        className="am-list-item am-list-btn"
                        onClick={() => {
                          setSelectedUser({
                            user_id: task.user_id,
                            telegram_id: task.telegram_id,
                            username: task.username,
                            first_name: task.first_name,
                            last_name: task.last_name,
                          });
                          setUserPositions(null);
                        }}
                      >
                        <div style={{ minWidth: 0 }}>
                          <div className="am-item-title">{formatTaskPair(task)}</div>
                          <div className="am-item-sub">
                            {formatUserLabel(task)} / Task #{task.task_id || '--'}
                          </div>
                        </div>
                        <div className="am-list-end" style={{ flexDirection: 'column', alignItems: 'flex-end', gap: 2 }}>
                          <span className={statusClass(task.status)}>{formatStatus(task.status)}</span>
                          <strong>{Number.isFinite(Number(task?.amount_usdt)) ? `$${Number(task.amount_usdt).toFixed(2)}` : '--'}</strong>
                        </div>
                      </button>
                    )) : <EmptyState text={taskLoading ? '正在加载活跃任务...' : '暂无活跃任务'} />}
                  </div>
                </div>
              </div>

              <div className="am-card">
                <div className="am-card-header">
                  <div className="am-card-title">用户详情</div>
                  {selectedUser?.user_id ? (
                    <button
                      type="button"
                      className="am-action-btn"
                      disabled={positionsLoading}
                      onClick={() => loadUserPositions(selectedUser.user_id)}
                    >
                      <RefreshCw size={12} className={positionsLoading ? 'animate-spin' : undefined} />
                      刷新仓位
                    </button>
                  ) : null}
                </div>

                {selectedUser ? (
                  <>
                    <div className="am-wallet-item">
                      <div className="am-wallet-head">
                        <div>
                          <div className="am-item-title">{formatUserLabel(selectedUser)}</div>
                          <div className="am-item-sub">
                            TG {selectedUser?.telegram_id || '--'} / 用户 ID {selectedUser?.user_id || '--'}
                          </div>
                        </div>
                        <div className="am-wallet-total">
                          <div className="am-item-sub">钱包</div>
                          <strong>{shortAddress(userPositions?.wallet?.address || '')}</strong>
                        </div>
                      </div>
                    </div>

                    {positionsError ? <div className="am-error">{positionsError}</div> : null}
                    {positionsLoading && userPositionsList.length === 0 ? <div className="panel-loading">正在加载用户仓位...</div> : null}
                    {!positionsLoading && userPositionsList.length === 0 ? <EmptyState text="当前用户没有活跃仓位。" /> : null}
                    <div className="am-list">
                      {userPositionsList.map((position) => (
                        <div
                          key={[
                            position?.chain,
                            position?.pool_id,
                            position?.position_id,
                            position?.task_id,
                          ].join(':')}
                          className="am-list-item"
                        >
                          <div style={{ minWidth: 0 }}>
                            <div className="am-item-title">{formatPositionPair(position)}</div>
                            <div className="am-item-sub">
                              {formatChain(position?.chain)} / Task #{position?.task_id || '--'} / 钱包 {shortAddress(position?.wallet_address || '')}
                            </div>
                          </div>
                          <div className="am-list-end" style={{ flexDirection: 'column', alignItems: 'flex-end', gap: 2 }}>
                            <span className={statusClass(position?.status)}>{formatStatus(position?.status)}</span>
                            <strong>{formatUsd(position?.totals?.wallet_usd || position?.position_amount_usd || 0)}</strong>
                          </div>
                        </div>
                      ))}
                    </div>
                  </>
                ) : <EmptyState text="从在线用户或活跃任务中选择一个用户查看详情。" />}
              </div>
            </>
          ) : null}

          {activeTab === 'system' ? (
            <>
              <div className="am-metric-row">
                <MetricCard label="系统配置" value={systemConfig ? '已加载' : '--'} tone="strong" />
                <MetricCard label="RPC 分组" value={String(rpcGroups.length)} />
                <MetricCard label="池子源分组" value={String(poolSourceGroups.length)} />
                <MetricCard label="Private Zap 链" value={String(privateZapChains.length)} />
                <MetricCard label="当前时间" value={formatDateTime(new Date())} />
              </div>

              <div className="am-card">
                <div className="am-card-header">
                  <div className="am-card-title">系统配置</div>
                  <button type="button" className="am-action-btn" disabled={systemLoading} onClick={loadSystemConfig}>
                    <RefreshCw size={12} className={systemLoading ? 'animate-spin' : undefined} />
                    刷新
                  </button>
                </div>
                {systemError ? <div className="am-error">{systemError}</div> : null}
                {systemLoading && !systemConfig ? <div className="panel-loading">正在加载系统配置...</div> : null}
                {!systemLoading && !systemConfig ? <EmptyState text="暂无系统配置数据" /> : null}
                {systemConfig ? (
                  <>
                    <div className="am-form">
                      <label className="am-field">
                        <span>最大报价偏差 (%)</span>
                        <input
                          type="number"
                          step="0.1"
                          value={systemDraft.zap_price_deviation_max_percent}
                          onChange={(event) => setSystemDraft((prev) => ({ ...prev, zap_price_deviation_max_percent: event.target.value }))}
                          placeholder={systemDefaults ? String(systemDefaults.price_deviation_max_percent) : '1'}
                        />
                      </label>
                      <label className="am-field">
                        <span>最低池子流动性 (USD)</span>
                        <input
                          type="number"
                          step="100"
                          value={systemDraft.zap_min_pool_liquidity_usd}
                          onChange={(event) => setSystemDraft((prev) => ({ ...prev, zap_min_pool_liquidity_usd: event.target.value }))}
                          placeholder={systemDefaults ? String(systemDefaults.min_pool_liquidity_usd) : '1000'}
                        />
                      </label>
                      <label className="am-field">
                        <span>开仓建议最小占比</span>
                        <input
                          type="number"
                          step="0.01"
                          value={systemDraft.open_position_target_share_min}
                          onChange={(event) => setSystemDraft((prev) => ({ ...prev, open_position_target_share_min: event.target.value }))}
                          placeholder={systemSizingDefaults ? String(systemSizingDefaults.target_share_min) : '0.2'}
                        />
                      </label>
                      <label className="am-field">
                        <span>开仓建议最大占比</span>
                        <input
                          type="number"
                          step="0.01"
                          value={systemDraft.open_position_target_share_max}
                          onChange={(event) => setSystemDraft((prev) => ({ ...prev, open_position_target_share_max: event.target.value }))}
                          placeholder={systemSizingDefaults ? String(systemSizingDefaults.target_share_max) : '0.65'}
                        />
                      </label>
                      <label className="am-field">
                        <span>开仓固定风险上限 (USD)</span>
                        <input
                          type="number"
                          step="10"
                          value={systemDraft.open_position_risk_cap_usd}
                          onChange={(event) => setSystemDraft((prev) => ({ ...prev, open_position_risk_cap_usd: event.target.value }))}
                          placeholder={systemSizingDefaults ? String(systemSizingDefaults.risk_cap_usd) : '500'}
                        />
                      </label>
                      <label className="am-field">
                        <span>开仓风险比例上限</span>
                        <input
                          type="number"
                          step="0.01"
                          value={systemDraft.open_position_risk_cap_ratio}
                          onChange={(event) => setSystemDraft((prev) => ({ ...prev, open_position_risk_cap_ratio: event.target.value }))}
                          placeholder={systemSizingDefaults ? String(systemSizingDefaults.risk_cap_ratio) : '0.2'}
                        />
                      </label>
                    </div>
                    <div className="am-actions">
                      <span className="am-item-sub">
                        默认值: 偏差 {systemDefaults?.price_deviation_max_percent ?? '--'} / 流动性 {systemDefaults?.min_pool_liquidity_usd ?? '--'}
                      </span>
                      <span className="am-item-sub">
                        建议默认值 占比 {systemSizingDefaults?.target_share_min ?? '--'} - {systemSizingDefaults?.target_share_max ?? '--'} / 风险 {systemSizingDefaults?.risk_cap_usd ?? '--'}U / {systemSizingDefaults?.risk_cap_ratio ?? '--'}
                      </span>
                      <button type="button" className="am-action-btn" disabled={systemSaving} onClick={handleSaveSystemConfig}>
                        {systemSaving ? '保存中...' : '保存配置'}
                      </button>
                    </div>
                  </>
                ) : null}
              </div>

              <div className="am-card">
                <div className="am-card-header">
                  <div className="am-card-title">RPC 节点池</div>
                  <button type="button" className="am-action-btn" disabled={rpcLoading} onClick={loadRPCPool}>
                    <RefreshCw size={12} className={rpcLoading ? 'animate-spin' : undefined} />
                    刷新
                  </button>
                </div>
                {rpcError ? <div className="am-error">{rpcError}</div> : null}
                <div className="am-form">
                  <label className="am-field">
                    <span>链</span>
                    <select value={rpcAddDraft.chain} onChange={(event) => setRpcAddDraft((prev) => ({ ...prev, chain: event.target.value }))}>
                      <option value="bsc">BSC</option>
                      <option value="base">Base</option>
                    </select>
                  </label>
                  <label className="am-field">
                    <span>协议</span>
                    <select value={rpcAddDraft.transport} onChange={(event) => setRpcAddDraft((prev) => ({ ...prev, transport: event.target.value }))}>
                      <option value="http">HTTP</option>
                      <option value="ws">WS</option>
                    </select>
                  </label>
                  <label className="am-field am-field-grow">
                    <span>URL</span>
                    <input
                      value={rpcAddDraft.url}
                      onChange={(event) => setRpcAddDraft((prev) => ({ ...prev, url: event.target.value }))}
                      placeholder={rpcAddDraft.transport === 'ws' ? 'wss://...' : 'https://...'}
                    />
                  </label>
                  <label className="am-field">
                    <span>名称</span>
                    <input
                      value={rpcAddDraft.name}
                      onChange={(event) => setRpcAddDraft((prev) => ({ ...prev, name: event.target.value }))}
                      placeholder="可选"
                    />
                  </label>
                  <label className="am-field am-field-check">
                    <input
                      type="checkbox"
                      checked={Boolean(rpcAddDraft.setCurrent)}
                      onChange={(event) => setRpcAddDraft((prev) => ({ ...prev, setCurrent: event.target.checked }))}
                    />
                    <span>添加后切为当前节点</span>
                  </label>
                </div>
                <div className="am-actions">
                  <button type="button" className="am-action-btn" disabled={rpcAdding} onClick={handleAddRPC}>
                    {rpcAdding ? '添加中...' : '添加节点'}
                  </button>
                </div>
                {rpcLoading && rpcGroups.length === 0 ? <div className="panel-loading">正在加载 RPC 节点池...</div> : null}
                {!rpcLoading && rpcGroups.length === 0 ? <EmptyState text="暂无 RPC 节点数据" /> : null}
                <div className="am-stack">
                  {rpcChains.map(([chain, groups]) => (
                    <div key={chain} className="am-rpc-group">
                      <div className="am-rpc-group-head">
                        <div className="am-card-title">{chain}</div>
                      </div>
                      <div className="am-stack">
                        {groups.map((group) => (
                          <div key={`${group?.chain}:${group?.transport}`} className="am-card">
                            <div className="am-card-header">
                              <div className="am-card-title">{formatTransport(group?.transport)}</div>
                              <span className="am-item-sub">当前来源 {String(group?.effective_source || 'none').toUpperCase()}</span>
                            </div>
                            <div className="am-list">
                              {Array.isArray(group?.endpoints) && group.endpoints.length > 0 ? group.endpoints.map((endpoint) => (
                                <RPCEndpointRow
                                  key={endpoint.id || `${group?.chain}:${group?.transport}:${endpoint?.url}`}
                                  endpoint={endpoint}
                                  onRename={(item, value) => runRPCAction(
                                    () => renameAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: item?.id, name: String(value || '').trim() }),
                                    '节点名称已更新'
                                  )}
                                  onCheck={(item) => runRPCAction(
                                    () => checkAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: item?.id }),
                                    '节点检测完成'
                                  )}
                                  onSwitch={(item) => runRPCAction(
                                    () => switchAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: item?.id }),
                                    '当前节点已切换'
                                  )}
                                  onDisable={(item) => runRPCAction(
                                    () => disableAdminRPCEndpointNextMonth({ apiBaseUrl, initData, endpointId: item?.id }),
                                    '节点已禁用到下月'
                                  )}
                                  onEnable={(item) => runRPCAction(
                                    () => enableAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: item?.id }),
                                    '节点已启用'
                                  )}
                                  onDelete={(item) => {
                                    if (typeof window !== 'undefined') {
                                      const confirmed = window.confirm(`确认删除节点 "${endpointDisplayName(item)}" 吗？`);
                                      if (!confirmed) return;
                                    }
                                    runRPCAction(
                                      () => deleteAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: item?.id }),
                                      '节点已删除'
                                    );
                                  }}
                                />
                              )) : <EmptyState text="暂无节点" />}
                            </div>
                          </div>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div className="am-card">
                <div className="am-card-header">
                  <div className="am-card-title">池子数据源</div>
                  <button type="button" className="am-action-btn" disabled={poolSourceLoading} onClick={loadPoolDataSources}>
                    <RefreshCw size={12} className={poolSourceLoading ? 'animate-spin' : undefined} />
                    刷新
                  </button>
                </div>
                {poolSourceError ? <div className="am-error">{poolSourceError}</div> : null}
                <div className="am-form">
                  <label className="am-field">
                    <span>来源类型</span>
                    <select value={poolSourceAddDraft.sourceType} onChange={(event) => updatePoolSourceDraft('sourceType', event.target.value)}>
                      <option value="market_pools">Market Pools</option>
                      <option value="poolm_top_fees">PoolM</option>
                    </select>
                  </label>
                  <label className="am-field">
                    <span>链</span>
                    <select value={poolSourceAddDraft.chain} onChange={(event) => updatePoolSourceDraft('chain', event.target.value)}>
                      <option value="bsc">BSC</option>
                      <option value="base">Base</option>
                    </select>
                  </label>
                  <label className="am-field am-field-grow">
                    <span>Base URL</span>
                    <input
                      value={poolSourceAddDraft.baseUrl}
                      onChange={(event) => updatePoolSourceDraft('baseUrl', event.target.value)}
                      placeholder={poolSourceAddDraft.sourceType === 'poolm_top_fees' ? 'https://mapi.poolm.xyz' : 'http://localhost:8080'}
                    />
                  </label>
                  <label className="am-field">
                    <span>路径</span>
                    <input
                      value={poolSourceAddDraft.pathTemplate}
                      onChange={(event) => updatePoolSourceDraft('pathTemplate', event.target.value)}
                      placeholder={poolSourceAddDraft.sourceType === 'poolm_top_fees' ? '默认 /top_fees' : '/api/market/pools'}
                    />
                  </label>
                  <label className="am-field">
                    <span>时间窗(分)</span>
                    <input
                      type="number"
                      min="1"
                      value={poolSourceAddDraft.timeframeMinutes}
                      onChange={(event) => updatePoolSourceDraft('timeframeMinutes', event.target.value)}
                    />
                  </label>
                  <label className="am-field">
                    <span>Limit</span>
                    <input
                      type="number"
                      min="1"
                      value={poolSourceAddDraft.limit}
                      onChange={(event) => updatePoolSourceDraft('limit', event.target.value)}
                    />
                  </label>
                  <label className="am-field am-field-grow">
                    <span>Protocols</span>
                    <input
                      value={poolSourceAddDraft.protocols}
                      onChange={(event) => updatePoolSourceDraft('protocols', event.target.value)}
                      placeholder="v3,v4"
                    />
                  </label>
                  <label className="am-field am-field-grow">
                    <span>DEX</span>
                    <input
                      value={poolSourceAddDraft.dexes}
                      onChange={(event) => updatePoolSourceDraft('dexes', event.target.value)}
                      placeholder="PancakeswapV3,UniswapV3,UniswapV4"
                    />
                  </label>
                  <label className="am-field">
                    <span>名称</span>
                    <input
                      value={poolSourceAddDraft.name}
                      onChange={(event) => updatePoolSourceDraft('name', event.target.value)}
                      placeholder="可选"
                    />
                  </label>
                  <label className="am-field am-field-check">
                    <input
                      type="checkbox"
                      checked={Boolean(poolSourceAddDraft.setCurrent)}
                      onChange={(event) => updatePoolSourceDraft('setCurrent', event.target.checked)}
                    />
                    <span>添加后切为当前来源</span>
                  </label>
                </div>
                <div className="am-actions">
                  <span className="am-item-sub">
                    支持 PoolM 与 Market Pools，DB 未配置时仍会使用 ENV 兜底来源。
                  </span>
                  <button type="button" className="am-action-btn" disabled={poolSourceAdding} onClick={handleAddPoolSource}>
                    {poolSourceAdding ? '添加中...' : '添加来源'}
                  </button>
                </div>
                {poolSourceLoading && poolSourceGroups.length === 0 ? <div className="panel-loading">正在加载池子数据源...</div> : null}
                {!poolSourceLoading && poolSourceGroups.length === 0 ? <EmptyState text="暂无池子数据源" /> : null}
                <div className="am-stack">
                  {poolSourceGroups.map((group) => (
                    <div key={`${group?.chain}:${group?.timeframe_minutes}`} className="am-rpc-group">
                      <div className="am-card-header">
                        <div className="am-card-title">{formatChain(group?.chain)} / {Number(group?.timeframe_minutes || 5)}m</div>
                        <span className="am-item-sub">
                          当前 {poolSourceDisplayName(group?.effective_source || group?.env_fallback)}
                        </span>
                      </div>
                      <div className="am-item-sub" style={{ marginBottom: 10 }}>
                        ENV 兜底 {formatPoolSourceUrl(group?.env_fallback)}
                      </div>
                      <div className="am-list">
                        {Array.isArray(group?.sources) && group.sources.length > 0 ? group.sources.map((source) => (
                          <PoolDataSourceRow
                            key={source.id || `${group?.chain}:${group?.timeframe_minutes}:${source?.base_url}`}
                            source={source}
                            onCheck={(item) => runPoolSourceAction(
                              () => checkAdminPoolDataSource({ apiBaseUrl, initData, sourceId: item?.id }),
                              '池子数据源检测完成'
                            )}
                            onSwitch={(item) => runPoolSourceAction(
                              () => switchAdminPoolDataSource({ apiBaseUrl, initData, sourceId: item?.id }),
                              '当前池子数据源已切换'
                            )}
                            onDisable={(item) => runPoolSourceAction(
                              () => disableAdminPoolDataSource({ apiBaseUrl, initData, sourceId: item?.id }),
                              '池子数据源已停用'
                            )}
                            onEnable={(item) => runPoolSourceAction(
                              () => enableAdminPoolDataSource({ apiBaseUrl, initData, sourceId: item?.id }),
                              '池子数据源已启用'
                            )}
                            onDelete={(item) => {
                              if (typeof window !== 'undefined') {
                                const confirmed = window.confirm(`确认删除池子源 "${poolSourceDisplayName(item)}" 吗？`);
                                if (!confirmed) return;
                              }
                              runPoolSourceAction(
                                () => deleteAdminPoolDataSource({ apiBaseUrl, initData, sourceId: item?.id }),
                                '池子数据源已删除'
                              );
                            }}
                          />
                        )) : <EmptyState text="当前仅使用 ENV 兜底来源" />}
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div className="am-card">
                <div className="am-card-header">
                  <div className="am-card-title">Private Zap</div>
                  <button type="button" className="am-action-btn" disabled={privateZapLoading} onClick={loadPrivateZap}>
                    <RefreshCw size={12} className={privateZapLoading ? 'animate-spin' : undefined} />
                    刷新
                  </button>
                </div>
                {privateZapError ? <div className="am-error">{privateZapError}</div> : null}
                {privateZapLoading && privateZapChains.length === 0 ? <div className="panel-loading">正在加载 Private Zap...</div> : null}
                {!privateZapLoading && privateZapChains.length === 0 ? <EmptyState text="暂无可用链" /> : null}
                <div className="am-stack">
                  {privateZapChains.map((chain) => (
                    <div key={chain} className="am-rpc-group">
                      <div className="am-card-header">
                        <div className="am-card-title">{formatChain(chain)}</div>
                        <span className="am-item-sub">清理绑定后，下次使用时重新部署</span>
                      </div>
                      <div className="am-btn-group">
                        {privateZapKinds.map((kind) => {
                          const busy = invalidatingKey === `${chain}:${kind}`;
                          return (
                            <button
                              type="button"
                              key={`${chain}:${kind}`}
                              className="am-action-btn"
                              disabled={busy}
                              onClick={() => handleInvalidatePrivateZap(chain, kind)}
                            >
                              {busy ? '处理中...' : `Invalidate ${formatPrivateZapKind(kind)}`}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </>
          ) : null}
        </div>
      ) : null}
    </PanelShell>
  );
}
