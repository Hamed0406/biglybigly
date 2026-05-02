/**
 * First-run setup wizard.
 *
 * Walks the operator through:
 *   1. Pasting the bootstrap token (printed to the server's stdout) to
 *      authenticate the setup request.
 *   2. Choosing a run mode — `server` (full UI + DB) or `agent` (collector
 *      that reports to a remote server).
 *   3. Configuring the instance name and, for agent mode, the server URL.
 *
 * Successful server-mode setup signals completion via `onComplete`. Agent-mode
 * setup shows a final step instructing the operator to restart the process so
 * it can re-launch in agent mode.
 */
import { useState } from 'react'

interface SetupPageProps {
  /** Called when server-mode setup succeeds; the app then loads normally. */
  onComplete: () => void
}

/** Multi-step first-run configuration wizard. */
export default function SetupPage({ onComplete }: SetupPageProps) {
  const [step, setStep] = useState(1)
  const [mode, setMode] = useState<'server' | 'agent'>('server')
  const [serverUrl, setServerUrl] = useState('')
  const [instanceName, setInstanceName] = useState('')
  const [bootstrapToken, setBootstrapToken] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const handleSubmit = async () => {
    setError('')

    if (!bootstrapToken.trim()) {
      setError('Bootstrap token is required (check your terminal output)')
      return
    }

    if (mode === 'agent' && !serverUrl.trim()) {
      setError('Server URL is required for agent mode')
      return
    }

    setSaving(true)
    try {
      const resp = await fetch('/api/setup/complete', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Bootstrap-Token': bootstrapToken,
        },
        body: JSON.stringify({
          mode,
          server_url: serverUrl,
          instance_name: instanceName || 'biglybigly',
        }),
      })

      const data = await resp.json()

      if (!resp.ok) {
        setError(data.error || 'Setup failed')
        return
      }

      if (mode === 'agent') {
        setStep(4) // show restart message
      } else {
        onComplete()
      }
    } catch (err) {
      setError('Failed to save configuration')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={styles.container}>
      <div style={styles.card}>
        <div style={styles.logo}>
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="#3b82f6" strokeWidth="2">
            <path d="M12 2L2 7l10 5 10-5-10-5z" />
            <path d="M2 17l10 5 10-5" />
            <path d="M2 12l10 5 10-5" />
          </svg>
        </div>
        <h1 style={styles.title}>Welcome to Biglybigly</h1>
        <p style={styles.subtitle}>Let's set up your instance</p>

        {/* Step indicator */}
        <div style={styles.steps}>
          {[1, 2, 3].map(s => (
            <div key={s} style={{
              ...styles.stepDot,
              backgroundColor: step >= s ? '#3b82f6' : '#374151',
            }} />
          ))}
        </div>

        {step === 1 && (
          <div style={styles.stepContent}>
            <h2 style={styles.stepTitle}>Bootstrap Token</h2>
            <p style={styles.hint}>
              Check your terminal — a bootstrap token was printed when the server started.
              This prevents unauthorized setup.
            </p>
            <input
              type="text"
              placeholder="Paste bootstrap token here"
              value={bootstrapToken}
              onChange={e => setBootstrapToken(e.target.value)}
              style={styles.input}
              autoFocus
            />
            <button
              style={styles.button}
              onClick={() => {
                if (!bootstrapToken.trim()) {
                  setError('Token is required')
                  return
                }
                setError('')
                setStep(2)
              }}
            >
              Next →
            </button>
          </div>
        )}

        {step === 2 && (
          <div style={styles.stepContent}>
            <h2 style={styles.stepTitle}>Choose Mode</h2>
            <p style={styles.hint}>How will this instance run?</p>

            <div style={styles.modeCards}>
              <div
                style={{
                  ...styles.modeCard,
                  borderColor: mode === 'server' ? '#3b82f6' : '#374151',
                }}
                onClick={() => setMode('server')}
              >
                <div style={styles.modeIcon}>🖥️</div>
                <h3 style={styles.modeTitle}>Server</h3>
                <p style={styles.modeDesc}>
                  Full platform with UI, database, and agent management. 
                  Central hub that agents connect to.
                </p>
              </div>
              <div
                style={{
                  ...styles.modeCard,
                  borderColor: mode === 'agent' ? '#3b82f6' : '#374151',
                }}
                onClick={() => setMode('agent')}
              >
                <div style={styles.modeIcon}>📡</div>
                <h3 style={styles.modeTitle}>Agent</h3>
                <p style={styles.modeDesc}>
                  Lightweight collector that monitors this host and 
                  sends data to a central server.
                </p>
              </div>
            </div>

            <button style={styles.button} onClick={() => setStep(3)}>
              Next →
            </button>
            <button style={styles.backButton} onClick={() => setStep(1)}>
              ← Back
            </button>
          </div>
        )}

        {step === 3 && (
          <div style={styles.stepContent}>
            <h2 style={styles.stepTitle}>Configuration</h2>

            <label style={styles.label}>Instance Name</label>
            <input
              type="text"
              placeholder="e.g., office-london, home-lab"
              value={instanceName}
              onChange={e => setInstanceName(e.target.value)}
              style={styles.input}
            />

            {mode === 'agent' && (
              <>
                <label style={styles.label}>Server URL</label>
                <input
                  type="url"
                  placeholder="http://your-server:8082"
                  value={serverUrl}
                  onChange={e => setServerUrl(e.target.value)}
                  style={styles.input}
                />
                <p style={styles.hint}>
                  The address of the Biglybigly server this agent will report to.
                </p>
              </>
            )}

            <button
              style={styles.button}
              onClick={handleSubmit}
              disabled={saving}
            >
              {saving ? 'Saving...' : 'Complete Setup ✓'}
            </button>
            <button style={styles.backButton} onClick={() => setStep(2)}>
              ← Back
            </button>
          </div>
        )}

        {step === 4 && (
          <div style={styles.stepContent}>
            <div style={{ fontSize: '48px', marginBottom: '16px' }}>✅</div>
            <h2 style={styles.stepTitle}>Setup Complete</h2>
            <p style={styles.hint}>
              Agent mode configured. Restart Biglybigly to connect to:
            </p>
            <code style={styles.code}>{serverUrl}</code>
            <p style={styles.hint}>
              Run: <code style={styles.inlineCode}>systemctl restart biglybigly</code> or restart the Docker container.
            </p>
          </div>
        )}

        {error && <div style={styles.error}>{error}</div>}
      </div>
    </div>
  )
}

