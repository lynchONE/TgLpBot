import React from 'react';

export default function ModuleHeader({
    title,
    subtitle,
    actions = null,
    children = null,
    className = '',
}) {
    return (
        <div className={`mt-3 rounded-2xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-white/10 dark:bg-[#111318] dark:shadow-none ${className}`.trim()}>
            <div className="flex items-center justify-between gap-3">
                <div>
                    <div className="text-sm font-semibold text-zinc-900 dark:text-white/90">{title}</div>
                    {subtitle ? (
                        <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-white/40">{subtitle}</div>
                    ) : null}
                </div>
                {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
            </div>
            {children ? <div className="mt-3">{children}</div> : null}
        </div>
    );
}

