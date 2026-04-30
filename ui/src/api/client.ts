import { Module } from '../types'

export async function getModules(): Promise<Module[]> {
  const res = await fetch('/api/modules')
  if (!res.ok) throw new Error('Failed to get modules')
  const data = await res.json()
  return data || []
}
