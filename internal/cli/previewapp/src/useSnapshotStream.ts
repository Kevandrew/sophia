import { useEffect, useEffectEvent, useState } from 'react';

interface SnapshotStreamState<T> {
  data: T | null;
  connected: boolean;
  loading: boolean;
  refreshing: boolean;
  error: string;
}

export function useSnapshotStream<T>(snapshotURL: string, eventsURL: string) {
  const [state, setState] = useState<SnapshotStreamState<T>>({
    data: null,
    connected: false,
    loading: true,
    refreshing: false,
    error: '',
  });

  const applySnapshot = useEffectEvent((payload: T) => {
    setState((current) => ({
      ...current,
      data: payload,
      loading: false,
      refreshing: false,
      error: '',
    }));
  });

  const markConnected = useEffectEvent((connected: boolean) => {
    setState((current) => ({ ...current, connected }));
  });

  const recordError = useEffectEvent((message: string) => {
    setState((current) => ({
      ...current,
      loading: false,
      refreshing: false,
      error: message || current.error,
    }));
  });

  useEffect(() => {
    let source: EventSource | null = null;
    let disposed = false;
    const controller = new AbortController();

    setState((current) => ({
      ...current,
      loading: current.data === null,
      refreshing: current.data !== null,
      connected: false,
      error: '',
    }));

    void fetch(snapshotURL, { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`snapshot request failed with status ${response.status}`);
        }
        const payload = (await response.json()) as T;
        if (!disposed) {
          applySnapshot(payload);
        }
      })
      .catch((error: unknown) => {
        if (disposed || (error instanceof DOMException && error.name === 'AbortError')) {
          return;
        }
        recordError(error instanceof Error ? error.message : 'snapshot request failed');
      });

    source = new EventSource(eventsURL);
    source.onopen = () => {
      if (!disposed) {
        markConnected(true);
      }
    };
    source.onerror = () => {
      if (!disposed) {
        markConnected(false);
      }
    };
    source.addEventListener('snapshot', (event) => {
      try {
        applySnapshot(JSON.parse((event as MessageEvent<string>).data) as T);
        markConnected(true);
      } catch {
        recordError('failed to decode live snapshot');
      }
    });
    source.addEventListener('error', (event) => {
      const raw = (event as MessageEvent<string>).data;
      if (!raw) {
        return;
      }
      try {
        const payload = JSON.parse(raw) as { message?: string };
        if (payload.message) {
          recordError(payload.message);
        }
      } catch {
        recordError('preview stream reported an error');
      }
    });

    return () => {
      disposed = true;
      controller.abort();
      source?.close();
    };
  }, [applySnapshot, eventsURL, markConnected, recordError, snapshotURL]);

  return state;
}
