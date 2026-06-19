import { cn } from './utils';

export function Panel({ className = '', children, ...props }) {
  return (
    <section className={cn('ds-panel', className)} {...props}>
      {children}
    </section>
  );
}

export function PanelHeader({ icon: Icon, title, subtitle, actions, className = '' }) {
  return (
    <header className={cn('ds-panel-header', className)}>
      <div className="ds-panel-title-row panel-title-wrap">
        {Icon ? (
          <div className="ds-panel-icon panel-icon-wrap" aria-hidden="true">
            <Icon size={16} />
          </div>
        ) : null}
        <div className="panel-title-texts">
          <h2 className="ds-panel-title">{title}</h2>
          {subtitle ? <p className="ds-panel-subtitle">{subtitle}</p> : null}
        </div>
      </div>
      {actions ? <div>{actions}</div> : null}
    </header>
  );
}

export function PanelBody({ className = '', children, ...props }) {
  return (
    <div className={cn('ds-panel-body', className)} {...props}>
      {children}
    </div>
  );
}

export function MetricTile({ label, value, tone = 'default', className = '' }) {
  return (
    <div className={cn('metric-card', `tone-${tone}`, className)}>
      <div className="metric-label">{label}</div>
      <div className="metric-value">{value}</div>
    </div>
  );
}
