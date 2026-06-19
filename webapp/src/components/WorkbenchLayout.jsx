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
  if (!activeWidgets.length) {
    return (
      <main className={className}>
        <div className="workbench-empty-state">
          <div className="workbench-empty-title">暂无可用模块</div>
          <div className="workbench-empty-text">
            当前登录态没有可展示的工作台模块。请重新登录，或在后台检查 WebApp 模块权限。
          </div>
        </div>
      </main>
    );
  }

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
