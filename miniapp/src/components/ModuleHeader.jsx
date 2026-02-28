import React from 'react';
import NumberFlowValue from './NumberFlowValue.jsx';

export default function ModuleHeader({
    title,
    subtitle,
    actions = null,
    children = null,
    className = '',
}) {
    const subtitleNode = typeof subtitle === 'string'
        ? <NumberFlowValue value={subtitle} formatter={() => subtitle} />
        : subtitle;

    return (
        <div className={`mt-3 rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3.5 shadow-sm dark:border-white/5 dark:bg-[#16181c] dark:shadow-none ${className}`.trim()}>
            <div className="flex items-center justify-between gap-3">
                <div className="shrink-0 flex flex-col justify-center max-w-[40%]">
                    <div className="text-[14px] leading-tight font-extrabold text-zinc-900 dark:text-white/95 whitespace-nowrap truncate">{title}</div>
                    {subtitle ? (
                        <div className="mt-0.5 text-[10px] text-zinc-500 dark:text-white/40 whitespace-nowrap truncate">{subtitleNode}</div>
                    ) : null}
                </div>
                {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
            </div>
            {children ? <div className="mt-2.5">{children}</div> : null}
        </div>
    );
}

