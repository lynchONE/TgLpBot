import { formatUsdCompact } from '../../lib/format';

export function parseAmountInput(value) {
    return Number(String(value || '').replace(/,/g, '').trim());
}

export function resolvePositionSlippage(position) {
    const candidates = [
        position?.task_slippage_tolerance,
        position?.slippage_tolerance,
        position?.task?.slippage_tolerance,
    ];
    for (const candidate of candidates) {
        const n = Number(candidate);
        if (Number.isFinite(n) && n >= 0 && n <= 100) {
            return n;
        }
    }
    return undefined;
}

export function roundPresetAmount(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return 0;
    if (num >= 1000) return Math.round(num / 50) * 50;
    if (num >= 200) return Math.round(num / 20) * 20;
    if (num >= 50) return Math.round(num / 10) * 10;
    if (num >= 10) return Math.round(num / 5) * 5;
    return Math.round(num * 10) / 10;
}

export function formatAmountInput(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '';
    if (num >= 100) return String(Math.round(num));
    return num.toFixed(num >= 10 ? 1 : 2).replace(/0+$/, '').replace(/\.$/, '');
}

export function formatRatioCompact(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 100) return `${Math.round(num)}%`;
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

export function buildAddLiquidityPresetOptions(referenceAmount) {
    const presets = [];
    const seen = new Set();

    const pushPreset = (value, hint) => {
        const rounded = roundPresetAmount(value);
        if (!(rounded > 0)) return;
        const key = rounded.toFixed(2);
        if (seen.has(key)) return;
        seen.add(key);
        presets.push({
            value: rounded,
            label: `${formatAmountInput(rounded)} USDT`,
            hint,
        });
    };

    if (referenceAmount > 0) {
        pushPreset(referenceAmount * 0.25, '25% 参考仓位');
        pushPreset(referenceAmount * 0.5, '50% 参考仓位');
        pushPreset(referenceAmount, '1x 参考仓位');
    }

    pushPreset(50, '固定金额');
    pushPreset(100, '固定金额');
    pushPreset(200, '固定金额');

    return presets.slice(0, 4);
}

export function parseOptionalPercent(raw) {
    const text = String(raw || '').trim();
    if (!text) return { valid: true, value: undefined };
    const num = Number(text);
    if (!Number.isFinite(num) || num < 0 || num > 100) {
        return { valid: false, value: undefined };
    }
    return { valid: true, value: num };
}

export function formatDCAIntervalHint(seconds) {
    const n = Number(seconds);
    if (!Number.isFinite(n) || n <= 0) return '立即执行';
    if (n < 1) return `${Math.round(n * 1000)}ms`;
    if (Number.isInteger(n)) return `${n}s`;
    return `${n.toFixed(1)}s`;
}

export function buildDCASummaryItems(amount, percentages) {
    const totalAmount = Number(amount);
    if (!Array.isArray(percentages) || percentages.length === 0) return [];
    return percentages.map((pct, idx) => ({
        key: `batch-${idx}`,
        label: idx === 0 ? '首批' : `第${idx + 1}批`,
        amount: Number.isFinite(totalAmount) && totalAmount > 0
            ? formatUsdCompact(totalAmount * (Number(pct) || 0) / 100)
            : '$--',
    }));
}

export function formatSharePercent(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num < 0) return '--';
    const percent = num * 100;
    if (percent >= 100) return `${Math.round(percent)}%`;
    if (percent >= 10) return `${percent.toFixed(1).replace(/\.0$/, '')}%`;
    return `${percent.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

export function formatPercentValue(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num < 0) return '--';
    if (num >= 10) return `${num.toFixed(1).replace(/\.0$/, '')}%`;
    if (num >= 1) return `${num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')}%`;
    return `${num.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')}%`;
}

export function formatUSDTValue(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num < 0) return '--';
    if (num === 0) return '0';
    if (num >= 1000) return num.toLocaleString(undefined, { maximumFractionDigits: 2 });
    if (num >= 1) return num.toFixed(2).replace(/0+$/, '').replace(/\.$/, '');
    return num.toFixed(4).replace(/0+$/, '').replace(/\.$/, '');
}

export function formatSizingModeLabel(mode) {
    switch (String(mode || '').trim()) {
        case 'conservative':
            return '保守';
        case 'neutral':
            return '均衡';
        case 'aggressive':
            return '激进';
        default:
            return '--';
    }
}

export function getSizingEfficiencyMeta(efficiency) {
    switch (String(efficiency || '').trim()) {
        case 'high':
            return {
                label: '资金利用高',
                chipClass: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200',
            };
        case 'medium':
            return {
                label: '资金利用适中',
                chipClass: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-200',
            };
        default:
            return {
                label: '资金利用低',
                chipClass: 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-200',
            };
    }
}

export function formatPriceValue(value) {
    const num = Number(value);
    if (!Number.isFinite(num) || num <= 0) return '--';
    if (num >= 1000) return num.toLocaleString(undefined, { maximumFractionDigits: 2 });
    if (num >= 1) return num.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 6 });
    return Number(num.toPrecision(6)).toString();
}

export function formatPriceInputValue(value) {
    const text = formatPriceValue(value);
    return text === '--' ? '' : text;
}
