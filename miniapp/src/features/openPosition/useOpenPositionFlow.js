import { useCallback, useEffect, useMemo, useRef } from 'react';
import {
    fetchWallets,
    openPosition,
    prepareOpenPosition,
    previewOpenPosition,
} from '../../lib/api';
import { storage } from '../../lib/storage';
import { STORAGE_OPEN_POSITION_WALLET_ID } from './constants';
import { parseOptionalPercent } from './format';
import { parseRangeInput } from './range';
import {
    buildEntrySwapConfirmKey,
    extractOpenPositionErrorChecks,
    resolveOpenPositionErrorPayload,
} from './safety';

export default function useOpenPositionFlow({
    apiBaseUrl,
    initData,
    hasInitData,
    multiWalletEnabled,
    walletsData,
    walletsError,
    walletsLoading,
    setWalletsData,
    setWalletsError,
    setWalletsLoading,
    draft,
    activeOpenPositionChecks,
    openPositionTickLowerValue,
    openPositionTickUpperValue,
    openPositionSelectedManualTickLower,
    openPositionSelectedManualTickUpper,
    openPositionIsSingleSidedSelection,
    openPositionGlobalDCAMinSplitAmount,
    operationProgress,
    setOperationProgress,
    requestConfirm,
    refreshRealtimePositionsNow,
}) {
    const {
        openPositionPool,
        setOpenPositionPool,
        openPositionAmount,
        openPositionRangeLower,
        openPositionRangeUpper,
        openPositionRangeInputMode,
        openPositionTickLower,
        openPositionTickUpper,
        openPositionPriceLower,
        openPositionPriceUpper,
        openPositionSlippage,
        openPositionError,
        setOpenPositionError,
        setOpenPositionPrepareChecks,
        setOpenPositionChecks,
        openPositionRiskAck,
        openPositionEntrySwapPreview,
        setOpenPositionEntrySwapPreview,
        setOpenPositionEntrySwapPreviewLoading,
        openPositionPreviewPending,
        setOpenPositionPreviewPending,
        openPositionPreviewSuspended,
        setOpenPositionPreviewSuspended,
        openPositionEntrySwapPreviewError,
        setOpenPositionEntrySwapPreviewError,
        setOpenPositionPreparePrivateZapInfo,
        setOpenPositionPrivateZapInfo,
        setOpenPositionPrepareTokenRisk,
        setOpenPositionPreviewTokenRisk,
        setOpenPositionRangeEditor,
        setOpenPositionPreviewRangeEditor,
        setOpenPositionSizingAdvice,
        openPositionEntrySwapSlippage,
        setOpenPositionEntrySwapSlippage,
        openPositionEntrySwapSlippageDirty,
        setOpenPositionEntrySwapConfirm,
        setOpenPositionLoading,
        openPositionDCAEnabled,
        openPositionDCAPercentages,
        openPositionDCAInterval,
        openPositionTaskMode,
        openPositionWalletId,
        setOpenPositionWalletId,
        resetOpenPositionDraft,
    } = draft;

    const lastOpenPositionRequestRef = useRef(null);

    const clearOpenPositionPreview = useCallback((clearChecks) => {
        setOpenPositionEntrySwapPreview(null);
        setOpenPositionEntrySwapPreviewLoading(false);
        setOpenPositionPreviewPending(false);
        setOpenPositionEntrySwapPreviewError('');
        setOpenPositionPrivateZapInfo(null);
        setOpenPositionSizingAdvice(null);
        if (clearChecks) setOpenPositionChecks([]);
        setOpenPositionPreviewRangeEditor(null);
        setOpenPositionPreviewTokenRisk(null);
    }, [
        setOpenPositionChecks,
        setOpenPositionEntrySwapPreview,
        setOpenPositionEntrySwapPreviewError,
        setOpenPositionEntrySwapPreviewLoading,
        setOpenPositionPreviewPending,
        setOpenPositionPreviewRangeEditor,
        setOpenPositionPreviewTokenRisk,
        setOpenPositionPrivateZapInfo,
        setOpenPositionSizingAdvice,
    ]);

    useEffect(() => {
        if (!openPositionPool || !hasInitData || !multiWalletEnabled) return;

        let aborted = false;
        const controller = new AbortController();

        setWalletsLoading(true);
        setWalletsError('');

        const chain = String(openPositionPool?.chain || '').trim().toLowerCase();
        fetchWallets({ apiBaseUrl, initData, chain, signal: controller.signal })
            .then((resp) => {
                if (aborted) return;
                setWalletsData(resp || null);

                const list = Array.isArray(resp?.wallets) ? resp.wallets : [];
                if (list.length === 0) {
                    setOpenPositionWalletId('');
                    storage.remove(STORAGE_OPEN_POSITION_WALLET_ID);
                    return;
                }

                const saved = String(storage.get(STORAGE_OPEN_POSITION_WALLET_ID) || '').trim();
                const savedOk = saved && list.some((w) => String(w?.id) === saved);
                const next = savedOk ? saved : String((list.find((w) => w?.is_default) || list[0])?.id || '');
                setOpenPositionWalletId(next);
                if (next) storage.set(STORAGE_OPEN_POSITION_WALLET_ID, next);
            })
            .catch((e) => {
                if (aborted) return;
                setWalletsData(null);
                setWalletsError(String(e?.message || e));
            })
            .finally(() => {
                if (aborted) return;
                setWalletsLoading(false);
            });

        return () => {
            aborted = true;
            controller.abort();
        };
    }, [
        apiBaseUrl,
        initData,
        hasInitData,
        multiWalletEnabled,
        openPositionPool,
        setOpenPositionWalletId,
        setWalletsData,
        setWalletsError,
        setWalletsLoading,
    ]);

    useEffect(() => {
        if (!openPositionPool || !hasInitData) {
            setOpenPositionPrepareChecks([]);
            setOpenPositionPreparePrivateZapInfo(null);
            setOpenPositionPrepareTokenRisk(null);
            setOpenPositionRangeEditor(null);
            return undefined;
        }

        let walletId;
        if (multiWalletEnabled && !walletsLoading && !walletsError) {
            const list = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
            if (list.length === 1) {
                const onlyId = Number(list[0]?.id);
                if (Number.isFinite(onlyId) && onlyId > 0) {
                    walletId = onlyId;
                }
            } else if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                if (Number.isFinite(wid) && wid > 0) {
                    walletId = wid;
                }
            }
        }

        let active = true;
        const controller = new AbortController();
        prepareOpenPosition({
            apiBaseUrl,
            initData,
            chain: openPositionPool?.chain || 'bsc',
            poolAddress: openPositionPool?.pool_address,
            poolVersion: openPositionPool?.protocol_version,
            walletId,
            signal: controller.signal,
        })
            .then((resp) => {
                if (!active) return;
                setOpenPositionPrepareChecks(Array.isArray(resp?.checks) ? resp.checks : []);
                setOpenPositionPreparePrivateZapInfo(resp?.private_zap && typeof resp.private_zap === 'object' ? resp.private_zap : null);
                setOpenPositionPrepareTokenRisk(resp?.token_risk && typeof resp.token_risk === 'object' ? resp.token_risk : null);
                setOpenPositionRangeEditor(resp?.range_editor && typeof resp.range_editor === 'object' ? resp.range_editor : null);
            })
            .catch(() => {
                if (!active || controller.signal.aborted) return;
                setOpenPositionPrepareChecks([]);
                setOpenPositionPreparePrivateZapInfo(null);
                setOpenPositionPrepareTokenRisk(null);
                setOpenPositionRangeEditor(null);
            });

        return () => {
            active = false;
            controller.abort();
        };
    }, [
        apiBaseUrl,
        initData,
        hasInitData,
        openPositionPool,
        multiWalletEnabled,
        walletsLoading,
        walletsError,
        walletsData,
        openPositionWalletId,
        setOpenPositionPrepareChecks,
        setOpenPositionPreparePrivateZapInfo,
        setOpenPositionPrepareTokenRisk,
        setOpenPositionRangeEditor,
    ]);

    const openPositionEntrySwapConfirmKey = useMemo(
        () => buildEntrySwapConfirmKey(openPositionEntrySwapPreview, openPositionEntrySwapSlippage),
        [openPositionEntrySwapPreview, openPositionEntrySwapSlippage],
    );

    useEffect(() => {
        if (!openPositionEntrySwapPreview?.required || openPositionEntrySwapSlippageDirty) return;
        const recommended = Number(openPositionEntrySwapPreview?.recommended_slippage_tolerance);
        const current = Number(openPositionEntrySwapPreview?.current_slippage_tolerance);
        const next = Number.isFinite(recommended) ? recommended : current;
        if (!Number.isFinite(next)) return;
        setOpenPositionEntrySwapSlippage(String(next));
    }, [
        openPositionEntrySwapPreview,
        openPositionEntrySwapSlippageDirty,
        setOpenPositionEntrySwapSlippage,
    ]);

    useEffect(() => {
        setOpenPositionEntrySwapConfirm(true);
    }, [openPositionEntrySwapConfirmKey, setOpenPositionEntrySwapConfirm]);

    useEffect(() => {
        if (openPositionPreviewSuspended) {
            setOpenPositionEntrySwapPreviewLoading(false);
            setOpenPositionPreviewPending(false);
            return undefined;
        }
        if (!openPositionPool || !hasInitData) {
            clearOpenPositionPreview(false);
            return undefined;
        }

        const amount = Number(String(openPositionAmount || '').trim());
        const range = parseRangeInput(openPositionRangeLower, openPositionRangeUpper);
        const taskSlippage = parseOptionalPercent(openPositionSlippage);
        const entrySwapSlippage = parseOptionalPercent(openPositionEntrySwapSlippage);
        const invalidPercentageRange = !range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100;
        const invalidManualTickRange = !Number.isInteger(openPositionSelectedManualTickLower)
            || !Number.isInteger(openPositionSelectedManualTickUpper)
            || openPositionSelectedManualTickLower >= openPositionSelectedManualTickUpper;
        if (
            !Number.isFinite(amount) ||
            amount <= 0 ||
            !taskSlippage.valid ||
            !entrySwapSlippage.valid ||
            (openPositionRangeInputMode === 'percentage' ? invalidPercentageRange : invalidManualTickRange)
        ) {
            clearOpenPositionPreview(true);
            return undefined;
        }

        let walletId = openPositionWalletId;
        if (multiWalletEnabled) {
            if (walletsLoading) {
                clearOpenPositionPreview(false);
                return undefined;
            }
            if (walletsError) {
                clearOpenPositionPreview(false);
                return undefined;
            }
            const list = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
            if (list.length === 0) {
                clearOpenPositionPreview(false);
                return undefined;
            }
            if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                walletId = wid;
                if (!Number.isFinite(wid) || wid <= 0) {
                    clearOpenPositionPreview(false);
                    return undefined;
                }
            } else {
                const onlyId = Number(list[0]?.id);
                if (Number.isFinite(onlyId) && onlyId > 0) {
                    walletId = onlyId;
                }
            }
        }

        let active = true;
        const controller = new AbortController();
        setOpenPositionPreviewPending(true);
        setOpenPositionEntrySwapPreviewLoading(false);
        setOpenPositionEntrySwapPreviewError('');

        const timer = setTimeout(async () => {
            try {
                const previewPayload = {
                    apiBaseUrl,
                    initData,
                    chain: openPositionPool?.chain || 'bsc',
                    poolAddress: openPositionPool?.pool_address,
                    poolVersion: openPositionPool?.protocol_version,
                    amount,
                    rangeInputMode: openPositionRangeInputMode === 'price' ? 'tick' : openPositionRangeInputMode,
                    slippageTolerance: taskSlippage.value,
                    entrySwapSlippageTolerance: entrySwapSlippage.value,
                    allowEntrySwap: true,
                    walletId,
                    ackLiquidityRisk: openPositionRiskAck,
                    taskMode: openPositionTaskMode,
                    signal: controller.signal,
                };
                if (openPositionRangeInputMode === 'percentage') {
                    previewPayload.rangeLowerPct = range.lower;
                    previewPayload.rangeUpperPct = range.upper;
                } else {
                    previewPayload.tickLower = openPositionSelectedManualTickLower;
                    previewPayload.tickUpper = openPositionSelectedManualTickUpper;
                }
                const resp = await previewOpenPosition(previewPayload);
                if (!active) return;
                setOpenPositionChecks(Array.isArray(resp?.checks) ? resp.checks : []);
                setOpenPositionEntrySwapPreview(resp?.entry_swap || { required: false });
                setOpenPositionPrivateZapInfo(resp?.private_zap && typeof resp.private_zap === 'object' ? resp.private_zap : null);
                setOpenPositionSizingAdvice(resp?.sizing_advice && typeof resp.sizing_advice === 'object' ? resp.sizing_advice : null);
                setOpenPositionPreviewRangeEditor(resp?.range_editor && typeof resp.range_editor === 'object' ? resp.range_editor : null);
                setOpenPositionPreviewTokenRisk(resp?.token_risk && typeof resp.token_risk === 'object' ? resp.token_risk : null);
            } catch (e) {
                if (!active || controller.signal.aborted) return;
                const msg = String(e?.message || e || '').trim();
                const payload = resolveOpenPositionErrorPayload(e);
                const failChecks = extractOpenPositionErrorChecks(e, 'preview_safety');
                const entrySwapInfo = payload?.entry_swap && typeof payload.entry_swap === 'object'
                    ? payload.entry_swap
                    : null;
                setOpenPositionEntrySwapPreview(entrySwapInfo);
                setOpenPositionPrivateZapInfo(payload?.private_zap && typeof payload.private_zap === 'object' ? payload.private_zap : null);
                setOpenPositionSizingAdvice(payload?.sizing_advice && typeof payload.sizing_advice === 'object' ? payload.sizing_advice : null);
                setOpenPositionChecks(failChecks);
                setOpenPositionPreviewRangeEditor(payload?.range_editor && typeof payload.range_editor === 'object' ? payload.range_editor : null);
                setOpenPositionPreviewTokenRisk(payload?.token_risk && typeof payload.token_risk === 'object' ? payload.token_risk : null);
                setOpenPositionEntrySwapPreviewError(failChecks.length > 0 ? '' : (msg || '获取前置兑换预览失败'));
            } finally {
                if (active) {
                    setOpenPositionEntrySwapPreviewLoading(false);
                    setOpenPositionPreviewPending(false);
                }
            }
        }, 350);

        return () => {
            active = false;
            clearTimeout(timer);
            controller.abort();
        };
    }, [
        apiBaseUrl,
        initData,
        hasInitData,
        openPositionPool,
        openPositionAmount,
        openPositionRangeInputMode,
        openPositionRangeLower,
        openPositionRangeUpper,
        openPositionTickLower,
        openPositionTickUpper,
        openPositionPriceLower,
        openPositionPriceUpper,
        openPositionSlippage,
        openPositionEntrySwapSlippage,
        openPositionRiskAck,
        multiWalletEnabled,
        walletsLoading,
        walletsError,
        walletsData,
        openPositionWalletId,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionTaskMode,
        openPositionPreviewSuspended,
        clearOpenPositionPreview,
        setOpenPositionChecks,
        setOpenPositionEntrySwapPreview,
        setOpenPositionEntrySwapPreviewError,
        setOpenPositionEntrySwapPreviewLoading,
        setOpenPositionPreviewPending,
        setOpenPositionPreviewRangeEditor,
        setOpenPositionPreviewTokenRisk,
        setOpenPositionPrivateZapInfo,
        setOpenPositionSizingAdvice,
    ]);

    const submitOpenPositionRequest = useCallback(async ({ submitPayload, totalBatches, pairTitle, dcaEnabled, closeDraft = false }) => {
        lastOpenPositionRequestRef.current = { submitPayload, totalBatches, pairTitle, dcaEnabled };
        setOperationProgress({
            operation: 'open_position',
            currentStep: 1,
            totalSteps: totalBatches,
            status: 'active',
            error: '',
            pair: pairTitle,
            dca: dcaEnabled,
        });
        if (closeDraft) {
            setOpenPositionPool(null);
            resetOpenPositionDraft();
        }
        try {
            const resp = await openPosition(submitPayload);
            const taskId = Number(resp?.task_id);
            lastOpenPositionRequestRef.current = null;
            setOpenPositionError('');
            setOpenPositionChecks([]);
            setOpenPositionEntrySwapPreview(null);
            setOpenPositionEntrySwapPreviewError('');
            setOpenPositionEntrySwapConfirm(true);
            setOpenPositionPreviewTokenRisk(null);
            setOperationProgress((prev) => (prev?.operation === 'open_position'
                ? {
                    ...prev,
                    taskId: Number.isFinite(taskId) && taskId > 0 ? taskId : prev.taskId,
                    currentStep: dcaEnabled ? 1 : totalBatches,
                    status: dcaEnabled ? 'active_dca' : 'done',
                    error: '',
                }
                : prev));
            try {
                await refreshRealtimePositionsNow({ setBusy: false, updateError: false });
            } catch (refreshErr) {
                console.error('[OpenPosition] Immediate position refresh failed:', refreshErr);
            }
            return true;
        } catch (e) {
            const msg = String(e?.message || e || '').trim();
            const payload = resolveOpenPositionErrorPayload(e);
            setOpenPositionPreviewTokenRisk(payload?.token_risk && typeof payload.token_risk === 'object' ? payload.token_risk : null);
            const entrySwapInfo = payload?.entry_swap && typeof payload.entry_swap === 'object'
                ? payload.entry_swap
                : null;
            const failChecks = extractOpenPositionErrorChecks(e, 'submit_safety');
            if (entrySwapInfo) {
                setOpenPositionEntrySwapPreview(entrySwapInfo);
                setOpenPositionEntrySwapPreviewError('');
            }
            if (failChecks.length > 0) {
                setOpenPositionChecks((prev) => {
                    const merged = Array.isArray(prev)
                        ? prev.filter((item) => !failChecks.some((next) => next?.key === item?.key))
                        : [];
                    return [...merged, ...failChecks];
                });
            }
            setOpenPositionError(msg || '开仓执行失败。');
            setOperationProgress((prev) => (prev?.operation === 'open_position'
                ? { ...prev, status: 'error', error: msg || '开仓执行失败。' }
                : prev));
            return false;
        }
    }, [
        refreshRealtimePositionsNow,
        resetOpenPositionDraft,
        setOpenPositionChecks,
        setOpenPositionEntrySwapConfirm,
        setOpenPositionEntrySwapPreview,
        setOpenPositionEntrySwapPreviewError,
        setOpenPositionError,
        setOpenPositionPool,
        setOpenPositionPreviewTokenRisk,
        setOperationProgress,
    ]);

    const handleOpenPosition = useCallback(async () => {
        if (!openPositionPool) return;
        if (!hasInitData) {
            setOpenPositionError('缺少 Telegram initData，请从 Mini App 内重新打开。');
            return;
        }
        const amount = Number(String(openPositionAmount || '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setOpenPositionError('请输入有效的开仓金额。');
            return;
        }
        const warnChecks = activeOpenPositionChecks.filter(c => c.status === 'warn');
        const failChecks = activeOpenPositionChecks.filter(c => c.status === 'fail');
        if (failChecks.length > 0) {
            setOpenPositionError(failChecks.map(c => c.detail || c.label).join('; '));
            return;
        }
        const requiresAck = warnChecks.some(c => c.extra?.risk_ack_required);
        const range = parseRangeInput(openPositionRangeLower, openPositionRangeUpper);
        if (openPositionRangeInputMode === 'percentage') {
            if (!range || range.lower <= 0 || range.upper <= 0 || range.lower >= 100 || range.upper >= 100) {
                setOpenPositionError('请输入 0 到 100 之间的有效百分比区间。');
                return;
            }
        } else if (openPositionRangeInputMode !== 'price' && (!Number.isInteger(openPositionTickLowerValue) || !Number.isInteger(openPositionTickUpperValue) || openPositionTickLowerValue >= openPositionTickUpperValue)) {
            setOpenPositionError('请输入有效的 Tick 区间。');
            return;
        }

        if (openPositionRangeInputMode !== 'percentage' && (!Number.isInteger(openPositionSelectedManualTickLower) || !Number.isInteger(openPositionSelectedManualTickUpper) || openPositionSelectedManualTickLower >= openPositionSelectedManualTickUpper)) {
            setOpenPositionError(openPositionRangeInputMode === 'price' ? '请输入有效的价格区间。' : '请输入有效的 Tick 区间。');
            return;
        }

        const slippageParsed = parseOptionalPercent(openPositionSlippage);
        if (!slippageParsed.valid) {
            setOpenPositionError('请输入 0 到 100 之间的有效滑点。');
            return;
        }
        const entrySwapSlippageParsed = parseOptionalPercent(openPositionEntrySwapSlippage);
        if (!entrySwapSlippageParsed.valid) {
            setOpenPositionError('请输入 0 到 100 之间的有效前置兑换滑点。');
            return;
        }
        let walletId = openPositionWalletId;

        if (multiWalletEnabled) {
            if (walletsLoading) {
                setOpenPositionError('钱包列表仍在加载，请稍后再试。');
                return;
            }
            if (walletsError) {
                setOpenPositionError(walletsError);
                return;
            }
            const list = Array.isArray(walletsData?.wallets) ? walletsData.wallets : [];
            if (list.length === 0) {
                setOpenPositionError('当前没有可用钱包。');
                return;
            }
            if (list.length > 1) {
                const wid = Number(openPositionWalletId);
                walletId = wid;
                if (!Number.isFinite(wid) || wid <= 0) {
                    setOpenPositionError('请选择开仓钱包。');
                    return;
                }
            } else {
                const onlyId = String(list[0]?.id || '').trim();
                walletId = onlyId;
                if (onlyId && String(openPositionWalletId || '') !== onlyId) {
                    setOpenPositionWalletId(onlyId);
                    storage.set(STORAGE_OPEN_POSITION_WALLET_ID, onlyId);
                }
            }
        }

        if (openPositionPreviewPending || openPositionPreviewSuspended) {
            setOpenPositionError('前置兑换预览仍在更新，请稍后再试。');
            return;
        }
        if (openPositionEntrySwapPreviewError) {
            setOpenPositionError(openPositionEntrySwapPreviewError);
            return;
        }

        const effectiveOpenPositionDCAEnabled = openPositionDCAEnabled
            && !openPositionIsSingleSidedSelection
            && !(openPositionGlobalDCAMinSplitAmount > 0 && amount < openPositionGlobalDCAMinSplitAmount);

        if (effectiveOpenPositionDCAEnabled) {
            if (openPositionDCAPercentages.length < 2 || openPositionDCAPercentages.length > 5) {
                setOpenPositionError('分批数量必须在 2 到 5 批之间。');
                return;
            }
            if (openPositionDCAPercentages.some((v) => !(Number(v) >= 5))) {
                setOpenPositionError('每批比例不能低于 5%。');
                return;
            }
            const sum = openPositionDCAPercentages.reduce((acc, v) => acc + (Number(v) || 0), 0);
            if (Math.abs(sum - 100) > 0.01) {
                setOpenPositionError(`分批比例总和必须等于 100%，当前为 ${sum.toFixed(2)}%。`);
                return;
            }
            const iv = Number(openPositionDCAInterval);
            if (!(Number.isFinite(iv) && iv >= 0 && iv <= 300)) {
                setOpenPositionError('分批间隔必须在 0 到 300 秒之间。');
                return;
            }
        }

        if (Number.isFinite(slippageParsed.value) && slippageParsed.value > 1) {
            const ok = await requestConfirm({
                title: '高滑点确认',
                message: `当前任务滑点为 ${slippageParsed.value}% ，已超过 1%，请确认是否继续。`,
                confirmText: '继续提交',
                cancelText: '返回修改',
                tone: 'danger',
            });
            if (!ok) return;
        }

        const totalBatches = effectiveOpenPositionDCAEnabled ? openPositionDCAPercentages.length : 1;
        const pairTitle = openPositionPool?.trading_pair || '';
        const submitPayload = {
            apiBaseUrl,
            initData,
            chain: openPositionPool?.chain || 'bsc',
            poolAddress: openPositionPool?.pool_address,
            poolVersion: openPositionPool?.protocol_version,
            amount,
            rangeInputMode: openPositionRangeInputMode === 'price' ? 'tick' : openPositionRangeInputMode,
            slippageTolerance: slippageParsed.value,
            entrySwapSlippageTolerance: openPositionEntrySwapPreview?.required ? entrySwapSlippageParsed.value : undefined,
            allowEntrySwap: true,
            confirmEntrySwap: Boolean(openPositionEntrySwapPreview?.required),
            walletId,
            ackLiquidityRisk: requiresAck && openPositionRiskAck,
            dcaEnabled: effectiveOpenPositionDCAEnabled,
            dcaPercentages: effectiveOpenPositionDCAEnabled ? openPositionDCAPercentages.map((v) => Number(v) || 0) : undefined,
            dcaIntervalSeconds: effectiveOpenPositionDCAEnabled ? Number(openPositionDCAInterval) : undefined,
            taskMode: openPositionTaskMode,
        };
        if (openPositionRangeInputMode === 'percentage') {
            submitPayload.rangeLowerPct = range.lower;
            submitPayload.rangeUpperPct = range.upper;
        } else {
            submitPayload.tickLower = openPositionSelectedManualTickLower;
            submitPayload.tickUpper = openPositionSelectedManualTickUpper;
        }
        setOpenPositionLoading(true);
        setOpenPositionError('');
        try {
            await submitOpenPositionRequest({
                submitPayload,
                totalBatches,
                pairTitle,
                dcaEnabled: effectiveOpenPositionDCAEnabled,
                closeDraft: true,
            });
        } finally {
            setOpenPositionLoading(false);
        }
    }, [
        activeOpenPositionChecks,
        apiBaseUrl,
        initData,
        hasInitData,
        multiWalletEnabled,
        openPositionPool,
        openPositionAmount,
        openPositionRangeLower,
        openPositionRangeUpper,
        openPositionRangeInputMode,
        openPositionTickLowerValue,
        openPositionTickUpperValue,
        openPositionSelectedManualTickLower,
        openPositionSelectedManualTickUpper,
        openPositionSlippage,
        openPositionEntrySwapSlippage,
        openPositionWalletId,
        openPositionPreviewPending,
        openPositionPreviewSuspended,
        openPositionEntrySwapPreviewError,
        openPositionDCAEnabled,
        openPositionIsSingleSidedSelection,
        openPositionGlobalDCAMinSplitAmount,
        openPositionDCAPercentages,
        openPositionDCAInterval,
        openPositionEntrySwapPreview,
        openPositionRiskAck,
        openPositionTaskMode,
        requestConfirm,
        setOpenPositionError,
        setOpenPositionLoading,
        setOpenPositionWalletId,
        submitOpenPositionRequest,
        walletsData,
        walletsError,
        walletsLoading,
    ]);

    const openPositionRetryAction = useMemo(() => (
        operationProgress?.operation === 'open_position'
            && operationProgress?.status === 'error'
            && lastOpenPositionRequestRef.current
            ? async () => {
                const attempt = lastOpenPositionRequestRef.current;
                if (!attempt) return;
                await submitOpenPositionRequest({ ...attempt, closeDraft: false });
            }
            : undefined
    ), [operationProgress, submitOpenPositionRequest]);

    return {
        handleOpenPosition,
        openPositionError,
        openPositionRetryAction,
    };
}
