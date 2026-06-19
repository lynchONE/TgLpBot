import { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';

export default function AdminDrawer({
  open,
  title,
  subtitle,
  onClose,
  headerExtra,
  children,
  width = 480,
}) {
  const panelRef = useRef(null);

  useEffect(() => {
    if (!open) return undefined;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const handler = (event) => {
      if (event.key === 'Escape') onClose?.();
    };
    window.addEventListener('keydown', handler);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener('keydown', handler);
    };
  }, [open, onClose]);

  if (typeof document === 'undefined') return null;

  const node = (
    <div className={`am-drawer ${open ? 'is-open' : ''}`} aria-hidden={!open}>
      <div className="am-drawer-backdrop" onClick={onClose} />
      <aside
        ref={panelRef}
        className="am-drawer-panel"
        style={{ width: `min(96vw, ${width}px)` }}
        role="dialog"
        aria-modal="true"
      >
        <header className="am-drawer-head">
          <div className="am-drawer-titles">
            <div className="am-drawer-title">{title || '详情'}</div>
            {subtitle ? <div className="am-drawer-subtitle">{subtitle}</div> : null}
          </div>
          <div className="am-drawer-actions">
            {headerExtra}
            <button type="button" className="am-drawer-close" onClick={onClose} aria-label="关闭">
              <X size={16} />
            </button>
          </div>
        </header>
        <div className="am-drawer-body">{children}</div>
      </aside>
    </div>
  );

  return createPortal(node, document.body);
}
