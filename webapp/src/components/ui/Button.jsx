import React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cn } from './utils';

export const Button = React.forwardRef(function Button(
  {
    asChild = false,
    variant = 'default',
    size = 'md',
    active = false,
    className = '',
    children,
    ...props
  },
  ref
) {
  const Component = asChild ? Slot : 'button';
  return (
    <Component
      ref={ref}
      data-variant={variant}
      data-size={size}
      data-active={active ? 'true' : undefined}
      className={cn('ds-button', className)}
      {...props}
    >
      {children}
    </Component>
  );
});

export const IconButton = React.forwardRef(function IconButton(
  {
    asChild = false,
    size = 'md',
    active = false,
    className = '',
    children,
    ...props
  },
  ref
) {
  const Component = asChild ? Slot : 'button';
  return (
    <Component
      ref={ref}
      data-size={size}
      data-active={active ? 'true' : undefined}
      className={cn('ds-icon-button', className)}
      {...props}
    >
      {children}
    </Component>
  );
});
