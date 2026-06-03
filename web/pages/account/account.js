// Account page.
// Reads ?id=<account-id> from the URL hash and renders account details.
// Replace fetchAccount() with a real api-client call when backend is ready.

// ── Mock data ────────────────────────────────────────────────────────────────

const MOCK_DB = {
  tresorerie: {
    name: 'Trésorerie',
    balance: 48_230.50,
    currency: 'EUR',
    transactions: [
      { date: '2026-06-03', label: 'Virement client Martin SA',    amount: +12_000.00 },
      { date: '2026-06-01', label: 'Virement client Dupont SARL',  amount:  +5_000.00 },
      { date: '2026-05-30', label: 'Loyer bureau juin',            amount:  -2_500.00 },
      { date: '2026-05-28', label: 'Facture EDF',                  amount:    -340.20 },
      { date: '2026-05-25', label: 'Abonnement SaaS infra',        amount:    -189.00 },
    ],
  },
  charges: {
    name: 'Charges',
    balance: 12_890.00,
    currency: 'EUR',
    transactions: [
      { date: '2026-06-01', label: 'Salaires mai',                 amount:  -8_500.00 },
      { date: '2026-05-28', label: 'Assurance professionnelle',    amount:    -420.00 },
      { date: '2026-05-20', label: 'Formation équipe',             amount:  -1_200.00 },
    ],
  },
  produits: {
    name: 'Produits',
    balance: 67_450.00,
    currency: 'EUR',
    transactions: [
      { date: '2026-06-02', label: 'Facture client #2024-089',     amount: +18_000.00 },
      { date: '2026-05-29', label: 'Facture client #2024-088',     amount:  +9_500.00 },
      { date: '2026-05-22', label: 'Avoir client Dupont',          amount:    -500.00 },
    ],
  },
  fournisseurs: {
    name: 'Fournisseurs',
    balance: -8_340.00,
    currency: 'EUR',
    transactions: [
      { date: '2026-06-01', label: 'Facture Tech Components SAS',  amount:  -3_200.00 },
      { date: '2026-05-27', label: 'Règlement fournisseur Leroy',  amount:  +2_000.00 },
      { date: '2026-05-20', label: 'Facture Bureau & Co',          amount:    -640.00 },
    ],
  },
}

async function fetchAccount(id) {
  // TODO: replace with real call
  // const api = document.querySelector('api-client')
  // return await api.finance.get(`/accounts/${id}`)
  await new Promise(r => setTimeout(r, 300))
  const data = MOCK_DB[id]
  if (!data) throw new Error(`Compte introuvable : ${id}`)
  return data
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function formatAmount(amount, currency) {
  return new Intl.NumberFormat('fr-FR', { style: 'currency', currency }).format(amount)
}

function formatDate(iso) {
  return new Intl.DateTimeFormat('fr-FR', { day: '2-digit', month: 'short', year: 'numeric' }).format(new Date(iso))
}

// ── Init ─────────────────────────────────────────────────────────────────────

export function init(main) {
  const params  = new URLSearchParams(window.location.hash.split('?')[1])
  const id      = params.get('id')

  const loading = main.querySelector('#account-loading')
  const error   = main.querySelector('#account-error')

  if (!id) {
    loading.style.display = 'none'
    error.style.display   = 'block'
    error.textContent     = 'Aucun compte sélectionné.'
    return
  }

  fetchAccount(id)
    .then(data  => render(main, id, data))
    .catch(err  => {
      loading.style.display = 'none'
      error.style.display   = 'block'
      error.textContent     = err.message
    })
}

function render(main, id, data) {
  main.querySelector('#account-loading').style.display = 'none'

  main.querySelector('#account-name').textContent = data.name
  main.querySelector('#account-id').textContent   = id

  const balanceEl = main.querySelector('#account-balance')
  balanceEl.textContent = formatAmount(data.balance, data.currency)
  balanceEl.classList.toggle('negative', data.balance < 0)

  const tbody = main.querySelector('#tx-body')
  tbody.innerHTML = data.transactions.map(tx => `
    <tr>
      <td class="date">${formatDate(tx.date)}</td>
      <td>${tx.label}</td>
      <td class="amount ${tx.amount >= 0 ? 'positive' : 'negative'}">
        ${formatAmount(tx.amount, data.currency)}
      </td>
    </tr>
  `).join('')
}
