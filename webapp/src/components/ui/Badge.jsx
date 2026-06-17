import React from 'react';
import { cn } from './utils';

export function Badge({ tone = 'default', className = '', children, ...props }) {
  return (
    <span data-tone={tone} className={cn('ds-badge', className)} {...props}>
      {children}
    </span>
  );
}

export function StatusBadge({ status = 'default', children, ...props }) {
  const toneByStatus = {
    ok: 'positive',
    success: 'positive',
    positive: 'positive',
    warn: 'warning',
    warning: 'warning',
    error: 'negative',
    danger: 'negative',
    negative: 'negative',
    active: 'accent',
  };
  const tone = Object.prototype.hasOwnProperty.call(toneByStatus, status)
    ? toneByStatus[status]
    : 'default';

  return (
    <Badge tone={tone} {...props}>
      {children}
    </Badge>
  );
}
