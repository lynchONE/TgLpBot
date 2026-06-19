import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Activity,
  KeyRound,
  RefreshCw,
  Search,
  Settings2,
  Shield,
} from 'lucide-react';
import {
  addAdminOKXConfig,
  addAdminPoolDataSource,
  addAdminRPCEndpoint,
  checkAdminOKXConfig,
  checkAdminPoolDataSource,
  checkAdminRPCEndpoint,
  deleteAdminOKXConfig,
  deleteAdminPoolDataSource,
  deleteAdminRPCEndpoint,
  disableAdminOKXConfig,
  disableAdminOKXConfigNextMonth,
  disableAdminPoolDataSource,
  disableAdminRPCEndpointNextMonth,
  enableAdminOKXConfig,
  enableAdminPoolDataSource,
  enableAdminRPCEndpoint,
  fetchAdminActiveTasks,
  fetchAdminOKXPool,
  fetchAdminOnlineUsers,
  fetchAdminPoolDataSources,
  fetchAdminPrivateZap,
  fetchAdminRPCPool,
  fetchAdminRealtimePositions,
  fetchSystemConfig,
  invalidateAdminPrivateZap,
  renameAdminOKXConfig,
  renameAdminRPCEndpoint,
  switchAdminOKXConfig,
  switchAdminPoolDataSource,
  switchAdminRPCEndpoint,
  updateAdminOKXConfig,
  updateAdminPoolDataSource,
  updateSystemConfig,
} from '../api';
import { formatUsd, shortAddress } from '../utils';
import PanelShell, { EmptyState } from './PanelShell';
import AdminAccessWorkbench from './AdminAccessWorkbench';
import ConfirmDialog from './ConfirmDialog';
import CustomSelect from './CustomSelect';
import AdminStatChip from './admin/AdminStatChip';
import AdminStatusDot from './admin/AdminStatusDot';
import AdminDrawer from './admin/AdminDrawer';

const CHAIN_OPTIONS = [
  { value: 'bsc', label: 'BSC' },
  { value: 'base', label: 'Base' },
];

const TRANSPORT_OPTIONS = [
  { value: 'http', label: 'HTTP' },
  { value: 'ws', label: 'WS' },
];

const POOL_SOURCE_TYPE_OPTIONS = [
  { value: 'market_pools', label: 'Market Pools' },
  { value: 'poolm_top_fees', label: 'PoolM' },
];

const ADMIN_SYSTEM_SECTIONS = [
  { key: 'config', label: '基础配置' },
  { key: 'rpc', label: 'RPC 节点' },
  { key: 'pool_sources', label: '池子数据源' },
  { key: 'okx', label: 'OKX 配置' },
  { key: 'private_zap', label: 'Private Zap' },
];

const TASK_STATUS_FILTERS = [
  { key: 'all', label: '全部' },
  { key: 'running', label: '运行中' },
  { key: 'opening', label: '开仓中' },
  { key: 'waiting', label: '等待中' },
  { key: 'stopping', label: '退出中' },
  { key: 'paused', label: '已暂停' },
];

function statusTone(status) {
  switch (String(status || '').trim().toLowerCase()) {
    case 'running': return 'ok';
    case 'opening': return 'accent';
    case 'waiting': return 'warn';
    case 'stopping':
    case 'error': return 'danger';
    default: return 'idle';
  }
}

function deriveRpcHealthSummary(rpcData) {
  const groups = Array.isArray(rpcData?.groups) ? rpcData.groups : [];
  let total = 0;
  let available = 0;
  let latencySum = 0;
  let latencyCount = 0;
  for (const group of groups) {
    const eps = Array.isArray(group?.endpoints) ? group.endpoints : [];
    for (const ep of eps) {
      total += 1;
      if (String(ep?.status || '').toLowerCase() !== 'unavailable') {
        available += 1;
        const lat = Number(ep?.last_latency_ms || 0);
        if (Number.isFinite(lat) && lat > 0) {
          latencySum += lat;
          latencyCount += 1;
        }
      }
    }
  }
  if (total === 0) return { tone: 'idle', value: '--', hint: '无节点' };
  const ratio = available / total;
  const tone = ratio >= 0.8 ? 'ok' : ratio >= 0.4 ? 'warn' : 'danger';
  const avg = latencyCount > 0 ? Math.round(latencySum / latencyCount) : 0;
  return {
    tone,
    value: `${available}/${total}`,
    hint: avg > 0 ? `均 ${avg}ms` : '可用 / 总数',
  };
}

function derivePoolSourceHealthSummary(poolData) {
  const groups = Array.isArray(poolData?.groups) ? poolData.groups : [];
  let total = 0;
  let enabled = 0;
  let withError = 0;
  for (const group of groups) {
    const sources = Array.isArray(group?.sources) ? group.sources : [];
    for (const src of sources) {
      total += 1;
      if (src?.is_enabled) enabled += 1;
      if (src?.last_error) withError += 1;
    }
  }
  if (total === 0) return { tone: 'idle', value: '--', hint: '无来源' };
  const tone = withError === 0 ? (enabled === total ? 'ok' : 'warn') : 'danger';
  return {
    tone,
    value: `${enabled}/${total}`,
    hint: withError > 0 ? `${withError} 错误` : '启用 / 总数',
  };
}

function deriveOKXHealthSummary(okxData) {
  const configs = Array.isArray(okxData?.configs) ? okxData.configs : [];
  let available = 0;
  let withError = 0;
  configs.forEach((cfg) => {
    const status = String(cfg?.status || '').trim().toLowerCase();
    if (cfg?.is_enabled && status !== 'unavailable') available += 1;
    if (cfg?.last_error) withError += 1;
  });
  if (configs.length === 0) {
    return {
      tone: okxData?.env_base_url ? 'idle' : 'warn',
      value: okxData?.env_base_url ? '.env' : '--',
      hint: okxData?.env_base_url ? '环境变量备用' : '未配置',
    };
  }
  return {
    tone: available > 0 ? (withError > 0 ? 'warn' : 'ok') : 'danger',
    value: `${available}/${configs.length}`,
    hint: withError > 0 ? `${withError} 错误` : '可用 / 总数',
  };
}

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

