import React from 'react';
import { createRoot } from 'react-dom/client';
import SwapModule from './components/SwapModule.jsx';

const MOCK_WALLETS = [
    {
        id: 101,
        address: '0x1111111111111111111111111111111111111111',
        name: 'Mock Alpha',
        is_default: true,
        native_balance: '1.234567',
        stable_balance: '520.50',
    },
    {
        id: 102,
        address: '0x2222222222222222222222222222222222222222',
        name: 'Mock Beta',
        is_default: false,
        native_balance: '0.083',
        stable_balance: '88.10',
    },
];

const originalFetch = window.fetch.bind(window);

function wait(ms, signal) {
    return new Promise((resolve, reject) => {
        const timer = window.setTimeout(resolve, ms);
        signal?.addEventListener('abort', () => {
            window.clearTimeout(timer);
            reject(new DOMException('Aborted', 'AbortError'));
        }, { once: true });
    });
}

window.fetch = async (input, init) => {
    const url = typeof input === 'string' ? input : input?.url || '';
    const endpoint = new URL(url, window.location.origin).searchParams.get('endpoint');
    if (endpoint === 'wallets') {
        await wait(1500, init?.signal);
        return Response.json({
            ok: true,
            chain: 'bsc',
            native_symbol: 'BNB',
            stable_symbol: 'USDT',
            wallets: MOCK_WALLETS,
        });
    }
    if (endpoint === 'wallet_swap_preview') {
        return Response.json({
            ok: true,
            chain: 'bsc',
            tokens: [
                {
                    address: '0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',
                    symbol: 'BNB',
                    name: 'BNB',
                    balance: '1.234567',
                    balance_raw: '1234567000000000000',
                    decimals: 18,
                    value_usdt: 760,
                    is_native: true,
                    can_swap: true,
                },
                {
                    address: '0x55d398326f99059ff775485246999027b3197955',
                    symbol: 'USDT',
                    name: 'Tether USD',
                    balance: '520.50',
                    balance_raw: '520500000000000000000',
                    decimals: 18,
                    value_usdt: 520.5,
                    can_swap: true,
                },
            ],
        });
    }
    if (endpoint === 'wallet_swap_token_metadata') {
        return Response.json({ ok: true, chain: 'bsc', tokens: [] });
    }
    return originalFetch(input, init);
};

function MockApp() {
    const [notices, setNotices] = React.useState([]);
    const showNotice = React.useCallback((message, tone = 'info') => {
        setNotices((prev) => [...prev.slice(-2), { message: String(message), tone }]);
    }, []);

    return (
        <div className="miniapp-shell min-h-screen bg-zinc-100 p-4 text-zinc-950 dark:bg-[#05070a] dark:text-white">
            <div className="mx-auto max-w-md">
                <SwapModule
                    apiBaseUrl=""
                    initData="mock-init-data"
                    hasInitData
                    pollIntervalSec={30}
                    multiChainEnabled
                    onNotice={showNotice}
                />
                {notices.length ? (
                    <div className="mt-3 space-y-2">
                        {notices.map((notice, index) => (
                            <div key={`${notice.message}:${index}`} className="rounded-xl bg-white px-3 py-2 text-xs text-zinc-700 shadow dark:bg-white/10 dark:text-white/70">
                                {notice.tone}: {notice.message}
                            </div>
                        ))}
                    </div>
                ) : null}
            </div>
        </div>
    );
}

createRoot(document.getElementById('root')).render(<MockApp />);
