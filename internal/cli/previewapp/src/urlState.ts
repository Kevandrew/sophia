import type { PreviewBootstrap } from './bootstrap';
import type { PreviewRoute } from './types';

export function readRouteState(bootstrap: PreviewBootstrap): PreviewRoute {
  const matchedCR = normalizePath(window.location.pathname).match(/^\/(\d+)$/);
  if (matchedCR) {
    return {
      kind: 'cr',
      cr_id: Number(matchedCR[1]),
    };
  }

  const params = new URLSearchParams(window.location.search);
  return {
    kind: 'dashboard',
    status: params.get('status')?.trim() || '',
    risk_tier: params.get('risk_tier')?.trim() || '',
    scope: params.get('scope')?.trim() || '',
    text: params.get('text')?.trim() || '',
    selected_cr_id:
      parsePositiveInt(params.get('selected_cr_id')) ??
      parsePositiveInt(params.get('selected')) ??
      bootstrap.selected_cr_id,
  };
}

export function writeDashboardRoute(route: Extract<PreviewRoute, { kind: 'dashboard' }>) {
  const params = new URLSearchParams();
  setIfPresent(params, 'status', route.status);
  setIfPresent(params, 'risk_tier', route.risk_tier);
  setIfPresent(params, 'scope', route.scope);
  setIfPresent(params, 'text', route.text);
  if (route.selected_cr_id && route.selected_cr_id > 0) {
    params.set('selected_cr_id', String(route.selected_cr_id));
  }
  const target = params.toString() ? `/?${params.toString()}` : '/';
  window.history.replaceState({}, '', target);
}

export function buildSnapshotURL(bootstrap: PreviewBootstrap, route: PreviewRoute) {
  const root = bootstrap.snapshot_root || stripQuery(bootstrap.snapshot_url) || '/__sophia_snapshot';
  return `${root}?${buildQuery(route).toString()}`;
}

export function buildEventsURL(bootstrap: PreviewBootstrap, route: PreviewRoute) {
  const root = bootstrap.events_root || stripQuery(bootstrap.events_url) || '/__sophia_events';
  return `${root}?${buildQuery(route).toString()}`;
}

function buildQuery(route: PreviewRoute) {
  const params = new URLSearchParams();
  if (route.kind === 'cr') {
    params.set('mode', 'cr');
    params.set('id', String(route.cr_id));
    return params;
  }

  params.set('mode', 'dashboard');
  setIfPresent(params, 'status', route.status);
  setIfPresent(params, 'risk_tier', route.risk_tier);
  setIfPresent(params, 'scope', route.scope);
  setIfPresent(params, 'text', route.text);
  if (route.selected_cr_id && route.selected_cr_id > 0) {
    params.set('selected_cr_id', String(route.selected_cr_id));
  }
  return params;
}

function normalizePath(pathname: string) {
  const trimmed = pathname.replace(/\/+$/, '');
  return trimmed || '/';
}

function setIfPresent(params: URLSearchParams, key: string, value: string) {
  if (value.trim()) {
    params.set(key, value.trim());
  }
}

function parsePositiveInt(raw: string | null) {
  if (!raw) {
    return undefined;
  }
  const value = Number(raw);
  return Number.isInteger(value) && value > 0 ? value : undefined;
}

function stripQuery(raw?: string) {
  if (!raw) {
    return '';
  }
  const [path] = raw.split('?');
  return path;
}
