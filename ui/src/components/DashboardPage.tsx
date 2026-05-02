/**
 * Home/landing page.
 *
 * Aggregates platform-wide signals into a single overview: agent counts and
 * health, DNS query/block totals, top blocked/queried domains, recent block
 * events, and any URLs currently down. Polls `/api/dashboard` every 10s.
 */
import { useState, useEffect, useCallback } from 'react';

/** Shape of the JSON payload returned by `/api/dashboard`. */
interface DashboardData {
  agent_count: number;
  agents_online: number;
  dns_total: number;
  dns_blocked: number;
  dns_blocked_pct: number;
  blocklist_size: number;
  net_flows: number;
  top_blocked: { domain: string; count: number }[];
  top_queried: { domain: string; count: number }[];
  agents: {
    name: string;
    os: string;
    cpu_percent: number;
    mem_percent: number;
    uptime: number;
    last_seen: number;
  }[];
  urls_down: { url: string; status_code: number; last_check: number }[];
  recent_blocks: { domain: string; agent: string; timestamp: number }[];
}

/** Format an uptime duration in seconds as `Xd Yh`, `Xh Ym`, or `Xm`. */
function formatUptime(secs: number): string {
  if (secs <= 0) return '-';
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

/** Compact a number using K/M suffixes (e.g. 1.2K, 3.4M). */
function formatNumber(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
  return String(n);
}

/** Format a Unix timestamp as a human-friendly relative time. */
function formatAgo(ts: number): string {
  if (!ts) return '-';
  const secs = Math.floor(Date.now() / 1000 - ts);
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
  return `${Math.floor(secs / 86400)}d ago`;
}

/** True if the agent has reported within the last 5 minutes. */
function isOnline(lastSeen: number): boolean {
  return (Date.now() / 1000 - lastSeen) < 300;
}

/** Aggregated landing page; auto-refreshes every 10 seconds. */
export default function DashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);

  const loadData = useCallback(async () => {
    try {
      const resp = await fetch('/api/dashboard');
      if (resp.ok) setData(await resp.json());
    } catch { /* ignore */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);
  useEffect(() => {
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [loadData]);

  if (loading) return <div style={{ padding: 40, textAlign: 'center', color: '#9ca3af' }}>Loading dashboard...</div>;
  if (!data) return <div style={{ padding: 40, textAlign: 'center', color: '#ef4444' }}>Failed to load dashboard</div>;

  return (
    <div style={{ fontFamily: 'sans-serif' }}>
      <h1 style={{ fontSize: 24, fontWeight: 'bold', marginBottom: 20 }}>🏠 Dashboard</h1>

      {/* Overview Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 24 }}>
        <StatCard
          label="Agents Online"
          value={`${data.agents_online}/${data.agent_count}`}
          color="#22c55e"
          icon="🖥"
          subtitle={data.agents_online === data.agent_count ? 'All connected' : `${data.agent_count - data.agents_online} offline`}
        />
        <StatCard
          label="DNS Queries (24h)"
          value={formatNumber(data.dns_total)}
          color="#3b82f6"
          icon="🔍"
          subtitle={`${data.dns_blocked_pct.toFixed(1)}% blocked`}
        />
        <StatCard
          label="DNS Blocked (24h)"
          value={formatNumber(data.dns_blocked)}
          color="#ef4444"
          icon="🛡"
          subtitle={`${formatNumber(data.blocklist_size)} in blocklist`}
        />
        <StatCard
          label="Network Flows (24h)"
          value={formatNumber(data.net_flows)}
          color="#8b5cf6"
          icon="🌐"
          subtitle="Active connections"
        />
      </div>

      {/* Alerts */}
      {data.urls_down.length > 0 && (
        <div style={{
          backgroundColor: '#fef2f2', border: '1px solid #fecaca',
          borderRadius: 8, padding: 16, marginBottom: 24,
        }}>
          <h3 style={{ fontSize: 14, fontWeight: 'bold', color: '#dc2626', marginBottom: 8 }}>
            ⚠️ URL Monitors Down ({data.urls_down.length})
          </h3>
          {data.urls_down.map(u => (
            <div key={u.url} style={{ fontSize: 13, color: '#7f1d1d', marginBottom: 4 }}>
              <span style={{ fontFamily: 'monospace' }}>{u.url}</span>
              <span style={{ marginLeft: 8, color: '#ef4444' }}>HTTP {u.status_code}</span>
            </div>
          ))}
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 24 }}>
        {/* Agents */}
        <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12 }}>🖥 Agents</h2>
          {data.agents.length > 0 ? data.agents.map(a => (
            <div key={a.name} style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '10px 0', borderBottom: '1px solid #f3f4f6',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{ fontSize: 10, color: isOnline(a.last_seen) ? '#22c55e' : '#ef4444' }}>●</span>
                <div>
                  <div style={{ fontSize: 14, fontWeight: 'bold' }}>{a.name}</div>
                  <div style={{ fontSize: 12, color: '#6b7280' }}>
                    {a.os} · up {formatUptime(a.uptime)}
                  </div>
                </div>
              </div>
              <div style={{ display: 'flex', gap: 16, fontSize: 12 }}>
                <MiniGauge label="CPU" value={a.cpu_percent} color="#3b82f6" />
                <MiniGauge label="MEM" value={a.mem_percent} color="#8b5cf6" />
              </div>
            </div>
          )) : (
            <p style={{ color: '#9ca3af', textAlign: 'center', padding: 20 }}>
              No agents connected yet
            </p>
          )}
        </div>

        {/* Recent Blocks */}
        <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12, color: '#ef4444' }}>🚫 Recent Blocks</h2>
          {data.recent_blocks.length > 0 ? data.recent_blocks.map((b, i) => (
            <div key={i} style={{
              display: 'flex', justifyContent: 'space-between', alignItems: 'center',
              padding: '6px 0', borderBottom: '1px solid #f3f4f6', fontSize: 13,
            }}>
              <span style={{ fontFamily: 'monospace', color: '#dc2626' }}>{b.domain}</span>
              <span style={{ color: '#9ca3af', fontSize: 12 }}>{formatAgo(b.timestamp)}</span>
            </div>
          )) : (
            <p style={{ color: '#9ca3af', textAlign: 'center', padding: 20 }}>
              No blocked queries yet
            </p>
          )}
        </div>
      </div>

      {/* Top Domains */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12, color: '#ef4444' }}>🚫 Top Blocked (24h)</h2>
          {data.top_blocked.length > 0 ? data.top_blocked.map((d, i) => {
            const max = data.top_blocked[0]?.count || 1;
            return (
              <div key={d.domain} style={{ marginBottom: 8 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 2 }}>
                  <span style={{ fontFamily: 'monospace' }}>{i + 1}. {d.domain}</span>
                  <span style={{ fontWeight: 'bold', color: '#ef4444' }}>{formatNumber(d.count)}</span>
                </div>
                <div style={{ height: 4, backgroundColor: '#fee2e2', borderRadius: 2 }}>
                  <div style={{ height: '100%', backgroundColor: '#ef4444', borderRadius: 2, width: `${(d.count / max) * 100}%` }} />
                </div>
              </div>
            );
          }) : <p style={{ color: '#9ca3af', textAlign: 'center' }}>No data yet</p>}
        </div>

        <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12, color: '#3b82f6' }}>🔍 Top Queried (24h)</h2>
          {data.top_queried.length > 0 ? data.top_queried.map((d, i) => {
            const max = data.top_queried[0]?.count || 1;
            return (
              <div key={d.domain} style={{ marginBottom: 8 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 2 }}>
                  <span style={{ fontFamily: 'monospace' }}>{i + 1}. {d.domain}</span>
                  <span style={{ fontWeight: 'bold', color: '#3b82f6' }}>{formatNumber(d.count)}</span>
                </div>
                <div style={{ height: 4, backgroundColor: '#dbeafe', borderRadius: 2 }}>
                  <div style={{ height: '100%', backgroundColor: '#3b82f6', borderRadius: 2, width: `${(d.count / max) * 100}%` }} />
                </div>
              </div>
            );
          }) : <p style={{ color: '#9ca3af', textAlign: 'center' }}>No data yet</p>}
        </div>
      </div>
    </div>
  );
}

