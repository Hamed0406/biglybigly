const API_BASE = '/api/netmon';

export interface Flow {
  id: number;
  agent_name: string;
  proto: string;
  local_ip: string;
  local_port: number;
  remote_ip: string;
  remote_port: number;
  hostname: string;
  pid: number;
  process: string;
  state: string;
  count: number;
  first_seen: number;
  last_seen: number;
}

export interface TopEntry {
  name: string;
  count: number;
}

export interface Stats {
  total_flows: number;
  total_hosts: number;
  active_now: number;
  unique_agents: number;
}

export async function getFlows(params?: {
  search?: string;
  agent?: string;
  proto?: string;
  limit?: number;
}): Promise<Flow[]> {
  const query = new URLSearchParams();
  if (params?.search) query.set('search', params.search);
  if (params?.agent) query.set('agent', params.agent);
  if (params?.proto) query.set('proto', params.proto);
  if (params?.limit) query.set('limit', String(params.limit));
  const res = await fetch(`${API_BASE}/flows?${query}`);
  if (!res.ok) throw new Error('Failed to get flows');
  return res.json();
}

export async function getTopHosts(limit?: number): Promise<TopEntry[]> {
  const query = limit ? `?limit=${limit}` : '';
  const res = await fetch(`${API_BASE}/top-hosts${query}`);
  if (!res.ok) throw new Error('Failed to get top hosts');
  return res.json();
}

export async function getTopPorts(): Promise<TopEntry[]> {
  const res = await fetch(`${API_BASE}/top-ports`);
  if (!res.ok) throw new Error('Failed to get top ports');
  return res.json();
}

export async function getStats(): Promise<Stats> {
  const res = await fetch(`${API_BASE}/stats`);
  if (!res.ok) throw new Error('Failed to get stats');
  return res.json();
}
