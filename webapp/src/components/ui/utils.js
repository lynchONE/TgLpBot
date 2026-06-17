export function cn(...values) {
  return values
    .flatMap((value) => {
      if (!value) return [];
      if (Array.isArray(value)) return value;
      if (typeof value === 'object') {
        return Object.entries(value)
          .filter(([, enabled]) => Boolean(enabled))
          .map(([key]) => key);
      }
      return [String(value)];
    })
    .filter(Boolean)
    .join(' ');
}
