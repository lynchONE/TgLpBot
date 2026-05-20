import React, { useCallback, useEffect, useMemo, useState } from 'react';
import BottomSheet from '../BottomSheet.jsx';
import PositionCard from '../PositionCard.jsx';
import StatusDot from './StatusDot.jsx';
import { fetchAdminRealtimePositions, fetchAdminUserAccess } from '../../lib/api';

function formatUserLabel(user) {
    if (!user) return '--';
    const first = String(user.first_name || '').trim();
    const last = String(user.last_name || '').trim();
    const username = String(user.username || '').trim();
    const fullName = [first, last].filter(Boolean).join(' ');
    if (fullName && username) return `${fullName} (@${username})`;
    if (fullName) return fullName;
    if (username) return `@${username}`;
    return `用户 ${user.user_id || '--'}`;
}

function shortAddress(address) {
    const s = String(address || '').trim();
    if (s.length < 12) return s || '--';
    return `${s.slice(0, 6)}…${s.slice(-4)}`;
}

export default function AdminUserDetailDrawer({
    open,
    user,
    apiBaseUrl,
    initData,
    hasInitData,
    pollIntervalSec = 15,
    onClose,
}) {
    const userId = user?.user_id || null;
    const [positions, setPositions] = useState(null);
    const [access, setAccess] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');

    const loadPositions = useCallback(async () => {
        if (!hasInitData || !userId) return;
        setLoading(true);
        setError('');
        try {
            const data = await fetchAdminRealtimePositions({ apiBaseUrl, initData, userId });
            setPositions(data || null);
        } catch (e) {
            setError(String(e?.message || e));
        } finally {
            setLoading(false);
        }
    }, [apiBaseUrl, initData, hasInitData, userId]);

    const loadAccess = useCallback(async () => {
        if (!hasInitData || !userId) return;
        try {
            const data = await fetchAdminUserAccess({ apiBaseUrl, initData, userId });
            setAccess(data || null);
        } catch {
            setAccess(null);
        }
    }, [apiBaseUrl, initData, hasInitData, userId]);

    useEffect(() => {
        if (!open) return undefined;
        setPositions(null);
        setAccess(null);
        setError('');
        loadPositions();
        loadAccess();
    }, [open, loadPositions, loadAccess]);

    useEffect(() => {
        if (!open || !hasInitData || !userId) return undefined;
        const timer = setInterval(loadPositions, Math.max(5, pollIntervalSec) * 1000);
        return () => clearInterval(timer);
    }, [open, hasInitData, userId, loadPositions, pollIntervalSec]);

    const positionsList = useMemo(() => (Array.isArray(positions?.positions) ? positions.positions : []), [positions]);
    const wallet = positions?.wallet || null;
    const accessTone = String(access?.status || '').toLowerCase() === 'active' ? 'ok' : 'danger';

    return (
        <BottomSheet
            open={open}
            onClose={onClose}
            title={user ? formatUserLabel(user) : '用户详情'}
            maxHeightClass="max-h-[92vh]"
            headerRight={(
                <button
                    type="button"
                    onClick={loadPositions}
                    disabled={loading || !userId}
                    className="inline-flex h-8 items-center gap-1 rounded-full bg-zinc-100 px-3 text-[11px] font-semibold text-zinc-700 transition hover:bg-zinc-200 disabled:opacity-40 dark:bg-white/10 dark:text-white/70 dark:hover:bg-white/15"
                >
                    {loading ? '刷新中…' : '刷新'}
                </button>
            )}
        >
            <div className="space-y-3">
                <div className="rounded-2xl border border-zinc-200 bg-zinc-50/80 p-3 dark:border-white/10 dark:bg-[#15171c]">
                    <div className="grid grid-cols-2 gap-x-3 gap-y-2 text-[11px]">
                        <div>
                            <div className="text-zinc-400 dark:text-white/35">TG ID</div>
                            <div className="mt-0.5 font-mono text-zinc-800 dark:text-white/85">{user?.telegram_id || '--'}</div>
                        </div>
                        <div>
                            <div className="text-zinc-400 dark:text-white/35">用户 ID</div>
                            <div className="mt-0.5 font-mono text-zinc-800 dark:text-white/85">{userId || '--'}</div>
                        </div>
                        <div className="col-span-2">
                            <div className="text-zinc-400 dark:text-white/35">钱包</div>
                            <div className="mt-0.5 break-all font-mono text-zinc-800 dark:text-white/85">{wallet?.address || '--'}</div>
                        </div>
                        <div>
                            <div className="text-zinc-400 dark:text-white/35">BNB 余额</div>
                            <div className="mt-0.5 font-mono text-zinc-800 dark:text-white/85">{wallet?.bnb_balance ?? '--'}</div>
                        </div>
                        <div>
                            <div className="flex items-center gap-1.5 text-zinc-400 dark:text-white/35">
                                <StatusDot tone={accessTone} size="xs" />
                                授权状态
                            </div>
                            <div className="mt-0.5 text-zinc-800 dark:text-white/85">
                                {access?.status === 'active' ? '生效中' : access?.status || '--'}
                            </div>
                        </div>
                    </div>
                </div>

                {error && (
                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-700 dark:text-red-200">
                        {error}
                    </div>
                )}

                {loading && positionsList.length === 0 && (
                    <div className="rounded-xl border border-zinc-200 bg-white/40 p-4 text-center text-xs text-zinc-500 dark:border-white/10 dark:bg-white/5 dark:text-white/60">
                        加载用户仓位中…
                    </div>
                )}

                {!loading && userId && positionsList.length === 0 && !error && (
                    <div className="rounded-xl border border-dashed border-zinc-200 p-6 text-center text-xs text-zinc-400 dark:border-white/10 dark:text-white/35">
                        当前用户没有活跃仓位
                    </div>
                )}

                {positionsList.length > 0 && (
                    <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-400 dark:text-white/35">
                        活跃仓位 · {positionsList.length}
                    </div>
                )}

                {positionsList.map((position) => (
                    <PositionCard
                        key={[
                            String(position?.chain || ''),
                            String(position?.version || ''),
                            String(position?.pool_id || ''),
                            String(position?.position_id || ''),
                            String(position?.task_id || ''),
                        ].join(':')}
                        position={position}
                        walletAddress={wallet?.address || ''}
                        bnbBalance={wallet?.bnb_balance || ''}
                        pollIntervalSec={pollIntervalSec}
                        updatedAt={positions?.updated_at}
                        allowTaskActions={false}
                    />
                ))}
            </div>
        </BottomSheet>
    );
}
