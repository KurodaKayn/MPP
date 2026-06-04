type TimerHandle = ReturnType<typeof setInterval>;

export interface WebAuthTokenSyncOptions {
  readToken: () => string | null;
  persistToken: (token: string) => Promise<unknown>;
  intervalMs?: number;
  maxAttempts?: number;
  onError?: (error: unknown) => void;
}

export function startWebAuthTokenSync(
  options: WebAuthTokenSyncOptions,
): () => void {
  const intervalMs = options.intervalMs ?? 1000;
  const maxAttempts = options.maxAttempts ?? 120;
  let attempts = 0;
  let lastPersistedToken: string | null = null;
  let timer: TimerHandle | null = null;

  function stop(): void {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  function syncToken(): void {
    attempts += 1;

    const token = options.readToken();

    if (token && token !== lastPersistedToken) {
      lastPersistedToken = token;
      void options.persistToken(token).catch((error) => {
        options.onError?.(error);
      });
    }

    if (attempts >= maxAttempts) {
      stop();
    }
  }

  syncToken();
  timer = setInterval(syncToken, intervalMs);

  return stop;
}
