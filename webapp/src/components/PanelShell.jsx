import React from 'react';

export function EmptyState({ text }) {
  return <div className="empty-state">{text}</div>;
}

export function MetricCard({ label, value, tone = 'default' }) {
  return (
    <div className={`metric-card tone-${tone}`}>
      <div className="metric-label">{label}</div>
      <div className="metric-value">{value}</div>
    </div>
  );
}

export default function PanelShell({ title, subtitle, icon: Icon, actions, children }) {
  return (
    <section className="panel-shell">
      <header className="panel-header">
        <div className="panel-title-wrap">
          <div className="panel-icon-wrap">{Icon ? <Icon size={16} /> : null}</div>
          <div className="panel-title-texts">
            <h2>{title}</h2>
            {subtitle ? <p>{subtitle}</p> : null}
          </div>
        </div>
        {actions ? <div className="panel-actions">{actions}</div> : null}
      </header>
      <div className="panel-body">{children}</div>
    </section>
  );
}
