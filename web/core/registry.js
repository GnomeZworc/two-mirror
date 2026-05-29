// Loads and registers all components listed in components.json.
// To add a component: git clone into components/, add the path here.
const response = await fetch('./components.json')
if (!response.ok) throw new Error('Failed to load components.json')

const components = await response.json()

for (const path of components) {
  await import(`../${path}`)
}
