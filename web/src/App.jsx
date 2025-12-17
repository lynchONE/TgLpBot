import React, { useEffect, useMemo, useState } from 'react';
import { Button, ConfigProvider, InputNumber, Layout, theme as antdTheme } from 'antd';
import PoolTable from './components/PoolTable';
import PositionCard from './components/PositionCard';
import ThemeToggle from './components/ThemeToggle';
import { useTheme } from './context/ThemeContext';
import { ConnectButton } from '@rainbow-me/rainbowkit';

const { Header, Content } = Layout;

function App() {
  const { theme } = useTheme();
  const isDark = theme === 'dark';
  const refreshStorageKey = 'lp_dashboard_refresh_seconds';
  const [refreshSeconds, setRefreshSeconds] = useState(() => {
    const savedRaw = typeof window !== 'undefined' ? window.localStorage.getItem(refreshStorageKey) : null;
    const saved = Number(savedRaw);
    if (Number.isFinite(saved) && saved > 0) return Math.max(5, Math.floor(saved));
    return 30;
  });
  const [showPools, setShowPools] = useState(true);
  const [showPosition, setShowPosition] = useState(true);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(refreshStorageKey, String(refreshSeconds));
  }, [refreshSeconds]);

  const refetchIntervalMs = useMemo(() => Math.max(5000, refreshSeconds * 1000), [refreshSeconds]);
  const showEmpty = !showPools && !showPosition;
  const gridMaxWidthClass = showPools ? 'max-w-[1600px]' : 'max-w-[720px]';
  const gridColsClass = showPools && showPosition ? 'xl:grid-cols-3' : 'xl:grid-cols-1';

  return (
    <ConfigProvider
      theme={{
        algorithm: isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        token: {
          colorPrimary: '#2563eb',
          borderRadiusLG: 16,
          colorBgBase: isDark ? '#0d0e10' : '#f8fafc',
          colorBgContainer: isDark ? '#151718' : '#ffffff',
          colorBgElevated: isDark ? '#1f2123' : '#ffffff',
        },
      }}
    >
      <Layout className="min-h-screen bg-slate-50 text-slate-900 dark:bg-[#0d0e10] dark:text-slate-100">
        <Header className="sticky top-0 z-50 flex h-16 items-center justify-between border-b border-slate-200/70 bg-white/70 px-4 backdrop-blur dark:border-slate-800/70 dark:bg-[#151718]/60 sm:px-6">
          <div className="flex items-center gap-3">
            <div className="grid h-9 w-9 place-items-center rounded-xl bg-gradient-to-br from-blue-600 to-purple-600 font-extrabold text-white shadow-sm ring-1 ring-black/10 dark:ring-white/10">
              TG
            </div>
            <div className="leading-tight">
              <div className="text-base font-semibold tracking-tight text-slate-900 dark:text-white">
                LP Dashboard
              </div>
              <div className="text-xs text-slate-500 dark:text-slate-400">
                Pools & positions · Auto refresh {refreshSeconds}s
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <ThemeToggle />
            <ConnectButton chainStatus="icon" showBalance={false} />
          </div>
        </Header>
        <Content className="px-4 py-6 sm:px-6">
          <div className={`mx-auto mb-4 flex ${gridMaxWidthClass} flex-col gap-3 sm:flex-row sm:items-center sm:justify-between`}>
            <div className="flex flex-wrap items-center gap-2">
              <Button size="small" type={showPools ? 'primary' : 'default'} onClick={() => setShowPools((v) => !v)}>
                {showPools ? 'Hide Top Pools' : 'Show Top Pools'}
              </Button>
              <Button
                size="small"
                type={showPosition ? 'primary' : 'default'}
                onClick={() => setShowPosition((v) => !v)}
              >
                {showPosition ? 'Hide My Position' : 'Show My Position'}
              </Button>
            </div>
            <div className="flex items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
              <span>Refresh</span>
              <InputNumber
                size="small"
                min={5}
                step={1}
                value={refreshSeconds}
                onChange={(v) => {
                  if (v == null) return;
                  setRefreshSeconds(Math.max(5, Math.floor(Number(v) || 5)));
                }}
              />
              <span>s (min 5s)</span>
            </div>
          </div>

          {showEmpty ? (
            <div className={`mx-auto ${gridMaxWidthClass} rounded-2xl border border-slate-200/70 bg-white/80 p-10 text-center shadow-sm ring-1 ring-black/5 dark:border-slate-800/70 dark:bg-[#151718]/70 dark:ring-white/10`}>
              <div className="text-base font-semibold text-slate-900 dark:text-white">Nothing to show</div>
              <div className="mt-2 text-xs text-slate-500 dark:text-slate-400">
                Use the buttons above to show Top Pools or My Position.
              </div>
            </div>
          ) : (
            <div className={`mx-auto grid ${gridMaxWidthClass} grid-cols-1 gap-6 ${gridColsClass}`}>
              {showPools && (
                <section
                  className={`rounded-2xl border border-slate-200/70 bg-white/80 p-6 shadow-sm ring-1 ring-black/5 dark:border-slate-800/70 dark:bg-[#151718]/70 dark:ring-white/10 ${
                    showPools && showPosition ? 'xl:col-span-2' : ''
                  }`}
                >
                  <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <div>
                      <h2 className="text-lg font-semibold text-slate-900 dark:text-white">Top Pools</h2>
                      <p className="text-xs text-slate-500 dark:text-slate-400">
                        Switch 5m/1h/6h/24h for volume, fees, APR, change
                      </p>
                    </div>
                    <Button size="small" onClick={() => setShowPools(false)}>
                      Collapse
                    </Button>
                  </div>
                  <PoolTable refetchIntervalMs={refetchIntervalMs} />
                </section>
              )}

              {showPosition && (
                <section className={`${showPools && showPosition ? 'xl:col-span-1' : ''} space-y-4`}>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <h2 className="text-lg font-semibold text-slate-900 dark:text-white">My Position</h2>
                      <p className="text-xs text-slate-500 dark:text-slate-400">Reads your first V3 LP position on-chain</p>
                    </div>
                    <Button size="small" onClick={() => setShowPosition(false)}>
                      Collapse
                    </Button>
                  </div>
                  <PositionCard refetchIntervalMs={refetchIntervalMs} />
                </section>
              )}
            </div>
          )}
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

export default App;