const styles: { [key: string]: React.CSSProperties } = {
  container: {
    minHeight: '100vh',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: '#0f172a',
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  },
  card: {
    backgroundColor: '#1e293b',
    borderRadius: '12px',
    padding: '48px',
    maxWidth: '520px',
    width: '100%',
    textAlign: 'center' as const,
    boxShadow: '0 25px 50px rgba(0,0,0,0.25)',
  },
  logo: {
    marginBottom: '16px',
  },
  title: {
    color: '#f1f5f9',
    fontSize: '24px',
    fontWeight: '700',
    margin: '0 0 8px 0',
  },
  subtitle: {
    color: '#94a3b8',
    fontSize: '14px',
    margin: '0 0 24px 0',
  },
  steps: {
    display: 'flex',
    justifyContent: 'center',
    gap: '8px',
    marginBottom: '32px',
  },
  stepDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
    transition: 'background-color 0.2s',
  },
  stepContent: {
    textAlign: 'left' as const,
  },
  stepTitle: {
    color: '#f1f5f9',
    fontSize: '18px',
    fontWeight: '600',
    margin: '0 0 8px 0',
  },
  hint: {
    color: '#94a3b8',
    fontSize: '13px',
    margin: '0 0 16px 0',
    lineHeight: '1.5',
  },
  label: {
    display: 'block',
    color: '#cbd5e1',
    fontSize: '13px',
    fontWeight: '500',
    marginBottom: '6px',
  },
  input: {
    width: '100%',
    padding: '10px 12px',
    backgroundColor: '#0f172a',
    border: '1px solid #374151',
    borderRadius: '6px',
    color: '#f1f5f9',
    fontSize: '14px',
    marginBottom: '16px',
    outline: 'none',
    boxSizing: 'border-box' as const,
  },
  button: {
    width: '100%',
    padding: '12px',
    backgroundColor: '#3b82f6',
    color: 'white',
    border: 'none',
    borderRadius: '6px',
    fontSize: '14px',
    fontWeight: '600',
    cursor: 'pointer',
    marginTop: '8px',
  },
  backButton: {
    width: '100%',
    padding: '10px',
    backgroundColor: 'transparent',
    color: '#94a3b8',
    border: '1px solid #374151',
    borderRadius: '6px',
    fontSize: '13px',
    cursor: 'pointer',
    marginTop: '8px',
  },
  modeCards: {
    display: 'flex',
    gap: '12px',
    marginBottom: '16px',
  },
  modeCard: {
    flex: 1,
    padding: '16px',
    backgroundColor: '#0f172a',
    border: '2px solid #374151',
    borderRadius: '8px',
    cursor: 'pointer',
    transition: 'border-color 0.2s',
  },
  modeIcon: {
    fontSize: '28px',
    marginBottom: '8px',
  },
  modeTitle: {
    color: '#f1f5f9',
    fontSize: '15px',
    fontWeight: '600',
    margin: '0 0 4px 0',
  },
  modeDesc: {
    color: '#94a3b8',
    fontSize: '12px',
    margin: 0,
    lineHeight: '1.4',
  },
  error: {
    marginTop: '16px',
    padding: '10px',
    backgroundColor: '#7f1d1d',
    color: '#fecaca',
    borderRadius: '6px',
    fontSize: '13px',
  },
  code: {
    display: 'block',
    padding: '12px',
    backgroundColor: '#0f172a',
    color: '#60a5fa',
    borderRadius: '6px',
    fontSize: '14px',
    fontFamily: 'monospace',
    marginBottom: '16px',
  },
  inlineCode: {
    backgroundColor: '#0f172a',
    color: '#60a5fa',
    padding: '2px 6px',
    borderRadius: '3px',
    fontFamily: 'monospace',
    fontSize: '13px',
  },
}
