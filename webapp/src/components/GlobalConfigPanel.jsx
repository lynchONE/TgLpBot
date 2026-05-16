import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Bell, Layers, Save, Settings2, Shield, Wallet } from 'lucide-react';
import { fetchGlobalConfig, saveGlobalConfig } from '../api';
import PanelShell, { EmptyState, MetricCard } from './PanelShell';
import CustomSelect from './CustomSelect';

const CHAIN_OPTIONS = [
  { value: 'bsc', label: 'BSC' },
  { value: 'base', label: 'Base' },
];

function formatRebalanceTimeout(value) {
  const seconds = Number(value);
  if (!Number.isFinite(seconds)) return '--';
  return seconds <= 0 ? '立即执行' : `${seconds}s`;
}

function parseDCAPercentages(raw) {
  if (Array.isArray(raw)) return raw.map((v) => Number(v) || 0);
  if (typeof raw === 'string' && raw.trim()) {
    try {
      const arr = JSON.parse(raw);
      if (Array.isArray(arr)) return arr.map((v) => Number(v) || 0);
    } catch {
      // 服务端旧配置可能是非 JSON 文本，保持默认批次。
    }
  }
  return [50, 50];
}

function buildDraft(cfg) {
  return {
    rebalance_timeout: cfg.rebalance_timeout ?? 10,
    slippage_tolerance: cfg.slippage_tolerance ?? 0.5,
    auto_reinvest: cfg.auto_reinvest ?? false,
    residual_tolerance: cfg.residual_tolerance ?? 1.0,
    zap_loss_tolerance: cfg.zap_loss_tolerance ?? 0.5,
    extra_notifications_enabled: cfg.extra_notifications_enabled ?? true,
    filter_chinese_tokens: cfg.filter_chinese_tokens ?? false,
    multi_chain_enabled: cfg.multi_chain_enabled ?? true,
    default_chain: cfg.default_chain || 'bsc',
    multi_wallet_enabled: cfg.multi_wallet_enabled ?? false,
    bark_enabled: cfg.bark_enabled ?? false,
    bark_server: cfg.bark_server || '',
    bark_group: cfg.bark_group || '',
    dca_enabled: cfg.dca_enabled ?? false,
    dca_percentages: parseDCAPercentages(cfg.dca_percentages_json ?? cfg.dca_percentages),
    dca_interval_seconds: cfg.dca_interval_seconds ?? 30,
    dca_min_split_amount_usdt: cfg.dca_min_split_amount_usdt ?? 0,
  };
}

function countEnabledFeatures(draft) {
  return [
    draft.auto_reinvest,
    draft.extra_notifications_enabled,
    draft.filter_chinese_tokens,
    draft.multi_chain_enabled,
    draft.multi_wallet_enabled,
    draft.bark_enabled,
    draft.dca_enabled,
  ].filter(Boolean).length;
}

function getChainLabel(chain) {
  const item = CHAIN_OPTIONS.find((option) => option.value === chain);
  return item ? item.label : String(chain || '--').toUpperCase();
}

