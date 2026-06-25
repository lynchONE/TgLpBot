import { Bell, Clock3, Gift, RadioTower, RefreshCw, X } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  fetchAlphaDataDirect,
  fetchAlphaOverview,
  fetchAlphaReminderConfig,
  fetchAlphaStabilityDirect,
  saveAlphaReminderConfig,
} from '../api';

const REFRESH_MS = 60_000;
const AIRDROP_LIMIT = 1;
const STABILITY_LIMIT = 3;
const CHINA_TIME_ZONE = 'Asia/Shanghai';
const DEFAULT_REMINDER_MINUTES = 3;
const DEFAULT_REMINDER_INTENSITY = 'ring';
const INTENSITY_OPTIONS = [
  { value: 'ring', label: '响铃' },
  { value: 'persistent_ring', label: '持续响铃' },
  { value: 'critical_ring', label: '强提醒' },
];

function readString(value) {
  if (value === undefined || value === null) return '';
  return String(value).trim();
}

function chinaDateParts(value = new Date()) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: CHINA_TIME_ZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(date);
  const map = {};
  parts.forEach((part) => {
    if (part.type !== 'literal') map[part.type] = part.value;
  });
  if (!map.year || !map.month || !map.day) return null;
  return map;
}

function formatChinaDay(value = new Date()) {
  const parts = chinaDateParts(value);
  if (!parts) return '';
  return `${parts.year}-${parts.month}-${parts.day}`;
}

function normalizeAirdropDay(value) {
  const raw = readString(value);
  const match = raw.match(/^(\d{4})[-/](\d{1,2})[-/](\d{1,2})$/);
  if (!match) return '';
  const [, year, month, day] = match;
  return `${year}-${month.padStart(2, '0')}-${day.padStart(2, '0')}`;
}

function formatAirdropDateTime(item) {
  const date = readString(item?.date);
  const time = readString(item?.time);
  return [date, time].filter(Boolean).join(' ');
}

function normalizeAirdrops(value, todayDay) {
  const currentDay = readString(todayDay);
  if (!currentDay || !Array.isArray(value)) return [];
  return value
    .map((item) => {
      const token = readString(item?.token).toUpperCase();
      const name = readString(item?.name);
      const day = normalizeAirdropDay(item?.date);
      return {
        token,
        name,
        day,
        amount: readString(item?.amount),
        points: readString(item?.points),
        dateTime: formatAirdropDateTime(item),
      };
    })
    .filter((item) => item.day === currentDay && (item.token || item.name));
}

function isStableStatus(status) {
  const value = readString(status).toLowerCase();
  const parts = value.split(':').filter(Boolean);
  const statusWord = parts[parts.length - 1] || '';
  return value.startsWith('green') || statusWord === 'stable';
}

function formatStabilityStatus(status) {
  const value = readString(status).toLowerCase();
  if (!value) return '';
  if (value.includes('no_trade')) return '无成交';
  if (value.includes('unstable')) return '不稳定';
  if (value.includes('stable')) return '稳定';
  if (value.includes('moderate')) return '中等';
  const parts = value.split(':').filter(Boolean);
  return parts[parts.length - 1] || value;
}

function parseOptionalNumber(value) {
  if (value === undefined || value === null || value === '') return NaN;
  const n = Number(value);
  return Number.isFinite(n) ? n : NaN;
}

function normalizeStabilityItems(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => ({
      name: readString(item?.n),
      status: formatStabilityStatus(item?.st),
      rawStatus: readString(item?.st),
      spread: parseOptionalNumber(item?.spr),
      depth: readString(item?.md),
    }))
    .filter((item) => item.name);
}

function formatSpread(value) {
  if (!Number.isFinite(value)) return '';
  if (Math.abs(value) >= 10) return `${value.toFixed(1)}%`;
  return `${value.toFixed(2)}%`;
}

function buildStabilitySummary(items) {
  const selected = items.slice(0, STABILITY_LIMIT);
  const unstableItems = selected.filter((item) => !isStableStatus(item.rawStatus));
  return {
    total: items.length,
    unstableCount: unstableItems.length,
    selected,
  };
}

