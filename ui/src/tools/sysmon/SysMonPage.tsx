import { useState, useEffect, useCallback } from 'react';
import {
  getSysmonCurrent, getSysmonHistory, getSysmonDisks, getSysmonAgents,
  SysmonSnapshot, SysmonDisk, SysmonAgent, HistoryPoint,
} from './api';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
}

function formatUptime(secs: number): string {
  const days = Math.floor(secs / 86400);
  const hours = Math.floor((secs % 86400) / 3600);
  const mins = Math.floor((secs % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function formatAgo(ts: number): string {
  if (!ts) return '-';
  const secs = Math.floor(Date.now() / 1000 - ts);
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  return `${Math.floor(secs / 3600)}h ago`;
}

// Simple SVG sparkline component
function Sparkline({ data, width = 300, height = 60, color = '#3b82f6' }: {
  data: number[]; width?: number; height?: number; color?: string;
}) {
  if (data.length < 2) return <div style={{ width, height, backgroundColor: '#f9fafb', borderRadius: 4 }} />;

  const max = Math.max(...data, 1);
  const min = Math.min(...data, 0);
  const range = max - min || 1;
  const stepX = width / (data.length - 1);

  const points = data.map((v, i) => {
    const x = i * stepX;
    const y = height - ((v - min) / range) * (height - 4) - 2;
    return `${x},${y}`;
  }).join(' ');

  const areaPoints = `0,${height} ${points} ${width},${height}`;

  return (
    <svg width={width} height={height} style={{ display: 'block' }}>
      <polygon points={areaPoints} fill={color} opacity="0.1" />
      <polyline points={points} fill="none" stroke={color} strokeWidth="2" />
    </svg>
  );
}

// Gauge component for CPU/Memory
function Gauge({ value, max, label, unit, color }: {
  value: number; max: number; label: string; unit: string; color: string;
}) {
  const pct = max > 0 ? (value / max) * 100 : 0;
  const displayVal = unit === '%' ? value.toFixed(1) : formatBytes(value);

  return (
    <div style={{
      backgroundColor: 'white', borderRadius: 8, padding: 16,
      boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
    }}>
      <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 8 }}>{label}</div>
      <div style={{ fontSize: 28, fontWeight: 'bold', color }}>{displayVal}</div>
      <div style={{
        height: 8, backgroundColor: '#e5e7eb', borderRadius: 4, marginTop: 8, overflow: 'hidden',
      }}>
        <div style={{
          height: '100%', backgroundColor: color, borderRadius: 4,
          width: `${Math.min(pct, 100)}%`,
          transition: 'width 0.5s ease',
        }} />
      </div>
      <div style={{ fontSize: 11, color: '#9ca3af', marginTop: 4 }}>
        {pct.toFixed(1)}%{max > 0 && unit !== '%' ? ` of ${formatBytes(max)}` : ''}
      </div>
    </div>
  );
}

export default function SysMonPage() {
  const [snapshots, setSnapshots] = useState<SysmonSnapshot[]>([]);
  const [history, setHistory] = useState<HistoryPoint[]>([]);
  const [disks, setDisks] = useState<SysmonDisk[]>([]);
  const [agents, setAgents] = useState<SysmonAgent[]>([]);
  const [selectedAgent, setSelectedAgent] = useState('');
  const [hours, setHours] = useState(1);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [loading, setLoading] = useState(false);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const agent = selectedAgent || undefined;
      const [cur, hist, dsk, ags] = await Promise.all([
        getSysmonCurrent(agent),
        getSysmonHistory(agent, hours),
        getSysmonDisks(agent),
        getSysmonAgents(),
      ]);
      setSnapshots(cur);
      setHistory(hist);
      setDisks(dsk);
      setAgents(ags);
    } catch (err) {
      console.error('Failed to load sysmon data:', err);
    } finally {
      setLoading(false);
    }
  }, [selectedAgent, hours]);

  useEffect(() => { loadData(); }, [loadData]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, [autoRefresh, loadData]);

  // Pick snapshot to display (first one, or selected agent's)
  const snap = snapshots.length > 0 ? snapshots[0] : null;
  const cpuColor = snap && snap.cpu_percent > 80 ? '#ef4444' : snap && snap.cpu_percent > 50 ? '#f59e0b' : '#22c55e';
  const memPct = snap && snap.mem_total > 0 ? (snap.mem_used / snap.mem_total) * 100 : 0;
  const memColor = memPct > 85 ? '#ef4444' : memPct > 60 ? '#f59e0b' : '#3b82f6';

  return (
    <div style={{ fontFamily: 'sans-serif' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <h1 style={{ fontSize: 24, fontWeight: 'bold' }}>System Monitor</h1>
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
            }}>
              All
            </button>
          )}
          {agents.map((a) => (
            <button key={a.name} onClick={() => setSelectedAgent(a.name)} style={{
              padding: '4px 12px', border: 'none', borderRadius: 16, cursor: 'pointer', fontSize: 13,
              fontWeight: selectedAgent === a.name ? 'bold' : 'normal',
              backgroundColor: selectedAgent === a.name ? '#3b82f6' : '#e5e7eb',
              color: selectedAgent === a.name ? 'white' : '#374151',
            }}>
              {a.name}
              <span style={{ fontSize: 11, opacity: 0.7, marginLeft: 4 }}>
                ({formatAgo(a.last_active)})
              </span>
            </button>
          ))}
        </div>
      )}

      {snap ? (
        <>
          {/* System info bar */}
          <div style={{
            backgroundColor: 'white', borderRadius: 8, padding: '12px 16px', marginBottom: 16,
            boxShadow: '0 1px 3px rgba(0,0,0,0.1)', display: 'flex', gap: 24, fontSize: 13, color: '#6b7280',
          }}>
            <span>🖥 <strong>{snap.hostname}</strong></span>
            <span>📦 {snap.os_info}</span>
            <span>⏱ Uptime: {formatUptime(snap.uptime_secs)}</span>
            {snap.load1 > 0 && <span>📊 Load: {snap.load1.toFixed(2)} / {snap.load5.toFixed(2)} / {snap.load15.toFixed(2)}</span>}
            <span style={{ marginLeft: 'auto' }}>Last update: {formatAgo(snap.collected_at)}</span>
          </div>

          {/* Gauges */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 20 }}>
            <Gauge value={snap.cpu_percent} max={100} label="CPU Usage" unit="%" color={cpuColor} />
            <Gauge value={snap.mem_used} max={snap.mem_total} label="Memory Used" unit="bytes" color={memColor} />
            <Gauge value={snap.mem_available} max={snap.mem_total} label="Memory Available" unit="bytes" color="#22c55e" />
            <div style={{
              backgroundColor: 'white', borderRadius: 8, padding: 16,
              boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
            }}>
              <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 8 }}>Disks</div>
              <div style={{ fontSize: 28, fontWeight: 'bold', color: '#8b5cf6' }}>{disks.length}</div>
              <div style={{ fontSize: 11, color: '#9ca3af', marginTop: 12 }}>
                Total: {formatBytes(disks.reduce((s, d) => s + d.total_bytes, 0))}
              </div>
            </div>
          </div>

          {/* History charts */}
          <div style={{
            backgroundColor: 'white', borderRadius: 8, padding: 16, marginBottom: 16,
            boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
          }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <h2 style={{ fontSize: 16, fontWeight: 'bold' }}>History</h2>
              <div style={{ display: 'flex', gap: 4 }}>
                {[1, 6, 12, 24].map((h) => (
                  <button key={h} onClick={() => setHours(h)} style={{
                    padding: '4px 10px', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12,
                    backgroundColor: hours === h ? '#3b82f6' : '#e5e7eb',
                    color: hours === h ? 'white' : '#374151',
                  }}>
                    {h}h
                  </button>
                ))}
              </div>
            </div>

            {history.length > 1 ? (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                <div>
                  <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 4 }}>CPU %</div>
                  <Sparkline data={history.map(p => p.cpu_percent)} color="#22c55e" width={400} height={80} />
                </div>
                <div>
                  <div style={{ fontSize: 13, color: '#6b7280', marginBottom: 4 }}>
                    Memory Used ({history.length > 0 ? formatBytes(history[history.length - 1].mem_total) : ''} total)
                  </div>
                  <Sparkline
                    data={history.map(p => p.mem_total > 0 ? (p.mem_used / p.mem_total) * 100 : 0)}
                    color="#3b82f6" width={400} height={80}
                  />
                </div>
              </div>
            ) : (
              <p style={{ color: '#9ca3af', textAlign: 'center', padding: 20 }}>
                Waiting for history data... metrics are collected every 30 seconds.
              </p>
            )}
          </div>

          {/* Disk usage */}
          {disks.length > 0 && (
            <div style={{
              backgroundColor: 'white', borderRadius: 8, padding: 16,
              boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
            }}>
              <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12 }}>Disk Usage</h2>
              {disks.map((d, i) => {
                const pct = d.total_bytes > 0 ? (d.used_bytes / d.total_bytes) * 100 : 0;
                const diskColor = pct > 90 ? '#ef4444' : pct > 75 ? '#f59e0b' : '#8b5cf6';
                return (
                  <div key={`${d.agent_name}-${d.mount_point}-${i}`}
                    style={{ marginBottom: 12, paddingBottom: 12, borderBottom: '1px solid #f3f4f6' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                      <span style={{ fontFamily: 'monospace', fontSize: 13 }}>
                        {d.mount_point}
                        {d.fs_type && <span style={{ color: '#9ca3af', marginLeft: 8 }}>({d.fs_type})</span>}
                        {agents.length > 1 && <span style={{ color: '#6b7280', marginLeft: 8 }}>— {d.agent_name}</span>}
                      </span>
                      <span style={{ fontSize: 13, color: diskColor, fontWeight: 'bold' }}>
                        {pct.toFixed(1)}% used
                      </span>
                    </div>
                    <div style={{ height: 8, backgroundColor: '#e5e7eb', borderRadius: 4, overflow: 'hidden' }}>
                      <div style={{
                        height: '100%', backgroundColor: diskColor, borderRadius: 4,
                        width: `${pct}%`, transition: 'width 0.5s ease',
                      }} />
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: '#9ca3af', marginTop: 2 }}>
                      <span>{formatBytes(d.used_bytes)} used</span>
                      <span>{formatBytes(d.avail_bytes)} free of {formatBytes(d.total_bytes)}</span>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </>
      ) : (
        <div style={{
          backgroundColor: 'white', borderRadius: 8, padding: 60,
          boxShadow: '0 1px 3px rgba(0,0,0,0.1)', textAlign: 'center', color: '#9ca3af',
        }}>
          <div style={{ fontSize: 48, marginBottom: 16 }}>💻</div>
          <div style={{ fontSize: 18, fontWeight: 'bold', color: '#374151', marginBottom: 8 }}>
            No system data yet
          </div>
          <div>
            Connect an agent to start monitoring system metrics.
            <br />Data will appear automatically once an agent reports in.
          </div>
        </div>
      )}

      {/* Multi-agent overview (when showing all) */}
      {!selectedAgent && snapshots.length > 1 && (
        <div style={{
          backgroundColor: 'white', borderRadius: 8, padding: 16, marginTop: 16,
          boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
        }}>
          <h2 style={{ fontSize: 16, fontWeight: 'bold', marginBottom: 12 }}>All Agents</h2>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '2px solid #e5e7eb', textAlign: 'left' }}>
                <th style={{ padding: '8px 6px' }}>Agent</th>
                <th style={{ padding: '8px 6px' }}>Hostname</th>
                <th style={{ padding: '8px 6px' }}>OS</th>
                <th style={{ padding: '8px 6px' }}>CPU</th>
                <th style={{ padding: '8px 6px' }}>Memory</th>
                <th style={{ padding: '8px 6px' }}>Uptime</th>
                <th style={{ padding: '8px 6px' }}>Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {snapshots.map((s) => {
                const memPct = s.mem_total > 0 ? (s.mem_used / s.mem_total) * 100 : 0;
                return (
                  <tr key={s.agent_name} style={{ borderBottom: '1px solid #f3f4f6', cursor: 'pointer' }}
                    onClick={() => setSelectedAgent(s.agent_name)}>
                    <td style={{ padding: 6, fontWeight: 'bold' }}>{s.agent_name}</td>
                    <td style={{ padding: 6, fontFamily: 'monospace' }}>{s.hostname}</td>
                    <td style={{ padding: 6, fontSize: 12, color: '#6b7280' }}>{s.os_info}</td>
                    <td style={{ padding: 6 }}>
                      <span style={{
                        padding: '2px 8px', borderRadius: 10, fontSize: 11, fontWeight: 'bold',
                        backgroundColor: s.cpu_percent > 80 ? '#fef2f2' : s.cpu_percent > 50 ? '#fffbeb' : '#f0fdf4',
                        color: s.cpu_percent > 80 ? '#ef4444' : s.cpu_percent > 50 ? '#f59e0b' : '#22c55e',
                      }}>
                        {s.cpu_percent.toFixed(1)}%
                      </span>
                    </td>
                    <td style={{ padding: 6 }}>
                      <span style={{
                        padding: '2px 8px', borderRadius: 10, fontSize: 11, fontWeight: 'bold',
                        backgroundColor: memPct > 85 ? '#fef2f2' : memPct > 60 ? '#fffbeb' : '#eff6ff',
                        color: memPct > 85 ? '#ef4444' : memPct > 60 ? '#f59e0b' : '#3b82f6',
                      }}>
                        {memPct.toFixed(1)}%
                      </span>
                      <span style={{ fontSize: 11, color: '#9ca3af', marginLeft: 4 }}>
                        {formatBytes(s.mem_used)} / {formatBytes(s.mem_total)}
                      </span>
                    </td>
                    <td style={{ padding: 6, fontSize: 12, color: '#6b7280' }}>{formatUptime(s.uptime_secs)}</td>
                    <td style={{ padding: 6, fontSize: 12, color: '#6b7280' }}>{formatAgo(s.collected_at)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
