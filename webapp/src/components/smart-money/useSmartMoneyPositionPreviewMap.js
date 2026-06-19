import { useEffect, useState } from 'react';
import { fetchSMPositionDetail } from '../../smartMoneyApi';
import { computePriceRange } from '../../utils';

const POSITION_PREVIEW_BATCH_SIZE = 4;

function parseMetricNumber(value) {
  if (value === null || value === undefined || value === '') return NaN;
  const raw = typeof value === 'string' ? value.replace(/,/g, '').trim() : value;
  const direct = Number(raw);
  if (Number.isFinite(direct)) return direct;
  const match = String(value).match(/-?\d+(\.\d+)?/);
  if (!match) return NaN;
  const parsed = Number(match[0]);
  return Number.isFinite(parsed) ? parsed : NaN;
}

export function resolvePositionPreviewFeeUsd(detail, position) {
  const liveFee = parseMetricNumber(detail?.totals?.fee_usd);
  if (Number.isFinite(liveFee)) return liveFee;
  if (String(position?.fee_status || '').trim() === 'unavailable') return NaN;
  return parseMetricNumber(position?.fee_usd);
}

function formatRangeDrift(value) {
  const num = Number(value);
  if (!Number.isFinite(num) || num < 0) return '--';
  if (num >= 100) return `${Math.round(num)}%`;
  if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
  return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

export function buildRangeStatusSummary(rangeState) {
  if (!rangeState) return null;
  if (rangeState.inRange) {
    return { text: '区间内', tone: 'positive' };
  }
  if (rangeState.outOfRange?.direction) {
    const direction = rangeState.outOfRange.direction === 'above' ? '高于区间' : '低于区间';
    return { text: `${direction} ${formatRangeDrift(rangeState.outOfRange.pct)}`, tone: 'negative' };
  }
  if (rangeState.inRange === false) {
    return { text: '已离开区间', tone: 'negative' };
  }
  return null;
}

export function getPositionSelectionKey(position) {
  const positionRef = String(position?.position_ref || '').trim();
  if (positionRef) return positionRef;
  const id = String(position?.id || '').trim();
  if (id) return id;
  const wallet = String(position?.wallet_address || '').trim().toLowerCase();
  const pool = String(position?.pool_address || '').trim().toLowerCase();
  const nft = String(position?.nft_token_id || '').trim();
  return [wallet, pool, nft].filter(Boolean).join(':');
}

export default function useSmartMoneyPositionPreviewMap(apiBaseUrl, positions) {
  const [previewMap, setPreviewMap] = useState({});

  useEffect(() => {
    const rows = Array.isArray(positions) ? positions : [];
    if (rows.length === 0) {
      setPreviewMap({});
      return undefined;
    }

    let cancelled = false;
    setPreviewMap({});

    const loadPreview = async (position) => {
      const key = getPositionSelectionKey(position);
      if (!key) return;
      try {
        const data = await fetchSMPositionDetail({
          apiBaseUrl,
          positionRef: position.position_ref,
          positionId: position.id,
        });
        if (cancelled) return;
        setPreviewMap((prev) => ({
          ...prev,
          [key]: {
            fetchedAt: Date.now(),
            currentValueUsd: Number.isFinite(Number(data?.current_value_usd))
              ? Number(data.current_value_usd)
              : Number(data?.totals?.position_usd || 0) + Number(data?.totals?.fee_usd || 0),
            feeUsd: resolvePositionPreviewFeeUsd(data, position),
            netInvestedUsd: Number(data?.net_invested_usd ?? position?.position_amount_usd ?? 0),
            rangeStatus: buildRangeStatusSummary(
              computePriceRange(data) || (data?.in_range === undefined ? null : { inRange: Boolean(data.in_range) })
            ),
            runningSince: String(data?.running_since || position?.opened_at || '').trim(),
          },
        }));
      } catch (error) {
        if (cancelled) return;
        setPreviewMap((prev) => ({
          ...prev,
          [key]: {
            ...(prev[key] || {}),
            fetchedAt: Date.now(),
            feeUsd: resolvePositionPreviewFeeUsd(null, position),
            runningSince: String(prev[key]?.runningSince || position?.opened_at || '').trim(),
          },
        }));
        throw error;
      }
    };

    (async () => {
      for (let index = 0; index < rows.length && !cancelled; index += POSITION_PREVIEW_BATCH_SIZE) {
        const batch = rows.slice(index, index + POSITION_PREVIEW_BATCH_SIZE);
        await Promise.all(batch.map((position) => loadPreview(position)));
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, positions]);

  return previewMap;
}