export default function GlobalConfigPanel({ apiBaseUrl, initData, hasInitData = true, embedded = false }) {
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [draft, setDraft] = useState({});

  const loadConfig = useCallback(async () => {
    if (!initData) return;
    setLoading(true);
    setError('');
    try {
      const resp = await fetchGlobalConfig({ apiBaseUrl, initData });
      const cfg = resp?.config || resp || {};
      setConfig(cfg);
      setDraft(buildDraft(cfg));
    } catch (e) {
      setError(String(e?.message || e));
    } finally {
      setLoading(false);
    }
  }, [apiBaseUrl, initData]);

  useEffect(() => {
    if (hasInitData) loadConfig();
  }, [hasInitData, loadConfig]);

  const handleSave = useCallback(async () => {
    if (!initData) return;
    setSaving(true);
    setError('');
    setSuccess('');
    try {
      const resp = await saveGlobalConfig({ apiBaseUrl, initData, config: draft });
      const cfg = resp?.config || resp || {};
      setConfig(cfg);
      setDraft(buildDraft(cfg));
      setSuccess('配置已保存');
      setTimeout(() => setSuccess(''), 3000);
    } catch (e) {
      setError(String(e?.message || e));
    } finally {
      setSaving(false);
    }
  }, [apiBaseUrl, initData, draft]);

  const updateDraft = (key, value) => setDraft((prev) => ({ ...prev, [key]: value }));

  const enabledFeatureCount = useMemo(() => countEnabledFeatures(draft), [draft]);
  const notificationMode = draft.bark_enabled
    ? '应用内 + Bark'
    : (draft.extra_notifications_enabled ? '应用内通知' : '静默');
  const riskTone = Number(draft.slippage_tolerance || 0) > 1 || Number(draft.zap_loss_tolerance || 0) > 1
    ? '偏激进'
    : '稳健';

  const body = (
    <div className="am-stack">
      {!hasInitData ? <EmptyState text="请先完成 Telegram 登录后查看配置" /> : null}
      {error ? <div className="am-error">{error}</div> : null}
      {success ? <div className="panel-success">{success}</div> : null}

      {loading && !config ? (
        <div className="panel-loading">加载中...</div>
      ) : config ? (
        <>
          <div className="am-metric-row">
            <MetricCard label="已启用项" value={`${enabledFeatureCount} 项`} tone="strong" />
            <MetricCard label="默认网络" value={getChainLabel(draft.default_chain)} />
            <MetricCard label="通知模式" value={notificationMode} />
            <MetricCard label="风险偏好" value={riskTone} />
          </div>

          <div className="am-card config-quick-card">
            <div className="am-card-header">
              <div className="am-card-title">
                <Settings2 size={14} />
                快捷开关
              </div>
              <span className="am-badge">交易保护 / 分批 / 钱包 / 通知</span>
            </div>
            <div className="settings-toggle-grid">
              <ToggleSwitch label="多钱包" active={draft.multi_wallet_enabled} onClick={() => updateDraft('multi_wallet_enabled', !draft.multi_wallet_enabled)} />
              <ToggleSwitch label="分批加仓" active={draft.dca_enabled} onClick={() => updateDraft('dca_enabled', !draft.dca_enabled)} />
              <ToggleSwitch label="Bark" active={draft.bark_enabled} onClick={() => updateDraft('bark_enabled', !draft.bark_enabled)} />
            </div>
          </div>

          <div className="am-two-col">
            <ConfigSection icon={Shield} title="交易保护" note={`再平衡：${formatRebalanceTimeout(draft.rebalance_timeout)}`}>
              <div className="am-form">
                <Field label="再平衡超时">
                  <input type="number" value={draft.rebalance_timeout} onChange={(e) => updateDraft('rebalance_timeout', Number(e.target.value) || 0)} />
                </Field>
                <Field label="滑点容忍 (%)">
                  <input type="number" step="0.1" value={draft.slippage_tolerance} onChange={(e) => updateDraft('slippage_tolerance', Number(e.target.value) || 0)} />
                </Field>
                <Field label="Zap 损耗容忍 (%)">
                  <input type="number" step="0.1" value={draft.zap_loss_tolerance} onChange={(e) => updateDraft('zap_loss_tolerance', Number(e.target.value) || 0)} />
                </Field>
                <Field label="残留容忍">
                  <input type="number" step="0.1" value={draft.residual_tolerance} onChange={(e) => updateDraft('residual_tolerance', Number(e.target.value) || 0)} />
                </Field>
              </div>
              <div className="settings-inline-row">
                <ToggleSwitch label="自动复投" active={draft.auto_reinvest} onClick={() => updateDraft('auto_reinvest', !draft.auto_reinvest)} />
              </div>
            </ConfigSection>

            <ConfigSection icon={Wallet} title="链与钱包" note="控制默认网络和钱包模式">
              <div className="am-form settings-form-one">
                <Field label="默认链">
                  <CustomSelect value={draft.default_chain} onChange={(value) => updateDraft('default_chain', value)} options={CHAIN_OPTIONS} />
                </Field>
              </div>
              <div className="settings-inline-row">
                <ToggleSwitch label="多链模式" active={draft.multi_chain_enabled} onClick={() => updateDraft('multi_chain_enabled', !draft.multi_chain_enabled)} />
                <ToggleSwitch label="多钱包模式" active={draft.multi_wallet_enabled} onClick={() => updateDraft('multi_wallet_enabled', !draft.multi_wallet_enabled)} />
              </div>
            </ConfigSection>
          </div>

          <DCAConfigSection draft={draft} updateDraft={updateDraft} />

          <ConfigSection icon={Bell} title="通知与过滤" note="消息触达和代币过滤策略">
            <div className="settings-inline-row">
              <ToggleSwitch label="日志通知" active={draft.extra_notifications_enabled} onClick={() => updateDraft('extra_notifications_enabled', !draft.extra_notifications_enabled)} />
              <ToggleSwitch label="过滤中文代币" active={draft.filter_chinese_tokens} onClick={() => updateDraft('filter_chinese_tokens', !draft.filter_chinese_tokens)} />
              <ToggleSwitch label="Bark 通知" active={draft.bark_enabled} onClick={() => updateDraft('bark_enabled', !draft.bark_enabled)} />
            </div>
            {draft.bark_enabled ? (
              <div className="am-form">
                <Field label="Bark Server">
                  <input type="text" value={draft.bark_server} onChange={(e) => updateDraft('bark_server', e.target.value)} placeholder="https://api.day.app" />
                </Field>
                <Field label="Bark 分组">
                  <input type="text" value={draft.bark_group} onChange={(e) => updateDraft('bark_group', e.target.value)} placeholder="LP Bot" />
                </Field>
              </div>
            ) : null}
          </ConfigSection>

          <button type="button" className="config-save-btn config-save-btn--asset" disabled={saving} onClick={handleSave}>
            <Save size={15} />
            {saving ? '保存中...' : '保存配置'}
          </button>
        </>
      ) : null}
    </div>
  );

  if (embedded) return body;

  return (
    <PanelShell title="全局配置" subtitle="管理交易、通知和链路等设置" icon={Settings2}>
      {body}
    </PanelShell>
  );
}

