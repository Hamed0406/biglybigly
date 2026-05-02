/**
 * Application root.
 *
 * Responsibilities:
 *  - Run the first-run setup check (`/api/setup/status`); show <SetupPage /> if
 *    setup hasn't been completed yet.
 *  - Load the list of registered modules from `/api/modules` and pass them to
 *    the <Shell /> for rendering the sidebar.
 *  - Route between the dashboard (when no module is selected) and the selected
 *    module's page using a static id → component map.
 */
import { useState, useEffect } from 'react'
import { getModules } from './api/client'
import { Module } from './types'
import Shell from './components/Shell'
import SetupPage from './components/SetupPage'
import DashboardPage from './components/DashboardPage'
import URLCheckPage from './tools/urlcheck/URLCheckPage'
import NetMonPage from './tools/netmon/NetMonPage'
import SysMonPage from './tools/sysmon/SysMonPage'
import DNSFilterPage from './tools/dnsfilter/DNSFilterPage'

// Static mapping of module id → page component. Adding a module to the platform
// requires adding a corresponding entry here.
const modulePages: { [key: string]: React.ComponentType } = {
  urlcheck: URLCheckPage,
  netmon: NetMonPage,
  sysmon: SysMonPage,
  dnsfilter: DNSFilterPage,
}

/** Top-level component that wires setup, module loading, and navigation. */
export default function App() {
  const [modules, setModules] = useState<Module[]>([])
  const [currentModule, setCurrentModule] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [setupComplete, setSetupComplete] = useState<boolean | null>(null)

  useEffect(() => {
    checkSetup()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const checkSetup = async () => {
    try {
      const resp = await fetch('/api/setup/status')
      const data = await resp.json()
      setSetupComplete(data.setup_complete)
      if (data.setup_complete) {
        loadModules()
      } else {
        setLoading(false)
      }
    } catch {
      // If the setup endpoint is missing (older server build), assume the
      // instance is already configured rather than blocking the UI.
      setSetupComplete(true)
      loadModules()
    }
  }

  const loadModules = async () => {
    try {
      const data = await getModules()
      setModules(data || [])
    } catch (err) {
      console.error('Failed to load modules:', err)
    } finally {
      setLoading(false)
    }
  }

  if (loading) {
    return <div style={{ padding: '20px' }}>Loading...</div>
  }

  if (!setupComplete) {
    return <SetupPage onComplete={() => {
      setSetupComplete(true)
      setLoading(true)
      loadModules()
    }} />
  }

  // currentModule === null means "Dashboard" (the home view).
  const CurrentPage = currentModule ? modulePages[currentModule] : null

  return (
    <Shell modules={modules} currentModule={currentModule} onSelectModule={setCurrentModule}>
      {CurrentPage ? <CurrentPage /> : <DashboardPage />}
    </Shell>
  )
}
