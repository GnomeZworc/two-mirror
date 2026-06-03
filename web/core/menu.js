// Dynamic menu loader.
// Fetches data for groups declared with empty children in navigation.json
// and injects them into side-nav after it is ready.
//
// Imported non-blocking from index.html — does not delay routing.
// Replace MOCK_* with real api-client calls when the backend is ready.

// ── Mock data ────────────────────────────────────────────────────────────────

const MOCK_ACCOUNTS = [
  { id: 'tresorerie',   name: 'Trésorerie'   },
  { id: 'charges',      name: 'Charges'       },
  { id: 'produits',     name: 'Produits'      },
  { id: 'fournisseurs', name: 'Fournisseurs'  },
]

async function fetchAccounts() {
  // TODO: replace with real call
  // const api = document.querySelector('api-client')
  // return await api.finance.list('/accounts')
  await new Promise(r => setTimeout(r, 500))  // simulate network latency
  return MOCK_ACCOUNTS
}

// ── Injection ────────────────────────────────────────────────────────────────

const nav = document.querySelector('side-nav')
if (!nav) throw new Error('menu.js: side-nav not found in DOM')

await nav.ready

const accounts = await fetchAccounts()
nav.setGroupChildren('Comptes', accounts.map(a => ({
  label: a.name,
  href:  `#/account?id=${a.id}`,
  icon:  'account',
})))
