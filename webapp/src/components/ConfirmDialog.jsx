import React from 'react';
import { AlertTriangle, X } from 'lucide-react';
import {
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
  IconButton,
} from './ui';

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
  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && !loading) onCancel?.();
      }}
    >
      <DialogContent className="confirm-dialog">
        <div className={`ui-dialog-icon${danger ? ' danger' : ''}`} aria-hidden="true">
          <AlertTriangle size={18} />
        </div>
        <DialogClose asChild>
          <IconButton type="button" className="ui-dialog-close" disabled={loading} aria-label="关闭">
            <X size={16} />
          </IconButton>
        </DialogClose>
        <DialogTitle>{title}</DialogTitle>
        {message ? <DialogDescription>{message}</DialogDescription> : null}
        <div className="ui-dialog-actions">
          <Button type="button" variant="ghost" onClick={onCancel} disabled={loading}>
            {cancelText}
          </Button>
          <Button type="button" variant={danger ? 'danger' : 'primary'} onClick={onConfirm} disabled={loading}>
            {loading ? '处理中...' : confirmText}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
