import React, { useCallback, useEffect, useRef, useState } from 'react';

export default function TaskActionMenu({ position, onPause, onStop, onDelete, onEditRange, onWithdrawLiquidity, onSwapDust, onTriggerRebalance, onToggleRebalance, onAddLiquidity, onClose, anchorRef }) {
  const [editMode, setEditMode] = useState(false);
  const [rangeLower, setRangeLower] = useState(String(position?.task_range_lower_pct || '2'));
  const [rangeUpper, setRangeUpper] = useState(String(position?.task_range_upper_pct || '2'));
  const [rangeUpperAuto, setRangeUpperAuto] = useState(true);
  const [amountUsdt, setAmountUsdt] = useState(String(position?.task_amount_usdt || ''));
  const [pending, setPending] = useState('');
  const menuRef = useRef(null);

  const taskId = Number(position?.task_id || 0);
  const taskPaused = Boolean(position?.task_paused);
  const taskRebalanceEnabled = position?.task_rebalance_enabled !== false;
  const hasLiquidity = Boolean(position?.has_liquidity);
  const statusLabel = String(position?.status_label || '');
  const isStopped = statusLabel.includes('已停止');
  const isStopping = statusLabel.includes('停止中') || statusLabel.includes('撤出中');

  // Close on outside click
  useEffect(() => {
    const handler = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handler);
    document.addEventListener('touchstart', handler);
    return () => {
      document.removeEventListener('mousedown', handler);
      document.removeEventListener('touchstart', handler);
    };
  }, [onClose]);

  const run = useCallback(async (key, fn) => {
    if (pending) return;
    setPending(key);
    try { await fn(); } finally { setPending(''); onClose(); }
  }, [pending, onClose]);

  const handleRangeLowerChange = useCallback((value) => {
    setRangeLower((prevLower) => {
      if (rangeUpperAuto || String(rangeUpper || '').trim() === '' || String(rangeUpper) === String(prevLower)) {
        setRangeUpper(value);
      }
      return value;
    });
  }, [rangeUpper, rangeUpperAuto]);

  const handleRangeUpperChange = useCallback((value) => {
    setRangeUpperAuto(false);
    setRangeUpper(value);
  }, []);

  const handleEditSubmit = useCallback(() => {
    const rl = Number(rangeLower);
    const ru = Number(rangeUpper);
    if (!Number.isFinite(rl) || rl <= 0 || !Number.isFinite(ru) || ru <= 0) return;
    const amt = Number(amountUsdt);
    run('range', () => onEditRange(taskId, rl, ru, Number.isFinite(amt) && amt > 0 ? amt : undefined));
  }, [rangeLower, rangeUpper, amountUsdt, taskId, onEditRange, run]);

  if (editMode) {
    return (
      <div className="task-popover" ref={menuRef} onClick={(e) => e.stopPropagation()}>
        <div className="task-popover-header">
          <span>修改再平衡参数</span>
          <button type="button" className="task-popover-close" onClick={onClose}>&times;</button>
        </div>
        <div className="task-popover-form">
          <div className="task-popover-row">
            <label className="task-popover-field">
              <span>下限 %</span>
              <input type="number" value={rangeLower} onChange={(e) => handleRangeLowerChange(e.target.value)} min="0.1" step="0.5" />
            </label>
            <label className="task-popover-field">
              <span>上限 %</span>
              <input type="number" value={rangeUpper} onChange={(e) => handleRangeUpperChange(e.target.value)} min="0.1" step="0.5" />
            </label>
          </div>
          <label className="task-popover-field">
            <span>金额 USDT (可选)</span>
            <input type="number" value={amountUsdt} onChange={(e) => setAmountUsdt(e.target.value)} min="0" step="10" placeholder="留空不变" />
          </label>
        </div>
        <div className="task-popover-actions">
          <button type="button" className="ghost-chip" onClick={() => setEditMode(false)} disabled={!!pending}>返回</button>
          <button type="button" className="accent-btn small" onClick={handleEditSubmit} disabled={!!pending}>
            {pending === 'range' ? '提交中...' : '确认'}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="task-popover" ref={menuRef} onClick={(e) => e.stopPropagation()}>
      <div className="task-popover-header">
        <span>任务 #{taskId}</span>
        <button type="button" className="task-popover-close" onClick={onClose}>&times;</button>
      </div>
      <div className="task-action-list">
        {onPause && !isStopping && (
          <button type="button" className="task-action-item" onClick={() => run('pause', () => onPause(taskId, !taskPaused))} disabled={!!pending}>
            {pending === 'pause' ? '处理中...' : taskPaused ? '恢复任务' : '暂停任务'}
          </button>
        )}
        {onEditRange && !isStopping && (
          <button type="button" className="task-action-item" onClick={() => setEditMode(true)} disabled={!!pending}>
            修改再平衡参数
          </button>
        )}
        {onAddLiquidity && !isStopped && !isStopping && (
          <button type="button" className="task-action-item" onClick={() => run('addLiq', () => onAddLiquidity(taskId, position))} disabled={!!pending}>
            {pending === 'addLiq' ? '处理中...' : '补充流动性'}
          </button>
        )}
        {onStop && !isStopped && !isStopping && (
          <button type="button" className="task-action-item warn" onClick={() => run('stop', () => onStop(taskId))} disabled={!!pending}>
            {pending === 'stop' ? '处理中...' : '停止任务'}
          </button>
        )}
        {onDelete && !isStopping && (
          <button type="button" className="task-action-item danger" onClick={() => run('delete', () => onDelete(taskId))} disabled={!!pending}>
            {pending === 'delete' ? '删除中...' : '删除任务'}
          </button>
        )}
      </div>
    </div>
  );
}
