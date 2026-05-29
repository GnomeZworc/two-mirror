// Single source of truth for configuration.
// Phase 2: replace the fetch with a call to the orchestrator.
const response = await fetch('./config.json')
if (!response.ok) throw new Error('Failed to load config.json')

export const config = await response.json()