function ConfigSection({ icon: Icon, title, note, children }) {
  return (
    <section className="am-card">
      <div className="am-card-header">
        <div className="am-card-title">
          <Icon size={14} />
          {title}
        </div>
        {note ? <span className="am-badge">{note}</span> : null}
      </div>
      <div className="settings-section-body">{children}</div>
    </section>
  );
}

function Field({ label, children }) {
  return (
    <label className="am-field">
      <span>{label}</span>
      {children}
    </label>
  );
}

function ToggleSwitch({ label, active, onClick }) {
  return (
    <button
      type="button"
      className={`am-toggle-pill${active ? ' active' : ''}`}
      onClick={onClick}
    >
      <span>{active ? 'ON' : 'OFF'}</span>
      <strong>{label}</strong>
    </button>
  );
}

function DCAConfigSection({ draft, updateDraft }) {
  const percentages = Array.isArray(draft.dca_percentages) ? draft.dca_percentages : [];
  const sum = percentages.reduce((acc, v) => acc + (Number(v) || 0), 0);
  const sumValid = Math.abs(sum - 100) < 0.01;

  const updatePct = (idx, value) => {
    const next = percentages.slice();
    next[idx] = Number(value) || 0;
    updateDraft('dca_percentages', next);
  };

  const addBatch = () => {
    if (percentages.length >= 5) return;
    const n = percentages.length + 1;
    const base = Math.floor((100 / n) * 100) / 100;
    const next = Array(n).fill(base);
    next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
    updateDraft('dca_percentages', next);
  };

  const removeBatch = (idx) => {
    if (percentages.length <= 2) return;
    updateDraft('dca_percentages', percentages.filter((_, i) => i !== idx));
  };

  const equalize = () => {
    const n = percentages.length || 2;
    const base = Math.floor((100 / n) * 100) / 100;
    const next = Array(n).fill(base);
    next[n - 1] = Math.round((100 - base * (n - 1)) * 100) / 100;
    updateDraft('dca_percentages', next);
  };

  return (
    <ConfigSection icon={Layers} title="分批加仓" note={sumValid ? `合计 ${sum.toFixed(2)}%` : '占比需等于 100%'}>
      <div className="settings-inline-row">
        <ToggleSwitch label="启用分批加仓" active={draft.dca_enabled} onClick={() => updateDraft('dca_enabled', !draft.dca_enabled)} />
      </div>

      {draft.dca_enabled ? (
        <>
          <div className="dca-batch-list">
            {percentages.map((value, idx) => (
              <div key={idx} className="dca-batch-row">
                <span className="dca-batch-label">{idx === 0 ? '首批' : `第 ${idx + 1} 批`}</span>
                <input
                  type="number"
                  step="0.1"
                  min="5"
                  max="100"
                  value={value}
                  onChange={(e) => updatePct(idx, e.target.value)}
                />
                <span className="dca-batch-unit">%</span>
                {percentages.length > 2 ? (
                  <button type="button" className="am-action-btn dca-remove-btn" onClick={() => removeBatch(idx)}>
                    删除
                  </button>
                ) : null}
              </div>
            ))}
          </div>

          <div className="settings-inline-row settings-actions-row">
            <button type="button" className="am-action-btn" onClick={equalize}>平均分配</button>
            <button type="button" className="am-action-btn" onClick={addBatch} disabled={percentages.length >= 5}>追加批次</button>
          </div>

          <div className="am-form">
            <Field label="批次间隔（0-300 秒）">
              <input
                type="number"
                step="0.001"
                min="0"
                max="300"
                value={draft.dca_interval_seconds ?? 30}
                onChange={(e) => updateDraft('dca_interval_seconds', Number(e.target.value) || 0)}
              />
            </Field>
            <Field label="最小拆分金额 (USDT)">
              <input
                type="number"
                step="0.1"
                min="0"
                value={draft.dca_min_split_amount_usdt ?? 0}
                onChange={(e) => updateDraft('dca_min_split_amount_usdt', Number(e.target.value) || 0)}
              />
            </Field>
          </div>
          <div className="settings-note">
            首批按正常开仓创建仓位；后续批次按间隔追加流动性。手动关仓或价格跑出区间时，剩余批次自动取消。
          </div>
        </>
      ) : (
        <div className="settings-note">
          关闭时一次性开仓成交；开启后按上方批次设置执行。
        </div>
      )}
    </ConfigSection>
  );
}
