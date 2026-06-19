
export default function AdminStatusDot({ tone = 'idle', pulse = false, size = 'sm', className = '' }) {
  const classes = ['am-status-dot', `am-status-dot-${size}`, `tone-${tone}`];
  if (pulse) classes.push('is-pulse');
  if (className) classes.push(className);
  return <span className={classes.join(' ')} aria-hidden="true" />;
}
