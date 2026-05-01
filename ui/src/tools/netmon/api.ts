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

export interface AgentInfo {
  name: string;
  flow_count: number;
  last_active: number;
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

export async function getTopHosts(limit?: number, agent?: string): Promise<TopEntry[]> {
  const query = new URLSearchParams();
  if (limit) query.set('limit', String(limit));
  if (agent) query.set('agent', agent);
  const res = await fetch(`${API_BASE}/top-hosts?${query}`);
  if (!res.ok) throw new Error('Failed to get top hosts');
  return res.json();
}

export async function getTopPorts(agent?: string): Promise<TopEntry[]> {
  const query = new URLSearchParams();
  if (agent) query.set('agent', agent);
  const res = await fetch(`${API_BASE}/top-ports?${query}`);
  if (!res.ok) throw new Error('Failed to get top ports');
  return res.json();
}

export async function getStats(agent?: string): Promise<Stats> {
  const query = new URLSearchParams();
  if (agent) query.set('agent', agent);
  const res = await fetch(`${API_BASE}/stats?${query}`);
  if (!res.ok) throw new Error('Failed to get stats');
  return res.json();
}

export async function getAgents(): Promise<AgentInfo[]> {
  const res = await fetch(`${API_BASE}/agents`);
  if (!res.ok) throw new Error('Failed to get agents');
  return res.json();
}

export interface GraphNode {
  id: string;
  label: string;
  type: 'agent' | 'host';
  size: number;
}

export interface GraphEdge {
  source: string;
  target: string;
  port: number;
  proto: string;
  count: number;
}

export interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export async function getGraph(agent?: string): Promise<GraphData> {
  const query = new URLSearchParams();
  if (agent) query.set('agent', agent);
  const res = await fetch(`${API_BASE}/graph?${query}`);
  if (!res.ok) throw new Error('Failed to get graph');
  return res.json();
}

export interface HostnameRecord {
  ip: string;
  hostname: string;
  agent_name: string;
  first_seen: number;
  last_seen: number;
  seen_count: number;
}

export interface HostnameStats {
  total_mappings: number;
  unique_ips: number;
  unique_names: number;
  new_today: number;
  last_updated: number;
}

export async function getHostnames(params?: { agent?: string; search?: string }): Promise<HostnameRecord[]> {
  const query = new URLSearchParams();
  if (params?.agent) query.set('agent', params.agent);
  if (params?.search) query.set('search', params.search);
  const res = await fetch(`${API_BASE}/hostnames?${query}`);
  if (!res.ok) throw new Error('Failed to get hostnames');
  return res.json();
}

export async function getRecentHostnames(agent?: string): Promise<HostnameRecord[]> {
  const query = new URLSearchParams();
  if (agent) query.set('agent', agent);
  const res = await fetch(`${API_BASE}/hostnames/recent?${query}`);
  if (!res.ok) throw new Error('Failed to get recent hostnames');
  return res.json();
}

export async function getHostnameStats(agent?: string): Promise<HostnameStats> {
  const query = new URLSearchParams();
  if (agent) query.set('agent', agent);
  const res = await fetch(`${API_BASE}/hostnames/stats?${query}`);
  if (!res.ok) throw new Error('Failed to get hostname stats');
  return res.json();
}