function normalizeReminderConfig(payload) {
  if (!payload || typeof payload !== 'object') {
    return {
      enabled: false,
      reminderMinutes: DEFAULT_REMINDER_MINUTES,
      intensity: DEFAULT_REMINDER_INTENSITY,
      barkReady: false,
      barkConfigured: false,
      barkEnabled: false,
    };
  }
  const minutes = Number(payload.reminder_minutes);
  return {
    enabled: Boolean(payload.enabled),
    reminderMinutes: Number.isFinite(minutes) ? minutes : DEFAULT_REMINDER_MINUTES,
    intensity: readString(payload.intensity) || DEFAULT_REMINDER_INTENSITY,
    barkReady: Boolean(payload.bark_ready),
    barkConfigured: Boolean(payload.bark_configured),
    barkEnabled: Boolean(payload.bark_enabled),
  };
}

function reminderStatusText(config) {
  if (!config.barkConfigured) return 'Bark 未配置';
  if (!config.barkEnabled) return 'Bark 未开启';
  if (!config.barkReady) return 'Bark 未就绪';
  return 'Bark 已就绪';
}

function normalizeReminderMinutes(value) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return DEFAULT_REMINDER_MINUTES;
  return Math.min(120, Math.max(1, n));
}

export default function AlphaTicker({ apiBaseUrl, initData, hasInitData }) {
  const [payload, setPayload] = useState(null);
  const payloadRef = useRef(null);
  const [todayDay, setTodayDay] = useState(() => formatChinaDay());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reminderOpen, setReminderOpen] = useState(false);
  const [reminder, setReminder] = useState(() => normalizeReminderConfig(null));
  const [reminderLoading, setReminderLoading] = useState(false);
  const [reminderSaving, setReminderSaving] = useState(false);
  const [reminderError, setReminderError] = useState('');
  const [reminderDraft, setReminderDraft] = useState({
    enabled: false,
    reminderMinutes: DEFAULT_REMINDER_MINUTES,
    intensity: DEFAULT_REMINDER_INTENSITY,
  });

  useEffect(() => {
    let active = true;
    let timer = null;
    let controller = null;

    async function load() {
      if (controller) controller.abort();
      controller = new AbortController();
      const nextTodayDay = formatChinaDay();
      if (nextTodayDay) {
        setTodayDay((prev) => (prev === nextTodayDay ? prev : nextTodayDay));
      }
      try {
        setError('');
        let data = await fetchAlphaOverview({ signal: controller.signal });
        const directErrors = {};
        if (!data?.data?.airdrops && data?.errors?.data) {
          try {
            data = { ...data, data: await fetchAlphaDataDirect({ signal: controller.signal }) };
          } catch (err) {
            directErrors.data = readString(err?.message) || 'direct fetch failed';
          }
        }
        if (!data?.stability?.items && data?.errors?.stability) {
          try {
            data = { ...data, stability: await fetchAlphaStabilityDirect({ signal: controller.signal }) };
          } catch (err) {
            directErrors.stability = readString(err?.message) || 'direct fetch failed';
          }
        }
        if (Object.keys(directErrors).length > 0) {
          data = { ...data, errors: { ...(data?.errors || {}), ...directErrors } };
        }
        if (!active) return;
        payloadRef.current = data;
        setPayload(data);
        setError('');
      } catch (err) {
        if (!active || err?.name === 'AbortError') return;
        if (!payloadRef.current) {
          setError(readString(err?.message) || 'Alpha 加载失败');
        }
      } finally {
        if (active) setLoading(false);
      }
    }

    load();
    timer = window.setInterval(load, REFRESH_MS);

    return () => {
      active = false;
      if (controller) controller.abort();
      if (timer) window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    if (!hasInitData || !initData) {
      const empty = normalizeReminderConfig(null);
      setReminder(empty);
      setReminderDraft({
        enabled: empty.enabled,
        reminderMinutes: empty.reminderMinutes,
        intensity: empty.intensity,
      });
      return undefined;
    }
    let active = true;
    let timer = null;
    let controller = null;
    async function loadReminder() {
      if (reminderOpen) return;
      if (controller) controller.abort();
      controller = new AbortController();
      try {
        setReminderLoading(true);
        setReminderError('');
        const data = await fetchAlphaReminderConfig({ apiBaseUrl, initData, signal: controller.signal });
        if (!active) return;
        const normalized = normalizeReminderConfig(data);
        setReminder(normalized);
        setReminderDraft({
          enabled: normalized.enabled,
          reminderMinutes: normalized.reminderMinutes,
          intensity: normalized.intensity,
        });
      } catch (err) {
        if (!active || err?.name === 'AbortError') return;
        setReminderError(readString(err?.message) || '提醒配置加载失败');
      } finally {
        if (active) setReminderLoading(false);
      }
    }
    loadReminder();
    timer = window.setInterval(loadReminder, REFRESH_MS);
    return () => {
      active = false;
      if (controller) controller.abort();
      if (timer) window.clearInterval(timer);
    };
  }, [apiBaseUrl, hasInitData, initData, reminderOpen]);

  async function saveReminder(nextDraft = reminderDraft) {
    if (!hasInitData || !initData) {
      setReminderError('登录后可设置 Bark 提醒');
      setReminderOpen(true);
      return;
    }
    try {
      setReminderSaving(true);
      setReminderError('');
      const data = await saveAlphaReminderConfig({
        apiBaseUrl,
        initData,
        enabled: Boolean(nextDraft.enabled),
        reminderMinutes: normalizeReminderMinutes(nextDraft.reminderMinutes),
        intensity: nextDraft.intensity,
      });
      const normalized = normalizeReminderConfig(data);
      setReminder(normalized);
      setReminderDraft({
        enabled: normalized.enabled,
        reminderMinutes: normalized.reminderMinutes,
        intensity: normalized.intensity,
      });
    } catch (err) {
      setReminderError(readString(err?.message) || '提醒配置保存失败');
    } finally {
      setReminderSaving(false);
    }
  }

  function toggleReminder() {
    if (!hasInitData || !initData) {
      setReminderOpen(true);
      setReminderError('登录后可设置 Bark 提醒');
      return;
    }
    setReminderOpen(true);
    if (reminder.enabled) {
      setReminderDraft({
        enabled: reminder.enabled,
        reminderMinutes: reminder.reminderMinutes,
        intensity: reminder.intensity,
      });
      return;
    }
    const nextDraft = {
      ...reminderDraft,
      enabled: true,
      reminderMinutes: normalizeReminderMinutes(reminderDraft.reminderMinutes),
      intensity: reminderDraft.intensity || DEFAULT_REMINDER_INTENSITY,
    };
    setReminderDraft(nextDraft);
    saveReminder(nextDraft);
  }

  const airdrops = useMemo(() => normalizeAirdrops(payload?.data?.airdrops, todayDay), [payload, todayDay]);
  const stability = useMemo(
    () => buildStabilitySummary(normalizeStabilityItems(payload?.stability?.items)),
    [payload],
  );
  const firstAirdrop = airdrops[0];
  const extraAirdropCount = Math.max(0, airdrops.length - AIRDROP_LIMIT);
  const visibleAirdrops = airdrops.slice(0, AIRDROP_LIMIT);

  if (loading && !payload) {
    return (
      <div className="alpha-ticker alpha-ticker-loading" aria-live="polite">
        <RefreshCw size={13} className="spin" />
        <span>Alpha 同步中</span>
      </div>
    );
  }

  if (error && !payload) {
    return (
      <div className="alpha-ticker alpha-ticker-error" aria-live="polite">
        <span>Alpha 暂无数据</span>
      </div>
    );
  }

  return (
    <div className="alpha-ticker" aria-live="polite">
      <div className="alpha-section alpha-airdrops">
        <span className="alpha-label">
          <Gift size={13} />
          今日空投
        </span>
        {visibleAirdrops.length ? (
          <div className="alpha-airdrop-list">
            {visibleAirdrops.map((item, index) => (
              <span className="alpha-airdrop-item" key={`${item.token}:${item.name}:${index}`}>
                <strong>{item.token || item.name}</strong>
                {item.name && item.name !== item.token ? <span className="alpha-name">{item.name}</span> : null}
                {item.amount ? <span className="alpha-meta">数量 {item.amount}</span> : null}
                {item.points ? <span className="alpha-meta">积分 {item.points}</span> : null}
                {item.dateTime ? (
                  <span className="alpha-time">
                    <Clock3 size={11} />
                    {item.dateTime}
                  </span>
                ) : null}
              </span>
            ))}
            {extraAirdropCount > 0 ? <span className="alpha-more">+{extraAirdropCount}</span> : null}
          </div>
        ) : (
          <span className="alpha-empty">暂无</span>
        )}
        <div className="alpha-reminder-wrap">
          <button
            type="button"
            className={`alpha-reminder-btn ${reminder.enabled ? 'active' : ''}`}
            onClick={toggleReminder}
            onContextMenu={(e) => {
              e.preventDefault();
              setReminderOpen(true);
            }}
            title={reminder.enabled ? `已开启：提前 ${reminder.reminderMinutes} 分钟提醒` : '开启空投提醒'}
            aria-label="空投提醒"
            aria-pressed={reminder.enabled}
            disabled={reminderSaving}
          >
            <Bell size={13} />
          </button>
          {reminderOpen ? (
            <div className="alpha-reminder-popover">
              <div className="alpha-reminder-head">
                <div>
                  <strong>空投提醒</strong>
                  <span>{hasInitData ? reminderStatusText(reminder) : '登录后可设置'}</span>
                </div>
                <button type="button" onClick={() => setReminderOpen(false)} aria-label="关闭提醒设置">
                  <X size={13} />
                </button>
              </div>
              <label className="alpha-reminder-toggle">
                <span>开启 Bark 提醒</span>
                <input
                  type="checkbox"
                  checked={Boolean(reminderDraft.enabled)}
                  onChange={(e) => setReminderDraft((prev) => ({ ...prev, enabled: e.target.checked }))}
                  disabled={!hasInitData || reminderSaving}
                />
              </label>
              <label className="alpha-reminder-field">
                <span>提前时间</span>
                <div>
                  <input
                    type="number"
                    min="1"
                    max="120"
                    step="1"
                    value={reminderDraft.reminderMinutes}
                    onChange={(e) => setReminderDraft((prev) => ({ ...prev, reminderMinutes: e.target.value }))}
                    disabled={!hasInitData || reminderSaving}
                  />
                  <em>分钟</em>
                </div>
              </label>
              <div className="alpha-reminder-field alpha-reminder-intensity">
                <span>提醒强度</span>
                <div>
                  {INTENSITY_OPTIONS.map((option) => (
                    <button
                      key={option.value}
                      type="button"
                      className={reminderDraft.intensity === option.value ? 'active' : ''}
                      onClick={() => setReminderDraft((prev) => ({ ...prev, intensity: option.value }))}
                      disabled={!hasInitData || reminderSaving}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
              </div>
              {reminderError ? <div className="alpha-reminder-error">{reminderError}</div> : null}
              <div className="alpha-reminder-actions">
                <span>{reminderLoading ? '加载中' : `默认提前 ${DEFAULT_REMINDER_MINUTES} 分钟`}</span>
                <button
                  type="button"
                  onClick={() => saveReminder()}
                  disabled={!hasInitData || reminderSaving}
                >
                  {reminderSaving ? '保存中' : '保存'}
                </button>
              </div>
            </div>
          ) : null}
        </div>
      </div>

      <div className="alpha-divider" />

      <div className="alpha-section alpha-stability">
        <span className="alpha-label">
          <RadioTower size={13} />
          稳定度
        </span>
        {stability.total > 0 ? (
          <>
            <div className="alpha-stability-list">
              {stability.selected.map((item) => (
                <span className="alpha-stability-item" key={`${item.name}:${item.rawStatus}`}>
                  <strong>{item.name}</strong>
                  {item.status ? <span>{item.status}</span> : null}
                  {formatSpread(item.spread) ? <span>{formatSpread(item.spread)}</span> : null}
                </span>
              ))}
            </div>
          </>
        ) : (
          <span className="alpha-empty">暂无</span>
        )}
      </div>

      {firstAirdrop?.dateTime ? <span className="alpha-mobile-time">{firstAirdrop.dateTime}</span> : null}
    </div>
  );
}
