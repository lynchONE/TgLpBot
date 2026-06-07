import { formatPercentInputValue } from '../../lib/format';
import {
    OPEN_POSITION_DEFAULT_GRID_OFFSET,
    OPEN_POSITION_GRID_RADIUS,
} from './constants';

export function tickToPoolPrice(tick, token0Decimals, token1Decimals) {
    const tickValue = Number(tick);
    if (!Number.isFinite(tickValue)) return NaN;
    const decAdj = Math.pow(10, (Number(token0Decimals) || 18) - (Number(token1Decimals) || 18));
    return Math.pow(1.0001, tickValue) * decAdj;
}

export function poolPriceToTick(price, token0Decimals, token1Decimals) {
    const priceValue = Number(price);
    if (!Number.isFinite(priceValue) || priceValue <= 0) return NaN;
    const decAdj = Math.pow(10, (Number(token0Decimals) || 18) - (Number(token1Decimals) || 18));
    const ratio = priceValue / decAdj;
    if (!Number.isFinite(ratio) || ratio <= 0) return NaN;
    return Math.log(ratio) / Math.log(1.0001);
}

export function normalizeDisplayPriceTickRange(
    lowerRaw,
    upperRaw,
    invertPrice,
    token0Decimals,
    token1Decimals,
    tickSpacing,
    minTick,
    maxTick,
) {
    const lowerDisplay = Number(String(lowerRaw || '').trim());
    const upperDisplay = Number(String(upperRaw || '').trim());
    if (!Number.isFinite(lowerDisplay) || !Number.isFinite(upperDisplay) || lowerDisplay <= 0 || upperDisplay <= 0) {
        return null;
    }
    const firstPoolPrice = invertPrice ? 1 / lowerDisplay : lowerDisplay;
    const secondPoolPrice = invertPrice ? 1 / upperDisplay : upperDisplay;
    const firstTick = poolPriceToTick(firstPoolPrice, token0Decimals, token1Decimals);
    const secondTick = poolPriceToTick(secondPoolPrice, token0Decimals, token1Decimals);
    if (!Number.isFinite(firstTick) || !Number.isFinite(secondTick)) return null;
    const spacing = Number(tickSpacing);
    const resolvedMinTick = Number(minTick);
    const resolvedMaxTick = Number(maxTick);
    let lowerTick = Number.isFinite(spacing) && spacing > 0
        ? roundDownToTickSpacing(Math.min(firstTick, secondTick), spacing)
        : Math.floor(Math.min(firstTick, secondTick));
    let upperTick = Number.isFinite(spacing) && spacing > 0
        ? roundUpToTickSpacing(Math.max(firstTick, secondTick), spacing)
        : Math.ceil(Math.max(firstTick, secondTick));
    if (Number.isFinite(resolvedMinTick)) lowerTick = Math.max(lowerTick, resolvedMinTick);
    if (Number.isFinite(resolvedMaxTick)) upperTick = Math.min(upperTick, resolvedMaxTick);
    if (Number.isFinite(spacing) && spacing > 0 && upperTick <= lowerTick) {
        if (Number.isFinite(resolvedMaxTick) && lowerTick + spacing > resolvedMaxTick) {
            lowerTick = upperTick - spacing;
        } else {
            upperTick = lowerTick + spacing;
        }
    }
    if (!Number.isFinite(lowerTick) || !Number.isFinite(upperTick) || upperTick <= lowerTick) return null;
    return { lowerTick: Math.trunc(lowerTick), upperTick: Math.trunc(upperTick) };
}

export function buildDisplayPriceRangeFromTicks(lowerTick, upperTick, invertPrice, token0Decimals, token1Decimals) {
    if (!Number.isInteger(lowerTick) || !Number.isInteger(upperTick) || upperTick <= lowerTick) return null;
    const firstPrice = tickToPoolPrice(lowerTick, token0Decimals, token1Decimals);
    const secondPrice = tickToPoolPrice(upperTick, token0Decimals, token1Decimals);
    if (!Number.isFinite(firstPrice) || !Number.isFinite(secondPrice) || firstPrice <= 0 || secondPrice <= 0) return null;
    const firstDisplay = invertPrice ? 1 / firstPrice : firstPrice;
    const secondDisplay = invertPrice ? 1 / secondPrice : secondPrice;
    if (!Number.isFinite(firstDisplay) || !Number.isFinite(secondDisplay) || firstDisplay <= 0 || secondDisplay <= 0) {
        return null;
    }
    return {
        lowerPrice: Math.min(firstDisplay, secondDisplay),
        upperPrice: Math.max(firstDisplay, secondDisplay),
    };
}

