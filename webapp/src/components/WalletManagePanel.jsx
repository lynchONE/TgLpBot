import { useCallback, useEffect, useMemo, useState } from 'react';
import { Copy, Download, Edit2, KeyRound, Plus, RefreshCw, ShieldCheck, Star, Trash2, WalletCards } from 'lucide-react';
import { fetchWallets, walletCRUD } from '../api';
import PanelShell, { EmptyState, MetricCard } from './PanelShell';
import { shortAddress } from '../utils';
import ConfirmDialog from './ConfirmDialog';

function parseBalance(value) {
  if (value === 'N/A' || value === undefined || value === null) return 0;
  const num = Number(value);
  return Number.isFinite(num) ? num : 0;
}

function formatBalance(value, digits = 4) {
  if (value === 'N/A' || value === undefined || value === null) return '-';
  const num = Number(value);
  if (!Number.isFinite(num)) return '-';
  return num.toFixed(digits);
}

export default function WalletManagePanel({ apiBaseUrl, initData, hasInitData = true, chain = 'bsc', embedded = false }) {
  const [wallets, setWallets] = useState([]);
  const [nativeSymbol, setNativeSymbol] = useState('BNB');
  const [stableSymbol, setStableSymbol] = useState('USDT');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [copiedAddr, setCopiedAddr] = useState('');

  const [crudAction, setCrudAction] = useState(null);
  const [crudForm, setCrudForm] = useState({ name: '', privateKey: '', walletId: null });
  const [crudLoading, setCrudLoading] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState(null);

  const load = useCallback(async () => {
    if (!initData) return;
    setLoading(true);
    setError('');
    try {
      const resp = await fetchWallets({ apiBaseUrl, initData, chain });
      setWallets(resp?.wallets || []);
      setNativeSymbol(resp?.native_symbol || 'BNB');
      setStableSymbol(resp?.stable_symbol || 'USDT');
    } catch (e) {
      setError(String(e?.message || e));
      setWallets([]);
    } finally {
      setLoading(false);
    }
  }, [apiBaseUrl, initData, chain]);

  useEffect(() => {
    if (hasInitData) {
      load();
      setCrudAction(null);
      setError('');
    }
  }, [load, hasInitData]);

  const totalNative = useMemo(
    () => wallets.reduce((sum, wallet) => sum + parseBalance(wallet.native_balance), 0),
    [wallets],
  );
  const totalStable = useMemo(
    () => wallets.reduce((sum, wallet) => sum + parseBalance(wallet.stable_balance), 0),
    [wallets],
  );
  const defaultWallet = useMemo(() => wallets.find((wallet) => wallet.is_default), [wallets]);

  const copyAddress = async (addr) => {
    try {
      await navigator.clipboard.writeText(addr);
      setCopiedAddr(addr);
      setTimeout(() => setCopiedAddr(''), 2000);
    } catch {
      // Clipboard access can be unavailable in some embedded browsers.
    }
  };

  const handleCrudSubmit = async (e) => {
    e.preventDefault();
    setCrudLoading(true);
    setError('');
    try {
      await walletCRUD({
        apiBaseUrl,
        initData,
        action: crudAction,
        privateKey: crudForm.privateKey,
        name: crudForm.name,
        walletId: crudForm.walletId,
      });
      setCrudAction(null);
      setCrudForm({ name: '', privateKey: '', walletId: null });
      await load();
    } catch (err) {
      setError(String(err?.message || err));
    } finally {
      setCrudLoading(false);
    }
  };

  const handleAction = async (action, wallet) => {
    if (action === 'delete') {
      setDeleteTarget(wallet);
      return;
    }
    if (action === 'set_default') {
      setLoading(true);
      setError('');
      try {
        await walletCRUD({ apiBaseUrl, initData, action, walletId: wallet.id });
        await load();
      } catch (err) {
        setError(String(err?.message || err));
        setLoading(false);
      }
      return;
    }
    if (action === 'rename') {
      setCrudAction('rename');
      setCrudForm({ name: wallet.name || '', privateKey: '', walletId: wallet.id });
    }
  };

  const confirmDelete = async () => {
    const wallet = deleteTarget;
    if (!wallet) return;
    setDeleteTarget(null);
    setLoading(true);
    setError('');
    try {
      await walletCRUD({ apiBaseUrl, initData, action: 'delete', walletId: wallet.id });
      await load();
    } catch (err) {
      setError(String(err?.message || err));
      setLoading(false);
    }
  };

  const renderCrudForm = () => {
    if (!crudAction) return null;
    const title = crudAction === 'import' ? '导入钱包' : crudAction === 'create' ? '创建钱包' : '重命名钱包';
    const description = crudAction === 'import'
      ? '使用私钥添加已有地址'
      : crudAction === 'create'
        ? '系统会生成一个新钱包并加入账户'
        : '只修改展示名称，不影响链上地址';
    const Icon = crudAction === 'import' ? KeyRound : crudAction === 'create' ? Plus : Edit2;

    return (
      <div className="am-card wallet-edit-card">
        <div className="wallet-edit-head">
          <div className="wallet-icon-chip">
            <Icon size={16} />
          </div>
          <div>
            <div className="am-card-title">{title}</div>
            <div className="wallet-muted">{description}</div>
          </div>
        </div>
        <form onSubmit={handleCrudSubmit} className="am-form wallet-form">
          {crudAction === 'import' ? (
            <label className="am-field am-field-grow">
              <span>私钥 (Hex)</span>
              <input
                type="text"
                value={crudForm.privateKey}
                onChange={(e) => setCrudForm({ ...crudForm, privateKey: e.target.value })}
                placeholder="输入私钥..."
                required
              />
            </label>
          ) : null}
          <label className="am-field am-field-grow">
            <span>钱包名称</span>
            <input
              type="text"
              value={crudForm.name}
              onChange={(e) => setCrudForm({ ...crudForm, name: e.target.value })}
              placeholder="如: 常用钱包1"
              required
            />
          </label>
          <div className="wallet-form-actions">
            <button type="button" onClick={() => setCrudAction(null)} className="am-action-btn">
              取消
            </button>
            <button type="submit" disabled={crudLoading} className="config-save-btn wallet-submit-btn">
              {crudLoading ? '处理中...' : '确定'}
            </button>
          </div>
        </form>
      </div>
    );
  };

  const body = (
    <div className="am-stack">
      {!hasInitData ? <EmptyState text="请先完成 Telegram 登录后查看钱包" /> : null}
      {error ? <div className="am-error">{error}</div> : null}

      <div className="am-metric-row">
        <MetricCard label="钱包数量" value={`${wallets.length} 个`} tone="strong" />
        <MetricCard label={`${nativeSymbol} 总计`} value={totalNative.toFixed(4)} />
        <MetricCard label={`${stableSymbol} 总计`} value={totalStable.toFixed(2)} />
        <MetricCard label="默认钱包" value={defaultWallet ? (defaultWallet.name || shortAddress(defaultWallet.address)) : '未设置'} />
      </div>

      {!crudAction ? (
        <div className="wallet-action-grid">
          <button
            type="button"
            onClick={() => {
              setCrudAction('create');
              setCrudForm({ name: '', privateKey: '', walletId: null });
            }}
            className="wallet-action-card"
          >
            <span className="wallet-icon-chip"><Plus size={16} /></span>
            <strong>创建新钱包</strong>
            <span>自动生成并加入当前账户</span>
          </button>
          <button
            type="button"
            onClick={() => {
              setCrudAction('import');
              setCrudForm({ name: '', privateKey: '', walletId: null });
            }}
            className="wallet-action-card"
          >
            <span className="wallet-icon-chip wallet-icon-chip--amber"><Download size={16} /></span>
            <strong>导入钱包</strong>
            <span>用私钥添加已有地址</span>
          </button>
        </div>
      ) : null}

      {renderCrudForm()}

      {loading && wallets.length === 0 ? (
        <div className="panel-loading">加载中...</div>
      ) : wallets.length === 0 ? (
        <EmptyState text="暂无钱包，请先创建或导入一个钱包" />
      ) : (
        <div className="wallet-list wallet-list--asset">
          {wallets.map((wallet) => (
            <WalletCard
              key={wallet.id || wallet.address}
              wallet={wallet}
              nativeSymbol={nativeSymbol}
              stableSymbol={stableSymbol}
              copied={copiedAddr === wallet.address}
              onCopy={copyAddress}
              onAction={handleAction}
            />
          ))}
        </div>
      )}

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="删除钱包"
        message={`确定要删除钱包 ${deleteTarget?.name || shortAddress(deleteTarget?.address || '')} 吗？`}
        confirmText="删除"
        danger
        loading={loading}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );

  if (embedded) return body;

  return (
    <PanelShell
      title="钱包管理"
      subtitle={`${wallets.length} 个钱包 · ${chain.toUpperCase()}`}
      icon={WalletCards}
      actions={(
        <button type="button" className="panel-action-btn" onClick={load} disabled={loading}>
          <RefreshCw size={12} className={loading ? 'animate-spin' : undefined} />
          {loading ? '刷新中...' : '刷新'}
        </button>
      )}
    >
      {body}
    </PanelShell>
  );
}

