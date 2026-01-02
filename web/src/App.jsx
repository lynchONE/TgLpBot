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
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const handleScroll = () => {
      setScrolled(window.scrollY > 20);
    };
    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

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
          fontFamily: 'Inter, sans-serif',
          colorPrimary: '#3b82f6', // blue-500
          borderRadius: 12,
          colorBgBase: 'transparent',
        },
        components: {
          Table: {
            colorBgContainer: 'transparent',
            headerBg: 'transparent',
            rowHoverBg: 'transparent',
          },
          Button: {
            controlHeightSM: 32,
            borderRadiusSM: 8,
            fontWeight: 500,
          }
        }
      }}
    >
      <Layout className="min-h-screen bg-transparent">
        {/* Background Gradients */}
        <div className="fixed inset-0 z-0 pointer-events-none overflow-hidden">
          <div className="absolute top-[-20%] left-[-10%] w-[50%] h-[50%] bg-purple-500/20 rounded-full blur-[120px] dark:bg-purple-900/20" />
          <div className="absolute bottom-[-20%] right-[-10%] w-[50%] h-[50%] bg-blue-500/20 rounded-full blur-[120px] dark:bg-blue-900/20" />
        </div>

        <Header
          className={`sticky top-0 z-50 flex h-20 items-center justify-between px-4 sm:px-8 transition-all duration-300 ${scrolled ? 'glass border-b border-border shadow-sm' : 'bg-transparent border-transparent'
            }`}
          style={{ background: scrolled ? undefined : 'transparent' }}
        >
          <div className="flex items-center gap-4">
            <div className="relative group">
              <div className="absolute -inset-1 bg-gradient-to-r from-blue-600 to-purple-600 rounded-xl blur opacity-25 group-hover:opacity-75 transition duration-500"></div>
              <div className="relative grid h-10 w-10 place-items-center rounded-xl bg-white dark:bg-black border border-slate-200 dark:border-white/10 shadow-sm">
                <span className="text-xl font-bold bg-gradient-to-br from-blue-600 to-purple-600 bg-clip-text text-transparent">T</span>
              </div>
            </div>
            <div className="leading-tight">
              <div className="font-display text-xl font-bold tracking-tight text-foreground">
                TgLpBot
              </div>
            </div>
          </div>
          <div className="flex items-center gap-4">
            <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 rounded-full bg-secondary/50 border border-border">
              <span className="text-xs font-medium text-muted-foreground">Refresh:</span>
              <div className="flex items-center gap-1">
                <InputNumber
                  size="small"
                  min={5}
                  step={1}
                  value={refreshSeconds}
                  bordered={false}
                  className="!w-12 !bg-transparent text-right font-medium"
                  controls={false}
                  onChange={(v) => {
                    if (v == null) return;
                    setRefreshSeconds(Math.max(5, Math.floor(Number(v) || 5)));
                  }}
                />
                <span className="text-xs text-muted-foreground">s</span>
              </div>
            </div>
            <div className="h-6 w-px bg-border mx-2 hidden sm:block"></div>
            <ThemeToggle />
            <ConnectButton chainStatus="icon" showBalance={false} accountStatus={{ smallScreen: 'avatar', largeScreen: 'full' }} />
          </div>
        </Header>

        <Content className="relative z-10 px-4 py-8 sm:px-8">
          <div className={`mx-auto mb-8 flex ${gridMaxWidthClass} flex-col gap-4 sm:flex-row sm:items-center sm:justify-between animate-fade-in`}>
            <div>
              <h1 className="font-display text-3xl font-bold text-foreground mb-1">Dashboard</h1>
              <p className="text-muted-foreground">Manage your liquidity positions and view market stats.</p>
            </div>

            <div className="flex items-center gap-2 bg-secondary/30 p-1 rounded-xl border border-border/50 backdrop-blur-sm">
              <button
                onClick={() => setShowPools(!showPools)}
                className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${showPools ? 'bg-card text-card-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}`}
              >
                Top Pools
              </button>
              <button
                onClick={() => setShowPosition(!showPosition)}
                className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${showPosition ? 'bg-card text-card-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}`}
              >
                My Position
              </button>
            </div>
          </div>

          {showEmpty ? (
            <div className={`mx-auto ${gridMaxWidthClass} glass-card rounded-3xl p-16 text-center animate-slide-up`}>
              <div className="inline-flex h-20 w-20 items-center justify-center rounded-full bg-secondary/50 mb-6">
                <span className="text-4xl">👻</span>
              </div>
              <h3 className="text-xl font-bold text-foreground mb-2">Nothing to see here</h3>
              <p className="text-muted-foreground max-w-sm mx-auto">
                You've hidden all the dashboard widgets. Use the toggles above to bring them back.
              </p>
            </div>
          ) : (
            <div className={`mx-auto grid ${gridMaxWidthClass} grid-cols-1 gap-8 ${gridColsClass}`}>
              {showPools && (
                <section className={`glass-card rounded-3xl p-6 md:p-8 animate-slide-up ${showPools && showPosition ? 'xl:col-span-2' : ''}`}>
                  <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                    <div>
                      <h2 className="font-display text-xl font-bold text-foreground flex items-center gap-2">
                        <span className="flex h-2 w-2 rounded-full bg-green-500 animate-pulse"></span>
                        Top Pools
                      </h2>
                      <p className="text-sm text-muted-foreground mt-1">
                        Live market data from The Graph
                      </p>
                    </div>
                  </div>
                  <PoolTable refetchIntervalMs={refetchIntervalMs} />
                </section>
              )}

              {showPosition && (
                <section className={`space-y-6 animate-slide-up ${showPools && showPosition ? 'xl:col-span-1' : ''} transition-all duration-500`} style={{ animationDelay: '100ms' }}>
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
