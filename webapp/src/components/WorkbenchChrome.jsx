import {
  LogOut,
  Maximize,
  Minimize,
  Settings,
  SlidersHorizontal,
} from 'lucide-react';
import LoginCodePanel from './LoginCodePanel';
import {
  Button,
  IconButton,
  Popover,
  PopoverContent,
  PopoverTrigger,
} from './ui';
import telegramLogo from '../img/telegram.svg';
import bnbLogo from '../img/bnb.svg';
import baseLogo from '../img/base.svg';

export function WorkModeBar({ onExit }) {
  return (
    <div className="work-mode-bar">
      <Button type="button" variant="ghost" size="sm" className="work-mode-exit-btn" onClick={onExit}>
        <Minimize size={14} />
        退出工作模式
      </Button>
    </div>
  );
}

function SettingsPopover({
  open,
  onOpenChange,
  refreshModuleConfig,
  maxRefreshIntervalSec,
  refreshIntervalDrafts,
  refreshIntervals,
  onRefreshDraftChange,
  onRefreshDraftCommit,
  onResetRefreshIntervals,
  accentThemes,
  accentTheme,
  onAccentThemeChange,
  hasTrackedPositions,
  klineHasTrackedPositionToken,
}) {
  return (
    <div className="settings-wrap">
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger asChild>
          <IconButton type="button" className="settings-btn" aria-label="设置" active={open}>
            <Settings size={15} />
          </IconButton>
        </PopoverTrigger>
        {open ? (
          <PopoverContent className="settings-popover" align="end">
            <div className="settings-row settings-row-stack">
              <span className="settings-label">接口刷新间隔</span>
              <div className="settings-refresh-list">
                {refreshModuleConfig.map((item) => {
                  const hasDraft = Object.prototype.hasOwnProperty.call(refreshIntervalDrafts, item.key);
                  const inputValue = hasDraft ? refreshIntervalDrafts[item.key] : String(refreshIntervals[item.key]);
                  return (
                    <label key={item.key} className="settings-refresh-row">
                      <span>{item.label}</span>
                      <div className="settings-input-wrap">
                        <input
                          type="number"
                          className="settings-input"
                          min={item.minSec}
                          max={maxRefreshIntervalSec}
                          value={inputValue}
                          onChange={(e) => onRefreshDraftChange(item.key, e.target.value)}
                          onBlur={() => onRefreshDraftCommit(item.key)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                              e.preventDefault();
                              onRefreshDraftCommit(item.key);
                              e.currentTarget.blur();
                            }
                          }}
                        />
                        <span className="settings-unit">秒</span>
                      </div>
                    </label>
                  );
                })}
              </div>
              <button type="button" className="settings-reset-btn" onClick={onResetRefreshIntervals}>
                恢复默认刷新
              </button>
            </div>
            <div className="settings-row settings-row-stack">
              <span className="settings-label">主题色</span>
              <div className="settings-theme-group">
                {accentThemes.map((theme) => (
                  <Button
                    key={theme.key}
                    type="button"
                    variant="ghost"
                    size="sm"
                    active={accentTheme === theme.key}
                    className={`settings-theme-btn ${accentTheme === theme.key ? 'active' : ''}`}
                    onClick={() => onAccentThemeChange(theme.key)}
                  >
                    <span className={`settings-theme-dot theme-dot-${theme.key}`} />
                    {theme.label}
                  </Button>
                ))}
              </div>
            </div>
            <div className="settings-hint">默认绿色，你也可以切回黄色主色。</div>
            <div className="settings-hint">
              各模块独立保存到当前浏览器；仓位会按当前是否有仓位自动切换，当前是{hasTrackedPositions ? '有仓位' : '无仓位'}档。
            </div>
            <div className="settings-hint">
              K 线会按当前展示代币是否命中仓位代币自动切换，当前是{klineHasTrackedPositionToken ? '有对应仓位' : '无对应仓位'}档。
            </div>
            <div className="settings-hint">我的资产最低 60 秒，K 线有仓位档最低 5 秒，无仓位档最低 10 秒。</div>
            <div className="settings-hint" style={{ marginTop: 6 }}>K线使用 REST 轮询刷新。</div>
          </PopoverContent>
        ) : null}
      </Popover>
    </div>
  );
}

