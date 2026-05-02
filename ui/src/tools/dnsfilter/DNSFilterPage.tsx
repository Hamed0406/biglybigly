import { useState, useEffect, useCallback } from 'react';
import {
  getDNSStats, getDNSQueries, getDNSAgents, getBlocklists, getRules,
  addBlocklist, deleteBlocklist, refreshBlocklists, addRule, deleteRule,
  DNSStats, DNSQuery, DNSAgent, DNSBlocklist, DNSRule,
} from './api';

function formatAgo(ts: number): string {
  if (!ts) return '-';
  const secs = Math.floor(Date.now() / 1000 - ts);
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
  return `${Math.floor(secs / 86400)}d ago`;
}

function formatTime(ts: number): string {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleTimeString();
}

function formatNumber(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
  return String(n);
}

export default function DNSFilterPage() {
  const [stats, setStats] = useState<DNSStats | null>(null);
  const [queries, setQueries] = useState<DNSQuery[]>([]);
  const [agents, setAgents] = useState<DNSAgent[]>([]);
  const [blocklists, setBlocklists] = useState<DNSBlocklist[]>([]);
  const [rules, setRules] = useState<DNSRule[]>([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [search, setSearch] = useState('');
  const [blockedOnly, setBlockedOnly] = useState(false);
  const [tab, setTab] = useState<'dashboard' | 'log' | 'blocklists' | 'rules'>('dashboard');
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [loading, setLoading] = useState(false);

  // Form state
  const [newListURL, setNewListURL] = useState('');
  const [newListName, setNewListName] = useState('');
  const [newRuleDomain, setNewRuleDomain] = useState('');
  const [newRuleAction, setNewRuleAction] = useState('block');

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const agent = selectedAgent || undefined;
      const [s, q, a, bl, rl] = await Promise.all([
        getDNSStats(agent),
        getDNSQueries({ agent, search: search || undefined, blocked: blockedOnly, limit: 200 }),
        getDNSAgents(),
        getBlocklists(),
        getRules(),
      ]);
      setStats(s);
      setQueries(q);
      setAgents(a);
      setBlocklists(bl);
      setRules(rl);
    } catch (err) {
      console.error('Failed to load DNS data:', err);
    } finally {
      setLoading(false);
    }
  }, [selectedAgent, search, blockedOnly]);

  useEffect(() => { loadData(); }, [loadData]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(loadData, 5000);
    return () => clearInterval(interval);
  }, [autoRefresh, loadData]);

  const handleAddBlocklist = async () => {
    if (!newListURL) return;
    try {
      await addBlocklist(newListURL, newListName || newListURL);
      setNewListURL('');
      setNewListName('');
      loadData();
    } catch { /* ignore */ }
  };

  const handleDeleteBlocklist = async (id: number) => {
    try { await deleteBlocklist(id); loadData(); } catch { /* ignore */ }
  };

  const handleRefresh = async () => {
    try { await refreshBlocklists(); setTimeout(loadData, 2000); } catch { /* ignore */ }
  };

  const handleAddRule = async () => {
    if (!newRuleDomain) return;
    try {
      await addRule(newRuleDomain, newRuleAction);
      setNewRuleDomain('');
      loadData();
    } catch { /* ignore */ }
  };

  const handleDeleteRule = async (id: number) => {
    try { await deleteRule(id); loadData(); } catch { /* ignore */ }
  };

  return (
    <div style={{ fontFamily: 'sans-serif' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h1 style={{ fontSize: 24, fontWeight: 'bold' }}>🛡 DNS Filter</h1>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          <label style={{ fontSize: 14, color: '#6b7280' }}>
            <input type="checkbox" checked={autoRefresh} onChange={(e) => setAutoRefresh(e.target.checked)}
              style={{ marginRight: 4 }} />
            Auto-refresh
          </label>
          <button onClick={loadData} disabled={loading} style={{
            padding: '6px 16px', backgroundColor: '#3b82f6', color: 'white',
            border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 14,
          }}>
            {loading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
      </div>

      {/* Agent selector */}
      {agents.length > 0 && (
        <div style={{
          display: 'flex', gap: 8, alignItems: 'center', marginBottom: 16,
          backgroundColor: 'white', borderRadius: 8, padding: '12px 16px',
          boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
        }}>
          <span style={{ fontSize: 14, fontWeight: 'bold', color: '#374151' }}>Agent:</span>
          {agents.length > 1 && (
            <button onClick={() => setSelectedAgent('')} style={{
              padding: '4px 12px', border: 'none', borderRadius: 16, cursor: 'pointer', fontSize: 13,
              fontWeight: selectedAgent === '' ? 'bold' : 'normal',
              backgroundColor: selectedAgent === '' ? '#3b82f6' : '#e5e7eb',
              color: selectedAgent === '' ? 'white' : '#374151',
            }}>All</button>
          )}
          {agents.map((a) => (
            <button key={a.name} onClick={() => setSelectedAgent(a.name)} style={{
              padding: '4px 12px', border: 'none', borderRadius: 16, cursor: 'pointer', fontSize: 13,
              fontWeight: selectedAgent === a.name ? 'bold' : 'normal',
              backgroundColor: selectedAgent === a.name ? '#3b82f6' : '#e5e7eb',
              color: selectedAgent === a.name ? 'white' : '#374151',
            }}>
              {a.name}
            </button>
          ))}
        </div>
      )}

      {/* Stats cards */}
      {stats && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 16, marginBottom: 20 }}>
          <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <div style={{ fontSize: 13, color: '#6b7280' }}>Total Queries</div>
            <div style={{ fontSize: 28, fontWeight: 'bold', color: '#3b82f6' }}>{formatNumber(stats.total_queries)}</div>
          </div>
          <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <div style={{ fontSize: 13, color: '#6b7280' }}>Blocked</div>
            <div style={{ fontSize: 28, fontWeight: 'bold', color: '#ef4444' }}>{formatNumber(stats.blocked_queries)}</div>
            <div style={{ fontSize: 12, color: '#ef4444' }}>{stats.blocked_percent.toFixed(1)}%</div>
          </div>
          <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <div style={{ fontSize: 13, color: '#6b7280' }}>Blocklist Size</div>
            <div style={{ fontSize: 28, fontWeight: 'bold', color: '#8b5cf6' }}>{formatNumber(stats.blocklist_size)}</div>
          </div>
          <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <div style={{ fontSize: 13, color: '#6b7280' }}>Unique Domains</div>
            <div style={{ fontSize: 28, fontWeight: 'bold', color: '#22c55e' }}>{formatNumber(stats.unique_domains)}</div>
          </div>
          <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <div style={{ fontSize: 13, color: '#6b7280' }}>Clients</div>
            <div style={{ fontSize: 28, fontWeight: 'bold', color: '#f59e0b' }}>{stats.unique_clients}</div>
          </div>
        </div>
      )}

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 4, marginBottom: 16 }}>
        {(['dashboard', 'log', 'blocklists', 'rules'] as const).map((t) => (
          <button key={t} onClick={() => setTab(t)} style={{
            padding: '8px 20px', border: 'none', borderRadius: 4, cursor: 'pointer',
            backgroundColor: tab === t ? '#3b82f6' : '#e5e7eb',
            color: tab === t ? 'white' : '#374151', fontWeight: tab === t ? 'bold' : 'normal',
          }}>
            {t === 'dashboard' ? '📊 Dashboard' : t === 'log' ? '📋 Query Log' : t === 'blocklists' ? '📝 Blocklists' : '⚙ Rules'}
          </button>
        ))}
      </div>

      {/* Dashboard tab */}
      {tab === 'dashboard' && stats && (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
          {/* Top Blocked */}
          <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12, color: '#ef4444' }}>🚫 Top Blocked Domains</h2>
            {stats.top_blocked.length > 0 ? stats.top_blocked.map((d, i) => {
              const max = stats.top_blocked[0]?.count || 1;
              return (
                <div key={d.domain} style={{ marginBottom: 8 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 2 }}>
                    <span style={{ fontFamily: 'monospace' }}>{i + 1}. {d.domain}</span>
                    <span style={{ fontWeight: 'bold', color: '#ef4444' }}>{d.count}</span>
                  </div>
                  <div style={{ height: 4, backgroundColor: '#fee2e2', borderRadius: 2 }}>
                    <div style={{ height: '100%', backgroundColor: '#ef4444', borderRadius: 2, width: `${(d.count / max) * 100}%` }} />
                  </div>
                </div>
              );
            }) : <p style={{ color: '#9ca3af', textAlign: 'center' }}>No blocked queries yet</p>}
          </div>

          {/* Top Queried */}
          <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
            <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12, color: '#3b82f6' }}>🔍 Top Queried Domains</h2>
            {stats.top_queried.length > 0 ? stats.top_queried.map((d, i) => {
              const max = stats.top_queried[0]?.count || 1;
              return (
                <div key={d.domain} style={{ marginBottom: 8 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 2 }}>
                    <span style={{ fontFamily: 'monospace' }}>{i + 1}. {d.domain}</span>
                    <span style={{ fontWeight: 'bold', color: '#3b82f6' }}>{d.count}</span>
                  </div>
                  <div style={{ height: 4, backgroundColor: '#dbeafe', borderRadius: 2 }}>
                    <div style={{ height: '100%', backgroundColor: '#3b82f6', borderRadius: 2, width: `${(d.count / max) * 100}%` }} />
                  </div>
                </div>
              );
            }) : <p style={{ color: '#9ca3af', textAlign: 'center' }}>No queries yet</p>}
          </div>
        </div>
      )}

      {/* Query Log tab */}
      {tab === 'log' && (
        <div style={{ backgroundColor: 'white', borderRadius: 8, padding: 16, boxShadow: '0 1px 3px rgba(0,0,0,0.1)' }}>
          <div style={{ display: 'flex', gap: 10, marginBottom: 16 }}>
            <input type="text" placeholder="Search domain..." value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{ flex: 1, padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: 4 }} />
            <label style={{ display: 'flex', alignItems: 'center', fontSize: 13, color: '#6b7280', gap: 4 }}>
              <input type="checkbox" checked={blockedOnly} onChange={(e) => setBlockedOnly(e.target.checked)} />
              Blocked only
            </label>
          </div>

          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead>
                <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                  <th style={{ padding: '8px 6px' }}>Time</th>
                  <th style={{ padding: '8px 6px' }}>Domain</th>
                  <th style={{ padding: '8px 6px' }}>Type</th>
                  <th style={{ padding: '8px 6px' }}>Status</th>
                  <th style={{ padding: '8px 6px' }}>Answer</th>
                  <th style={{ padding: '8px 6px' }}>Speed</th>
                  <th style={{ padding: '8px 6px' }}>Client</th>
                  <th style={{ padding: '8px 6px' }}>Agent</th>
                </tr>
              </thead>
              <tbody>
                {queries.map((q) => (
                  <tr key={q.id} style={{ borderBottom: '1px solid #f3f4f6', backgroundColor: q.blocked ? '#fef2f2' : undefined }}>
                    <td style={{ padding: 6, fontSize: 12, color: '#6b7280', whiteSpace: 'nowrap' }}>{formatTime(q.timestamp)}</td>
                    <td style={{ padding: 6, fontFamily: 'monospace' }}>{q.domain}</td>
                    <td style={{ padding: 6, fontSize: 12 }}>{q.qtype}</td>
                    <td style={{ padding: 6 }}>
                      <span style={{
                        padding: '2px 8px', borderRadius: 10, fontSize: 11, fontWeight: 'bold',
                        backgroundColor: q.blocked ? '#fef2f2' : '#f0fdf4',
                        color: q.blocked ? '#ef4444' : '#22c55e',
                      }}>
                        {q.blocked ? '🚫 Blocked' : '✅ Allowed'}
                      </span>
                    </td>
                    <td style={{ padding: 6, fontFamily: 'monospace', fontSize: 12, color: '#6b7280', maxWidth: 150, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {q.answer || '-'}
                    </td>
                    <td style={{ padding: 6, fontSize: 12, color: '#6b7280' }}>
                      {q.blocked ? '-' : `${q.upstream_ms}ms`}
                    </td>
                    <td style={{ padding: 6, fontFamily: 'monospace', fontSize: 12 }}>{q.client_ip}</td>
                    <td style={{ padding: 6, fontSize: 12 }}>{q.agent_name}</td>
                  </tr>
                ))}
                {queries.length === 0 && (
                  <tr>
                    <td colSpan={8} style={{ padding: 40, textAlign: 'center', color: '#9ca3af' }}>
                      No DNS queries recorded yet. Start the agent with DNS proxy enabled.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Blocklists tab */}
      {tab === 'blocklists' && (
        <div>
          <div style={{
            backgroundColor: 'white', borderRadius: 8, padding: 16, marginBottom: 16,
            boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
          }}>
            <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12 }}>Add Blocklist</h2>
            <div style={{ display: 'flex', gap: 10 }}>
              <input type="text" placeholder="Blocklist URL (hosts file format)" value={newListURL}
                onChange={(e) => setNewListURL(e.target.value)}
                style={{ flex: 2, padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: 4 }} />
              <input type="text" placeholder="Name (optional)" value={newListName}
                onChange={(e) => setNewListName(e.target.value)}
                style={{ flex: 1, padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: 4 }} />
              <button onClick={handleAddBlocklist} style={{
                padding: '8px 16px', backgroundColor: '#22c55e', color: 'white',
                border: 'none', borderRadius: 4, cursor: 'pointer',
              }}>Add</button>
              <button onClick={handleRefresh} style={{
                padding: '8px 16px', backgroundColor: '#8b5cf6', color: 'white',
                border: 'none', borderRadius: 4, cursor: 'pointer',
              }}>↻ Refresh All</button>
            </div>
          </div>

          <div style={{
            backgroundColor: 'white', borderRadius: 8, padding: 16,
            boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
          }}>
            <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12 }}>Active Blocklists</h2>
            {blocklists.length > 0 ? blocklists.map((bl) => (
              <div key={bl.id} style={{
                display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                padding: '12px 0', borderBottom: '1px solid #f3f4f6',
              }}>
                <div>
                  <div style={{ fontWeight: 'bold', fontSize: 14 }}>{bl.name}</div>
                  <div style={{ fontSize: 12, color: '#6b7280', fontFamily: 'monospace' }}>{bl.url}</div>
                  <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 2 }}>
                    {bl.entry_count > 0 ? `${formatNumber(bl.entry_count)} domains` : 'Not loaded yet'}
                    {bl.last_updated > 0 && ` · Updated ${formatAgo(bl.last_updated)}`}
                  </div>
                </div>
                <button onClick={() => handleDeleteBlocklist(bl.id)} style={{
                  padding: '4px 12px', backgroundColor: '#fee2e2', color: '#ef4444',
                  border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 13,
                }}>Remove</button>
              </div>
            )) : (
              <p style={{ color: '#9ca3af', textAlign: 'center', padding: 20 }}>
                No blocklists configured. Add one above.
              </p>
            )}
          </div>
        </div>
      )}

      {/* Rules tab */}
      {tab === 'rules' && (
        <div>
          <div style={{
            backgroundColor: 'white', borderRadius: 8, padding: 16, marginBottom: 16,
            boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
          }}>
            <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12 }}>Add Custom Rule</h2>
            <div style={{ display: 'flex', gap: 10 }}>
              <input type="text" placeholder="Domain (e.g., ads.example.com)" value={newRuleDomain}
                onChange={(e) => setNewRuleDomain(e.target.value)}
                style={{ flex: 1, padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: 4 }} />
              <select value={newRuleAction} onChange={(e) => setNewRuleAction(e.target.value)}
                style={{ padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: 4 }}>
                <option value="block">🚫 Block</option>
                <option value="allow">✅ Allow</option>
              </select>
              <button onClick={handleAddRule} style={{
                padding: '8px 16px', backgroundColor: '#3b82f6', color: 'white',
                border: 'none', borderRadius: 4, cursor: 'pointer',
              }}>Add Rule</button>
            </div>
            <div style={{ fontSize: 12, color: '#9ca3af', marginTop: 8 }}>
              Allow rules override blocklists. Use them to whitelist domains that are incorrectly blocked.
            </div>
          </div>

          <div style={{
            backgroundColor: 'white', borderRadius: 8, padding: 16,
            boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
          }}>
            <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12 }}>Custom Rules</h2>
            {rules.length > 0 ? (
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                <thead>
                  <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                    <th style={{ padding: '8px 6px' }}>Domain</th>
                    <th style={{ padding: '8px 6px' }}>Action</th>
                    <th style={{ padding: '8px 6px' }}>Added</th>
                    <th style={{ padding: '8px 6px' }}></th>
                  </tr>
                </thead>
                <tbody>
                  {rules.map((r) => (
                    <tr key={r.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                      <td style={{ padding: 6, fontFamily: 'monospace' }}>{r.domain}</td>
                      <td style={{ padding: 6 }}>
                        <span style={{
                          padding: '2px 8px', borderRadius: 10, fontSize: 11, fontWeight: 'bold',
                          backgroundColor: r.action === 'block' ? '#fef2f2' : '#f0fdf4',
                          color: r.action === 'block' ? '#ef4444' : '#22c55e',
                        }}>
                          {r.action === 'block' ? '🚫 Block' : '✅ Allow'}
                        </span>
                      </td>
                      <td style={{ padding: 6, fontSize: 12, color: '#6b7280' }}>{formatAgo(r.created_at)}</td>
                      <td style={{ padding: 6 }}>
                        <button onClick={() => handleDeleteRule(r.id)} style={{
                          padding: '2px 8px', backgroundColor: '#fee2e2', color: '#ef4444',
                          border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12,
                        }}>Delete</button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <p style={{ color: '#9ca3af', textAlign: 'center', padding: 20 }}>
                No custom rules. Blocklist rules are applied automatically.
              </p>
            )}
          </div>
        </div>
      )}

      {/* Empty state when no stats */}
      {!stats && (
        <div style={{
          backgroundColor: 'white', borderRadius: 8, padding: 60,
          boxShadow: '0 1px 3px rgba(0,0,0,0.1)', textAlign: 'center', color: '#9ca3af',
        }}>
          <div style={{ fontSize: 48, marginBottom: 16 }}>🛡</div>
          <div style={{ fontSize: 18, fontWeight: 'bold', color: '#374151', marginBottom: 8 }}>
            DNS Filter Ready
          </div>
          <div>
            Start an agent to begin filtering DNS queries.
            <br />The agent runs a DNS proxy on 127.0.0.1:53 — set your machine's DNS to 127.0.0.1.
          </div>
        </div>
      )}
    </div>
  );
}
