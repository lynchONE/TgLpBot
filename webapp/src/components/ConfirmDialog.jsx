import React, { useEffect } from 'react';
import { AlertTriangle, X } from 'lucide-react';

export default function ConfirmDialog({
  open,
  title = '确认操作',
  message = '',
  confirmText = '确认',
  cancelText = '取消',
  danger = false,
  loading = false,
  onConfirm,
  onCancel,
}) {
  useEffect(() => {
    if (!open) return undefined;
    const onKeyDown = (event) => {
      if (event.key === 'Escape' && !loading) onCancel?.();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [loading, onCancel, open]);

  if (!open) return null;

  return (
    <div className="ui-dialog-layer" role="presentation">
      <button type="button" className="ui-dialog-backdrop" aria-label="关闭" onClick={loading ? undefined : onCancel} />
      <div className="ui-dialog" role="dialog" aria-modal="true" aria-labelledby="ui-dialog-title">
        <div className={`ui-dialog-icon${danger ? ' danger' : ''}`} aria-hidden="true">
          <AlertTriangle size={18} />
        </div>
        <button type="button" className="ui-dialog-close" onClick={onCancel} disabled={loading} aria-label="关闭">
          <X size={16} />
        </button>
        <h3 id="ui-dialog-title">{title}</h3>
        {message ? <p>{message}</p> : null}
        <div className="ui-dialog-actions">
          <button type="button" className="ui-dialog-cancel" onClick={onCancel} disabled={loading}>
            {cancelText}
          </button>
          <button type="button" className={`ui-dialog-confirm${danger ? ' danger' : ''}`} onClick={onConfirm} disabled={loading}>
            {loading ? '处理中...' : confirmText}
          </button>
        </div>
      </div>
    </div>
  );
}
