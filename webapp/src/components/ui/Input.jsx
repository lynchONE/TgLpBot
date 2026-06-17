import React from 'react';
import { cn } from './utils';

export const Input = React.forwardRef(function Input({ className = '', ...props }, ref) {
  return <input ref={ref} className={cn('ds-input', className)} {...props} />;
});

export function Field({ label, hint, error, className = '', children }) {
  return (
    <label className={cn('ds-field', className)}>
      {label ? <span className="ds-label">{label}</span> : null}
      {children}
      {hint && !error ? <span className="settings-hint">{hint}</span> : null}
      {error ? <span className="error-text">{error}</span> : null}
    </label>
  );
}

export const NumberInput = React.forwardRef(function NumberInput(props, ref) {
  return <Input ref={ref} type="number" inputMode="decimal" {...props} />;
});
