import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App.jsx';
import RuntimeErrorBoundary, { RuntimeErrorPanel } from './components/RuntimeErrorBoundary.jsx';
import './styles.css';

const rootElement = document.getElementById('root');
const root = ReactDOM.createRoot(rootElement);

function renderRuntimeError(error) {
  root.render(<RuntimeErrorPanel error={error} />);
}

window.addEventListener('error', (event) => {
  if (rootElement && rootElement.childElementCount === 0) {
    renderRuntimeError(event.error || event.message);
  }
});

window.addEventListener('unhandledrejection', (event) => {
  if (rootElement && rootElement.childElementCount === 0) {
    renderRuntimeError(event.reason);
  }
});

root.render(
  <React.StrictMode>
    <RuntimeErrorBoundary>
      <App />
    </RuntimeErrorBoundary>
  </React.StrictMode>
);
