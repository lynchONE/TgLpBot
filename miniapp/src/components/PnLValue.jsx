import React from 'react';

function classifySign(value) {
    if (value === null || value === undefined || value === '') return 'flat';
    if (typeof value === 'number') {
        if (Number.isNaN(value)) return 'flat';
        if (value > 0) return 'pos';
        if (value < 0) return 'neg';
        return 'flat';
    }
    const text = String(value).trim();
    if (!text) return 'flat';
    if (text.startsWith('+')) return 'pos';
    if (text.startsWith('-') || text.startsWith('−')) return 'neg';
    const num = Number(text.replace(/[+,%$\s]/g, '').replace('−', '-'));
    if (Number.isFinite(num)) {
        if (num > 0) return 'pos';
        if (num < 0) return 'neg';
    }
    return 'flat';
}

const ARROW_POS = '▲';
const ARROW_NEG = '▼';
const ARROW_FLAT = '—';

/**
 * PnL 值的双编码呈现：符号 + 颜色。
 * - sign: 'auto' 自动从 value 推断，或显式传 'pos'/'neg'/'flat'
 * - 默认显示 children；如果 children 为空，则 fallback 到 value 原值
 * - animate=true 时给箭头加上微浮动动画
 */
export default function PnLValue({
    value,
    sign = 'auto',
    showArrow = true,
    animate = true,
    className = '',
    children,
}) {
    const resolvedSign = sign === 'auto' ? classifySign(value) : sign;
    const colorClass =
        resolvedSign === 'pos'
            ? 'text-emerald-600 dark:text-[#34d399]'
            : resolvedSign === 'neg'
                ? 'text-rose-600 dark:text-[#f87171]'
                : 'text-zinc-500 dark:text-white/55';

    let arrow = null;
    if (showArrow) {
        if (resolvedSign === 'pos') {
            arrow = <span className={`mr-1 inline-block ${animate ? 'pnl-arrow-up' : ''}`} aria-hidden="true">{ARROW_POS}</span>;
        } else if (resolvedSign === 'neg') {
            arrow = <span className={`mr-1 inline-block ${animate ? 'pnl-arrow-down' : ''}`} aria-hidden="true">{ARROW_NEG}</span>;
        } else {
            arrow = <span className="mr-1 inline-block text-zinc-400 dark:text-white/30" aria-hidden="true">{ARROW_FLAT}</span>;
        }
    }

    return (
        <span className={`inline-flex items-baseline ${colorClass} ${className}`}>
            {arrow}
            <span>{children ?? value}</span>
        </span>
    );
}
