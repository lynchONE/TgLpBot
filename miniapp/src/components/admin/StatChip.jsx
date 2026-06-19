import NumberFlowValue from '../NumberFlowValue.jsx';
import StatusDot from './StatusDot.jsx';

const TONE_ACCENT = {
    ok: 'before:bg-emerald-500',
    warn: 'before:bg-amber-500',
    danger: 'before:bg-red-500',
    idle: 'before:bg-zinc-400 dark:before:bg-zinc-600',
    accent: 'before:bg-sky-500',
};

const TONE_VALUE_TEXT = {
    ok: 'text-emerald-600 dark:text-emerald-300',
    warn: 'text-amber-600 dark:text-amber-300',
    danger: 'text-red-600 dark:text-red-300',
    idle: 'text-zinc-900 dark:text-white/90',
    accent: 'text-sky-600 dark:text-sky-300',
};

function isNumericLike(value) {
    if (value === null || value === undefined) return false;
    if (typeof value === 'number') return Number.isFinite(value);
    if (typeof value !== 'string') return false;
    return /^-?\d+(?:\.\d+)?$/.test(value.trim());
}

export default function StatChip({
    label,
    value,
    tone = 'idle',
    hint,
    onClick,
    pulse = false,
    formatOptions,
    className = '',
}) {
    const accent = TONE_ACCENT[tone] || TONE_ACCENT.idle;
    const valueClass = TONE_VALUE_TEXT[tone] || TONE_VALUE_TEXT.idle;
    const interactive = typeof onClick === 'function';

    const rendered = isNumericLike(value)
        ? <NumberFlowValue value={Number(value)} formatOptions={formatOptions || { maximumFractionDigits: 0 }} />
        : (value ?? '--');

    const baseClass = `group relative overflow-hidden rounded-2xl border border-zinc-200/70 bg-white/65 px-3 py-2.5 backdrop-blur-sm transition before:absolute before:left-0 before:top-0 before:h-full before:w-[3px] before:rounded-r-full dark:border-white/10 dark:bg-[#0f1116]/85 ${accent}`;
    const hoverClass = interactive
        ? 'cursor-pointer hover:border-zinc-300 hover:bg-white dark:hover:border-white/15 dark:hover:bg-[#15171c] active:scale-[0.98]'
        : '';

    const Tag = interactive ? 'button' : 'div';

    return (
        <Tag
            type={interactive ? 'button' : undefined}
            onClick={onClick}
            className={`${baseClass} ${hoverClass} ${className}`.trim()}
        >
            <div className="flex items-center justify-between gap-2">
                <div className="text-[10px] font-semibold uppercase tracking-[0.12em] text-zinc-500 dark:text-white/45">
                    {label}
                </div>
                {pulse ? <StatusDot tone={tone === 'idle' ? 'accent' : tone} pulse size="xs" /> : null}
            </div>
            <div className={`mt-1 truncate text-[18px] font-black leading-tight tabular-nums ${valueClass}`}>
                {rendered}
            </div>
            {hint ? (
                <div className="mt-0.5 truncate text-[10px] font-medium text-zinc-400 dark:text-white/35">
                    {hint}
                </div>
            ) : null}
        </Tag>
    );
}
