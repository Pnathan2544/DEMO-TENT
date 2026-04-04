/**
 * Server-side API helper. Never import this in Client Components.
 * Fetches the Go API directly (no rewrite hop) with the forwarded cookie header.
 */
const API = process.env.API_URL!;

export async function apiFetch(
  path: string,
  cookieHeader: string,
  init?: RequestInit
): Promise<Response> {
  return fetch(`${API}/api/v1${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
      cookie: cookieHeader,
    },
    cache: "no-store",
  });
}
