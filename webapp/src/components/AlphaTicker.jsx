import { AlertTriangle, Clock3, Gift, RadioTower, RefreshCw } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { fetchAlphaDataDirect, fetchAlphaOverview, fetchAlphaStabilityDirect } from '../api';

const REFRESH_MS = 60_000;
const AIRDROP_LIMIT = 2;
const STABILITY_LIMIT = 3;

function readString(value) {
  if (value === undefined || value === null) return '';
  return String(value).trim();
}

function formatAirdropDateTime(item) {
  const date = readString(item?.date);
  const time = readString(item?.time);
  return [date, time].filter(Boolean).join(' ');
}

function normalizeAirdrops(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => {
      const token = readString(item?.token).toUpperCase();
      const name = readString(item?.name);
      return {
        token,
        name,
        amount: readString(item?.amount),
        points: readString(item?.points),
        dateTime: formatAirdropDateTime(item),
      };
    })
    .filter((item) => item.token || item.name);
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

function hasAlphaErrors(payload) {
  return Boolean(payload?.errors && Object.keys(payload.errors).length > 0);
}

function alphaErrorText(errors) {
  if (!errors || typeof errors !== 'object') return '';
  return Object.entries(errors)
    .map(([key, value]) => `${key}: ${readString(value)}`)
    .filter(Boolean)
    .join(' / ');
}

export default function AlphaTicker() {
  const [payload, setPayload] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    let timer = null;
    let controller = null;

    async function load() {
      if (controller) controller.abort();
      controller = new AbortController();
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
        setPayload(data);
        setError(hasAlphaErrors(data) ? alphaErrorText(data.errors) || 'Alpha 部分更新失败' : '');
      } catch (err) {
        if (!active || err?.name === 'AbortError') return;
        setError(readString(err?.message) || 'Alpha 加载失败');
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

  const airdrops = useMemo(() => normalizeAirdrops(payload?.data?.airdrops), [payload]);
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
        <AlertTriangle size={13} />
        <span>{error}</span>
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

      {error ? <span className="alpha-soft-error" title={error}>更新失败</span> : null}
      {firstAirdrop?.dateTime ? <span className="alpha-mobile-time">{firstAirdrop.dateTime}</span> : null}
    </div>
  );
}
