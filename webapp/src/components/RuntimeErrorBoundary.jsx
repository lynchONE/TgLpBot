import React from 'react';

function formatError(error) {
  if (!error) return 'Unknown runtime error';
  if (error instanceof Error) {
    return error.stack || error.message || String(error);
  }
  if (typeof error === 'object') {
    return JSON.stringify(error, null, 2);
  }
  return String(error);
}

export function RuntimeErrorPanel({ error }) {
  const text = formatError(error);
  return (
    <main className="runtime-error-shell">
      <section className="runtime-error-panel">
        <div className="runtime-error-eyebrow">Runtime error</div>
        <h1>WebApp 启动失败</h1>
        <p>前端运行时抛出了错误。错误没有被吞掉，下面是浏览器捕获到的第一条信息。</p>
        <pre>{text}</pre>
      </section>
    </main>
  );
}

export default class RuntimeErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { error: null };
  }

  static getDerivedStateFromError(error) {
    return { error };
  }

  componentDidCatch(error) {
    setTimeout(() => {
      throw error;
    }, 0);
  }

  render() {
    if (this.state.error) {
      return <RuntimeErrorPanel error={this.state.error} />;
    }
    return this.props.children;
  }
}
