import { useState, useEffect, useCallback } from 'react';
import { getFlows, getTopHosts, getTopPorts, getStats, getAgents, getGraph, getHostnames, getHostnameStats, Flow, TopEntry, Stats, AgentInfo, GraphData, HostnameRecord, HostnameStats as HStats } from './api';
import NetworkMap from './NetworkMap';

export default function NetMonPage() {
  const [flows, setFlows] = useState<Flow[]>([]);
  const [topHosts, setTopHosts] = useState<TopEntry[]>([]);
  const [topPorts, setTopPorts] = useState<TopEntry[]>([]);
  const [stats, setStats] = useState<Stats | null>(null);
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [graphData, setGraphData] = useState<GraphData>({ nodes: [], edges: [] });
  const [hostnames, setHostnames] = useState<HostnameRecord[]>([]);
  const [hostnameStats, setHostnameStats] = useState<HStats | null>(null);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [search, setSearch] = useState('');
  const [proto, setProto] = useState('');
  const [hostnameSearch, setHostnameSearch] = useState('');
  const [tab, setTab] = useState<'flows' | 'hosts' | 'ports' | 'map' | 'hostnames'>('flows');
  const [loading, setLoading] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const agent = selectedAgent || undefined;
      const [f, h, p, s, a, gr, hn, hs] = await Promise.all([
        getFlows({ search, proto: proto || undefined, agent, limit: 200 }),
        getTopHosts(20, agent),
        getTopPorts(agent),
        getStats(agent),
        getAgents(),
        getGraph(agent),
        getHostnames({ agent, search: hostnameSearch || undefined }),
        getHostnameStats(agent),
      ]);
      setFlows(f);
      setTopHosts(h);
      setTopPorts(p);
      setStats(s);
      setAgents(a);
      setGraphData(gr);
      setHostnames(hn);
      setHostnameStats(hs);
    } catch (err) {
      console.error('Failed to load data:', err);
    } finally {
      setLoading(false);
    }
  }, [search, proto, selectedAgent, hostnameSearch]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [autoRefresh, loadData]);

  const formatTime = (ts: number) => {
    if (!ts) return '-';
    return new Date(ts * 1000).toLocaleString();
  };

  const formatAgo = (ts: number) => {
    if (!ts) return '-';
    const secs = Math.floor(Date.now() / 1000 - ts);
    if (secs < 60) return `${secs}s ago`;
    if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
    if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
    return `${Math.floor(secs / 86400)}d ago`;
  };

  const stateColor = (state: string) => {
    if (state === 'ESTABLISHED') return '#22c55e';
    if (state === 'TIME_WAIT' || state === 'CLOSE_WAIT') return '#eab308';
    if (state === 'SYN_SENT') return '#3b82f6';
    return '#6b7280';
  };

  return (
    <div style={{ fontFamily: 'sans-serif' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
        <h1 style={{ fontSize: '24px', fontWeight: 'bold' }}>Network Monitor</h1>
        <div style={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
          <label style={{ fontSize: '14px', color: '#6b7280' }}>
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
              style={{ marginRight: '4px' }}
            />
            Auto-refresh
          </label>
          <button
            onClick={loadData}
            disabled={loading}
            style={{
              padding: '6px 16px', backgroundColor: '#3b82f6', color: 'white',
              border: 'none', borderRadius: '4px', cursor: 'pointer', fontSize: '14px',
            }}
          >
            {loading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
      </div>

      {/* Agent selector */}
      {agents.length > 1 && (
        <div style={{
          display: 'flex', gap: '8px', alignItems: 'center', marginBottom: '16px',
          backgroundColor: 'white', borderRadius: '8px', padding: '12px 16px',
          boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
        }}>
          <span style={{ fontSize: '14px', fontWeight: 'bold', color: '#374151' }}>Agent:</span>
          <button
            onClick={() => setSelectedAgent('')}
            style={{
              padding: '4px 12px', border: 'none', borderRadius: '16px', cursor: 'pointer',
              fontSize: '13px', fontWeight: selectedAgent === '' ? 'bold' : 'normal',
              backgroundColor: selectedAgent === '' ? '#3b82f6' : '#e5e7eb',
              color: selectedAgent === '' ? 'white' : '#374151',
            }}
          >
            All ({agents.reduce((sum, a) => sum + a.flow_count, 0)})
          </button>
          {agents.map((a) => (
            <button
              key={a.name}
              onClick={() => setSelectedAgent(a.name)}
              style={{
                padding: '4px 12px', border: 'none', borderRadius: '16px', cursor: 'pointer',
                fontSize: '13px', fontWeight: selectedAgent === a.name ? 'bold' : 'normal',
                backgroundColor: selectedAgent === a.name ? '#3b82f6' : '#e5e7eb',
                color: selectedAgent === a.name ? 'white' : '#374151',
              }}
            >
              {a.name} ({a.flow_count})
            </button>
          ))}
        </div>
      )}

      {/* Stats cards */}
      {stats && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '16px', marginBottom: '20px' }}>
          {[
            { label: 'Total Flows', value: stats.total_flows, color: '#3b82f6' },
            { label: 'Unique Hosts', value: stats.total_hosts, color: '#8b5cf6' },
            { label: 'Active Now', value: stats.active_now, color: '#22c55e' },
            { label: 'Agents', value: stats.unique_agents, color: '#f59e0b' },
          ].map((card) => (
            <div
              key={card.label}
              style={{
                backgroundColor: 'white', borderRadius: '8px', padding: '16px',
                boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
              }}
            >
              <div style={{ fontSize: '13px', color: '#6b7280' }}>{card.label}</div>
              <div style={{ fontSize: '28px', fontWeight: 'bold', color: card.color }}>{card.value}</div>
            </div>
          ))}
        </div>
      )}

      {/* Tabs */}
      <div style={{ display: 'flex', gap: '4px', marginBottom: '16px' }}>
        {(['flows', 'hosts', 'ports', 'map', 'hostnames'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              padding: '8px 20px', border: 'none', borderRadius: '4px', cursor: 'pointer',
              backgroundColor: tab === t ? '#3b82f6' : '#e5e7eb',
              color: tab === t ? 'white' : '#374151', fontWeight: tab === t ? 'bold' : 'normal',
            }}
          >
            {t === 'flows' ? 'Connections' : t === 'hosts' ? 'Top Hosts' : t === 'ports' ? 'Top Ports' : t === 'map' ? '🗺 Map' : '🔍 Hostnames'}
          </button>
        ))}
      </div>

      {/* Flows tab */}
      {tab === 'flows' && (
        <div style={{ backgroundColor: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <div style={{ display: 'flex', gap: '10px', marginBottom: '16px' }}>
            <input
              type="text"
              placeholder="Search IP, hostname, or process..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{ flex: 1, padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: '4px' }}
            />
            <select
              value={proto}
              onChange={(e) => setProto(e.target.value)}
              style={{ padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: '4px' }}
            >
              <option value="">All Protocols</option>
              <option value="tcp">TCP</option>
              <option value="tcp6">TCP6</option>
              <option value="udp">UDP</option>
              <option value="udp6">UDP6</option>
            </select>
          </div>

          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '13px' }}>
              <thead>
                <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                  <th style={{ padding: '8px 6px' }}>Remote Host</th>
                  <th style={{ padding: '8px 6px' }}>Port</th>
                  <th style={{ padding: '8px 6px' }}>Proto</th>
                  <th style={{ padding: '8px 6px' }}>State</th>
                  <th style={{ padding: '8px 6px' }}>Process</th>
                  <th style={{ padding: '8px 6px' }}>Count</th>
                  <th style={{ padding: '8px 6px' }}>Last Seen</th>
                  <th style={{ padding: '8px 6px' }}>Agent</th>
                </tr>
              </thead>
              <tbody>
                {flows.map((f) => (
                  <tr key={f.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                    <td style={{ padding: '6px', fontFamily: 'monospace' }}>
                      {f.hostname ? (
                        <span title={f.remote_ip}>{f.hostname}</span>
                      ) : (
                        f.remote_ip
                      )}
                    </td>
                    <td style={{ padding: '6px', fontFamily: 'monospace' }}>{f.remote_port}</td>
                    <td style={{ padding: '6px' }}>{f.proto}</td>
                    <td style={{ padding: '6px' }}>
                      <span style={{
                        display: 'inline-block', padding: '2px 8px', borderRadius: '10px',
                        fontSize: '11px', fontWeight: 'bold',
                        backgroundColor: stateColor(f.state) + '20', color: stateColor(f.state),
                      }}>
                        {f.state || '-'}
                      </span>
                    </td>
                    <td style={{ padding: '6px', fontSize: '12px', color: '#6b7280' }}>
                      {f.process || '-'}
                      {f.pid > 0 && <span style={{ color: '#9ca3af' }}> ({f.pid})</span>}
                    </td>
                    <td style={{ padding: '6px', textAlign: 'center' }}>{f.count}</td>
                    <td style={{ padding: '6px', fontSize: '12px', color: '#6b7280' }} title={formatTime(f.last_seen)}>
                      {formatAgo(f.last_seen)}
                    </td>
                    <td style={{ padding: '6px', fontSize: '12px' }}>{f.agent_name}</td>
                  </tr>
                ))}
                {flows.length === 0 && (
                  <tr>
                    <td colSpan={8} style={{ padding: '20px', textAlign: 'center', color: '#9ca3af' }}>
                      No connections recorded yet. Monitoring is active — data will appear shortly.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Top Hosts tab */}
      {tab === 'hosts' && (
        <div style={{ backgroundColor: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h2 style={{ fontSize: '16px', fontWeight: 'bold', marginBottom: '12px' }}>Top Hosts by Connection Count</h2>
          {topHosts.map((h, i) => {
            const maxCount = topHosts[0]?.count || 1;
            return (
              <div key={h.name} style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '8px' }}>
                <span style={{ width: '30px', textAlign: 'right', color: '#6b7280', fontSize: '13px' }}>
                  {i + 1}.
                </span>
                <div style={{ flex: 1 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '2px' }}>
                    <span style={{ fontFamily: 'monospace', fontSize: '13px' }}>{h.name}</span>
                    <span style={{ fontSize: '13px', fontWeight: 'bold', color: '#3b82f6' }}>{h.count}</span>
                  </div>
                  <div style={{ height: '6px', backgroundColor: '#e5e7eb', borderRadius: '3px', overflow: 'hidden' }}>
                    <div
                      style={{
                        height: '100%', backgroundColor: '#3b82f6', borderRadius: '3px',
                        width: `${(h.count / maxCount) * 100}%`,
                      }}
                    />
                  </div>
                </div>
              </div>
            );
          })}
          {topHosts.length === 0 && (
            <p style={{ color: '#9ca3af', textAlign: 'center', padding: '20px' }}>No data yet.</p>
          )}
        </div>
      )}

      {/* Top Ports tab */}
      {tab === 'ports' && (
        <div style={{ backgroundColor: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <h2 style={{ fontSize: '16px', fontWeight: 'bold', marginBottom: '12px' }}>Top Ports by Connection Count</h2>
          {topPorts.map((p, i) => {
            const maxCount = topPorts[0]?.count || 1;
            return (
              <div key={p.name} style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '8px' }}>
                <span style={{ width: '30px', textAlign: 'right', color: '#6b7280', fontSize: '13px' }}>
                  {i + 1}.
                </span>
                <div style={{ flex: 1 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '2px' }}>
                    <span style={{ fontFamily: 'monospace', fontSize: '13px' }}>{p.name}</span>
                    <span style={{ fontSize: '13px', fontWeight: 'bold', color: '#8b5cf6' }}>{p.count}</span>
                  </div>
                  <div style={{ height: '6px', backgroundColor: '#e5e7eb', borderRadius: '3px', overflow: 'hidden' }}>
                    <div
                      style={{
                        height: '100%', backgroundColor: '#8b5cf6', borderRadius: '3px',
                        width: `${(p.count / maxCount) * 100}%`,
                      }}
                    />
                  </div>
                </div>
              </div>
            );
          })}
          {topPorts.length === 0 && (
            <p style={{ color: '#9ca3af', textAlign: 'center', padding: '20px' }}>No data yet.</p>
          )}
        </div>
      )}
      {/* Map tab */}
      {tab === 'map' && (
        <div>
          <div style={{ marginBottom: '8px', fontSize: '13px', color: '#6b7280' }}>
            Network topology — {graphData.nodes.length} nodes, {graphData.edges.length} connections
            {selectedAgent && <span> (filtered to <strong>{selectedAgent}</strong>)</span>}
          </div>
          {graphData.nodes.length > 0 ? (
            <NetworkMap data={graphData} height={550} />
          ) : (
            <div style={{
              backgroundColor: 'white', borderRadius: '8px', padding: '60px',
              boxShadow: '0 1px 3px rgba(0,0,0,0.1)', textAlign: 'center', color: '#9ca3af',
            }}>
              No connection data yet. Wait for agents to report flows.
            </div>
          )}
        </div>
      )}

      {/* Hostnames tab */}
      {tab === 'hostnames' && (
        <div>
          {/* Hostname stats cards */}
          {hostnameStats && (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '16px', marginBottom: '16px' }}>
              {[
                { label: 'Total Mappings', value: hostnameStats.total_mappings, color: '#8b5cf6' },
                { label: 'Unique IPs', value: hostnameStats.unique_ips, color: '#3b82f6' },
                { label: 'Unique Hostnames', value: hostnameStats.unique_names, color: '#22c55e' },
                { label: 'New Today', value: hostnameStats.new_today, color: '#f59e0b' },
              ].map((card) => (
                <div
                  key={card.label}
                  style={{
                    backgroundColor: 'white', borderRadius: '8px', padding: '16px',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
                  }}
                >
                  <div style={{ fontSize: '13px', color: '#6b7280' }}>{card.label}</div>
                  <div style={{ fontSize: '28px', fontWeight: 'bold', color: card.color }}>{card.value}</div>
                </div>
              ))}
            </div>
          )}

          <div style={{ backgroundColor: 'white', borderRadius: '8px', padding: '16px', boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <div style={{ display: 'flex', gap: '10px', marginBottom: '16px' }}>
              <input
                type="text"
                placeholder="Search IP or hostname..."
                value={hostnameSearch}
                onChange={(e) => setHostnameSearch(e.target.value)}
                style={{ flex: 1, padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: '4px' }}
              />
            </div>

            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '13px' }}>
                <thead>
                  <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                    <th style={{ padding: '8px 6px' }}>IP Address</th>
                    <th style={{ padding: '8px 6px' }}>Hostname</th>
                    <th style={{ padding: '8px 6px' }}>Agent</th>
                    <th style={{ padding: '8px 6px' }}>Times Seen</th>
                    <th style={{ padding: '8px 6px' }}>First Seen</th>
                    <th style={{ padding: '8px 6px' }}>Last Seen</th>
                  </tr>
                </thead>
                <tbody>
                  {hostnames.map((h, i) => (
                    <tr key={`${h.ip}-${h.hostname}-${h.agent_name}-${i}`} style={{ borderBottom: '1px solid #f3f4f6' }}>
                      <td style={{ padding: '6px', fontFamily: 'monospace' }}>{h.ip}</td>
                      <td style={{ padding: '6px', fontFamily: 'monospace', color: '#3b82f6' }}>{h.hostname}</td>
                      <td style={{ padding: '6px', fontSize: '12px' }}>{h.agent_name}</td>
                      <td style={{ padding: '6px', textAlign: 'center' }}>{h.seen_count}</td>
                      <td style={{ padding: '6px', fontSize: '12px', color: '#6b7280' }}>
                        {formatTime(h.first_seen)}
                      </td>
                      <td style={{ padding: '6px', fontSize: '12px', color: '#6b7280' }} title={formatTime(h.last_seen)}>
                        {formatAgo(h.last_seen)}
                      </td>
                    </tr>
                  ))}
                  {hostnames.length === 0 && (
                    <tr>
                      <td colSpan={6} style={{ padding: '20px', textAlign: 'center', color: '#9ca3af' }}>
                        No hostname mappings yet. The enricher runs every 60 seconds.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
