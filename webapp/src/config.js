function fallbackApiBaseUrl() {
  const host = window.location.hostname;
  if (host === 'localhost' || host === '127.0.0.1') return 'http://localhost:8080';
  return '';
}

function normalizeChain(value) {
  const chain = String(value || '').trim().toLowerCase();
  return chain === 'base' ? 'base' : 'bsc';
}

export const WEBAPP_CONFIG = {
  apiBaseUrl: String(import.meta.env.VITE_API_BASE_URL || '').trim() || fallbackApiBaseUrl(),
  defaultChain: normalizeChain(import.meta.env.VITE_DEFAULT_CHAIN || 'bsc'),
  telegramBotId: String(import.meta.env.VITE_TELEGRAM_BOT_ID || '').trim(),
  telegramBotUsername: String(import.meta.env.VITE_TELEGRAM_BOT_USERNAME || '').trim(),
};
