import { useMemo } from 'react';
import { Activity, Brain, Eye, Flame, Settings, Users, Wallet } from 'lucide-react';

function StatCard({ label, value, color }) {
  return (
    <div className="smd-stat-card">
      <div className="smd-stat-label">{label}</div>
      <div className={`smd-stat-value ${color || ''}`}>{value ?? '--'}</div>
    </div>
  );
}

function SmartMoneyMonitorBanner({ summary }) {
  return (
    <div className={`smd-monitor-banner${summary.enabled ? '' : ' off'}`}>
      <div className="smd-monitor-pill">
        <span className="smd-monitor-dot" />
        {summary.label}
      </div>
      <div className="smd-monitor-detail">{summary.detail}</div>
    </div>
  );
}

function SmartMoneyStatsGrid({ stats }) {
  return (
    <div className="smd-stats-grid">
      <StatCard label="活跃池子" value={stats.active_pool_count} />
      <StatCard label="监控钱包" value={stats.monitored_wallet_count} />
      <StatCard label="持仓笔数" value={stats.open_position_count} />
      <StatCard label="今日关闭" value={stats.closed_today_count} color="red" />
    </div>
  );
}

function SmartMoneyTabs({ view, isAdmin, onChange }) {
  return (
    <div className="smd-tabs">
      {[
        { key: 'pools', label: '池子视图', icon: Eye },
        { key: 'wallets', label: '钱包视图', icon: Wallet },
        { key: 'watch_activity', label: '特别关注', icon: Activity },
        { key: 'settings', label: '合约视图', icon: Settings },
      ].map(({ key, label, icon: Icon }) => (
        <button
          key={key}
          type="button"
          className={`smd-tab${view === key ? ' active' : ''}`}
          onClick={() => onChange(key)}
          aria-pressed={view === key}
        >
          <Icon size={16} /> {label}
        </button>
      ))}
      <button
        key="golden_dog"
        type="button"
        className={`smd-tab${view === 'golden_dog' ? ' active' : ''}`}
        onClick={() => onChange('golden_dog')}
        aria-pressed={view === 'golden_dog'}
      >
        <Flame size={16} /> 监控通知
      </button>
      <button
        key="auto_follow"
        type="button"
        className={`smd-tab${view === 'auto_follow' ? ' active' : ''}`}
        onClick={() => onChange('auto_follow')}
        aria-pressed={view === 'auto_follow'}
      >
        <Users size={16} /> 自动跟单
      </button>
      {isAdmin ? (
        <button
          key="assets"
          type="button"
          className={`smd-tab${view === 'assets' ? ' active' : ''}`}
          onClick={() => onChange('assets')}
          aria-pressed={view === 'assets'}
        >
          <Wallet size={16} /> 聪明钱资产
        </button>
      ) : null}
    </div>
  );
}

export function buildSmartMoneyMonitorSummary(stats) {
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
}

export default function SmartMoneyShell({
  stats,
  isDetail,
  isAdmin,
  view,
  onViewChange,
  children,
}) {
  const monitorSummary = useMemo(() => buildSmartMoneyMonitorSummary(stats), [stats]);

  return (
    <section className="panel-shell">
      <header className="panel-header">
        <div className="panel-title-wrap">
          <div className="panel-icon-wrap"><Brain size={16} /></div>
          <div className="panel-title-texts">
            <h2>聪明钱</h2>
            {!isDetail && <p>{isAdmin ? '监控、钱包、合约、通知、资产' : '监控、钱包、合约、通知'}</p>}
          </div>
        </div>
      </header>
      <div className="panel-body">
        {stats && !isDetail ? <SmartMoneyMonitorBanner summary={monitorSummary} /> : null}
        {stats && !isDetail ? <SmartMoneyStatsGrid stats={stats} /> : null}
        {!isDetail ? <SmartMoneyTabs view={view} isAdmin={isAdmin} onChange={onViewChange} /> : null}
        {children}
      </div>
    </section>
  );
}
