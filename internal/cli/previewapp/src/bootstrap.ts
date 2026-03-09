export interface PreviewBootstrap {
  mode?: string;
  cr_id?: number;
  selected_cr_id?: number;
  snapshot_url?: string;
  snapshot_root?: string;
  events_url?: string;
  events_root?: string;
  close_url?: string;
  delegate_launch_url?: string;
}

export function readBootstrap(): PreviewBootstrap {
  const node = document.getElementById('cr-preview-bootstrap');
  if (!node) {
    return {};
  }
  try {
    const raw = node.textContent || '{}';
    const parsed = JSON.parse(raw);
    return typeof parsed === 'object' && parsed !== null ? parsed as PreviewBootstrap : {};
  } catch {
    return {};
  }
}
