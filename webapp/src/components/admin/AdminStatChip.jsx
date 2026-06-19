import NumberFlowValue from '../NumberFlowValue';
import AdminStatusDot from './AdminStatusDot';

function isNumericLike(value) {
  if (value === null || value === undefined) return false;
  if (typeof value === 'number') return Number.isFinite(value);
  if (typeof value !== 'string') return false;
  return /^-?\d+(?:\.\d+)?$/.test(value.trim());
}

export default function AdminStatChip({
  label,
  value,
  tone = 'idle',
  hint,
  onClick,
  pulse = false,
  formatOptions,
  className = '',
}) {
  const interactive = typeof onClick === 'function';
  const classes = ['am-stat-chip', `tone-${tone}`];
  if (interactive) classes.push('is-clickable');
  if (className) classes.push(className);

  const valueNode = isNumericLike(value)
    ? <NumberFlowValue value={Number(value)} formatOptions={formatOptions || { maximumFractionDigits: 0 }} />
    : (value ?? '--');

  const inner = (
    <>
      <div className="am-stat-chip-head">
        <span className="am-stat-chip-label">{label}</span>
        {pulse ? <AdminStatusDot tone={tone === 'idle' ? 'accent' : tone} pulse size="xs" /> : null}
      </div>
      <div className="am-stat-chip-value">{valueNode}</div>
      {hint ? <div className="am-stat-chip-hint">{hint}</div> : null}
    </>
  );

  if (interactive) {
    return (
      <button type="button" className={classes.join(' ')} onClick={onClick}>
        {inner}
      </button>
    );
  }
  return <div className={classes.join(' ')}>{inner}</div>;
}
