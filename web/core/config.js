const response = await fetch('./config.json')
if (!response.ok) throw new Error('Failed to load config.json')

export const config = await response.json()
