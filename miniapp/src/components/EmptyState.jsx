import React from 'react';

/**
 * 统一空状态组件。
 * - icon: ReactNode（可选）
 * - title: 主标题
 * - description: 描述文字（可选）
 * - cta: { label, onClick, variant } 行动按钮（可选）
 * - compact: 紧凑模式（用在小卡片或列表项内）
 */
export default function EmptyState({
    icon,
    title,
    description,
    cta,
    compact = false,
    className = '',
}) {
    const padding = compact ? 'px-4 py-6' : 'px-6 py-10';
    const ctaButtonClass =
        cta?.variant === 'primary'
            ? 'inline-flex items-center justify-center gap-1.5 rounded-xl bg-zinc-900 px-4 py-2 text-sm font-semibold text-white shadow-sm transition hover:bg-zinc-800 active:scale-[0.98] dark:bg-white/10 dark:text-white dark:hover:bg-white/20'
            : 'inline-flex items-center justify-center gap-1.5 rounded-xl border border-zinc-200 bg-white px-4 py-2 text-sm font-semibold text-zinc-700 shadow-sm transition hover:bg-zinc-50 active:scale-[0.98] dark:border-white/10 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10';

    return (
        <div
            className={`flex flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-zinc-200 bg-white/60 text-center text-sm text-zinc-500 dark:border-white/10 dark:bg-[#14171c]/60 dark:text-white/55 ${padding} ${className}`}
        >
            {icon ? <div className="text-zinc-400 dark:text-white/35">{icon}</div> : null}
            <div className="font-semibold text-zinc-700 dark:text-white/85">{title}</div>
            {description ? (
                <div className="max-w-sm text-[12px] leading-relaxed text-zinc-500 dark:text-white/45">
                    {description}
                </div>
            ) : null}
            {cta ? (
                <button type="button" onClick={cta.onClick} className={`mt-2 ${ctaButtonClass}`}>
                    {cta.label}
                </button>
            ) : null}
        </div>
    );
}
