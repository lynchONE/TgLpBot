import { cn } from './utils';

export function Skeleton({ className = '', ...props }) {
  return <div className={cn('ds-skeleton', className)} {...props} />;
}

export function EmptyState({ text, children, className = '' }) {
  return (
    <div className={cn('ds-empty-state', className)}>
      {children !== undefined && children !== null ? children : text}
    </div>
  );
}