function formatOKXSource(source) {
  const value = String(source || '').trim().toLowerCase();
  if (value === 'db') return '配置池';
  if (value === 'env') return '.env';
  return source || '--';
}

function formatOKXReason(reason) {
  const value = String(reason || '').trim().toLowerCase();
  if (value === 'quota_exhausted') return '额度用尽';
  if (value === 'rate_limited') return '频率限制';
  if (value === 'health_fail') return '健康检查失败';
  if (value === 'auth_fail') return '认证失败';
  if (value === 'manual') return '手动禁用';
  return reason || '';
}

function okxConfigDisplayName(config) {
  const name = String(config?.name || '').trim();
  if (name) return name;
  const url = String(config?.base_url || '').trim();
  if (!url) return config?.id ? `#${config.id}` : '--';
  try {
    return new URL(url).host || url;
  } catch {
    return url;
  }
}

function isOKXUnavailable(config) {
  return !config?.is_enabled || String(config?.status || '').trim().toLowerCase() === 'unavailable';
}

function formatOKXConfigUrl(config) {
  return String(config?.base_url_masked || config?.base_url || '').trim() || '--';
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

function poolSourceToDraft(source) {
  return {
    sourceType: String(source?.source_type || 'market_pools'),
    chain: String(source?.chain || 'bsc'),
    baseUrl: String(source?.base_url || ''),
    pathTemplate: String(source?.path_template || ''),
    timeframeMinutes: Number(source?.timeframe_minutes || 5),
    limit: Number(source?.limit || 100),
    protocols: Array.isArray(source?.protocols) ? source.protocols.join(',') : '',
    dexes: Array.isArray(source?.dexes) ? source.dexes.join(',') : '',
    name: String(source?.name || ''),
    setCurrent: false,
  };
}

function nextPoolSourceDraft(prev, key, value) {
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

function EditablePoolDataSourceRow({
  source,
  onUpdate,
  onCheck,
  onSwitch,
  onDisable,
  onEnable,
  onDelete,
  updating = false,
}) {
  const enabled = Boolean(source?.is_enabled);
  const current = Boolean(source?.is_current);
  const coverage = formatCoverage(source);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(() => poolSourceToDraft(source));

  useEffect(() => {
    setDraft(poolSourceToDraft(source));
    setEditing(false);
  }, [source?.id, source?.name, source?.source_type, source?.chain, source?.timeframe_minutes, source?.limit, source?.base_url, source?.path_template]);

  const updateDraft = useCallback((key, value) => {
    setDraft((prev) => nextPoolSourceDraft(prev, key, value));
  }, []);

  const submit = useCallback(() => {
    onUpdate?.(source, {
      ...draft,
      protocols: splitCSV(draft.protocols),
      dexes: splitCSV(draft.dexes),
    });
  }, [draft, onUpdate, source]);

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
        <button type="button" className="am-action-btn" onClick={() => setEditing((prev) => !prev)}>
          {editing ? '收起' : '编辑'}
        </button>
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

      {editing ? (
        <div className="am-form" style={{ flexBasis: '100%', marginTop: 12 }}>
          <label className="am-field">
            <span>来源类型</span>
            <CustomSelect
              value={draft.sourceType}
              onChange={(value) => updateDraft('sourceType', value)}
              options={POOL_SOURCE_TYPE_OPTIONS}
            />
          </label>
          <label className="am-field">
            <span>链</span>
            <CustomSelect
              value={draft.chain}
              onChange={(value) => updateDraft('chain', value)}
              options={CHAIN_OPTIONS}
            />
          </label>
          <label className="am-field am-field-grow">
            <span>Base URL</span>
            <input
              value={draft.baseUrl}
              onChange={(event) => updateDraft('baseUrl', event.target.value)}
              placeholder={draft.sourceType === 'poolm_top_fees' ? 'https://mapi.poolm.xyz' : 'http://localhost:8080'}
            />
          </label>
          <label className="am-field">
            <span>路径</span>
            <input
              value={draft.pathTemplate}
              onChange={(event) => updateDraft('pathTemplate', event.target.value)}
              placeholder={draft.sourceType === 'poolm_top_fees' ? '默认 /top-fees' : '/api/market/pools'}
            />
          </label>
          <label className="am-field">
            <span>窗口(分钟)</span>
            <input
              type="number"
              min="1"
              value={draft.timeframeMinutes}
              onChange={(event) => updateDraft('timeframeMinutes', event.target.value)}
            />
          </label>
          <label className="am-field">
            <span>Limit</span>
            <input
              type="number"
              min="1"
              value={draft.limit}
              onChange={(event) => updateDraft('limit', event.target.value)}
            />
          </label>
          <label className="am-field am-field-grow">
            <span>Protocols</span>
            <input
              value={draft.protocols}
              onChange={(event) => updateDraft('protocols', event.target.value)}
              placeholder="v3,v4"
            />
          </label>
          <label className="am-field am-field-grow">
            <span>DEX</span>
            <input
              value={draft.dexes}
              onChange={(event) => updateDraft('dexes', event.target.value)}
              placeholder="PancakeswapV3,UniswapV3,UniswapV4"
            />
          </label>
          <label className="am-field">
            <span>名称</span>
            <input
              value={draft.name}
              onChange={(event) => updateDraft('name', event.target.value)}
              placeholder="留空使用域名"
            />
          </label>
          <label className="am-field am-field-check">
            <input
              type="checkbox"
              checked={Boolean(draft.setCurrent)}
              onChange={(event) => updateDraft('setCurrent', event.target.checked)}
            />
            <span>保存后切为当前来源</span>
          </label>
          <div className="am-actions" style={{ marginLeft: 'auto' }}>
            <button
              type="button"
              className="am-action-btn"
              disabled={updating || !String(draft.baseUrl || '').trim()}
              onClick={submit}
            >
              {updating ? '保存中...' : '保存修改'}
            </button>
            <button
              type="button"
              className="am-action-btn"
              disabled={updating}
              onClick={() => {
                setDraft(poolSourceToDraft(source));
                setEditing(false);
              }}
            >
              取消
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function OKXConfigRow({
  config,
  onRename,
  onUpdate,
  onCheck,
  onSwitch,
  onDisable,
  onDisableNextMonth,
  onEnable,
  onDelete,
}) {
  const [renameValue, setRenameValue] = useState(okxConfigDisplayName(config));
  const [baseURL, setBaseURL] = useState(String(config?.base_url || '').trim());
  const [apiKey, setAPIKey] = useState('');
  const [secretKey, setSecretKey] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const unavailable = isOKXUnavailable(config);

  useEffect(() => {
    setRenameValue(okxConfigDisplayName(config));
    setBaseURL(String(config?.base_url || '').trim());
    setAPIKey('');
    setSecretKey('');
    setPassphrase('');
  }, [config?.id, config?.name, config?.base_url]);

  return (
    <div className="am-list-item am-list-item-wrap">
      <div style={{ minWidth: 0, flex: 1 }}>
        <div className="am-item-title">{okxConfigDisplayName(config)}</div>
        <div className="am-item-sub">
          {formatOKXConfigUrl(config)} / {config?.api_key_masked || 'no key'} / 延迟 {Number(config?.last_latency_ms || 0) > 0 ? `${Number(config.last_latency_ms)}ms` : '--'}
        </div>
        <div className="am-actions" style={{ justifyContent: 'flex-start' }}>
          <span className={unavailable ? 'am-badge am-badge-warn' : 'am-badge am-badge-ok'}>
            {unavailable ? '不可用' : '可用'}
          </span>
          {config?.is_current ? <span className="am-badge">当前配置</span> : null}
          {!config?.is_enabled ? <span className="am-badge am-badge-warn">已停用</span> : null}
          {config?.disabled_until ? (
            <span className="am-badge am-badge-warn">
              禁用至 {formatDateTime(config.disabled_until)}
              {config?.disabled_reason ? ` / ${formatOKXReason(config.disabled_reason)}` : ''}
            </span>
          ) : null}
        </div>
        <div className="am-item-sub">
          检测 {formatDateTime(config?.last_checked_at)} / 成功 {formatDateTime(config?.last_success_at)} / 连续失败 {Number(config?.consecutive_failures || 0)}
        </div>
        {config?.last_error ? <div className="am-error">{config.last_error}</div> : null}
        <div className="am-rename">
          <span>名称</span>
          <input
            value={renameValue}
            onChange={(event) => setRenameValue(event.target.value)}
            placeholder="配置名称"
          />
          <button
            type="button"
            className="am-action-btn"
            onClick={() => onRename?.(config, renameValue)}
          >
            改名
          </button>
        </div>
        <div className="am-okx-edit">
          <label>
            <span>Base URL</span>
            <input value={baseURL} onChange={(event) => setBaseURL(event.target.value)} />
          </label>
          <label>
            <span>API Key</span>
            <input value={apiKey} onChange={(event) => setAPIKey(event.target.value)} placeholder="留空不修改" autoComplete="off" />
          </label>
          <label>
            <span>Secret</span>
            <input type="password" value={secretKey} onChange={(event) => setSecretKey(event.target.value)} placeholder="留空不修改" autoComplete="new-password" />
          </label>
          <label>
            <span>Passphrase</span>
            <input type="password" value={passphrase} onChange={(event) => setPassphrase(event.target.value)} placeholder="留空不修改" autoComplete="new-password" />
          </label>
          <button
            type="button"
            className="am-action-btn"
            onClick={() => onUpdate?.(config, {
              name: renameValue,
              baseUrl: baseURL,
              apiKey,
              secretKey,
              passphrase,
            })}
          >
            保存连接
          </button>
        </div>
      </div>

      <div className="am-btn-group">
        <button type="button" className="am-action-btn" onClick={() => onCheck?.(config)}>检测</button>
        <button
          type="button"
          className="am-action-btn"
          disabled={unavailable || config?.is_current}
          onClick={() => onSwitch?.(config)}
        >
          切换
        </button>
        {unavailable ? (
          <button type="button" className="am-action-btn" onClick={() => onEnable?.(config)}>启用</button>
        ) : (
          <button type="button" className="am-action-btn" onClick={() => onDisable?.(config)}>停用</button>
        )}
        <button
          type="button"
          className="am-action-btn"
          disabled={unavailable}
          onClick={() => onDisableNextMonth?.(config)}
        >
          禁用到下月
        </button>
        <button type="button" className="am-action-btn" onClick={() => onDelete?.(config)}>删除</button>
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
  const [systemSection, setSystemSection] = useState('config');
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
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [taskStatusFilter, setTaskStatusFilter] = useState('all');

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
  const [poolSourceUpdatingId, setPoolSourceUpdatingId] = useState(0);
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

  const [okxData, setOKXData] = useState(null);
  const [okxLoading, setOKXLoading] = useState(false);
  const [okxError, setOKXError] = useState('');
  const [okxAdding, setOKXAdding] = useState(false);
  const [okxAddDraft, setOKXAddDraft] = useState({
    name: '',
    baseUrl: 'https://www.okx.com/api/v6/dex/aggregator',
    apiKey: '',
    secretKey: '',
    passphrase: '',
    setCurrent: false,
  });

  const [privateZapData, setPrivateZapData] = useState(null);
  const [privateZapLoading, setPrivateZapLoading] = useState(false);
  const [privateZapError, setPrivateZapError] = useState('');
  const [invalidatingKey, setInvalidatingKey] = useState('');
  const [confirmAction, setConfirmAction] = useState(null);
  const [confirmBusy, setConfirmBusy] = useState(false);

  const userPositionsList = useMemo(
    () => (Array.isArray(userPositions?.positions) ? userPositions.positions : []),
    [userPositions?.positions]
  );
  const isReady = hasInitData && isAdmin;

  const normalizedQuery = String(query || '').trim().toLowerCase();

  const filteredOnlineUsers = useMemo(() => {
    if (!normalizedQuery) return onlineUsers;
    return onlineUsers.filter((user) => {
      const hay = [
        user?.username,
        user?.first_name,
        user?.last_name,
        user?.telegram_id,
        user?.user_id,
      ].map((v) => String(v || '').toLowerCase()).join(' ');
      return hay.includes(normalizedQuery);
    });
  }, [onlineUsers, normalizedQuery]);

  const filteredActiveTasks = useMemo(() => {
    return activeTasks.filter((task) => {
      if (taskStatusFilter !== 'all') {
        if (taskStatusFilter === 'paused') {
          if (!task?.paused) return false;
        } else if (String(task?.status || '').toLowerCase() !== taskStatusFilter) {
          return false;
        }
      }
      if (!normalizedQuery) return true;
      const hay = [
        task?.username,
        task?.first_name,
        task?.last_name,
        task?.telegram_id,
        task?.user_id,
        task?.task_id,
        task?.token0_symbol,
        task?.token1_symbol,
      ].map((v) => String(v || '').toLowerCase()).join(' ');
      return hay.includes(normalizedQuery);
    });
  }, [activeTasks, normalizedQuery, taskStatusFilter]);

  const rpcHealthSummary = useMemo(() => deriveRpcHealthSummary(rpcData), [rpcData]);
  const poolSourceHealthSummary = useMemo(() => derivePoolSourceHealthSummary(poolSourceData), [poolSourceData]);
  const okxHealthSummary = useMemo(() => deriveOKXHealthSummary(okxData), [okxData]);

  const openUserDrawer = useCallback((user) => {
    if (!user) return;
    setSelectedUser({
      user_id: user.user_id,
      telegram_id: user.telegram_id,
      username: user.username,
      first_name: user.first_name,
      last_name: user.last_name,
    });
    setUserPositions(null);
    setDrawerOpen(true);
  }, []);

  const closeUserDrawer = useCallback(() => {
    setDrawerOpen(false);
  }, []);

  const showNotice = useCallback((message) => {
    setNotice(message);
    setTimeout(() => setNotice(''), 3000);
  }, []);

  const closeConfirmAction = useCallback(() => {
    if (confirmBusy) return;
    setConfirmAction(null);
  }, [confirmBusy]);

  const runConfirmAction = useCallback(async () => {
    const action = confirmAction?.action;
    if (confirmBusy || typeof action !== 'function') return;
    setConfirmBusy(true);
    try {
      await action();
    } finally {
      setConfirmBusy(false);
      setConfirmAction(null);
    }
  }, [confirmAction, confirmBusy]);

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

  const loadOKXPool = useCallback(async () => {
    if (!isReady) return;
    setOKXLoading(true);
    setOKXError('');
    try {
      const response = await fetchAdminOKXPool({ apiBaseUrl, initData });
      setOKXData(response || null);
    } catch (err) {
      setOKXError(errorText(err));
    } finally {
      setOKXLoading(false);
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
      loadOKXPool();
      loadPrivateZap();
    }
  }, [activeTab, loadActiveTasks, loadOKXPool, loadOnlineUsers, loadPoolDataSources, loadPrivateZap, loadRPCPool, loadSystemConfig]);

  useEffect(() => {
    if (!isReady || activeTab !== 'operations') return undefined;
    const timer = setInterval(() => {
      loadOnlineUsers();
      loadActiveTasks();
      if (selectedUser?.user_id) loadUserPositions(selectedUser.user_id);
    }, Math.max(2, Number(refreshInterval || 10)) * 1000);
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
    loadOKXPool();
    loadPrivateZap();
  }, [
    activeTab,
    loadActiveTasks,
    loadOKXPool,
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
    setPoolSourceAddDraft((prev) => nextPoolSourceDraft(prev, key, value));
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

  const handleUpdatePoolSource = useCallback(async (source, draft) => {
    if (!isReady) return;
    const sourceId = Number(source?.id);
    const baseUrl = String(draft?.baseUrl || '').trim();
    if (!sourceId) {
      setPoolSourceError('缺少池子数据源 ID');
      return;
    }
    if (!baseUrl) {
      setPoolSourceError('请先填写 Base URL');
      return;
    }
    setPoolSourceUpdatingId(sourceId);
    setPoolSourceError('');
    try {
      await updateAdminPoolDataSource({
        apiBaseUrl,
        initData,
        sourceId,
        name: String(draft.name || '').trim(),
        sourceType: draft.sourceType,
        chain: draft.chain,
        timeframeMinutes: Number(draft.timeframeMinutes) || 5,
        limit: Number(draft.limit) || 100,
        baseUrl,
        pathTemplate: String(draft.pathTemplate || '').trim(),
        protocols: Array.isArray(draft.protocols) ? draft.protocols : [],
        dexes: Array.isArray(draft.dexes) ? draft.dexes : [],
        setCurrent: Boolean(draft.setCurrent),
      });
      await loadPoolDataSources();
      showNotice('池子数据源已保存');
    } catch (err) {
      setPoolSourceError(errorText(err));
    } finally {
      setPoolSourceUpdatingId(0);
    }
  }, [apiBaseUrl, initData, isReady, loadPoolDataSources, showNotice]);

  const runOKXAction = useCallback(async (runner, successMessage) => {
    try {
      await runner();
      await loadOKXPool();
      if (successMessage) showNotice(successMessage);
    } catch (err) {
      setOKXError(errorText(err));
    }
  }, [loadOKXPool, showNotice]);

  const handleAddOKX = useCallback(async () => {
    if (!isReady) return;
    const baseUrl = String(okxAddDraft.baseUrl || '').trim();
    const apiKey = String(okxAddDraft.apiKey || '').trim();
    const secretKey = String(okxAddDraft.secretKey || '').trim();
    const passphrase = String(okxAddDraft.passphrase || '').trim();
    if (!baseUrl || !apiKey || !secretKey || !passphrase) {
      setOKXError('请填写 Base URL、API Key、Secret 和 Passphrase');
      return;
    }
    setOKXAdding(true);
    setOKXError('');
    try {
      await addAdminOKXConfig({
        apiBaseUrl,
        initData,
        name: String(okxAddDraft.name || '').trim(),
        baseUrl,
        apiKey,
        secretKey,
        passphrase,
        setCurrent: Boolean(okxAddDraft.setCurrent),
      });
      setOKXAddDraft((prev) => ({
        ...prev,
        name: '',
        apiKey: '',
        secretKey: '',
        passphrase: '',
        setCurrent: false,
      }));
      await loadOKXPool();
      showNotice('OKX 配置已添加');
    } catch (err) {
      setOKXError(errorText(err));
    } finally {
      setOKXAdding(false);
    }
  }, [apiBaseUrl, initData, isReady, loadOKXPool, okxAddDraft, showNotice]);

  const handleInvalidatePrivateZap = useCallback((chain, kind) => {
    if (!isReady) return;
    setConfirmAction({
      title: '清理 Private Zap',
      message: `确认清理 ${formatChain(chain)} / ${formatPrivateZapKind(kind)} 绑定吗？`,
      confirmText: '清理',
      danger: true,
      action: async () => {
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
      },
    });
  }, [apiBaseUrl, initData, isReady, loadPrivateZap, showNotice]);

  const rpcGroups = useMemo(
    () => (Array.isArray(rpcData?.groups) ? rpcData.groups : []),
    [rpcData?.groups]
  );
  const poolSourceGroups = useMemo(
    () => (Array.isArray(poolSourceData?.groups) ? poolSourceData.groups : []),
    [poolSourceData?.groups]
  );
  const okxConfigs = useMemo(
    () => (Array.isArray(okxData?.configs) ? okxData.configs : []),
    [okxData?.configs]
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
      subtitle={activeTab === 'operations' ? '运行管理' : activeTab === 'access' ? '授权公告' : '系统'}
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
              className={`am-tab-btn ${activeTab === 'access' ? 'active' : ''}`}
              onClick={() => setActiveTab('access')}
            >
              <KeyRound size={12} />
              授权公告
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

          {activeTab === 'access' ? (
            <AdminAccessWorkbench
              apiBaseUrl={apiBaseUrl}
              initData={initData}
              hasInitData={hasInitData}
              onNotice={showNotice}
            />
          ) : null}

          {activeTab === 'operations' ? (
            <>
              <div className="am-stat-row">
                <AdminStatChip
                  label="在线用户"
                  value={onlineUsers.length}
                  tone={onlineUsers.length > 0 ? 'ok' : 'idle'}
                  pulse={onlineUsers.length > 0}
                  hint={onlineLoading ? '同步中…' : `${filteredOnlineUsers.length} 命中`}
                />
                <AdminStatChip
                  label="活跃任务"
                  value={activeTasks.length}
                  tone={activeTasks.length > 0 ? 'accent' : 'idle'}
                  hint={taskLoading ? '同步中…' : `${filteredActiveTasks.length} 命中`}
                />
                <AdminStatChip
                  label="RPC 节点"
                  value={rpcHealthSummary.value}
                  tone={rpcHealthSummary.tone}
                  hint={rpcHealthSummary.hint}
                  onClick={() => { setActiveTab('system'); setSystemSection('rpc'); }}
                />
                <AdminStatChip
                  label="池子源"
                  value={poolSourceHealthSummary.value}
                  tone={poolSourceHealthSummary.tone}
                  hint={poolSourceHealthSummary.hint}
                  onClick={() => { setActiveTab('system'); setSystemSection('pool_sources'); }}
                />
                <AdminStatChip
                  label="OKX 配置"
                  value={okxHealthSummary.value}
                  tone={okxHealthSummary.tone}
                  hint={okxHealthSummary.hint}
                  onClick={() => { setActiveTab('system'); setSystemSection('okx'); }}
                />
              </div>

              <div className="am-search-bar">
                <Search size={14} />
                <input
                  type="text"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="搜索 @username / TG ID / 任务 / 交易对"
                />
                {query ? (
                  <button type="button" className="am-action-btn" onClick={() => setQuery('')} style={{ padding: '4px 9px', fontSize: 10.5 }}>清空</button>
                ) : null}
              </div>

              <div className="am-filter-pills">
                {TASK_STATUS_FILTERS.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    className={`am-filter-pill ${taskStatusFilter === item.key ? 'active' : ''}`}
                    onClick={() => setTaskStatusFilter(item.key)}
                  >
                    {item.label}
                  </button>
                ))}
              </div>

              <div className="am-ops-grid">
                <section>
                  <div className="am-section-cap">
                    <span>在线用户 · {filteredOnlineUsers.length}</span>
                    <button type="button" disabled={onlineLoading} onClick={loadOnlineUsers}>
                      {onlineLoading ? '刷新中…' : '刷新'}
                    </button>
                  </div>
                  {onlineError ? <div className="am-error">{onlineError}</div> : null}
                  {filteredOnlineUsers.length > 0 ? (
                    <div className="am-dense-list">
                      {filteredOnlineUsers.map((user) => {
                        const selected = Number(user?.user_id) === Number(selectedUser?.user_id);
                        return (
                          <button
                            type="button"
                            key={user.user_id || user.telegram_id}
                            className={`am-dense-row ${selected ? 'selected' : ''}`}
                            onClick={() => openUserDrawer(user)}
                          >
                            <AdminStatusDot tone="ok" pulse size="sm" />
                            <div className="am-dense-main">
                              <div className="am-dense-title">{formatUserLabel(user)}</div>
                              <div className="am-dense-sub">
                                TG {user?.telegram_id || '--'} · ID {user?.user_id || '--'} · {formatDateTime(user?.updated_at)}
                              </div>
                            </div>
                            <div className="am-dense-end">
                              <div className="am-dense-end-value">{Number(user?.total_tasks || 0)}</div>
                              <div className="am-dense-end-label">任务</div>
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  ) : (
                    <EmptyState text={onlineLoading
                      ? '正在加载在线用户...'
                      : (onlineUsers.length > 0 ? '没有匹配的在线用户' : '暂无在线用户')}
                    />
                  )}
                </section>

                <section>
                  <div className="am-section-cap">
                    <span>活跃任务 · {filteredActiveTasks.length}</span>
                    <button type="button" disabled={taskLoading} onClick={loadActiveTasks}>
                      {taskLoading ? '刷新中…' : '刷新'}
                    </button>
                  </div>
                  {taskError ? <div className="am-error">{taskError}</div> : null}
                  {filteredActiveTasks.length > 0 ? (
                    <div className="am-dense-list">
                      {filteredActiveTasks.map((task) => {
                        const tone = statusTone(task.status);
                        return (
                          <button
                            type="button"
                            key={task.task_id || `${task.user_id}:${task.pool_id}`}
                            className="am-dense-row"
                            onClick={() => openUserDrawer({
                              user_id: task.user_id,
                              telegram_id: task.telegram_id,
                              username: task.username,
                              first_name: task.first_name,
                              last_name: task.last_name,
                            })}
                          >
                            <AdminStatusDot tone={tone} pulse={tone === 'ok' || tone === 'accent'} size="sm" />
                            <div className="am-dense-main">
                              <div className="am-dense-title">
                                {formatTaskPair(task)}
                                <span className={`am-tag tone-${tone}`}>{formatStatus(task.status)}</span>
                                {task.paused ? <span className="am-tag tone-warn">暂停</span> : null}
                              </div>
                              <div className="am-dense-sub">
                                {formatUserLabel(task)} · #{task.task_id || '--'}
                              </div>
                            </div>
                            <div className="am-dense-end">
                              <div className="am-dense-end-value">{Number.isFinite(Number(task?.amount_usdt)) ? `$${Number(task.amount_usdt).toFixed(2)}` : '--'}</div>
                              <div className="am-dense-end-label">持仓</div>
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  ) : (
                    <EmptyState text={taskLoading
                      ? '正在加载活跃任务...'
                      : (activeTasks.length > 0 ? '没有匹配的活跃任务' : '暂无活跃任务')}
                    />
                  )}
                </section>
              </div>
            </>
          ) : null}

          {activeTab === 'system' ? (
            <>
              <div className="am-stat-row">
                <AdminStatChip
                  label="基础配置"
                  value={systemConfig ? '已加载' : '--'}
                  tone={systemConfig ? 'ok' : 'idle'}
                  hint={systemConfig ? '可调整' : '尚未加载'}
                />
                <AdminStatChip
                  label="RPC 节点"
                  value={rpcHealthSummary.value}
                  tone={rpcHealthSummary.tone}
                  hint={rpcHealthSummary.hint}
                />
                <AdminStatChip
                  label="池子源"
                  value={poolSourceHealthSummary.value}
                  tone={poolSourceHealthSummary.tone}
                  hint={poolSourceHealthSummary.hint}
                />
                <AdminStatChip
                  label="OKX 配置"
                  value={okxHealthSummary.value}
                  tone={okxHealthSummary.tone}
                  hint={okxHealthSummary.hint}
                />
                <AdminStatChip
                  label="Private Zap"
                  value={privateZapChains.length}
                  tone={privateZapChains.length > 0 ? 'accent' : 'idle'}
                  hint={`${privateZapChains.length} 条链`}
                />
              </div>

              <div className="am-system-tabs" role="tablist" aria-label="系统配置分类">
                {ADMIN_SYSTEM_SECTIONS.map((section) => (
                  <button
                    key={section.key}
                    type="button"
                    role="tab"
                    aria-selected={systemSection === section.key}
                    className={`am-tab-btn ${systemSection === section.key ? 'active' : ''}`}
                    onClick={() => setSystemSection(section.key)}
                  >
                    {section.label}
                  </button>
                ))}
              </div>

              {systemSection === 'config' ? (
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
              ) : null}

              {systemSection === 'rpc' ? (
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
                    <CustomSelect
                      value={rpcAddDraft.chain}
                      onChange={(value) => setRpcAddDraft((prev) => ({ ...prev, chain: value }))}
                      options={CHAIN_OPTIONS}
                    />
                  </label>
                  <label className="am-field">
                    <span>协议</span>
                    <CustomSelect
                      value={rpcAddDraft.transport}
                      onChange={(value) => setRpcAddDraft((prev) => ({ ...prev, transport: value }))}
                      options={TRANSPORT_OPTIONS}
                    />
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
                                    setConfirmAction({
                                      title: '删除 RPC 节点',
                                      message: `确认删除节点 "${endpointDisplayName(item)}" 吗？`,
                                      confirmText: '删除',
                                      danger: true,
                                      action: () => runRPCAction(
                                        () => deleteAdminRPCEndpoint({ apiBaseUrl, initData, endpointId: item?.id }),
                                        '节点已删除'
                                      ),
                                    });
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
              ) : null}

              {systemSection === 'pool_sources' ? (
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
                    <CustomSelect
                      value={poolSourceAddDraft.sourceType}
                      onChange={(value) => updatePoolSourceDraft('sourceType', value)}
                      options={POOL_SOURCE_TYPE_OPTIONS}
                    />
                  </label>
                  <label className="am-field">
                    <span>链</span>
                    <CustomSelect
                      value={poolSourceAddDraft.chain}
                      onChange={(value) => updatePoolSourceDraft('chain', value)}
                      options={CHAIN_OPTIONS}
                    />
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
                          <EditablePoolDataSourceRow
                            key={source.id || `${group?.chain}:${group?.timeframe_minutes}:${source?.base_url}`}
                            source={source}
                            updating={poolSourceUpdatingId === Number(source?.id)}
                            onUpdate={handleUpdatePoolSource}
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
                              setConfirmAction({
                                title: '删除池子源',
                                message: `确认删除池子源 "${poolSourceDisplayName(item)}" 吗？`,
                                confirmText: '删除',
                                danger: true,
                                action: () => runPoolSourceAction(
                                  () => deleteAdminPoolDataSource({ apiBaseUrl, initData, sourceId: item?.id }),
                                  '池子数据源已删除'
                                ),
                              });
                            }}
                          />
                        )) : <EmptyState text="当前仅使用 ENV 兜底来源" />}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
              ) : null}

              {systemSection === 'okx' ? (
              <div className="am-card">
                <div className="am-card-header">
                  <div className="am-card-title">OKX 配置池</div>
                  <button type="button" className="am-action-btn" disabled={okxLoading} onClick={loadOKXPool}>
                    <RefreshCw size={12} className={okxLoading ? 'animate-spin' : undefined} />
                    刷新
                  </button>
                </div>
                {okxError ? <div className="am-error">{okxError}</div> : null}
                <div className="am-okx-summary">
                  <div>
                    <span>当前来源</span>
                    <strong>{formatOKXSource(okxData?.effective_source)}</strong>
                    {okxData?.effective_config_id ? <em>#{okxData.effective_config_id}</em> : null}
                  </div>
                  <div>
                    <span>当前 Base URL</span>
                    <strong>{okxData?.effective_base_url_masked || okxData?.effective_base_url || '--'}</strong>
                  </div>
                  <div>
                    <span>当前 API Key</span>
                    <strong>{okxData?.effective_api_key_masked || '--'}</strong>
                  </div>
                  <div>
                    <span>.env 备用</span>
                    <strong>{okxData?.env_base_url_masked || okxData?.env_base_url || '--'}</strong>
                  </div>
                </div>
                <div className="am-form">
                  <label className="am-field">
                    <span>名称</span>
                    <input
                      value={okxAddDraft.name}
                      onChange={(event) => setOKXAddDraft((prev) => ({ ...prev, name: event.target.value }))}
                      placeholder="可选，留空使用域名"
                    />
                  </label>
                  <label className="am-field am-field-grow">
                    <span>Base URL</span>
                    <input
                      value={okxAddDraft.baseUrl}
                      onChange={(event) => setOKXAddDraft((prev) => ({ ...prev, baseUrl: event.target.value }))}
                      placeholder="https://www.okx.com/api/v6/dex/aggregator"
                    />
                  </label>
                  <label className="am-field">
                    <span>API Key</span>
                    <input
                      value={okxAddDraft.apiKey}
                      onChange={(event) => setOKXAddDraft((prev) => ({ ...prev, apiKey: event.target.value }))}
                      autoComplete="off"
                    />
                  </label>
                  <label className="am-field">
                    <span>Secret</span>
                    <input
                      type="password"
                      value={okxAddDraft.secretKey}
                      onChange={(event) => setOKXAddDraft((prev) => ({ ...prev, secretKey: event.target.value }))}
                      autoComplete="new-password"
                    />
                  </label>
                  <label className="am-field">
                    <span>Passphrase</span>
                    <input
                      type="password"
                      value={okxAddDraft.passphrase}
                      onChange={(event) => setOKXAddDraft((prev) => ({ ...prev, passphrase: event.target.value }))}
                      autoComplete="new-password"
                    />
                  </label>
                  <label className="am-field am-field-check">
                    <input
                      type="checkbox"
                      checked={Boolean(okxAddDraft.setCurrent)}
                      onChange={(event) => setOKXAddDraft((prev) => ({ ...prev, setCurrent: event.target.checked }))}
                    />
                    <span>添加后切为当前配置</span>
                  </label>
                </div>
                <div className="am-actions">
                  <span className="am-item-sub">
                    DB 配置为空或不可用时，系统会继续使用 .env OKX 配置。
                  </span>
                  <button type="button" className="am-action-btn" disabled={okxAdding} onClick={handleAddOKX}>
                    {okxAdding ? '添加中...' : '添加 OKX 配置'}
                  </button>
                </div>
                {okxLoading && okxConfigs.length === 0 ? <div className="panel-loading">正在加载 OKX 配置池...</div> : null}
                {!okxLoading && okxConfigs.length === 0 ? <EmptyState text="暂无 OKX DB 配置，当前使用 .env 兜底。" /> : null}
                <div className="am-list">
                  {okxConfigs.map((config) => (
                    <OKXConfigRow
                      key={config.id || `${config?.base_url}:${config?.api_key_masked}`}
                      config={config}
                      onRename={(item, value) => runOKXAction(
                        () => renameAdminOKXConfig({ apiBaseUrl, initData, configId: item?.id, name: String(value || '').trim() }),
                        'OKX 配置名称已更新'
                      )}
                      onUpdate={(item, value) => runOKXAction(
                        () => updateAdminOKXConfig({
                          apiBaseUrl,
                          initData,
                          configId: item?.id,
                          name: String(value?.name || '').trim(),
                          baseUrl: String(value?.baseUrl || '').trim(),
                          apiKey: String(value?.apiKey || '').trim(),
                          secretKey: String(value?.secretKey || '').trim(),
                          passphrase: String(value?.passphrase || '').trim(),
                        }),
                        'OKX 配置连接信息已保存'
                      )}
                      onCheck={(item) => runOKXAction(
                        () => checkAdminOKXConfig({ apiBaseUrl, initData, configId: item?.id }),
                        'OKX 配置检测完成'
                      )}
                      onSwitch={(item) => runOKXAction(
                        () => switchAdminOKXConfig({ apiBaseUrl, initData, configId: item?.id }),
                        '当前 OKX 配置已切换'
                      )}
                      onDisable={(item) => runOKXAction(
                        () => disableAdminOKXConfig({ apiBaseUrl, initData, configId: item?.id }),
                        'OKX 配置已停用'
                      )}
                      onDisableNextMonth={(item) => runOKXAction(
                        () => disableAdminOKXConfigNextMonth({ apiBaseUrl, initData, configId: item?.id }),
                        'OKX 配置已禁用到下月'
                      )}
                      onEnable={(item) => runOKXAction(
                        () => enableAdminOKXConfig({ apiBaseUrl, initData, configId: item?.id }),
                        'OKX 配置已启用'
                      )}
                      onDelete={(item) => {
                        setConfirmAction({
                          title: '删除 OKX 配置',
                          message: `确认删除 OKX 配置 "${okxConfigDisplayName(item)}" 吗？`,
                          confirmText: '删除',
                          danger: true,
                          action: () => runOKXAction(
                            () => deleteAdminOKXConfig({ apiBaseUrl, initData, configId: item?.id }),
                            'OKX 配置已删除'
                          ),
                        });
                      }}
                    />
                  ))}
                </div>
              </div>
              ) : null}

              {systemSection === 'private_zap' ? (
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
              ) : null}
            </>
          ) : null}
        </div>
      ) : null}
      <ConfirmDialog
        open={Boolean(confirmAction)}
        title={confirmAction?.title}
        message={confirmAction?.message}
        confirmText={confirmAction?.confirmText}
        danger={Boolean(confirmAction?.danger)}
        loading={confirmBusy}
        onConfirm={runConfirmAction}
        onCancel={closeConfirmAction}
      />
      <AdminDrawer
        open={drawerOpen}
        title={selectedUser ? formatUserLabel(selectedUser) : '用户详情'}
        subtitle={selectedUser ? `TG ${selectedUser?.telegram_id || '--'} · 用户 ID ${selectedUser?.user_id || '--'}` : ''}
        headerExtra={selectedUser?.user_id ? (
          <button
            type="button"
            className="am-action-btn"
            disabled={positionsLoading}
            onClick={() => loadUserPositions(selectedUser.user_id)}
            style={{ padding: '6px 10px' }}
          >
            <RefreshCw size={12} className={positionsLoading ? 'animate-spin' : undefined} />
            刷新
          </button>
        ) : null}
        onClose={closeUserDrawer}
      >
        {selectedUser ? (
          <>
            <div className="am-drawer-meta">
              <div>
                <div className="am-drawer-meta-key">TG ID</div>
                <div className="am-drawer-meta-val">{selectedUser?.telegram_id || '--'}</div>
              </div>
              <div>
                <div className="am-drawer-meta-key">用户 ID</div>
                <div className="am-drawer-meta-val">{selectedUser?.user_id || '--'}</div>
              </div>
              <div className="am-drawer-meta-row-2">
                <div className="am-drawer-meta-key">钱包</div>
                <div className="am-drawer-meta-val">{userPositions?.wallet?.address || '--'}</div>
              </div>
              <div>
                <div className="am-drawer-meta-key">BNB 余额</div>
                <div className="am-drawer-meta-val">{userPositions?.wallet?.bnb_balance || '--'}</div>
              </div>
              <div>
                <div className="am-drawer-meta-key">活跃仓位</div>
                <div className="am-drawer-meta-val">{userPositionsList.length}</div>
              </div>
            </div>

            {positionsError ? <div className="am-error">{positionsError}</div> : null}
            {positionsLoading && userPositionsList.length === 0 ? (
              <div className="panel-loading">正在加载用户仓位...</div>
            ) : null}
            {!positionsLoading && userPositionsList.length === 0 && !positionsError ? (
              <EmptyState text="当前用户没有活跃仓位。" />
            ) : null}

            {userPositionsList.length > 0 ? (
              <div className="am-dense-list">
                {userPositionsList.map((position) => {
                  const tone = statusTone(position?.status);
                  return (
                    <div
                      key={[position?.chain, position?.pool_id, position?.position_id, position?.task_id].join(':')}
                      className="am-dense-row"
                      style={{ cursor: 'default' }}
                    >
                      <AdminStatusDot tone={tone} size="sm" />
                      <div className="am-dense-main">
                        <div className="am-dense-title">
                          {formatPositionPair(position)}
                          <span className={`am-tag tone-${tone}`}>{formatStatus(position?.status)}</span>
                        </div>
                        <div className="am-dense-sub">
                          {formatChain(position?.chain)} · Task #{position?.task_id || '--'} · {shortAddress(position?.wallet_address || '')}
                        </div>
                      </div>
                      <div className="am-dense-end">
                        <div className="am-dense-end-value">{formatUsd(position?.totals?.wallet_usd || position?.position_amount_usd || 0)}</div>
                        <div className="am-dense-end-label">仓位</div>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : null}
          </>
        ) : (
          <EmptyState text="未选择用户。" />
        )}
      </AdminDrawer>
    </PanelShell>
  );
}
