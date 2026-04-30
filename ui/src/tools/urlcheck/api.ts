const API_BASE = '/api/urlcheck';

export interface URL {
  id: number;
  url: string;
  status: number | null;
  last_check: number | null;
  created_at: number;
  updated_at: number;
}

export interface CheckResult {
  status: number;
  response_time: number;
  error?: string;
}

export interface HistoryEntry {
  id: number;
  status: number;
  response_time: number;
  error?: string;
  checked_at: number;
}

export async function listURLs(): Promise<URL[]> {
  const res = await fetch(`${API_BASE}/urls`);
  if (!res.ok) throw new Error('Failed to list URLs');
  return res.json();
}

export async function addURL(url: string): Promise<{ id: number }> {
  const res = await fetch(`${API_BASE}/urls`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url }),
  });
  if (!res.ok) throw new Error('Failed to add URL');
  return res.json();
}

export async function deleteURL(id: number): Promise<void> {
  const res = await fetch(`${API_BASE}/urls/${id}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete URL');
}

export async function checkURL(id: number): Promise<CheckResult> {
  const res = await fetch(`${API_BASE}/check/${id}`);
  if (!res.ok) throw new Error('Failed to check URL');
  return res.json();
}

export async function getHistory(id: number): Promise<HistoryEntry[]> {
  const res = await fetch(`${API_BASE}/history/${id}`);
  if (!res.ok) throw new Error('Failed to get history');
  return res.json();
}
