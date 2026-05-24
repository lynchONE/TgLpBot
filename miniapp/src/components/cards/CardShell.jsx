import React from 'react';

const PADDING_MAP = {
    none: '',
    xs: 'px-2.5 py-2',
    sm: 'px-3 py-2.5',
    md: 'px-4 py-3.5',
    lg: 'px-5 py-5',
    xl: 'px-6 py-6',
};

const VARIANT_MAP = {
    base:
        'rounded-2xl border border-zinc-200/80 bg-white dark:border-white/10 dark:bg-[#14171c]',
    elevated:
        'rounded-2xl border border-zinc-200/80 bg-white shadow-[0_8px_24px_-12px_rgba(15,23,42,0.18)] dark:border-white/10 dark:bg-[#14171c] dark:shadow-[0_8px_24px_-12px_rgba(0,0,0,0.6)]',
    flat:
        'rounded-xl border border-zinc-200/60 bg-zinc-50/80 dark:border-white/5 dark:bg-[#1c2026]/80',
    inset:
        'rounded-xl border border-zinc-200/60 bg-zinc-50/60 dark:border-white/5 dark:bg-[#0f1116]/60',
    ghost:
        'rounded-2xl border border-transparent bg-transparent',
};

/**
 * 统一卡片容器。
 * - variant: base | elevated | flat | inset | ghost
 * - padding: none | xs | sm | md | lg | xl
 * - 默认 base + md，可通过 className 追加任意样式
 */
export default function CardShell({
    variant = 'base',
    padding = 'md',
    className = '',
    as: Tag = 'div',
    children,
    ...rest
}) {
    const base = VARIANT_MAP[variant] ?? VARIANT_MAP.base;
    const pad = PADDING_MAP[padding] ?? PADDING_MAP.md;
    return (
        <Tag className={`${base} ${pad} ${className}`} {...rest}>
            {children}
        </Tag>
    );
}
