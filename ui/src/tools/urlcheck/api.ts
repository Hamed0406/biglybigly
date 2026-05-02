/**
 * URL Monitor API client.
 *
 * Typed fetch wrappers for `/api/urlcheck/*` — CRUD on monitored URLs,
 * on-demand HTTP HEAD checks, and per-URL check history.
 */
const API_BASE = '/api/urlcheck';

/** A monitored URL plus its last observed status. */
export interface URL {
  id: number;
  url: string;
  status: number | null;
  last_check: number | null;
  created_at: number;
  updated_at: number;
}

/** Result of an on-demand check (returned by `checkURL`). */
export interface CheckResult {
  status: number;
  response_time: number;
  error?: string;
}

/** A single historical check record. */
export interface HistoryEntry {
  id: number;
  status: number;
  response_time: number;
  error?: string;
  checked_at: number;
}

/** Lists every monitored URL. */
export async function listURLs(): Promise<URL[]> {
  const res = await fetch(`${API_BASE}/urls`);
  if (!res.ok) throw new Error('Failed to list URLs');
  return res.json();
}

/** Adds a URL to the monitored set. */
export async function addURL(url: string): Promise<{ id: number }> {
  const res = await fetch(`${API_BASE}/urls`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  });
  if (!res.ok) throw new Error('Failed to add URL');
  return res.json();
}

/** Removes a monitored URL by id. */
export async function deleteURL(id: number): Promise<void> {
  const res = await fetch(`${API_BASE}/urls/${id}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete URL');
}

/** Triggers an immediate availability check and returns the result. */
export async function checkURL(id: number): Promise<CheckResult> {
  const res = await fetch(`${API_BASE}/check/${id}`);
  if (!res.ok) throw new Error('Failed to check URL');
  return res.json();
}

/** Fetches the historical check log for a URL. */
export async function getHistory(id: number): Promise<HistoryEntry[]> {
  const res = await fetch(`${API_BASE}/history/${id}`);
  if (!res.ok) throw new Error('Failed to get history');
  return res.json();
}
