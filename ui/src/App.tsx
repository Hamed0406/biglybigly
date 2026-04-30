import { useState, useEffect } from 'react'
import { getModules } from './api/client'
import { Module } from './types'
import Shell from './components/Shell'
import SetupPage from './components/SetupPage'
import URLCheckPage from './tools/urlcheck/URLCheckPage'
import NetMonPage from './tools/netmon/NetMonPage'

const modulePages: { [key: string]: React.ComponentType } = {
  urlcheck: URLCheckPage,
  netmon: NetMonPage,
}

export default function App() {
  const [modules, setModules] = useState<Module[]>([])
  const [currentModule, setCurrentModule] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [setupComplete, setSetupComplete] = useState<boolean | null>(null)

  useEffect(() => {
    checkSetup()
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
      // If setup endpoint fails, assume complete (backward compat)
      setSetupComplete(true)
      loadModules()
    }
  }

  const loadModules = async () => {
    try {
      const data = await getModules()
      setModules(data || [])
      if (data && data.length > 0) {
        setCurrentModule(data[0].id)
      }
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

  const CurrentPage = currentModule ? modulePages[currentModule] : null

  return (
    <Shell modules={modules} currentModule={currentModule} onSelectModule={setCurrentModule}>
      {CurrentPage ? <CurrentPage /> : <div style={{ padding: '20px' }}>Select a module</div>}
    </Shell>
  )
}
