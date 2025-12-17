import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App.jsx';
import './index.css';

import '@rainbow-me/rainbowkit/styles.css';
import {
  getDefaultConfig,
  RainbowKitProvider,
  darkTheme,
  lightTheme,
} from '@rainbow-me/rainbowkit';
import { WagmiProvider } from 'wagmi';
import {
  mainnet,
  polygon,
  optimism,
  arbitrum,
  base,
  bsc,
} from 'wagmi/chains';
import {
  QueryClientProvider,
  QueryClient,
} from '@tanstack/react-query';
import { ThemeProvider, useTheme } from './context/ThemeContext';

// Note: Replace with actual WalletConnect Project ID for production
const config = getDefaultConfig({
  appName: 'TgLpBot Dashboard',
  projectId: 'YOUR_PROJECT_ID',
  chains: [mainnet, bsc, arbitrum, optimism, polygon, base],
});

const queryClient = new QueryClient();

const ThemedRainbowKitProvider = ({ children }) => {
  const { theme } = useTheme();
  const isDark = theme === 'dark';

  const common = {
    accentColor: '#2563eb', // blue-600
    accentColorForeground: 'white',
    borderRadius: 'medium',
  };

  return (
    <RainbowKitProvider theme={isDark ? darkTheme(common) : lightTheme(common)}>
      {children}
    </RainbowKitProvider>
  );
};

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <WagmiProvider config={config}>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <ThemedRainbowKitProvider>
            <App />
          </ThemedRainbowKitProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </WagmiProvider>
  </React.StrictMode>,
);
