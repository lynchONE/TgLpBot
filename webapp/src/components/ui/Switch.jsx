import React from 'react';
import * as SwitchPrimitive from '@radix-ui/react-switch';
import { cn } from './utils';

export const Switch = React.forwardRef(function Switch({ className = '', ...props }, ref) {
  return (
    <SwitchPrimitive.Root ref={ref} className={cn('ds-switch', className)} {...props}>
      <SwitchPrimitive.Thumb className="ds-switch-thumb" />
    </SwitchPrimitive.Root>
  );
});
