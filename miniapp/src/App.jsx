import React, { useEffect, useMemo, useRef, useState } from 'react';
import PositionCard from './components/PositionCard.jsx';
import { fetchRealtimePositions } from './lib/api';
import { getTelegramWebApp } from './lib/telegram';

function resolveApiBaseUrl() {
    const queryApiBase = new URLSearchParams(window.location.search).get('apiBaseUrl');
    if (queryApiBase && queryApiBase.trim()) return queryApiBase.trim();

    const envBase = String(import.meta.env.VITE_API_BASE_URL || '').trim();
    if (envBase) {
        try {
            const pageProto = window.location.protocol;
            const envProto = new URL(envBase).protocol;
            if (pageProto === 'https:' && envProto === 'http:') {
                return '';
            }
        } catch {
            // ignore URL parse errors and keep envBase as-is
        }
        return envBase;
    }

    const host = window.location.hostname;
    if (host === 'localhost' || host === '127.0.0.1') {
        return 'http://localhost:8080';
    }

    // Production default: same-origin `/api/*` (e.g. via Vercel Function proxy)
    return '';
}

function useInitData() {
    const [initData, setInitData] = useState('');
    useEffect(() => {
        const tg = getTelegramWebApp();
        if (!tg) {
            const fromQuery = new URLSearchParams(window.location.search).get('initData');
            if (fromQuery) setInitData(fromQuery);
            return;
        }
        try {
            tg.ready?.();
            tg.expand?.();
        } catch {
            // ignore
        }
        setInitData(tg.initData || '');
    }, []);
    return initData;
}

export default function App() {
    const initData = useInitData();
    const [data, setData] = useState(null);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(false);
    const pollRef = useRef(null);

    const pollIntervalSec = Math.max(1, Number(data?.poll_interval_sec || 1));
    const updatedAt = data?.updated_at;

    const walletAddress = data?.wallet?.address || '';
    const bnbBalance = data?.wallet?.bnb_balance || '0.000000';
    const positions = data?.positions || [];

    const apiBaseUrl = useMemo(() => resolveApiBaseUrl(), []);

    useEffect(() => {
        if (!initData) return;
        let aborted = false;
        const controller = new AbortController();

        const run = async () => {
            setLoading(true);
            setError('');
            try {
                const resp = await fetchRealtimePositions({ apiBaseUrl, initData, signal: controller.signal });
                if (aborted) return;
                setData(resp);
            } catch (e) {
                if (aborted) return;
                setError(String(e?.message || e));
            } finally {
                if (!aborted) setLoading(false);
            }
        };

        run();

        if (pollRef.current) clearInterval(pollRef.current);
        pollRef.current = setInterval(run, pollIntervalSec * 1000);

        return () => {
            aborted = true;
            controller.abort();
            if (pollRef.current) clearInterval(pollRef.current);
        };
    }, [apiBaseUrl, initData, pollIntervalSec]);

    return (
        <div className="min-h-screen bg-[#0b0c0f] px-4 py-4 text-white">
            <header className="mb-4 flex items-center justify-between">
                <div>
                    <div className="text-lg font-extrabold tracking-tight">实时仓位</div>
                    <div className="mt-0.5 text-xs text-white/40">
                        {walletAddress ? `钱包：${walletAddress.slice(0, 6)}...${walletAddress.slice(-4)}` : '加载钱包中...'}
                    </div>
                </div>
                <div className="text-right">
                    <div className="text-[11px] text-white/40">刷新间隔</div>
                    <div className="text-sm font-semibold tabular-nums">{pollIntervalSec}s</div>
                </div>
            </header>

            {error ? (
                <div className="mb-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">
                    {error}
                </div>
            ) : null}

            {loading && !data ? (
                <div className="rounded-2xl border border-white/10 bg-white/5 p-6 text-sm text-white/60">加载中...</div>
            ) : null}

            {!loading && data && positions.length === 0 ? (
                <div className="rounded-2xl border border-white/10 bg-white/5 p-6 text-sm text-white/60">
                    暂无仓位。请先在机器人里创建/导入钱包并开仓。
                </div>
            ) : null}

            <div className="space-y-4">
                {positions.map((p) => (
                    <PositionCard
                        key={`${p.version}:${p.position_id}`}
                        position={p}
                        walletAddress={walletAddress}
                        bnbBalance={bnbBalance}
                        pollIntervalSec={pollIntervalSec}
                        updatedAt={updatedAt}
                    />
                ))}
            </div>

            {data?.warnings?.length ? (
                <div className="mt-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 text-xs text-amber-200">
                    <div className="font-semibold">提示</div>
                    <ul className="mt-1 list-disc space-y-1 pl-4">
                        {data.warnings.map((w, i) => (
                            <li key={String(i)}>{w}</li>
                        ))}
                    </ul>
                </div>
            ) : null}
        </div>
    );
}
