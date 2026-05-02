// DNS Filter API client

export interface DNSStats {
  total_queries: number;
  blocked_queries: number;
  blocked_percent: number;
  unique_clients: number;
  unique_domains: number;
  blocklist_size: number;
  top_blocked: TopDomain[];
  top_queried: TopDomain[];
}

export interface TopDomain {
  domain: string;
  count: number;
}

export interface DNSQuery {
  id: number;
  agent_name: string;
  domain: string;
  qtype: string;
  client_ip: string;
  blocked: boolean;
  upstream_ms: number;
  answer: string;
  timestamp: number;
}

export interface DNSBlocklist {
  id: number;
  url: string;
  name: string;
  enabled: boolean;
  entry_count: number;
  last_updated: number;
  created_at: number;
}

export interface DNSRule {
  id: number;
  domain: string;
  action: string;
  created_at: number;
}

export interface DNSAgent {
  name: string;
  query_count: number;
  blocked_count: number;
  last_active: number;
}

export async function getDNSStats(agent?: string, hours = 24): Promise<DNSStats> {
  const params = new URLSearchParams();
  if (agent) params.set('agent', agent);
  params.set('hours', String(hours));
  const res = await fetch(`/api/dnsfilter/stats?${params}`);
  if (!res.ok) throw new Error('Failed to get DNS stats');
  return res.json();
}

export async function getDNSQueries(opts?: {
  agent?: string; search?: string; blocked?: boolean; limit?: number;
}): Promise<DNSQuery[]> {
  const params = new URLSearchParams();
  if (opts?.agent) params.set('agent', opts.agent);
  if (opts?.search) params.set('search', opts.search);
  if (opts?.blocked) params.set('blocked', 'true');
  if (opts?.limit) params.set('limit', String(opts.limit));
  const res = await fetch(`/api/dnsfilter/queries?${params}`);
  if (!res.ok) throw new Error('Failed to get DNS queries');
  return res.json();
}

export async function getDNSAgents(): Promise<DNSAgent[]> {
  const res = await fetch('/api/dnsfilter/agents');
  if (!res.ok) throw new Error('Failed to get DNS agents');
  return res.json();
}

export async function getBlocklists(): Promise<DNSBlocklist[]> {
  const res = await fetch('/api/dnsfilter/blocklists');
  if (!res.ok) throw new Error('Failed to get blocklists');
  return res.json();
}

export async function addBlocklist(url: string, name: string): Promise<void> {
  const res = await fetch('/api/dnsfilter/blocklists', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url, name }),
  });
  if (!res.ok) throw new Error('Failed to add blocklist');
}

export async function deleteBlocklist(id: number): Promise<void> {
  const res = await fetch(`/api/dnsfilter/blocklists/${id}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete blocklist');
}

export async function refreshBlocklists(): Promise<void> {
  const res = await fetch('/api/dnsfilter/blocklists/refresh', { method: 'POST' });
  if (!res.ok) throw new Error('Failed to refresh blocklists');
}

export async function getRules(): Promise<DNSRule[]> {
  const res = await fetch('/api/dnsfilter/rules');
  if (!res.ok) throw new Error('Failed to get rules');
  return res.json();
}

export async function addRule(domain: string, action: string): Promise<void> {
  const res = await fetch('/api/dnsfilter/rules', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ domain, action }),
  });
  if (!res.ok) throw new Error('Failed to add rule');
}

export async function deleteRule(id: number): Promise<void> {
  const res = await fetch(`/api/dnsfilter/rules/${id}`, { method: 'DELETE' });
  if (!res.ok) throw new Error('Failed to delete rule');
}
