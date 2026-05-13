import React, { useEffect, useRef, useState } from 'react';
import { Check, ChevronDown } from 'lucide-react';

export default function CustomSelect({
  value,
  onChange,
  options = [],
  placeholder = '请选择',
  disabled = false,
  className = '',
  buttonClassName = '',
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  const selected = options.find((item) => String(item.value) === String(value));

  useEffect(() => {
    if (!open) return undefined;
    const onPointerDown = (event) => {
      if (ref.current && !ref.current.contains(event.target)) setOpen(false);
    };
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [open]);

  return (
    <div ref={ref} className={`ui-select ${className}`}>
      <button
        type="button"
        disabled={disabled}
        className={`ui-select-trigger ${buttonClassName}`}
        onClick={() => !disabled && setOpen((current) => !current)}
      >
        <span className="ui-select-value">{selected?.label || placeholder}</span>
        <ChevronDown size={15} className={open ? 'open' : ''} />
      </button>
      {open ? (
        <div className="ui-select-menu">
          {options.map((item) => {
            const active = String(item.value) === String(value);
            return (
              <button
                key={String(item.value)}
                type="button"
                className={`ui-select-option${active ? ' active' : ''}`}
                onClick={() => {
                  onChange?.(item.value);
                  setOpen(false);
                }}
              >
                <span>{item.label}</span>
                {active ? <Check size={14} /> : null}
              </button>
            );
          })}
        </div>
      ) : null}
    </div>
  );
}
