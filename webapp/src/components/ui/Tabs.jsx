import React from 'react';
import * as TabsPrimitive from '@radix-ui/react-tabs';
import { cn } from './utils';

export const Tabs = TabsPrimitive.Root;

export const TabsList = React.forwardRef(function TabsList({ className = '', ...props }, ref) {
  return <TabsPrimitive.List ref={ref} className={cn('ds-tabs-list', className)} {...props} />;
});

export const TabsTrigger = React.forwardRef(function TabsTrigger({ className = '', ...props }, ref) {
  return <TabsPrimitive.Trigger ref={ref} className={cn('ds-tab', className)} {...props} />;
});

export const TabsContent = TabsPrimitive.Content;
