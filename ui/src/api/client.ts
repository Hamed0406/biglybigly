/**
 * Platform-level API helpers (modules, agents, setup, etc.).
 * Module-specific calls live under `src/tools/<id>/api.ts`.
 */
import { Module } from '../types'

/** Fetches the list of modules registered on the server. */
export async function getModules(): Promise<Module[]> {
  const res = await fetch('/api/modules')
  if (!res.ok) throw new Error('Failed to get modules')
  const data = await res.json()
  return data || []
}
