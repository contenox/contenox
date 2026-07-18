import i18n from '../i18n';

/**
 * API base URL for `apiFetch`.
 * - Prefer `VITE_API_BASE_URL` when set (e.g. pointing at a remote API).
 * - In the browser, always use `window.location.origin` so `/api` stays same-origin:
 *   Vite dev (`make dev-beam`) proxies `/api` to the local server and auth cookies apply.
 * - Falling back to a separate origin while the app runs on :5173 breaks cookies and yields 403
 *   on protected routes (previous bug when env vars failed to load).
 */
function resolveApiOrigin(): string {
  const explicit = import.meta.env.VITE_API_BASE_URL;
  if (explicit !== undefined && explicit !== '') {
    return explicit;
  }
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin;
  }
  return 'http://localhost:32123';
}

const API_BASE_URL = resolveApiOrigin();
const envTimeout = import.meta.env.VITE_API_TIMEOUT;
const parsedTimeout = envTimeout ? parseInt(envTimeout, 10) : NaN;
const API_TIMEOUT = !isNaN(parsedTimeout) ? parsedTimeout : 100000;

// Browser auth is pure-cookie BFF: the server's login flow sets an HttpOnly
// `auth_token` session cookie that the browser cannot read, and every request
// below uses `credentials: 'same-origin'` so that cookie rides along. The UI
// never handles the token in JS — no localStorage, no Authorization header. A
// raw-token `Authorization: Bearer` remains a server-side option for
// programmatic/remote clients, but is deliberately never produced here.

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

export type ApiFetchOptions = RequestInit & {
  /**
   * Client-side abort timer in milliseconds.
   * - Omit or pass undefined: use the global API_TIMEOUT default.
   * - Pass null: no timer — only the caller's AbortSignal can cancel the request.
   *   Use this for long-running requests (e.g. agentic chat) where the browser
   *   tab lifetime is the natural timeout and the user has a Stop button.
   */
  timeoutMs?: number | null;
};

// --- API Fetch Helper ---
export async function apiFetch<T>(url: string, options?: ApiFetchOptions): Promise<T> {
  const externalSignal = options?.signal ?? null;
  const controller = new AbortController();

  // null → no timer; undefined → global default; number → caller-supplied value.
  const timeout = options?.timeoutMs === null ? null : (options?.timeoutMs ?? API_TIMEOUT);
  let timedOut = false;
  const timeoutId =
    timeout !== null
      ? setTimeout(() => {
          timedOut = true;
          controller.abort();
        }, timeout)
      : null;

  // If the caller's signal aborts, abort ours too.
  if (externalSignal) {
    if (externalSignal.aborted) controller.abort();
    else externalSignal.addEventListener('abort', () => controller.abort(), { once: true });
  }

  try {
    const headers = new Headers(options?.headers);
    const isFormData = options?.body instanceof FormData;

    if (!isFormData && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json');
    }

    headers.set('Accept-Language', i18n.language);

    const response = await fetch(new URL(url, API_BASE_URL).toString(), {
      credentials: 'same-origin',
      ...options,
      headers,
      signal: controller.signal,
    });
    if (timeoutId !== null) clearTimeout(timeoutId);

    if (!response.ok) {
      let errorMessage = i18n.t('errors.unknown');
      let errorDetails = null;
      const contentType = response.headers.get('Content-Type');

      try {
        if (contentType?.includes('application/json')) {
          const errorBody: unknown = await response.json();
          const apiError = isRecord(errorBody) ? errorBody.error : undefined;

          if (isRecord(apiError)) {
            errorMessage =
              typeof apiError.message === 'string' ? apiError.message : i18n.t('errors.unknown');
            errorDetails = {
              type: apiError.type,
              code: apiError.code,
              param: apiError.param,
              raw: errorBody,
            };
          } else {
            errorMessage =
              isRecord(errorBody) && typeof errorBody.message === 'string'
                ? errorBody.message
                : JSON.stringify(errorBody) || i18n.t('errors.unknown');
            errorDetails = { raw: errorBody };
          }
        } else {
          errorMessage = await response.text();
        }
      } catch {
        errorMessage = response.statusText || errorMessage;
      }

      throw new ApiError(errorMessage, response.status, errorDetails ?? undefined);
    }

    try {
      return await response.json();
    } catch (error) {
      throw new ApiError(i18n.t('errors.invalidResponse'), response.status, { cause: error });
    }
  } catch (error) {
    if (timeoutId !== null) clearTimeout(timeoutId);

    if (error instanceof ApiError) throw error;

    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError(timedOut ? i18n.t('errors.timeout') : i18n.t('errors.cancelled'), 0);
    }

    if (error instanceof Error) {
      throw new ApiError(error.message, 0);
    }

    throw new ApiError(i18n.t('errors.unknown'), 0);
  }
}

/** GET binary body; same origin, timeout, and error handling as apiFetch. */
export async function apiFetchBinary(url: string, init?: RequestInit): Promise<ArrayBuffer> {
  const externalSignal = init?.signal ?? null;
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), API_TIMEOUT);
  if (externalSignal) {
    if (externalSignal.aborted) controller.abort();
    else externalSignal.addEventListener('abort', () => controller.abort(), { once: true });
  }
  try {
    const headers = new Headers(init?.headers);
    headers.set('Accept-Language', i18n.language);
    const response = await fetch(new URL(url, API_BASE_URL).toString(), {
      ...init,
      credentials: 'same-origin',
      headers,
      signal: controller.signal,
    });
    clearTimeout(timeoutId);
    if (!response.ok) {
      let errorMessage = i18n.t('errors.unknown');
      try {
        errorMessage = await response.text();
      } catch {
        errorMessage = response.statusText || errorMessage;
      }
      throw new ApiError(errorMessage, response.status);
    }
    return await response.arrayBuffer();
  } catch (error) {
    clearTimeout(timeoutId);
    if (error instanceof ApiError) throw error;
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw new ApiError(i18n.t('errors.timeout'), 0);
    }
    if (error instanceof Error) {
      throw new ApiError(error.message, 0);
    }
    throw new ApiError(i18n.t('errors.unknown'), 0);
  }
}
