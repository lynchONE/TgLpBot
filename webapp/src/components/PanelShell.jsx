import React from 'react';
import { EmptyState as UiEmptyState, MetricTile, Panel, PanelBody, PanelHeader } from './ui';

export function EmptyState({ text }) {
  return <UiEmptyState text={text} className="empty-state" />;
}

export function MetricCard({ label, value, tone = 'default' }) {
  return <MetricTile label={label} value={value} tone={tone} />;
}

export default function PanelShell({ title, subtitle, icon: Icon, actions, children }) {
  return (
    <Panel className="panel-shell">
      <PanelHeader
        icon={Icon}
        title={title}
        subtitle={subtitle}
        actions={actions ? <div className="panel-actions">{actions}</div> : null}
        className="panel-header"
      />
      <PanelBody className="panel-body">{children}</PanelBody>
    </Panel>
  );
}
