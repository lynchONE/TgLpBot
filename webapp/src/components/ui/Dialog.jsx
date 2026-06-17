import React from 'react';
import * as DialogPrimitive from '@radix-ui/react-dialog';
import { cn } from './utils';

export const Dialog = DialogPrimitive.Root;
export const DialogTrigger = DialogPrimitive.Trigger;
export const DialogClose = DialogPrimitive.Close;

export const DialogContent = React.forwardRef(function DialogContent(
  { className = '', children, ...props },
  ref
) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="ds-dialog-overlay" />
      <DialogPrimitive.Content ref={ref} className={cn('ds-dialog', className)} {...props}>
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  );
});

export const DialogTitle = React.forwardRef(function DialogTitle({ className = '', ...props }, ref) {
  return <DialogPrimitive.Title ref={ref} className={cn('ds-dialog-title', className)} {...props} />;
});

export const DialogDescription = React.forwardRef(function DialogDescription({ className = '', ...props }, ref) {
  return <DialogPrimitive.Description ref={ref} className={cn('ds-dialog-description', className)} {...props} />;
});
