import type { PreviewBootstrap } from './bootstrap';

interface AppProps {
  bootstrap: PreviewBootstrap;
}

export function App({ bootstrap }: AppProps) {
  const mode = bootstrap.mode || 'dashboard';
  const currentRef = mode === 'per_cr'
    ? `CR-${bootstrap.cr_id ?? '?'}`
    : bootstrap.selected_cr_id
      ? `CR-${bootstrap.selected_cr_id}`
      : 'Dashboard';

  return (
    <div className="preview-app-shell">
      <section className="preview-panel">
        <p className="eyebrow">Local Preview Shell</p>
        <h1>Sophia preview runtime is serving frontend assets from Go.</h1>
        <p className="body">
          This shell is intentionally minimal. The next stack layer ports the real dashboard
          and CR detail UI onto this runtime.
        </p>
        <dl className="meta-grid">
          <div>
            <dt>Mode</dt>
            <dd>{mode}</dd>
          </div>
          <div>
            <dt>Current</dt>
            <dd>{currentRef}</dd>
          </div>
          <div>
            <dt>Snapshot</dt>
            <dd>{bootstrap.snapshot_url || 'Unavailable'}</dd>
          </div>
          <div>
            <dt>Events</dt>
            <dd>{bootstrap.events_url || 'Unavailable'}</dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
