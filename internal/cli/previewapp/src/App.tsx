import { startTransition, useDeferredValue, useEffect, useMemo, useState } from 'react';
import type { PreviewBootstrap } from './bootstrap';
import type {
  CRSnapshot,
  DashboardCRRow,
  DashboardSnapshot,
  DashboardTimelineEvent,
  DelegationLaunch,
  PreviewRoute,
  PreviewTask,
  RecentCheckpoint,
  StackLineageNode,
  StackNativity,
} from './types';
import { buildEventsURL, buildSnapshotURL, readRouteState, writeDashboardRoute } from './urlState';
import { useSnapshotStream } from './useSnapshotStream';

export function App({ bootstrap }: { bootstrap: PreviewBootstrap }) {
  const [route, setRoute] = useState<PreviewRoute>(() => readRouteState(bootstrap));
  const [searchInput, setSearchInput] = useState(() =>
    route.kind === 'dashboard' ? route.text : '',
  );
  const deferredSearch = useDeferredValue(searchInput);

  useEffect(() => {
    const handlePopstate = () => {
      const nextRoute = readRouteState(bootstrap);
      startTransition(() => {
        setRoute(nextRoute);
        if (nextRoute.kind === 'dashboard') {
          setSearchInput(nextRoute.text);
        }
      });
    };
    window.addEventListener('popstate', handlePopstate);
    return () => window.removeEventListener('popstate', handlePopstate);
  }, [bootstrap]);

  useEffect(() => {
    if (route.kind !== 'dashboard' || deferredSearch === route.text) {
      return;
    }
    const nextRoute = { ...route, text: deferredSearch };
    startTransition(() => setRoute(nextRoute));
    writeDashboardRoute(nextRoute);
  }, [deferredSearch, route]);

  const snapshotURL = useMemo(() => buildSnapshotURL(bootstrap, route), [bootstrap, route]);
  const eventsURL = useMemo(() => buildEventsURL(bootstrap, route), [bootstrap, route]);
  const stream = useSnapshotStream<DashboardSnapshot | CRSnapshot>(snapshotURL, eventsURL);

  useEffect(() => {
    if (route.kind !== 'dashboard' || !isDashboardSnapshot(stream.data)) {
      return;
    }
    const selectedID = normalizePositiveInt(stream.data.dashboard.selected_cr_id);
    if (selectedID === route.selected_cr_id) {
      return;
    }
    const nextRoute = { ...route, selected_cr_id: selectedID };
    startTransition(() => setRoute(nextRoute));
    writeDashboardRoute(nextRoute);
  }, [route, stream.data]);

  if (route.kind === 'dashboard') {
    return (
      <DashboardView
        connected={stream.connected}
        error={stream.error}
        payload={isDashboardSnapshot(stream.data) ? stream.data : null}
        refreshing={stream.refreshing}
        route={route}
        searchInput={searchInput}
        setSearchInput={setSearchInput}
        setRoute={(nextRoute) => {
          startTransition(() => setRoute(nextRoute));
          writeDashboardRoute(nextRoute);
        }}
      />
    );
  }

  return (
    <CRDetailView
      bootstrap={bootstrap}
      connected={stream.connected}
      error={stream.error}
      payload={isCRSnapshot(stream.data) ? stream.data : null}
      refreshing={stream.refreshing}
      route={route}
    />
  );
}

