// Sysmon API client

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

export interface SysmonDisk {
  agent_name: string;
  mount_point: string;
  fs_type: string;
  total_bytes: number;
  used_bytes: number;
  avail_bytes: number;
}

export interface SysmonAgent {
  name: string;
  snapshot_count: number;
  last_active: number;
}

export interface HistoryPoint {
  cpu_percent: number;
  mem_used: number;
  mem_total: number;
  collected_at: number;
}

export async function getSysmonCurrent(agent?: string): Promise<SysmonSnapshot[]> {
  const params = agent ? `?agent=${encodeURIComponent(agent)}` : '';
  const res = await fetch(`/api/sysmon/current${params}`);
  if (!res.ok) throw new Error('Failed to get sysmon current');
  return res.json();
}

export async function getSysmonHistory(agent?: string, hours = 1): Promise<HistoryPoint[]> {
  const params = new URLSearchParams();
  if (agent) params.set('agent', agent);
  params.set('hours', String(hours));
  const res = await fetch(`/api/sysmon/history?${params}`);
  if (!res.ok) throw new Error('Failed to get sysmon history');
  return res.json();
}

export async function getSysmonDisks(agent?: string): Promise<SysmonDisk[]> {
  const params = agent ? `?agent=${encodeURIComponent(agent)}` : '';
  const res = await fetch(`/api/sysmon/disks${params}`);
  if (!res.ok) throw new Error('Failed to get sysmon disks');
  return res.json();
}

export async function getSysmonAgents(): Promise<SysmonAgent[]> {
  const res = await fetch('/api/sysmon/agents');
  if (!res.ok) throw new Error('Failed to get sysmon agents');
  return res.json();
}
