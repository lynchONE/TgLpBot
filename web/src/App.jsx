import React, { useEffect, useMemo, useState, lazy, Suspense } from 'react';
import { ConfigProvider, InputNumber, Layout, theme as antdTheme } from 'antd';
import { ThemeProvider, useTheme } from './context/ThemeContext';
import { ConnectButton, getDefaultConfig, RainbowKitProvider, darkTheme } from '@rainbow-me/rainbowkit';
import { WagmiProvider } from 'wagmi';
import { bsc, mainnet, arbitrum } from 'wagmi/chains';
import { QueryClientProvider, QueryClient } from '@tanstack/react-query';
import '@rainbow-me/rainbowkit/styles.css';

// Lazy load heavy components
const PoolGrid = lazy(() => import('./components/PoolTable'));
const PositionCard = lazy(() => import('./components/PositionCard'));
const ThemeToggle = lazy(() => import('./components/ThemeToggle'));

// QueryClient - optimized but still real-time
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false, // Don't refetch on tab switch (saves requests)
      retry: 1, // Only retry once on failure (faster error handling)
    },
  },
});

// Wagmi config - only include chains we actually use
const wagmiConfig = getDefaultConfig({
  appName: 'TgLpBot Dashboard',
  projectId: 'YOUR_PROJECT_ID',
  chains: [bsc, mainnet, arbitrum], // Reduced chain list
});

const { Content } = Layout;

// Loading placeholder for lazy components
const ComponentLoader = () => (
  <div className="animate-pulse bg-white/5 rounded-2xl h-full min-h-[200px]"></div>
);

