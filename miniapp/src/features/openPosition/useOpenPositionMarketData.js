import { useEffect, useRef } from 'react';
import { fetchGlobalConfig, fetchPoolLiquidityDistribution } from '../../lib/api';
import { fetchSMPoolStats } from '../../lib/smartMoneyApi';
import {
    normalizeOpenPositionPoolVersion,
    resolveOpenPositionPoolChain,
} from './safety';

export default function useOpenPositionMarketData({
    apiBaseUrl,
    initData,
    hasInitData,
    openPositionPool,
    openPositionShowLiquidityChart,
    setGlobalConfig,
    setOpenPositionSmartRanges,
    setOpenPositionSmartRangesLoading,
    setOpenPositionLiqProfile,
    setOpenPositionLiqProfileLoading,
    setOpenPositionLiqProfileError,
}) {
    useEffect(() => {
        if (!openPositionPool || !hasInitData) return;

        let aborted = false;
        const controller = new AbortController();

        fetchGlobalConfig({ apiBaseUrl, initData, signal: controller.signal })
            .then((resp) => {
                if (aborted) return;
                setGlobalConfig(resp?.config || resp || null);
            })
            .catch(() => {
                // ignore; keep existing config
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [apiBaseUrl, initData, hasInitData, openPositionPool, setGlobalConfig]);

    useEffect(() => {
        if (!openPositionPool) return;

        let aborted = false;
        const controller = new AbortController();
        const poolAddress = String(openPositionPool?.pool_address || '').trim();
        if (!poolAddress) {
            setOpenPositionSmartRanges([]);
            setOpenPositionSmartRangesLoading(false);
            return undefined;
        }

        setOpenPositionSmartRangesLoading(true);
        fetchSMPoolStats({ apiBaseUrl, poolAddress, signal: controller.signal })
            .then((resp) => {
                if (aborted) return;
                const nextGroups = Array.isArray(resp?.range_groups) ? resp.range_groups : [];
                setOpenPositionSmartRanges((prev) => (nextGroups.length > 0 ? nextGroups : prev));
            })
            .catch(() => {
                if (aborted) return;
            })
            .finally(() => {
                if (aborted) return;
                setOpenPositionSmartRangesLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [
        apiBaseUrl,
        openPositionPool,
        setOpenPositionSmartRanges,
        setOpenPositionSmartRangesLoading,
    ]);

    const openPositionLiqInFlightRef = useRef(false);

    useEffect(() => {
        if (!openPositionPool || !hasInitData || !openPositionShowLiquidityChart) {
            setOpenPositionLiqProfile(null);
            setOpenPositionLiqProfileError('');
            return undefined;
        }
        const poolAddress = String(openPositionPool?.pool_address || '').trim();
        const chain = resolveOpenPositionPoolChain(openPositionPool, 'bsc');
        const protocol = normalizeOpenPositionPoolVersion(openPositionPool);
        if (!poolAddress || !protocol) {
            setOpenPositionLiqProfile(null);
            return undefined;
        }
        setOpenPositionLiqProfile(null);
        const ctrl = new AbortController();
        setOpenPositionLiqProfileLoading(true);
        setOpenPositionLiqProfileError('');
        fetchPoolLiquidityDistribution({
            apiBaseUrl,
            initData,
            chain,
            protocol,
            address: poolAddress,
            radius: 24,
            signal: ctrl.signal,
        })
            .then((data) => {
                if (ctrl.signal.aborted) return;
                setOpenPositionLiqProfile(data);
            })
            .catch((err) => {
                if (ctrl.signal.aborted) return;
                const msg = String(err?.message || err || '');
                if (/page could not be found|<html|<!doctype/i.test(msg)) {
                    console.warn('[liquidity_distribution] endpoint unavailable', msg.slice(0, 200));
                    setOpenPositionLiqProfileError('流动性分布接口不可用');
                } else {
                    setOpenPositionLiqProfileError(msg.slice(0, 60));
                }
                setOpenPositionLiqProfile(null);
            })
            .finally(() => {
                if (!ctrl.signal.aborted) setOpenPositionLiqProfileLoading(false);
            });

        const timer = setInterval(() => {
            if (document.hidden) return;
            if (openPositionLiqInFlightRef.current) return;
            openPositionLiqInFlightRef.current = true;
            fetchPoolLiquidityDistribution({
                apiBaseUrl, initData, chain, protocol, address: poolAddress, radius: 24,
            })
                .then((data) => { setOpenPositionLiqProfile(data); setOpenPositionLiqProfileError(''); })
                .catch((err) => {
                    const msg = String(err?.message || err || '');
                    if (/page could not be found|<html|<!doctype/i.test(msg)) {
                        setOpenPositionLiqProfileError('流动性分布接口不可用');
                    } else {
                        setOpenPositionLiqProfileError(msg.slice(0, 60));
                    }
                })
                .finally(() => { openPositionLiqInFlightRef.current = false; });
        }, 3000);

        return () => {
            ctrl.abort();
            clearInterval(timer);
        };
    }, [
        apiBaseUrl,
        initData,
        hasInitData,
        openPositionPool,
        openPositionShowLiquidityChart,
        setOpenPositionLiqProfile,
        setOpenPositionLiqProfileError,
        setOpenPositionLiqProfileLoading,
    ]);
}
