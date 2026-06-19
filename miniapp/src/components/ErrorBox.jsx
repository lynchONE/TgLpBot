import { AlertTriangle, XCircle, Info } from 'lucide-react';

const TONE_MAP = {
    error: {
        wrap: 'border-rose-200 bg-rose-50/80 text-rose-700 dark:border-rose-500/30 dark:bg-rose-500/10 dark:text-rose-200',
        icon: XCircle,
    },
    warning: {
        wrap: 'border-amber-200 bg-amber-50/80 text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200',
        icon: AlertTriangle,
    },
    info: {
        wrap: 'border-sky-200 bg-sky-50/80 text-sky-800 dark:border-sky-500/30 dark:bg-sky-500/10 dark:text-sky-200',
        icon: Info,
    },
};

/**
 * 统一错误/警告/信息提示框。
 * - tone: error | warning | info
 * - title: 可选标题
 * - children: 描述内容（也接受 message 属性作为短形式）
 * - action: 行动按钮 { label, onClick }
 */
export default function ErrorBox({
    tone = 'error',
    title,
    message,
    action,
    icon: IconOverride,
    className = '',
    children,
}) {
    const cfg = TONE_MAP[tone] ?? TONE_MAP.error;
    const IconComp = IconOverride ?? cfg.icon;
    return (
        <div
            role="alert"
            className={`flex items-start gap-2.5 rounded-xl border px-3 py-2.5 text-sm ${cfg.wrap} ${className}`}
        >
            {IconComp ? <IconComp className="mt-0.5 h-4 w-4 shrink-0" /> : null}
            <div className="min-w-0 flex-1">
                {title ? <div className="font-semibold leading-snug">{title}</div> : null}
                {children ?? (message ? <div className="leading-relaxed">{message}</div> : null)}
            </div>
            {action ? (
                <button
                    type="button"
                    onClick={action.onClick}
                    className="shrink-0 rounded-lg border border-current/30 bg-white/40 px-2.5 py-1 text-[12px] font-semibold transition hover:bg-white/60 dark:bg-black/20 dark:hover:bg-black/30"
                >
                    {action.label}
                </button>
            ) : null}
        </div>
    );
}