function DashboardView({
  connected,
  error,
  payload,
  refreshing,
  route,
  searchInput,
  setSearchInput,
  setRoute,
}: {
  connected: boolean;
  error: string;
  payload: DashboardSnapshot | null;
  refreshing: boolean;
  route: Extract<PreviewRoute, { kind: 'dashboard' }>;
  searchInput: string;
  setSearchInput: (value: string) => void;
  setRoute: (route: Extract<PreviewRoute, { kind: 'dashboard' }>) => void;
}) {
  const selected = payload?.selected_cr ?? null;

  return (
    <div className="preview-app-shell app-frame">
      <div className="app-statusbar">
        <StatusPill connected={connected} refreshing={refreshing} />
        <div className="status-copy">
          <strong>Dashboard</strong>
          <span>
            {payload ? `Generated ${formatTimestamp(payload.generated_at)}` : 'Waiting for dashboard snapshot'}
          </span>
        </div>
        {payload ? (
          <div className="status-metrics">
            <MetricChip label="Visible" value={`${payload.dashboard.counts.list_returned}/${payload.dashboard.counts.list_total}`} />
            <MetricChip label="Timeline" value={`${payload.dashboard.counts.timeline_returned}/${payload.dashboard.counts.timeline_total}`} />
          </div>
        ) : null}
      </div>

      {error ? <InlineBanner tone="error" message={error} /> : null}

      <div className="dashboard-grid">
        <aside className="dashboard-sidebar">
          <section className="panel filter-panel">
            <div className="section-heading">
              <p className="eyebrow">Filters</p>
              <h2>Live CR query</h2>
            </div>
            <div className="filter-grid">
              <label className="field">
                <span>Status</span>
                <select
                  value={route.status}
                  onChange={(event) => setRoute({ ...route, status: event.target.value })}
                >
                  <option value="">All</option>
                  <option value="open">Open</option>
                  <option value="in_progress">In Progress</option>
                  <option value="merged">Merged</option>
                  <option value="abandoned">Abandoned</option>
                </select>
              </label>
              <label className="field">
                <span>Risk Tier</span>
                <select
                  value={route.risk_tier}
                  onChange={(event) => setRoute({ ...route, risk_tier: event.target.value })}
                >
                  <option value="">All</option>
                  <option value="low">Low</option>
                  <option value="medium">Medium</option>
                  <option value="high">High</option>
                </select>
              </label>
              <label className="field field-span">
                <span>Scope Prefix</span>
                <input
                  type="text"
                  value={route.scope}
                  placeholder="internal/cli"
                  onChange={(event) => setRoute({ ...route, scope: event.target.value })}
                />
              </label>
              <label className="field field-span">
                <span>Search</span>
                <input
                  type="search"
                  value={searchInput}
                  placeholder="Search title, branch, or description"
                  onChange={(event) => setSearchInput(event.target.value)}
                />
              </label>
            </div>
          </section>

          <section className="panel list-panel">
            <div className="section-heading">
              <p className="eyebrow">CR Index</p>
              <h2>Matching change requests</h2>
            </div>
            {!payload ? <LoadingCard label="Loading dashboard snapshot" /> : null}
            {payload && payload.crs.length === 0 ? (
              <EmptyState
                title="No CRs match this query"
                body="Adjust the filters or search text to widen the snapshot request."
              />
            ) : null}
            <div className="cr-list">
              {payload?.crs.map((row) => (
                <CRRow
                  key={row.id}
                  row={row}
                  selected={payload.dashboard.selected_cr_id === row.id}
                  onSelect={() => setRoute({ ...route, selected_cr_id: row.id })}
                />
              ))}
            </div>
          </section>
        </aside>

        <section className="dashboard-detail">
          {!payload ? <LoadingCard label="Waiting for selected CR summary" /> : null}
          {payload && selected ? (
            <>
              <section className="panel detail-panel hero-panel">
                <div className="hero-header">
                  <div>
                    <p className="eyebrow">Selected</p>
                    <h1>{selected.title}</h1>
                    <p className="hero-copy">
                      {selected.description || selected.contract_why || 'No CR description recorded.'}
                    </p>
                  </div>
                  <a className="action-link" href={`/${selected.id}`}>
                    Open full detail
                  </a>
                </div>

                <div className="detail-pill-row">
                  <StatusBadge status={selected.lifecycle_state || selected.status} />
                  <DetailPill label="Branch" tone="neutral" value={selected.branch} />
                  <DetailPill
                    label="Base"
                    tone="neutral"
                    value={selected.base_branch || selected.base_ref || 'unknown'}
                  />
                  <DetailPill
                    label="Risk"
                    tone={riskTone(selected.risk_tier)}
                    value={formatEnum(selected.risk_tier || 'unknown')}
                  />
                </div>

                <div className="summary-grid">
                  <SummaryCard label="Intent" value={selected.contract_why || 'No CR why recorded.'} />
                  <SummaryCard
                    label="Scope"
                    value={
                      selected.contract_scope.length > 0
                        ? selected.contract_scope.join(', ')
                        : 'No explicit contract scope.'
                    }
                  />
                  <SummaryCard
                    label="Stack Context"
                    value={describeStack(selected.stack_nativity, selected.stack_lineage)}
                  />
                  <SummaryCard
                    label="Next Action"
                    value={
                      selected.action_reason ||
                      selected.action_required ||
                      'No active workflow blocker for the selected CR.'
                    }
                  />
                </div>

                {selected.suggested_commands?.length ? (
                  <div className="command-strip">
                    {selected.suggested_commands.map((command) => (
                      <code key={command}>{command}</code>
                    ))}
                  </div>
                ) : null}
              </section>

              <section className="panel timeline-panel">
                <div className="section-heading">
                  <p className="eyebrow">Activity</p>
                  <h2>Live filtered timeline</h2>
                </div>
                <div className="timeline-list">
                  {payload.timeline.length === 0 ? (
                    <EmptyState
                      title="No recent events"
                      body="The current dashboard snapshot did not return timeline events."
                    />
                  ) : (
                    payload.timeline.map((event) => (
                      <TimelineEntry
                        key={`${event.cr_id}:${event.ts}:${event.type}:${event.summary}`}
                        event={event}
                        highlighted={event.cr_id === selected.id}
                      />
                    ))
                  )}
                </div>
              </section>
            </>
          ) : null}
        </section>
      </div>
    </div>
  );
}

