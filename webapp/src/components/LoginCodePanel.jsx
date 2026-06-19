import { Check, Copy } from 'lucide-react';

export default function LoginCodePanel({ loginCode, onCancel, className = '' }) {
  if (!loginCode) return null;

  return (
    <div className={`login-code-box ${className}`.trim()}>
      <div className="login-code-top">
        <div className="login-code-badge">验证码</div>
        <div className="login-code-value">{loginCode}</div>
      </div>
      <div className="login-code-cmd-row">
        <code className="login-code-cmd">/weblogin {loginCode}</code>
        <button
          type="button"
          className="login-copy-btn"
          onClick={(e) => {
            navigator.clipboard.writeText(`/weblogin ${loginCode}`);
            const btn = e.currentTarget;
            btn.classList.add('copied');
            setTimeout(() => btn.classList.remove('copied'), 1500);
          }}
          aria-label="复制登录指令"
        >
          <Copy size={12} className="copy-icon" />
          <Check size={12} className="check-icon" />
        </button>
      </div>
      <div className="login-code-hint">在 Telegram Bot 中发送上方指令完成登录</div>
      {onCancel ? (
        <button type="button" className="ghost-chip" onClick={onCancel}>
          取消
        </button>
      ) : null}
    </div>
  );
}
