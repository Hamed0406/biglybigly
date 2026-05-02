/**
 * SysMon API client.
 *
 * Typed fetch wrappers for `/api/sysmon/*` — current snapshots, historical
 * series for sparklines, mounted-disk usage, and the agent index.
 */

/** Most recent CPU/memory/load snapshot reported by an agent. */
export interface SysmonSnapshot {
  id: number;
  agent_name: string;
  cpu_percent: number;
  mem_total: number;
  mem_used: number;
  mem_available: number;
  load1: number;
  load5: number;
  load15: number;
  os_info: string;
  hostname: string;
  uptime_secs: number;
  collected_at: number;
}

/** Per-mountpoint disk usage. */
export interface SysmonDisk {
  agent_name: string;
  mount_point: string;
  fs_type: string;
  total_bytes: number;
  used_bytes: number;
  avail_bytes: number;
}

/** Per-agent activity summary used by the agent picker. */
export interface SysmonAgent {
  name: string;
  snapshot_count: number;
  last_active: number;
}

/** Single point in a historical CPU/memory time series. */
export interface HistoryPoint {
  cpu_percent: number;
  mem_used: number;
  mem_total: number;
  collected_at: number;
}

/** Fetches the latest snapshot for each agent (or just one if specified). */
export async function getSysmonCurrent(agent?: string): Promise<SysmonSnapshot[]> {
  const params = agent ? `?agent=${encodeURIComponent(agent)}` : '';
  const res = await fetch(`/api/sysmon/current${params}`);
  if (!res.ok) throw new Error('Failed to get sysmon current');
  return res.json();
}

/** Fetches historical CPU/memory points over the requested window. */
export async function getSysmonHistory(agent?: string, hours = 1): Promise<HistoryPoint[]> {
  const params = new URLSearchParams();
  if (agent) params.set('agent', agent);
  params.set('hours', String(hours));
  const res = await fetch(`/api/sysmon/history?${params}`);
  if (!res.ok) throw new Error('Failed to get sysmon history');
  return res.json();
}

/** Fetches per-mountpoint disk usage for the given agent (or all agents). */
export async function getSysmonDisks(agent?: string): Promise<SysmonDisk[]> {
  const params = agent ? `?agent=${encodeURIComponent(agent)}` : '';
  const res = await fetch(`/api/sysmon/disks${params}`);
  if (!res.ok) throw new Error('Failed to get sysmon disks');
  return res.json();
}

/** Lists all agents that have reported sysmon snapshots. */
export async function getSysmonAgents(): Promise<SysmonAgent[]> {
  const res = await fetch('/api/sysmon/agents');
  if (!res.ok) throw new Error('Failed to get sysmon agents');
  return res.json();
}
