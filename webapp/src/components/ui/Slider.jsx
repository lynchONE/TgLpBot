import React from 'react';
import * as SliderPrimitive from '@radix-ui/react-slider';
import { cn } from './utils';

export const Slider = React.forwardRef(function Slider({ className = '', ...props }, ref) {
  return (
    <SliderPrimitive.Root ref={ref} className={cn('ds-slider', className)} {...props}>
      <SliderPrimitive.Track className="ds-slider-track">
        <SliderPrimitive.Range className="ds-slider-range" />
      </SliderPrimitive.Track>
      <SliderPrimitive.Thumb className="ds-slider-thumb" />
    </SliderPrimitive.Root>
  );
});
