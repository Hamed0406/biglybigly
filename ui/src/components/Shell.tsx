/**
 * Application chrome: dark sidebar (Dashboard + module list) plus a top bar
 * displaying the active module's name. Renders the routed page as `children`.
 */
import { Module } from '../types'

interface ShellProps {
  /** Modules registered on the server, used to populate the sidebar. */
  modules: Module[]
  /** Currently selected module id, or `null` for the Dashboard view. */
  currentModule: string | null
  /** Notifies the parent that the user picked a different sidebar entry. */
  onSelectModule: (id: string | null) => void
  /** The active page rendered in the main content area. */
  children: React.ReactNode
}

/** Sidebar + topbar layout shared by every authenticated page. */
export default function Shell({ modules, currentModule, onSelectModule, children }: ShellProps) {
  return (
    <div style={{ display: 'flex', height: '100vh', fontFamily: 'sans-serif' }}>
      {/* Sidebar */}
      <div
        style={{
          width: '250px',
          backgroundColor: '#1f2937',
          color: 'white',
          padding: '20px',
          overflowY: 'auto',
          borderRight: '1px solid #374151',
        }}
      >
        <h1
          style={{ fontSize: '20px', marginBottom: '30px', fontWeight: 'bold', cursor: 'pointer' }}
          onClick={() => onSelectModule(null)}
        >
          🏠 Biglybigly
        </h1>
        <nav style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <button
            onClick={() => onSelectModule(null)}
            style={{
              padding: '12px',
              backgroundColor: currentModule === null ? '#3b82f6' : 'transparent',
              border: 'none',
              color: 'white',
              textAlign: 'left',
              cursor: 'pointer',
              borderRadius: '6px',
              fontSize: '14px',
              fontWeight: currentModule === null ? 'bold' : 'normal',
              transition: 'background-color 0.2s',
            }}
            onMouseOver={(e) => {
              if (currentModule !== null) {
                (e.target as HTMLButtonElement).style.backgroundColor = '#374151'
              }
            }}
            onMouseOut={(e) => {
              if (currentModule !== null) {
                (e.target as HTMLButtonElement).style.backgroundColor = 'transparent'
              }
            }}
          >
            📊 Dashboard
          </button>
          {modules.map((mod) => (
            <button
              key={mod.id}
              onClick={() => onSelectModule(mod.id)}
              style={{
                padding: '12px',
                backgroundColor: currentModule === mod.id ? '#3b82f6' : 'transparent',
                border: 'none',
                color: 'white',
                textAlign: 'left',
                cursor: 'pointer',
                borderRadius: '6px',
                fontSize: '14px',
                fontWeight: currentModule === mod.id ? 'bold' : 'normal',
                transition: 'background-color 0.2s',
              }}
              onMouseOver={(e) => {
                if (currentModule !== mod.id) {
                  (e.target as HTMLButtonElement).style.backgroundColor = '#374151'
                }
              }}
              onMouseOut={(e) => {
                if (currentModule !== mod.id) {
                  (e.target as HTMLButtonElement).style.backgroundColor = 'transparent'
                }
              }}
            >
              {mod.icon && (
                <span
                  style={{
                    display: 'inline-block',
                    width: '16px',
                    height: '16px',
                    marginRight: '8px',
                    verticalAlign: 'middle',
                  }}
                  dangerouslySetInnerHTML={{ __html: mod.icon }}
                />
              )}
              {mod.name}
            </button>
          ))}
        </nav>
      </div>

      {/* Main content */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        {/* Top bar */}
        <div
          style={{
            height: '60px',
            backgroundColor: 'white',
            borderBottom: '1px solid #e5e7eb',
            padding: '0 20px',
            display: 'flex',
            alignItems: 'center',
            boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
          }}
        >
          <h2 style={{ fontSize: '18px', fontWeight: 'bold', color: '#1f2937' }}>
            {modules.find((m) => m.id === currentModule)?.name || 'Biglybigly'}
          </h2>
        </div>

        {/* Content */}
        <div style={{ flex: 1, overflow: 'auto', padding: '20px' }}>{children}</div>
      </div>
    </div>
  )
}