export function estimateDisplayGridStepPercent(currentTick, tickSpacing, invertPrice, token0Decimals, token1Decimals) {
    const baseTick = Number(currentTick);
    const spacing = Number(tickSpacing);
    if (!Number.isFinite(baseTick) || !Number.isFinite(spacing) || spacing <= 0) return null;
    const currentPoolPrice = tickToPoolPrice(baseTick, token0Decimals, token1Decimals);
    const nextPoolPrice = tickToPoolPrice(baseTick + spacing, token0Decimals, token1Decimals);
    if (!Number.isFinite(currentPoolPrice) || currentPoolPrice <= 0 || !Number.isFinite(nextPoolPrice) || nextPoolPrice <= 0) {
        return null;
    }
    const currentDisplay = invertPrice ? 1 / currentPoolPrice : currentPoolPrice;
    const nextDisplay = invertPrice ? 1 / nextPoolPrice : nextPoolPrice;
    if (!Number.isFinite(currentDisplay) || currentDisplay <= 0 || !Number.isFinite(nextDisplay) || nextDisplay <= 0) {
        return null;
    }
    return Math.abs(((nextDisplay / currentDisplay) - 1) * 100);
}

export function nudgeDisplayPriceBoundary(target, delta, invertPrice, tickSpacing, lowerTick, upperTick, minTick, maxTick) {
    const spacing = Number(tickSpacing);
    let nextLower = Number(lowerTick);
    let nextUpper = Number(upperTick);
    if (!Number.isFinite(spacing) || spacing <= 0) return null;
    if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper) || nextUpper <= nextLower) return null;

    const changedRawBoundary = target === 'lower'
        ? (invertPrice ? 'upper' : 'lower')
        : (invertPrice ? 'lower' : 'upper');

    if (target === 'lower') {
        if (invertPrice) nextUpper += delta * spacing;
        else nextLower -= delta * spacing;
    } else if (invertPrice) {
        nextLower -= delta * spacing;
    } else {
        nextUpper += delta * spacing;
    }

    const resolvedMinTick = Number(minTick);
    const resolvedMaxTick = Number(maxTick);
    if (Number.isFinite(resolvedMinTick)) nextLower = Math.max(nextLower, resolvedMinTick);
    if (Number.isFinite(resolvedMaxTick)) nextUpper = Math.min(nextUpper, resolvedMaxTick);

    if (changedRawBoundary === 'lower') {
        if (Number.isFinite(resolvedMaxTick) && nextLower > resolvedMaxTick - spacing) {
            nextLower = resolvedMaxTick - spacing;
        }
        if (nextLower >= nextUpper) nextUpper = nextLower + spacing;
        if (Number.isFinite(resolvedMaxTick) && nextUpper > resolvedMaxTick) {
            nextUpper = resolvedMaxTick;
            nextLower = nextUpper - spacing;
        }
    } else {
        if (Number.isFinite(resolvedMinTick) && nextUpper < resolvedMinTick + spacing) {
            nextUpper = resolvedMinTick + spacing;
        }
        if (nextUpper <= nextLower) nextLower = nextUpper - spacing;
        if (Number.isFinite(resolvedMinTick) && nextLower < resolvedMinTick) {
            nextLower = resolvedMinTick;
            nextUpper = nextLower + spacing;
        }
    }

    if (!Number.isInteger(nextLower) || !Number.isInteger(nextUpper) || nextUpper <= nextLower) return null;
    return { lowerTick: nextLower, upperTick: nextUpper };
}

export function roundDownToTickSpacing(tick, tickSpacing) {
    const spacing = Number(tickSpacing);
    const value = Number(tick);
    if (!Number.isFinite(spacing) || spacing <= 0 || !Number.isFinite(value)) return 0;
    const remainder = value % spacing;
    if (remainder === 0) return value;
    return value < 0 ? value - remainder - spacing : value - remainder;
}