export function TopBar({
  hasInitData,
  loginUser,
  loginCode,
  loginBusy,
  showSettings,
  onSettingsOpenChange,
  onStartLogin,
  onCancelLogin,
  onLogout,
  settings,
}) {
  const firstName = loginUser && typeof loginUser.first_name === 'string' ? loginUser.first_name.trim() : '';
  const username = loginUser && typeof loginUser.username === 'string' ? loginUser.username.trim() : '';
  const photoUrl = loginUser && typeof loginUser.photo_url === 'string' ? loginUser.photo_url.trim() : '';

  return (
    <header className="top-bar">
      <div className="title-block">
        <div className="eyebrow">lynch</div>
      </div>

      <div className="top-actions">
        {hasInitData && loginUser ? (
          <div className="user-chip">
            {photoUrl ? (
              <img src={photoUrl} alt="avatar" className="user-avatar" />
            ) : (
              <div className="user-avatar fallback">{firstName ? firstName.slice(0, 1) : '?'}</div>
            )}
            <div className="user-meta">
              <div className="user-name">{firstName ? firstName : 'Telegram User'}</div>
              <div className="user-sub">@{username ? username : 'unknown'}</div>
            </div>
            <SettingsPopover
              open={showSettings}
              onOpenChange={onSettingsOpenChange}
              {...settings}
            />
            <Button type="button" variant="ghost" size="sm" className="logout-btn" onClick={onLogout}>
              <LogOut size={13} />
              退出
            </Button>
          </div>
        ) : loginCode && hasInitData ? (
          <LoginCodePanel loginCode={loginCode} onCancel={onCancelLogin} />
        ) : (
          <IconButton
            type="button"
            className="telegram-icon-btn"
            onClick={onStartLogin}
            disabled={loginBusy}
            title="获取登录验证码"
            aria-label="获取登录验证码"
          >
            <img src={telegramLogo} alt="Telegram" />
          </IconButton>
        )}
      </div>
    </header>
  );
}

export function WorkbenchConfigPanel({
  hasInitData,
  chain,
  onChainChange,
  availableWidgets,
  widgets,
  onToggleWidget,
  onEnterWorkMode,
}) {
  if (!hasInitData) return null;

  return (
    <section className="config-panel">
      <div className="config-head">
        <SlidersHorizontal size={14} />
        <span>布局与链设置</span>
      </div>

      <div className="chain-toggles">
        <Button type="button" variant="ghost" size="sm" active={chain === 'bsc'} className={`chain-btn ${chain === 'bsc' ? 'active' : ''}`} onClick={() => onChainChange('bsc')}>
          <img src={bnbLogo} alt="BSC" className="chain-icon" />
          <span>BSC</span>
        </Button>
        <Button type="button" variant="ghost" size="sm" active={chain === 'base'} className={`chain-btn ${chain === 'base' ? 'active' : ''}`} onClick={() => onChainChange('base')}>
          <img src={baseLogo} alt="Base" className="chain-icon" />
          <span>Base</span>
        </Button>
      </div>

      <div className="widget-toggles">
        {availableWidgets.map((item) => (
          <Button
            type="button"
            key={item.key}
            variant="ghost"
            size="sm"
            active={widgets.includes(item.key)}
            className={`toggle-chip ${widgets.includes(item.key) ? 'active' : ''}`}
            onClick={() => onToggleWidget(item.key)}
          >
            {item.label}
          </Button>
        ))}
        <Button type="button" variant="ghost" size="sm" className="work-mode-btn" onClick={onEnterWorkMode}>
          <Maximize size={13} />
          工作模式
        </Button>
      </div>
    </section>
  );
}
