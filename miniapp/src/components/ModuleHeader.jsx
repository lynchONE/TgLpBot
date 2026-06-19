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
        <div className={`mini-module-header mt-3 rounded-2xl border border-zinc-200 bg-white/40 backdrop-blur-md p-3.5 shadow-sm dark:border-white/5 dark:bg-[#16181c] dark:shadow-none ${className}`.trim()}>
            <div className="mini-module-header-main flex items-center justify-between gap-3">
                <div className="mini-module-title flex min-w-0 flex-col justify-center">
                    <div className="mini-module-title-text truncate text-[14px] font-extrabold leading-tight text-zinc-900 dark:text-white/95">{title}</div>
                    {subtitle ? (
                        <div className="mini-module-subtitle mt-0.5 truncate text-[10px] text-zinc-500 dark:text-white/40">{subtitleNode}</div>
                    ) : null}
                </div>
                {actions ? <div className="mini-module-actions flex shrink-0 items-center gap-2">{actions}</div> : null}
            </div>
            {children ? <div className="mini-module-header-body mt-2.5">{children}</div> : null}
        </div>
    );
}
