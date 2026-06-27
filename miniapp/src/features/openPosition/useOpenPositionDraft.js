import { useRef, useState } from 'react';
import { storage } from '../../lib/storage';
import {
    STORAGE_OPEN_POSITION_HIDE_WALLET_BALANCES,
    STORAGE_OPEN_POSITION_WALLET_ID,
} from './constants';

export default function useOpenPositionDraft() {
    const [openPositionPool, setOpenPositionPool] = useState(null);
    const [openPositionStep, setOpenPositionStep] = useState(0); // 开仓向导当前步：0 资金 / 1 区间 / 2 策略&确认
    const [openPositionAmount, setOpenPositionAmount] = useState('');
    const [openPositionRangeLower, setOpenPositionRangeLower] = useState('');
    const [openPositionRangeUpper, setOpenPositionRangeUpper] = useState('');
    const [openPositionRangeUpperAuto, setOpenPositionRangeUpperAuto] = useState(true);
    const [openPositionRangeInputMode, setOpenPositionRangeInputMode] = useState('percentage');
    const [openPositionTickLower, setOpenPositionTickLower] = useState('');
    const [openPositionTickUpper, setOpenPositionTickUpper] = useState('');
    const [openPositionPriceLower, setOpenPositionPriceLower] = useState('');
    const [openPositionPriceUpper, setOpenPositionPriceUpper] = useState('');
    const [openPositionInvertPrice, setOpenPositionInvertPrice] = useState(false);
    const [openPositionGridBoundaryTarget, setOpenPositionGridBoundaryTarget] = useState('lower');
    const [openPositionSlippage, setOpenPositionSlippage] = useState('');
    const [openPositionSwapProviderPolicy, setOpenPositionSwapProviderPolicy] = useState('best');
    const [openPositionError, setOpenPositionError] = useState('');
    const [openPositionPrepareChecks, setOpenPositionPrepareChecks] = useState([]);
    const [openPositionChecks, setOpenPositionChecks] = useState([]);
    const [openPositionRiskAck, setOpenPositionRiskAck] = useState(false);
    const [openPositionEntrySwapPreview, setOpenPositionEntrySwapPreview] = useState(null);
    const [openPositionEntrySwapPreviewLoading, setOpenPositionEntrySwapPreviewLoading] = useState(false);
    const [openPositionPreviewPending, setOpenPositionPreviewPending] = useState(false);
    const [openPositionPreviewSuspended, setOpenPositionPreviewSuspended] = useState(false);
    const [openPositionEntrySwapPreviewError, setOpenPositionEntrySwapPreviewError] = useState('');
    const openPositionDefaultRangeSeededRef = useRef(false);
    const openPositionPreviewResumeTimerRef = useRef(null);
    const openPositionAutoSingleSideRangeRef = useRef('');
    const [openPositionPreparePrivateZapInfo, setOpenPositionPreparePrivateZapInfo] = useState(null);
    const [openPositionPrivateZapInfo, setOpenPositionPrivateZapInfo] = useState(null);
    const [openPositionPrepareTokenRisk, setOpenPositionPrepareTokenRisk] = useState(null);
    const [openPositionPreviewTokenRisk, setOpenPositionPreviewTokenRisk] = useState(null);
    const [openPositionRangeEditor, setOpenPositionRangeEditor] = useState(null);
    const [openPositionPreviewRangeEditor, setOpenPositionPreviewRangeEditor] = useState(null);
    const [openPositionSizingAdvice, setOpenPositionSizingAdvice] = useState(null);
    const [openPositionEntrySwapSlippage, setOpenPositionEntrySwapSlippage] = useState('');
    const [openPositionEntrySwapSlippageDirty, setOpenPositionEntrySwapSlippageDirty] = useState(false);
    const [openPositionEntrySwapConfirm, setOpenPositionEntrySwapConfirm] = useState(true);
    const [openPositionLoading, setOpenPositionLoading] = useState(false);
    const [openPositionSmartRanges, setOpenPositionSmartRanges] = useState([]);
    const [openPositionSmartRangesLoading, setOpenPositionSmartRangesLoading] = useState(false);
    const [openPositionDCAEnabled, setOpenPositionDCAEnabled] = useState(false);
    const [openPositionDCAPercentages, setOpenPositionDCAPercentages] = useState([50, 50]);
    const [openPositionDCAInterval, setOpenPositionDCAInterval] = useState(30);
    const [openPositionDCAExpanded, setOpenPositionDCAExpanded] = useState(false);
    const [openPositionTaskMode, setOpenPositionTaskMode] = useState('pause');
    const [openPositionWalletBalancesHidden, setOpenPositionWalletBalancesHidden] = useState(() => storage.get(STORAGE_OPEN_POSITION_HIDE_WALLET_BALANCES) === '1');
    const [openPositionLiqProfile, setOpenPositionLiqProfile] = useState(null);
    const [openPositionLiqProfileLoading, setOpenPositionLiqProfileLoading] = useState(false);
    const [openPositionLiqProfileError, setOpenPositionLiqProfileError] = useState('');
    const [openPositionWalletId, setOpenPositionWalletId] = useState(() => storage.get(STORAGE_OPEN_POSITION_WALLET_ID) || '');

    const resetOpenPositionDraft = () => {
        openPositionDefaultRangeSeededRef.current = false;
        openPositionAutoSingleSideRangeRef.current = '';
        if (openPositionPreviewResumeTimerRef.current) {
            window.clearTimeout(openPositionPreviewResumeTimerRef.current);
            openPositionPreviewResumeTimerRef.current = null;
        }
        setOpenPositionAmount('');
        setOpenPositionRangeLower('');
        setOpenPositionRangeUpper('');
        setOpenPositionRangeUpperAuto(true);
        setOpenPositionRangeInputMode('percentage');
        setOpenPositionTickLower('');
        setOpenPositionTickUpper('');
        setOpenPositionPriceLower('');
        setOpenPositionPriceUpper('');
        setOpenPositionGridBoundaryTarget('lower');
        setOpenPositionSlippage('');
        setOpenPositionSwapProviderPolicy('best');
        setOpenPositionPrepareChecks([]);
        setOpenPositionEntrySwapPreview(null);
        setOpenPositionEntrySwapPreviewLoading(false);
        setOpenPositionPreviewPending(false);
        setOpenPositionPreviewSuspended(false);
        setOpenPositionEntrySwapPreviewError('');
        setOpenPositionPreparePrivateZapInfo(null);
        setOpenPositionPrivateZapInfo(null);
        setOpenPositionPrepareTokenRisk(null);
        setOpenPositionPreviewTokenRisk(null);
        setOpenPositionRangeEditor(null);
        setOpenPositionPreviewRangeEditor(null);
        setOpenPositionSizingAdvice(null);
        setOpenPositionEntrySwapSlippage('');
        setOpenPositionEntrySwapSlippageDirty(false);
        setOpenPositionEntrySwapConfirm(true);
        setOpenPositionDCAExpanded(false);
        setOpenPositionTaskMode('pause');
        setOpenPositionStep(0);
        setOpenPositionError('');
        setOpenPositionChecks([]);
        setOpenPositionRiskAck(false);
    };

    return {
        openPositionPool,
        setOpenPositionPool,
        openPositionStep,
        setOpenPositionStep,
        openPositionAmount,
        setOpenPositionAmount,
        openPositionRangeLower,
        setOpenPositionRangeLower,
        openPositionRangeUpper,
        setOpenPositionRangeUpper,
        openPositionRangeUpperAuto,
        setOpenPositionRangeUpperAuto,
        openPositionRangeInputMode,
        setOpenPositionRangeInputMode,
        openPositionTickLower,
        setOpenPositionTickLower,
        openPositionTickUpper,
        setOpenPositionTickUpper,
        openPositionPriceLower,
        setOpenPositionPriceLower,
        openPositionPriceUpper,
        setOpenPositionPriceUpper,
        openPositionInvertPrice,
        setOpenPositionInvertPrice,
        openPositionGridBoundaryTarget,
        setOpenPositionGridBoundaryTarget,
        openPositionSlippage,
        setOpenPositionSlippage,
        openPositionSwapProviderPolicy,
        setOpenPositionSwapProviderPolicy,
        openPositionError,
        setOpenPositionError,
        openPositionPrepareChecks,
        setOpenPositionPrepareChecks,
        openPositionChecks,
        setOpenPositionChecks,
        openPositionRiskAck,
        setOpenPositionRiskAck,
        openPositionEntrySwapPreview,
        setOpenPositionEntrySwapPreview,
        openPositionEntrySwapPreviewLoading,
        setOpenPositionEntrySwapPreviewLoading,
        openPositionPreviewPending,
        setOpenPositionPreviewPending,
        openPositionPreviewSuspended,
        setOpenPositionPreviewSuspended,
        openPositionEntrySwapPreviewError,
        setOpenPositionEntrySwapPreviewError,
        openPositionDefaultRangeSeededRef,
        openPositionPreviewResumeTimerRef,
        openPositionAutoSingleSideRangeRef,
        openPositionPreparePrivateZapInfo,
        setOpenPositionPreparePrivateZapInfo,
        openPositionPrivateZapInfo,
        setOpenPositionPrivateZapInfo,
        openPositionPrepareTokenRisk,
        setOpenPositionPrepareTokenRisk,
        openPositionPreviewTokenRisk,
        setOpenPositionPreviewTokenRisk,
        openPositionRangeEditor,
        setOpenPositionRangeEditor,
        openPositionPreviewRangeEditor,
        setOpenPositionPreviewRangeEditor,
        openPositionSizingAdvice,
        setOpenPositionSizingAdvice,
        openPositionEntrySwapSlippage,
        setOpenPositionEntrySwapSlippage,
        openPositionEntrySwapSlippageDirty,
        setOpenPositionEntrySwapSlippageDirty,
        openPositionEntrySwapConfirm,
        setOpenPositionEntrySwapConfirm,
        openPositionLoading,
        setOpenPositionLoading,
        openPositionSmartRanges,
        setOpenPositionSmartRanges,
        openPositionSmartRangesLoading,
        setOpenPositionSmartRangesLoading,
        openPositionDCAEnabled,
        setOpenPositionDCAEnabled,
        openPositionDCAPercentages,
        setOpenPositionDCAPercentages,
        openPositionDCAInterval,
        setOpenPositionDCAInterval,
        openPositionDCAExpanded,
        setOpenPositionDCAExpanded,
        openPositionTaskMode,
        setOpenPositionTaskMode,
        openPositionWalletBalancesHidden,
        setOpenPositionWalletBalancesHidden,
        openPositionLiqProfile,
        setOpenPositionLiqProfile,
        openPositionLiqProfileLoading,
        setOpenPositionLiqProfileLoading,
        openPositionLiqProfileError,
        setOpenPositionLiqProfileError,
        openPositionWalletId,
        setOpenPositionWalletId,
        resetOpenPositionDraft,
    };
}
