import React from 'react';

function resolveErrorText(value) {
    if (!value) return '';
    if (typeof value === 'string') return value;
    if (value instanceof Error) return value.message || String(value);
    try {
        return JSON.stringify(value);
    } catch {
        return String(value);
    }
}

export default class ErrorBoundary extends React.Component {
    constructor(props) {
        super(props);
        this.state = { errorText: '' };
        this.onWindowError = this.onWindowError.bind(this);
        this.onUnhandledRejection = this.onUnhandledRejection.bind(this);
    }

    static getDerivedStateFromError(error) {
        return { errorText: resolveErrorText(error) || '页面发生错误' };
    }

    componentDidCatch(error) {
        try {
            // eslint-disable-next-line no-console
            console.error('[MiniApp] render error:', error);
        } catch {
            // ignore
        }
        if (!this.state.errorText) {
            this.setState({ errorText: resolveErrorText(error) || '页面发生错误' });
        }
    }

    componentDidMount() {
        if (typeof window === 'undefined') return;
        window.addEventListener('error', this.onWindowError);
        window.addEventListener('unhandledrejection', this.onUnhandledRejection);
    }

    componentWillUnmount() {
        if (typeof window === 'undefined') return;
        window.removeEventListener('error', this.onWindowError);
        window.removeEventListener('unhandledrejection', this.onUnhandledRejection);
    }

    onWindowError(event) {
        if (this.state.errorText) return;
        const text = resolveErrorText(event?.error) || resolveErrorText(event?.message) || '页面发生错误';
        this.setState({ errorText: text });
    }

    onUnhandledRejection(event) {
        if (this.state.errorText) return;
        const text = resolveErrorText(event?.reason) || '页面发生错误';
        this.setState({ errorText: text });
    }

    render() {
        if (this.state.errorText) {
            const text = this.state.errorText;
            return (
                <div className="min-h-screen bg-white px-4 py-6 text-zinc-900 dark:bg-[#0b0f14] dark:text-white/90">
                    <div className="mx-auto max-w-xl">
                        <div className="rounded-2xl border border-red-500/30 bg-red-500/10 p-4">
                            <div className="text-sm font-extrabold text-red-700 dark:text-red-200">页面发生错误</div>
                            <div className="mt-2 break-words text-xs text-red-700/90 dark:text-red-200/90">{text}</div>
                            <div className="mt-4 flex flex-wrap gap-2">
                                <button
                                    type="button"
                                    onClick={() => window.location.reload()}
                                    className="rounded-xl bg-red-600 px-4 py-2 text-xs font-semibold text-white hover:bg-red-700 active:bg-red-800"
                                >
                                    重新加载
                                </button>
                                <button
                                    type="button"
                                    onClick={() => {
                                        try {
                                            window.Telegram?.WebApp?.close?.();
                                        } catch {
                                            // ignore
                                        }
                                    }}
                                    className="rounded-xl bg-zinc-200 px-4 py-2 text-xs font-semibold text-zinc-700 hover:bg-zinc-300 active:bg-zinc-400 dark:bg-white/10 dark:text-white/80 dark:hover:bg-white/15"
                                >
                                    关闭
                                </button>
                            </div>
                        </div>

                        <div className="mt-4 text-[11px] text-zinc-500 dark:text-white/40">
                            建议：从机器人重新打开页面；或升级 Telegram/系统 WebView 后再试。
                        </div>
                    </div>
                </div>
            );
        }

        return this.props.children;
    }
}

