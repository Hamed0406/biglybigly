export interface Module {
  id: string
  name: string
  version: string
  icon: string
}

export interface ModulePage {
  [key: string]: React.ComponentType<{ id: string }>
}