function App() {
  const { theme } = useTheme();
  // V2 is always "Dark Mode" aesthetically, light mode is just a semantic requirement 
  // but for the "Command Center" vibe, we force dark algorithm unless explicitly light.
  const isDark = true;

  const refreshStorageKey = 'lp_dashboard_refresh_seconds';
  const [refreshSeconds, setRefreshSeconds] = useState(() => {
    const savedRaw = typeof window !== 'undefined' ? window.localStorage.getItem(refreshStorageKey) : null;
    const saved = Number(savedRaw);
    if (Number.isFinite(saved) && saved > 0) return Math.max(5, Math.floor(saved));
    return 30;
  });

  // V2 Navigation State
  const [activeView, setActiveView] = useState('dashboard'); // 'dashboard', 'pools', 'settings'

  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(refreshStorageKey, String(refreshSeconds));
  }, [refreshSeconds]);

  const refetchIntervalMs = useMemo(() => Math.max(5000, refreshSeconds * 1000), [refreshSeconds]);

  return (
    <ConfigProvider
      theme={{
        algorithm: antdTheme.darkAlgorithm,
        token: {
          fontFamily: 'Inter, sans-serif',
          colorPrimary: '#06b6d4', // neon-cyan
          borderRadius: 8,
          colorBgBase: '#020617', // midnight-950
        },
      }}
    >
      <Layout className="min-h-screen bg-transparent relative selection:bg-neon-cyan/30 selection:text-neon-cyan">

        {/* Top Stats Bar */}
        <div className="fixed top-0 inset-x-0 h-12 z-50 bg-midnight-950/80 backdrop-blur-md border-b border-white/5 flex items-center justify-between px-4 sm:px-6">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <div className="w-2 h-2 rounded-full bg-neon-green animate-pulse"></div>
              <span className="text-[10px] font-mono font-bold text-slate-400 uppercase tracking-widest">System Online</span>
            </div>
            <div className="h-4 w-px bg-white/10"></div>
            <div className="flex items-center gap-1.5 opacity-60 hover:opacity-100 transition-opacity">
              <span className="text-[10px] text-slate-500 uppercase font-bold">ETH</span>
              <span className="text-xs font-mono text-neon-blue">$3,240.50</span>
            </div>
            <div className="flex items-center gap-1.5 opacity-60 hover:opacity-100 transition-opacity hidden sm:flex">
              <span className="text-[10px] text-slate-500 uppercase font-bold">Gas</span>
              <span className="text-xs font-mono text-neon-purple">15 Gwei</span>
            </div>
          </div>

          <div className="flex items-center gap-4">
            <ConnectButton.Custom>
              {({ account, chain, openAccountModal, openChainModal, openConnectModal, mounted }) => {
                const ready = mounted;
                const connected = ready && account && chain;
                return (
                  <div
                    {...(!ready && { 'aria-hidden': true, 'style': { opacity: 0, pointerEvents: 'none', userSelect: 'none' } })}
                  >
                    {(() => {
                      if (!connected) {
                        return (
                          <button onClick={openConnectModal} className="text-xs font-bold bg-neon-blue/10 text-neon-blue border border-neon-blue/20 px-3 py-1.5 rounded hover:bg-neon-blue/20 transition-all">
                            CONNECT LINK
                          </button>
                        );
                      }
                      return (
                        <div className="flex items-center gap-2">
                          <button onClick={openChainModal} className="hidden sm:flex items-center gap-1 text-[10px] font-bold bg-white/5 border border-white/10 px-2 py-1 rounded text-slate-300 hover:text-white transition-colors">
                            {chain.hasIcon && (
                              <div style={{ background: chain.iconBackground, width: 12, height: 12, borderRadius: 999, overflow: 'hidden', marginRight: 4 }}>
                                {chain.iconUrl && (
                                  <img alt={chain.name ?? 'Chain icon'} src={chain.iconUrl} style={{ width: 12, height: 12 }} />
                                )}
                              </div>
                            )}
                            {chain.name}
                          </button>
                          <button onClick={openAccountModal} className="flex items-center gap-2 text-xs font-bold font-mono text-neon-cyan border border-neon-cyan/20 bg-neon-cyan/5 px-2 py-1 rounded hover:bg-neon-cyan/10 transition-colors shadow-[0_0_10px_rgba(6,182,212,0.1)]">
                            {account.displayName}
                            <span className="text-slate-500">
                              {account.displayBalance ? ` (${account.displayBalance})` : ''}
                            </span>
                          </button>
                        </div>
                      );
                    })()}
                  </div>
                );
              }}
            </ConnectButton.Custom>
          </div>
        </div>

        {/* Main Content Area */}
        <Content className="pt-20 pb-32 px-4 sm:px-6 max-w-[1800px] mx-auto w-full">

          {/* Header Greeting */}
          <div className="mb-10 animate-fade-in">
            <h1 className="font-display text-4xl sm:text-5xl font-bold text-white mb-2 tracking-tight">
              Command Center
            </h1>
            <p className="text-slate-400 text-lg max-w-2xl font-light">
              Real-time liquidity management and market intelligence.
            </p>
          </div>

          {/* BENTO GRID LAYOUT */}
          <div className="grid grid-cols-1 xl:grid-cols-4 gap-6 animate-slide-up">

            {/* Active View Logic */}
            {activeView === 'dashboard' && (
              <>
                {/* LEFT: Position Card Widget (Takes 1/4 on XL) */}
                <div className="xl:col-span-1 h-[420px] xl:h-auto">
                  <Suspense fallback={<ComponentLoader />}><PositionCard refetchIntervalMs={refetchIntervalMs} /></Suspense>
                </div>

                {/* RIGHT: Market Grid (Takes 3/4 on XL) */}
                <div className="xl:col-span-3 min-h-[500px]">
                  <Suspense fallback={<ComponentLoader />}><PoolGrid refetchIntervalMs={refetchIntervalMs} /></Suspense>
                </div>
              </>
            )}

            {activeView === 'pools' && (
              <div className="col-span-1 xl:col-span-4 min-h-[80vh]">
                <PoolGrid refetchIntervalMs={refetchIntervalMs} />
              </div>
            )}

            {activeView === 'settings' && (
              <div className="col-span-1 xl:col-span-4 min-h-[50vh] flex items-center justify-center">
                <div className="glass-panel p-10 rounded-3xl max-w-md w-full">
                  <h2 className="text-2xl font-display font-bold text-white mb-6">System Configuration</h2>

                  <div className="space-y-6">
                    <div>
                      <label className="text-xs font-bold text-slate-500 uppercase tracking-widest block mb-2">Auto-Refresh Rate</label>
                      <div className="flex items-center gap-4">
                        <InputNumber
                          size="large"
                          min={5}
                          step={1}
                          value={refreshSeconds}
                          onChange={(v) => v && setRefreshSeconds(v)}
                          className="w-full !bg-midnight-950 !border-white/10 !text-white"
                        />
                        <span className="text-slate-400 font-mono">SEC</span>
                      </div>
                      <p className="text-[10px] text-slate-500 mt-2">Lower values increase RPC usage.</p>
                    </div>

                    <div className="pt-6 border-t border-white/5">
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-slate-300">Theme Preference</span>
                        <ThemeToggle />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            )}

          </div>
        </Content>

        {/* Floating Dock Navigation */}
        <div className="fixed bottom-8 inset-x-0 z-50 flex justify-center pointer-events-none">
          <div className="glass-panel px-6 py-3 rounded-2xl flex items-center gap-6 pointer-events-auto transform hover:scale-105 transition-transform duration-300 shadow-[0_20px_40px_rgba(0,0,0,0.4)]">

            <button
              onClick={() => setActiveView('dashboard')}
              className={`group relative p-2 rounded-xl transition-all ${activeView === 'dashboard' ? 'bg-white/10 text-white' : 'text-slate-500 hover:text-slate-300'}`}
            >
              <div className="absolute -top-1 right-0 opacity-0 group-hover:opacity-100 transition-opacity">
                <span className="relative flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-neon-cyan opacity-75"></span>
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-neon-cyan"></span>
                </span>
              </div>
              <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
              </svg>
              <span className="absolute -bottom-8 left-1/2 -translate-x-1/2 text-[10px] font-bold uppercase tracking-wider opacity-0 group-hover:opacity-100 transition-opacity bg-midnight-950 px-2 py-0.5 rounded border border-white/10 text-white">Dashboard</span>
            </button>

            <button
              onClick={() => setActiveView('pools')}
              className={`group relative p-2 rounded-xl transition-all ${activeView === 'pools' ? 'bg-white/10 text-white' : 'text-slate-500 hover:text-slate-300'}`}
            >
              <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6" />
              </svg>
              <span className="absolute -bottom-8 left-1/2 -translate-x-1/2 text-[10px] font-bold uppercase tracking-wider opacity-0 group-hover:opacity-100 transition-opacity bg-midnight-950 px-2 py-0.5 rounded border border-white/10 text-white">Market</span>
            </button>

            <div className="w-px h-8 bg-white/10"></div>

            <button
              onClick={() => setActiveView('settings')}
              className={`group relative p-2 rounded-xl transition-all ${activeView === 'settings' ? 'bg-white/10 text-white' : 'text-slate-500 hover:text-slate-300'}`}
            >
              <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
              <span className="absolute -bottom-8 left-1/2 -translate-x-1/2 text-[10px] font-bold uppercase tracking-wider opacity-0 group-hover:opacity-100 transition-opacity bg-midnight-950 px-2 py-0.5 rounded border border-white/10 text-white">System</span>
            </button>
          </div>
        </div>
      </Layout>
    </ConfigProvider>
  );
}

// Wrap App with all providers for proper lazy loading
const AppWithProviders = () => (
  <WagmiProvider config={wagmiConfig}>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <RainbowKitProvider theme={darkTheme({ accentColor: '#06b6d4', borderRadius: 'medium' })}>
          <App />
        </RainbowKitProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </WagmiProvider>
);

export default AppWithProviders;
