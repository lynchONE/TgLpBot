import { useEffect, useState } from 'react';

export default function ConfirmDialog({
    open,
    title = '确认',
    message = '',
    confirmText = '确认',
    cancelText = '取消',
    danger = false,
    onConfirm,
    onCancel,
    loading = false,
}) {
    const [isVisible, setIsVisible] = useState(false);
    const [isAnimating, setIsAnimating] = useState(false);

    useEffect(() => {
        if (open) {
            setIsVisible(true);
            requestAnimationFrame(() => {
                requestAnimationFrame(() => setIsAnimating(true));
            });
        } else {
            setIsAnimating(false);
            const timer = setTimeout(() => setIsVisible(false), 200);
            return () => clearTimeout(timer);
        }
    }, [open]);

    if (!isVisible) return null;

    return (
        <div className="fixed inset-0 z-[70] flex items-center justify-center px-4">
            {/* Backdrop */}
            <div
                className={`absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity duration-200 ${isAnimating ? 'opacity-100' : 'opacity-0'}`}
                onClick={onCancel}
                aria-hidden="true"
            />

            {/* Dialog */}
            <div
                className={`relative w-full max-w-sm overflow-hidden rounded-2xl border border-zinc-200/50 bg-white/90 shadow-2xl backdrop-blur-xl transition-all duration-200 dark:border-white/10 dark:bg-[#1a1d24]/95 ${isAnimating ? 'scale-100 opacity-100' : 'scale-95 opacity-0'}`}
            >
                <div className="p-5">
                    {title && (
                        <h3 className="text-base font-bold text-zinc-900 dark:text-white/90">
                            {title}
                        </h3>
                    )}
                    {message && (
                        <p className="mt-2 text-sm leading-relaxed text-zinc-600 dark:text-white/60 whitespace-pre-line">
                            {message}
                        </p>
                    )}
                </div>
                <div className="flex items-center gap-3 border-t border-zinc-200/50 bg-zinc-50/50 px-5 py-3 dark:border-white/5 dark:bg-white/[0.02]">
                    <button
                        type="button"
                        onClick={onCancel}
                        disabled={loading}
                        className="flex-1 rounded-xl border border-zinc-200 bg-white px-4 py-2.5 text-sm font-semibold text-zinc-700 transition-colors hover:bg-zinc-50 active:bg-zinc-100 dark:border-white/10 dark:bg-white/5 dark:text-white/70 dark:hover:bg-white/10"
                    >
                        {cancelText}
                    </button>
                    <button
                        type="button"
                        onClick={onConfirm}
                        disabled={loading}
                        className={`flex-1 rounded-xl px-4 py-2.5 text-sm font-semibold transition-colors
                            ${loading ? 'cursor-not-allowed opacity-50' : ''}
                            ${danger
                                ? 'bg-red-500 text-white hover:bg-red-600 active:bg-red-700'
                                : 'bg-emerald-500 text-white hover:bg-emerald-600 active:bg-emerald-700 dark:bg-emerald-600 dark:hover:bg-emerald-500'
                            }`}
                    >
                        {loading ? (
                            <span className="flex items-center justify-center gap-2">
                                <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                                    <circle className="opacity-25" cx="12" cy="12" r="10" />
                                    <path className="opacity-75" d="M4 12a8 8 0 018-8" />
                                </svg>
                                处理中...
                            </span>
                        ) : confirmText}
                    </button>
                </div>
            </div>
        </div>
    );
}

