/**
 * Shared TypeScript types that mirror the Go JSON structs returned by the
 * platform API. Keep these in sync with the corresponding Go definitions.
 */

/** Metadata for a registered module, as returned by `/api/modules`. */
export interface Module {
  id: string
  name: string
  version: string
  icon: string
}

/** Map from module id to its lazy-loaded React page component. */
export interface ModulePage {
  [key: string]: React.ComponentType<{ id: string }>
}