function WalletCard({ wallet, nativeSymbol, stableSymbol, copied, onCopy, onAction }) {
  return (
    <div className="wallet-card wallet-card--asset">
      <div className="wallet-card-main">
        <div className={`wallet-icon-chip ${wallet.is_default ? 'wallet-icon-chip--ok' : ''}`}>
          {wallet.is_default ? <ShieldCheck size={17} /> : <WalletCards size={17} />}
        </div>
        <div className="wallet-card-copy">
          <div className="wallet-card-header">
            <span className="wallet-name">{wallet.name || `钱包 ${wallet.id}`}</span>
            {wallet.is_default ? <span className="wallet-badge">默认</span> : null}
          </div>
          <button type="button" className="wallet-addr" onClick={() => onCopy(wallet.address)} title="点击复制">
            {shortAddress(wallet.address)}
            {copied ? <span className="copy-ok"> 已复制</span> : null}
          </button>
        </div>
        <button type="button" className="am-icon-btn wallet-copy-btn" onClick={() => onCopy(wallet.address)} title="复制地址">
          <Copy size={14} />
        </button>
      </div>

      <div className="wallet-balances wallet-balances--asset">
        <BalanceCell label={nativeSymbol} value={formatBalance(wallet.native_balance, 4)} />
        <BalanceCell label={stableSymbol} value={formatBalance(wallet.stable_balance, 2)} active />
      </div>

      <div className="wallet-card-actions">
        {!wallet.is_default ? (
          <button type="button" onClick={() => onAction('set_default', wallet)} className="am-action-btn">
            <Star size={12} />
            设为默认
          </button>
        ) : null}
        <button type="button" onClick={() => onAction('rename', wallet)} className="am-action-btn">
          <Edit2 size={12} />
          重命名
        </button>
        <button type="button" onClick={() => onAction('delete', wallet)} className="am-action-btn wallet-danger-btn">
          <Trash2 size={12} />
          删除
        </button>
      </div>
    </div>
  );
}

function BalanceCell({ label, value, active = false }) {
  return (
    <div className={`wallet-balance-cell${active ? ' active' : ''}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
