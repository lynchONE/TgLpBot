import React from 'react';
import { Check, ChevronLeft, ChevronRight, X } from 'lucide-react';

const OPEN_POSITION_STEPS = [
    { k: 0, label: '资金' },
    { k: 1, label: '区间' },
    { k: 2, label: '确认' },
];

export function OpenPositionFooter({
    openPositionError,
    openPositionStep,
    openPositionLoading,
    openPositionStep0Valid,
    openPositionStep1Valid,
    openPositionSubmitDisabled,
    brand,
    onPrevious,
    onNext,
    onSubmit,
}) {
    const nextDisabled = openPositionStep === 0 ? !openPositionStep0Valid : !openPositionStep1Valid;

    return (
        <div className="space-y-3">
            {openPositionError ? (
                <div className="rounded-2xl border border-red-500/40 bg-gradient-to-br from-red-500/10 to-transparent p-4 text-red-800 shadow-sm dark:border-red-500/30 dark:text-red-200">
                    <div className="flex items-start gap-3">
                        <div className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-red-500/20 text-red-600 dark:text-red-400">
                            <X className="h-3 w-3" strokeWidth={3} />
                        </div>
                        <div className="text-[12px] font-medium leading-relaxed">
                            {openPositionError}
                        </div>
                    </div>
                </div>
            ) : null}
            <div className="op-footer-actions flex items-center gap-2">
                {openPositionStep > 0 ? (
                    <button
                        type="button"
                        onClick={onPrevious}
                        disabled={openPositionLoading}
                        className="op-footer-secondary inline-flex shrink-0 items-center gap-1 rounded-2xl border border-zinc-200 bg-white px-4 py-3 text-sm font-semibold text-zinc-700 transition hover:bg-zinc-50 active:scale-[0.98] disabled:opacity-50 dark:border-white/10 dark:bg-white/5 dark:text-white/75 dark:hover:bg-white/10"
                    >
                        <ChevronLeft className="h-4 w-4" />
                        上一步
                    </button>
                ) : null}
                {openPositionStep < 2 ? (
                    <button
                        type="button"
                        onClick={onNext}
                        disabled={nextDisabled}
                        className={`op-footer-primary inline-flex flex-1 items-center justify-center gap-1 rounded-2xl px-3 py-3 text-sm font-semibold shadow-sm transition active:scale-[0.99] ${nextDisabled
                            ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 shadow-none dark:bg-white/10 dark:text-white/30'
                            : brand.solidButtonClass
                            }`}
                    >
                        下一步
                        <ChevronRight className="h-4 w-4" />
                    </button>
                ) : (
                    <button
                        type="button"
                        onClick={onSubmit}
                        disabled={openPositionSubmitDisabled}
                        className={`op-footer-primary flex-1 rounded-2xl px-3 py-3 text-sm font-semibold shadow-sm transition ${openPositionSubmitDisabled
                            ? 'cursor-not-allowed bg-zinc-200 text-zinc-500 shadow-none dark:bg-white/10 dark:text-white/30'
                            : brand.solidButtonClass
                            }`}
                    >
                        {openPositionLoading ? '开仓中...' : '确认开仓'}
                    </button>
                )}
            </div>
        </div>
    );
}

export function OpenPositionStepIndicator({ openPositionStep, brand, onStepClick }) {
    return (
        <div className="mb-3 flex items-center gap-1.5">
            {OPEN_POSITION_STEPS.map((s, i) => {
                const active = openPositionStep === s.k;
                const done = openPositionStep > s.k;
                return (
                    <React.Fragment key={s.k}>
                        <button
                            type="button"
                            onClick={() => { if (s.k < openPositionStep) onStepClick(s.k); }}
                            disabled={s.k > openPositionStep}
                            data-state={active ? 'active' : done ? 'done' : 'idle'}
                            className={`op-step-button op-step-button--visible flex shrink-0 items-center gap-1.5 rounded-full px-1.5 py-1 text-[12px] font-semibold transition ${active ? brand.textClass : done ? 'text-zinc-500 dark:text-white/55' : 'text-zinc-400 dark:text-white/30'}`}
                        >
                            <span
                                data-state={active ? 'active' : done ? 'done' : 'idle'}
                                className={`op-step-dot--visible flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-bold ${active ? brand.solidButtonClass : done ? 'bg-zinc-300 text-white dark:bg-white/25 dark:text-white' : 'bg-zinc-200 text-zinc-400 dark:bg-white/10 dark:text-white/40'}`}
                            >
                                {done ? <Check className="h-3 w-3" strokeWidth={3} /> : i + 1}
                            </span>
                            {s.label}
                        </button>
                        {i < 2 ? (
                            <div
                                data-state={done ? 'done' : active ? 'active' : 'idle'}
                                className={`op-step-connector op-step-connector--visible h-[2px] flex-1 rounded-full ${done ? 'bg-zinc-300 dark:bg-white/25' : 'bg-zinc-200 dark:bg-white/15'}`}
                                aria-hidden="true"
                            />
                        ) : null}
                    </React.Fragment>
                );
            })}
        </div>
    );
}
