import React from 'react';
import { GripVertical } from 'lucide-react';

export default function WorkbenchLayout({
  className,
  activeWidgets,
  panelMap,
  hotPoolsPanelHeight,
  draggingKey,
  dragOverKey,
  onDragOverWidget,
  onDropWidget,
  onDragEnd,
  onDragStartWidget,
}) {
  return (
    <main className={className}>
      {activeWidgets.map((widget) => (
        <div
          key={widget.key}
          className={`module-slot module-${widget.key} ${
            draggingKey === widget.key ? 'dragging' : ''
          } ${dragOverKey === widget.key ? 'drop-target' : ''}`}
          style={widget.key === 'hot_pools' ? { '--hot-pools-panel-height': `${hotPoolsPanelHeight}px` } : undefined}
          onDragOver={(event) => onDragOverWidget(event, widget.key)}
          onDrop={(event) => onDropWidget(event, widget.key)}
          onDragEnd={onDragEnd}
        >
          <div
            className="drag-hint"
            draggable
            title="按住拖动调整模块顺序"
            onDragStart={(event) => onDragStartWidget(event, widget.key)}
            onDragEnd={onDragEnd}
          >
            <GripVertical size={12} />
          </div>
          {panelMap[widget.key]}
        </div>
      ))}
    </main>
  );
}