export function roundUpToTickSpacing(tick, tickSpacing) {
    const spacing = Number(tickSpacing);
    const value = Number(tick);
    if (!Number.isFinite(spacing) || spacing <= 0 || !Number.isFinite(value)) return 0;
    const down = roundDownToTickSpacing(value, spacing);
    return down === value ? value : down + spacing;
}

export function buildGridBins(editor, radius = OPEN_POSITION_GRID_RADIUS) {
    const currentTick = Number(editor?.current_tick);
    const tickSpacing = Number(editor?.tick_spacing);
    if (!Number.isFinite(currentTick) || !Number.isFinite(tickSpacing) || tickSpacing <= 0) return [];
    const anchorLower = Number.isFinite(Number(editor?.anchor_tick_lower))
        ? Number(editor.anchor_tick_lower)
        : roundDownToTickSpacing(currentTick, tickSpacing);
    const anchorUpper = Number.isFinite(Number(editor?.anchor_tick_upper))
        ? Number(editor.anchor_tick_upper)
        : anchorLower + tickSpacing;
    const bins = [];
    for (let idx = -radius; idx <= radius; idx += 1) {
        let lowerTick;
        let upperTick;
        if (idx === 0) {
            lowerTick = anchorLower;
            upperTick = anchorUpper;
        } else if (idx > 0) {
            lowerTick = anchorUpper + (idx - 1) * tickSpacing;
            upperTick = lowerTick + tickSpacing;
        } else {
            upperTick = anchorLower + (idx + 1) * tickSpacing;
            lowerTick = upperTick - tickSpacing;
        }
        bins.push({
            key: `grid-${idx}`,
            index: idx,
            lowerTick,
            upperTick,
            isCurrent: idx === 0,
        });
    }
    return bins;
}

export function buildDefaultFocusedTickRange(editor, gridOffset = OPEN_POSITION_DEFAULT_GRID_OFFSET) {
    const currentTick = Number(editor?.current_tick);
    const tickSpacing = Number(editor?.tick_spacing);
    if (!Number.isFinite(currentTick) || !Number.isFinite(tickSpacing) || tickSpacing <= 0) return null;
    const offset = Math.max(1, Number(gridOffset) || OPEN_POSITION_DEFAULT_GRID_OFFSET);
    const anchorLower = Number.isFinite(Number(editor?.anchor_tick_lower))
        ? Number(editor.anchor_tick_lower)
        : roundDownToTickSpacing(currentTick, tickSpacing);
    const anchorUpper = Number.isFinite(Number(editor?.anchor_tick_upper))
        ? Number(editor.anchor_tick_upper)
        : anchorLower + tickSpacing;
    if (!Number.isInteger(anchorLower) || !Number.isInteger(anchorUpper) || anchorUpper <= anchorLower) return null;
    let lowerTick = anchorLower - offset * tickSpacing;
    let upperTick = anchorUpper + offset * tickSpacing;
    const minTick = Number(editor?.min_tick);
    const maxTick = Number(editor?.max_tick);
    if (Number.isFinite(minTick)) lowerTick = Math.max(lowerTick, minTick);
    if (Number.isFinite(maxTick)) upperTick = Math.min(upperTick, maxTick);
    if (upperTick <= lowerTick) {
        upperTick = lowerTick + tickSpacing;
        if (Number.isFinite(maxTick) && upperTick > maxTick) {
            upperTick = maxTick;
            lowerTick = upperTick - tickSpacing;
        }
    }
    if (!Number.isInteger(lowerTick) || !Number.isInteger(upperTick) || upperTick <= lowerTick) return null;
    return { lowerTick, upperTick };
}

export function buildDefaultFocusedPercentageRange(editor, gridOffset = OPEN_POSITION_DEFAULT_GRID_OFFSET) {
    const focused = buildDefaultFocusedTickRange(editor, gridOffset);
    const currentTick = Number(editor?.current_tick);
    if (!focused || !Number.isFinite(currentTick)) return null;
    const lowerPct = (1 - Math.pow(1.0001, focused.lowerTick - currentTick)) * 100;
    const upperPct = (Math.pow(1.0001, focused.upperTick - currentTick) - 1) * 100;
    if (!(lowerPct > 0) || !(upperPct > 0)) return null;
    return {
        lowerValue: formatPercentInputValue(lowerPct),
        upperValue: formatPercentInputValue(upperPct),
    };
}
