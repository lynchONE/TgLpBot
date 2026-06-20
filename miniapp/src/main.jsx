import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App.jsx';
import ErrorBoundary from './components/ErrorBoundary.jsx';
import './index.css';
import './okx-theme.css';

if (import.meta.env.DEV && new URLSearchParams(window.location.search).get('mockSwap') === '1') {
    import('./swap-module.mock.jsx');
} else {
    ReactDOM.createRoot(document.getElementById('root')).render(
        <React.StrictMode>
            <ErrorBoundary>
                <App />
            </ErrorBoundary>
        </React.StrictMode>
    );
}
