import React, { Suspense, lazy } from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';

// Lazy load the main App (includes heavy wagmi/rainbowkit)
const App = lazy(() => import('./App.jsx'));

// Simple loading spinner (displayed while App loads)
const LoadingFallback = () => (
  <div className="fixed inset-0 bg-midnight-950 flex flex-col items-center justify-center">
    <div className="relative">
      <div className="absolute inset-0 bg-neon-cyan blur-xl opacity-20 animate-pulse"></div>
      <div className="relative w-16 h-16 border-4 border-neon-cyan/20 border-t-neon-cyan rounded-full animate-spin"></div>
    </div>
    <p className="mt-6 text-sm font-mono text-neon-cyan/80 tracking-widest animate-pulse">
      INITIALIZING...
    </p>
  </div>
);

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <Suspense fallback={<LoadingFallback />}>
      <App />
    </Suspense>
  </React.StrictMode>,
);