function CRDetailView({
  bootstrap,
  connected,
  error,
  payload,
  refreshing,
  route,
}: {
  bootstrap: PreviewBootstrap;
  connected: boolean;
  error: string;
  payload: CRSnapshot | null;
  refreshing: boolean;
  route: Extract<PreviewRoute, { kind: 'cr' }>;
}) {
  const [selectedTaskIDs, setSelectedTaskIDs] = useState<number[]>([]);
  const [launchMessage, setLaunchMessage] = useState<{ tone: 'neutral' | 'error'; text: string }>({
    tone: 'neutral',
    text: '',
  });
  const [launchPending, setLaunchPending] = useState(false);

  useEffect(() => {
    if (!payload?.tasks?.length) {
      setSelectedTaskIDs([]);
      return;
    }
    const allowed = new Set(payload.tasks.map((task) => task.id));
    setSelectedTaskIDs((current) => {
      const kept = current.filter((id) => allowed.has(id));
      if (kept.length > 0) {
        return kept;
      }
      return (payload.delegation_launch?.default_task_ids || []).filter((id) => allowed.has(id));
    });
  }, [payload]);

  const launchDelegation = async () => {
    if (!payload?.cr || !bootstrap.delegate_launch_url || launchPending) {
      return;
    }
    setLaunchPending(true);
    setLaunchMessage({ tone: 'neutral', text: 'Launching delegation…' });
    try {
      const response = await fetch(bootstrap.delegate_launch_url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          cr_id: payload.cr.id,
          task_ids: selectedTaskIDs,
        }),
      });
      const body = (await response.json()) as {
        ok?: boolean;
        data?: { run?: { status?: string } };
        error?: { message?: string };
      };
      if (!response.ok || body.ok === false) {
        throw new Error(body.error?.message || `launch failed with status ${response.status}`);
      }
      setLaunchMessage({
        tone: 'neutral',
        text: `Delegation accepted${body.data?.run?.status ? ` (${body.data.run.status})` : ''}.`,
      });
    } catch (launchError) {
      setLaunchMessage({
        tone: 'error',
        text: launchError instanceof Error ? launchError.message : 'delegation launch failed',
      });
    } finally {
      setLaunchPending(false);
    }
  };

  return (
    <div className="preview-app-shell app-frame">
      <div className="app-statusbar">
        <StatusPill connected={connected} refreshing={refreshing} />
        <div className="status-copy">
          <strong>CR-{route.cr_id}</strong>
          <span>{payload ? `Generated ${formatTimestamp(payload.generated_at)}` : 'Waiting for CR snapshot'}</span>
        </div>
        <a className="action-link secondary" href={`/?selected_cr_id=${route.cr_id}`}>
          Back to dashboard
        </a>
      </div>

      {error ? <InlineBanner tone="error" message={error} /> : null}
      {!payload ? <LoadingCard label={`Loading CR-${route.cr_id} detail`} /> : null}

      {payload ? (
        <div className="detail-layout">
          <section className="panel detail-panel hero-panel">
            {payload.stack_nativity?.is_child && payload.stack_nativity.parent_cr_id ? (
              <a className="context-banner" href={`/${payload.stack_nativity.parent_cr_id}`}>
                <span className="mono-label">Parent</span>
                <strong>CR-{payload.stack_nativity.parent_cr_id}</strong>
                <span>{payload.stack_nativity.parent_title || 'Parent change request'}</span>
              </a>
            ) : null}

            <div className="hero-header">
              <div>
                <p className="eyebrow">Per-CR Detail</p>
                <h1>{payload.cr.title}</h1>
                <p className="hero-copy">{payload.cr.description || 'No CR description recorded.'}</p>
              </div>
              <button
                className="action-link"
                disabled={!payload.delegation_launch?.available || launchPending}
                onClick={launchDelegation}
                type="button"
              >
                {launchPending ? 'Launching…' : 'Launch delegation'}
              </button>
            </div>

            <div className="detail-pill-row">
              <StatusBadge status={payload.cr.lifecycle_state || payload.cr.status} />
              <DetailPill label="Branch" tone="neutral" value={payload.cr.branch} />
              <DetailPill
                label="Base"
                tone="neutral"
                value={payload.cr.base_branch || payload.cr.base_ref || 'unknown'}
              />
              <DetailPill
                label="Risk"
                tone={riskTone(payload.trust?.risk_tier || payload.contract?.risk_tier_hint)}
                value={formatEnum(payload.trust?.risk_tier || payload.contract?.risk_tier_hint || 'unknown')}
              />
              {payload.stack_nativity?.role_label ? (
                <DetailPill label="Role" tone="accent" value={payload.stack_nativity.role_label} />
              ) : null}
            </div>

            <div className="summary-grid">
              <SummaryCard label="Why" value={payload.contract?.why || 'No CR why recorded.'} />
              <SummaryCard
                label="Scope"
                value={
                  payload.contract?.scope?.length
                    ? payload.contract.scope.join(', ')
                    : 'No explicit contract scope.'
                }
              />
              <SummaryCard
                label="Workflow"
                value={
                  payload.status?.action_reason ||
                  payload.status?.action_required ||
                  'No active workflow blocker.'
                }
              />
              <SummaryCard
                label="Lineage"
                value={describeStack(payload.stack_nativity, payload.stack_lineage)}
              />
            </div>

            {launchMessage.text ? <InlineBanner message={launchMessage.text} tone={launchMessage.tone} /> : null}
            {!payload.delegation_launch?.available && payload.delegation_launch?.reason ? (
              <InlineBanner message={payload.delegation_launch.reason} tone="neutral" />
            ) : null}
          </section>

          <div className="detail-columns">
            <section className="panel detail-panel">
              <div className="section-heading">
                <p className="eyebrow">Tasks</p>
                <h2>Checkpoint and delegation surface</h2>
              </div>
              {!payload.tasks.length ? (
                <EmptyState
                  title="No tasks recorded"
                  body="This CR does not currently expose task-level checkpoints or delegation targets."
                />
              ) : (
                <div className="task-list">
                  {payload.tasks.map((task) => (
                    <TaskCard
                      key={task.id}
                      launch={payload.delegation_launch}
                      onToggle={(checked) =>
                        setSelectedTaskIDs((current) => {
                          if (checked) {
                            return current.includes(task.id) ? current : [...current, task.id];
                          }
                          return current.filter((id) => id !== task.id);
                        })
                      }
                      selected={selectedTaskIDs.includes(task.id)}
                      task={task}
                    />
                  ))}
                </div>
              )}
            </section>

            <section className="panel detail-panel">
              <div className="section-heading">
                <p className="eyebrow">Trust + Validation</p>
                <h2>Gate posture</h2>
              </div>
              <div className="summary-grid">
                <SummaryCard
                  label="Trust Verdict"
                  tone={trustTone(payload.trust?.verdict)}
                  value={formatEnum(payload.trust?.verdict || 'unknown')}
                />
                <SummaryCard
                  label="Validation"
                  tone={payload.validation?.valid ? 'ok' : 'error'}
                  value={payload.validation?.valid ? 'Passing' : 'Failing'}
                />
                <SummaryCard
                  label="Required Actions"
                  value={
                    payload.trust?.required_actions?.length
                      ? payload.trust.required_actions.join(' • ')
                      : 'No required actions.'
                  }
                />
                <SummaryCard
                  label="Advisories"
                  value={
                    payload.trust?.advisories?.length
                      ? payload.trust.advisories.join(' • ')
                      : 'No advisories.'
                  }
                />
              </div>
              {payload.validation?.errors?.length ? (
                <ul className="issue-list error">
                  {payload.validation.errors.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              ) : null}
              {payload.validation?.warnings?.length ? (
                <ul className="issue-list warn">
                  {payload.validation.warnings.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              ) : null}
            </section>
          </div>

          <div className="detail-columns">
            <section className="panel detail-panel">
              <div className="section-heading">
                <p className="eyebrow">Recent Events</p>
                <h2>Live CR timeline</h2>
              </div>
              <div className="timeline-list">
                {!payload.recent_events.length ? (
                  <EmptyState
                    title="No recent events"
                    body="The current CR snapshot did not return timeline activity."
                  />
                ) : (
                  payload.recent_events.map((event) => (
                    <TimelineEntry
                      key={`${event.ts}:${event.type}:${event.summary}`}
                      event={{
                        actor: event.actor,
                        cr_id: payload.cr.id,
                        cr_status: payload.cr.status,
                        cr_title: payload.cr.title,
                        ref: event.ref,
                        redacted: false,
                        summary: event.summary,
                        ts: event.ts,
                        type: event.type,
                        cr_uid: payload.cr.uid,
                      }}
                      highlighted
                    />
                  ))
                )}
              </div>
            </section>

            <section className="panel detail-panel">
              <div className="section-heading">
                <p className="eyebrow">Checkpoints</p>
                <h2>Recent checkpoint history</h2>
              </div>
              {!payload.recent_checkpoints.length ? (
                <EmptyState
                  title="No checkpoints recorded"
                  body="Task checkpoints will appear here as commits are captured against this CR."
                />
              ) : (
                <div className="checkpoint-list">
                  {payload.recent_checkpoints.map((checkpoint) => (
                    <CheckpointCard key={`${checkpoint.task_id}:${checkpoint.at}:${checkpoint.commit}`} checkpoint={checkpoint} />
                  ))}
                </div>
              )}
              {payload.files_changed?.length ? (
                <div className="command-strip">
                  {payload.files_changed.map((file) => (
                    <code key={file}>{file}</code>
                  ))}
                </div>
              ) : null}
            </section>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function CRRow({
  onSelect,
  row,
  selected,
}: {
  onSelect: () => void;
  row: DashboardCRRow;
  selected: boolean;
}) {
  return (
    <article className={`cr-row${selected ? ' selected' : ''}`} onClick={onSelect}>
      <div className="row-topline">
        <span className="mono-label">CR-{row.id}</span>
        <StatusBadge status={row.status} />
      </div>
      <h3>{row.title}</h3>
      <p>{row.description || row.contract_why || 'No description recorded.'}</p>
      <div className="row-meta">
        <span>{row.branch}</span>
        <span>{formatRelativeTimestamp(row.updated_at)}</span>
      </div>
      <div className="row-footer">
        <DetailPill label="Risk" tone={riskTone(row.risk_tier)} value={formatEnum(row.risk_tier || 'unknown')} />
        <DetailPill label="Tasks" tone={row.tasks.open ? 'warn' : 'neutral'} value={`${row.tasks.done}/${row.tasks.total}`} />
        {row.stack_nativity?.child_count ? (
          <DetailPill
            label="Children"
            tone={row.stack_nativity.pending_child_count ? 'accent' : 'neutral'}
            value={row.stack_nativity.child_count}
          />
        ) : null}
      </div>
    </article>
  );
}

function TaskCard({
  launch,
  onToggle,
  selected,
  task,
}: {
  launch?: DelegationLaunch;
  onToggle: (checked: boolean) => void;
  selected: boolean;
  task: PreviewTask;
}) {
  const selectable = launch?.available && task.status !== 'done';
  return (
    <article className="task-card">
      <div className="task-header">
        <div>
          <span className="mono-label">Task {task.id}</span>
          <h3>{task.title}</h3>
        </div>
        {selectable ? (
          <label className="checkbox-chip">
            <input checked={selected} onChange={(event) => onToggle(event.target.checked)} type="checkbox" />
            <span>Select</span>
          </label>
        ) : (
          <StatusBadge status={task.status} />
        )}
      </div>
      {task.contract?.intent ? <p>{task.contract.intent}</p> : null}
      {task.contract?.acceptance_criteria?.length ? (
        <ul className="mini-list">
          {task.contract.acceptance_criteria.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      ) : null}
      {task.checkpoint_commit ? (
        <div className="task-meta">
          <code>{task.checkpoint_commit.slice(0, 8)}</code>
          <span>{formatTimestamp(task.checkpoint_at)}</span>
        </div>
      ) : null}
    </article>
  );
}

function TimelineEntry({
  event,
  highlighted,
}: {
  event: DashboardTimelineEvent;
  highlighted: boolean;
}) {
  return (
    <article className={`timeline-row${highlighted ? ' highlighted' : ''}`}>
      <div className="timeline-marker" />
      <div className="timeline-body">
        <div className="row-topline">
          <span className="mono-label">
            CR-{event.cr_id} · {formatEnum(event.type)}
          </span>
          <span className="timeline-ts">{formatTimestamp(event.ts)}</span>
        </div>
        <h3>{event.summary}</h3>
        <p>
          {event.actor || 'unknown actor'}
          {event.ref ? ` · ${event.ref}` : ''}
          {event.redacted ? ' · redacted' : ''}
        </p>
      </div>
    </article>
  );
}

function CheckpointCard({ checkpoint }: { checkpoint: RecentCheckpoint }) {
  return (
    <article className="checkpoint-card">
      <div className="row-topline">
        <span className="mono-label">Task {checkpoint.task_id}</span>
        <StatusBadge status={checkpoint.status} />
      </div>
      <h3>{checkpoint.title}</h3>
      <p>{checkpoint.message || checkpoint.reason || 'Checkpoint captured without a message.'}</p>
      <div className="task-meta">
        <code>{checkpoint.commit ? checkpoint.commit.slice(0, 8) : 'no-commit'}</code>
        <span>{formatTimestamp(checkpoint.at)}</span>
      </div>
    </article>
  );
}

function StatusPill({ connected, refreshing }: { connected: boolean; refreshing: boolean }) {
  const tone = !connected ? 'error' : refreshing ? 'warn' : 'ok';
  const label = !connected ? 'Reconnecting' : refreshing ? 'Refreshing' : 'Live';
  return <span className={`status-pill ${tone}`}>{label}</span>;
}

function StatusBadge({ status }: { status: string }) {
  return <span className={`status-badge ${statusTone(status)}`}>{formatEnum(status)}</span>;
}

function MetricChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="metric-chip">
      <strong>{value}</strong>
      <span>{label}</span>
    </span>
  );
}

function DetailPill({
  label,
  tone,
  value,
}: {
  label: string;
  tone: 'neutral' | 'warn' | 'ok' | 'error' | 'accent';
  value: number | string;
}) {
  return (
    <span className={`detail-pill ${tone}`}>
      <strong>{label}</strong>
      <span>{String(value)}</span>
    </span>
  );
}

function SummaryCard({
  label,
  tone = 'neutral',
  value,
}: {
  label: string;
  tone?: 'neutral' | 'warn' | 'ok' | 'error';
  value: string;
}) {
  return (
    <article className={`summary-card ${tone}`}>
      <span>{label}</span>
      <p>{value}</p>
    </article>
  );
}

function InlineBanner({
  message,
  tone,
}: {
  message: string;
  tone: 'neutral' | 'error';
}) {
  return <div className={`inline-banner ${tone}`}>{message}</div>;
}

function LoadingCard({ label }: { label: string }) {
  return (
    <section className="panel detail-panel loading-panel">
      <p className="eyebrow">Loading</p>
      <h2>{label}</h2>
      <p className="hero-copy">Fetching the latest snapshot and attaching the live SSE stream.</p>
    </section>
  );
}

function EmptyState({ body, title }: { body: string; title: string }) {
  return (
    <article className="empty-state">
      <h3>{title}</h3>
      <p>{body}</p>
    </article>
  );
}

function describeStack(nativity?: StackNativity, lineage?: StackLineageNode[]) {
  const parts: string[] = [];
  if (nativity?.role_label) {
    parts.push(nativity.role_label);
  }
  if (nativity?.is_child && nativity.parent_cr_id) {
    parts.push(`Child of CR-${nativity.parent_cr_id}`);
  }
  if (nativity?.is_root_parent && nativity.child_count) {
    parts.push(`Root parent with ${nativity.child_count} children`);
  }
  if (lineage?.length) {
    parts.push(lineage.map((node) => `CR-${node.id}`).join(' → '));
  }
  return parts.join(' • ') || 'Standalone change request.';
}

function normalizePositiveInt(value?: number | null) {
  return typeof value === 'number' && value > 0 ? value : undefined;
}

function statusTone(status: string) {
  switch (status) {
    case 'merged':
    case 'done':
    case 'trusted':
      return 'ok';
    case 'open':
    case 'in_progress':
    case 'needs_attention':
      return 'warn';
    case 'abandoned':
    case 'failed':
    case 'untrusted':
      return 'error';
    default:
      return 'neutral';
  }
}

function trustTone(status?: string) {
  return statusTone(status || '');
}

function riskTone(risk?: string) {
  switch (risk) {
    case 'high':
      return 'error';
    case 'medium':
      return 'warn';
    case 'low':
      return 'ok';
    default:
      return 'neutral';
  }
}

function formatEnum(value: string) {
  return value
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (match) => match.toUpperCase());
}

function formatTimestamp(raw?: string) {
  if (!raw) {
    return 'Unknown time';
  }
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return raw;
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(parsed);
}

function formatRelativeTimestamp(raw?: string) {
  if (!raw) {
    return 'Unknown update';
  }
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return raw;
  }
  const diffMinutes = Math.round((Date.now() - parsed.getTime()) / 60000);
  if (diffMinutes < 1) {
    return 'just now';
  }
  if (diffMinutes < 60) {
    return `${diffMinutes}m ago`;
  }
  if (diffMinutes < 24 * 60) {
    return `${Math.round(diffMinutes / 60)}h ago`;
  }
  return `${Math.round(diffMinutes / (24 * 60))}d ago`;
}

function isDashboardSnapshot(value: DashboardSnapshot | CRSnapshot | null): value is DashboardSnapshot {
  return Boolean(value && 'dashboard' in value);
}

function isCRSnapshot(value: DashboardSnapshot | CRSnapshot | null): value is CRSnapshot {
  return Boolean(value && 'cr' in value && 'tasks' in value);
}