function StatCard({ label, value, color, icon, subtitle }: {
  label: string; value: string; color: string; icon: string; subtitle: string;
}) {
  return (
    <div style={{
      backgroundColor: 'white', borderRadius: 8, padding: 16,
      boxShadow: '0 1px 3px rgba(0,0,0,0.1)', borderLeft: `4px solid ${color}`,
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <div style={{ fontSize: 12, color: '#6b7280', marginBottom: 4 }}>{label}</div>
          <div style={{ fontSize: 28, fontWeight: 'bold', color }}>{value}</div>
          <div style={{ fontSize: 11, color: '#9ca3af', marginTop: 2 }}>{subtitle}</div>
        </div>
        <span style={{ fontSize: 24 }}>{icon}</span>
      </div>
    </div>
  );
}

function MiniGauge({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div style={{ textAlign: 'center', minWidth: 45 }}>
      <div style={{ fontSize: 11, color: '#6b7280' }}>{label}</div>
      <div style={{ fontSize: 14, fontWeight: 'bold', color }}>
        {value.toFixed(0)}%
      </div>
      <div style={{ height: 3, backgroundColor: '#e5e7eb', borderRadius: 2, marginTop: 2 }}>
        <div style={{
          height: '100%', borderRadius: 2, backgroundColor: value > 80 ? '#ef4444' : color,
          width: `${Math.min(value, 100)}%`,
        }} />
      </div>
    </div>
  );
}
