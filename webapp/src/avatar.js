const avatarModules = import.meta.glob('./icon/avatar_*.png', { eager: true, import: 'default' });

const AVATAR_URLS = Object.values(avatarModules);

export function walletAvatarIndex(address) {
  let hash = 0;
  const value = String(address || '').toLowerCase();
  for (let i = 0; i < value.length; i += 1) {
    hash = ((hash << 5) - hash + value.charCodeAt(i)) | 0;
  }
  return Math.abs(hash) % Math.max(AVATAR_URLS.length, 1);
}

export function walletAvatarUrl(address) {
  return AVATAR_URLS[walletAvatarIndex(address)] || AVATAR_URLS[0] || '';
}
