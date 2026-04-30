import { useState, useEffect } from 'react'
import { getModules } from './api/client'
import { Module } from './types'
import Shell from './components/Shell'
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

  useEffect(() => {
    loadModules()
  }, [])

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

  const CurrentPage = currentModule ? modulePages[currentModule] : null

  return (
    <Shell modules={modules} currentModule={currentModule} onSelectModule={setCurrentModule}>
      {CurrentPage ? <CurrentPage /> : <div style={{ padding: '20px' }}>Select a module</div>}
    </Shell>
  )
}
