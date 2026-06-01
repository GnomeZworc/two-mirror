// Auth service component.
//
// Declare before api-client in the shell:
//   <login-gate></login-gate>
//
// Exposes:
//   gate.ready          → Promise<credentials> — awaited by api-client before any call
//   gate.credentials    → current credentials or null
//   gate.logout()       → clears session and reloads
//
// Credentials shape (extensible for Phase 2):
//   {
//     netbox_url:   string,
//     netbox_token: string,
//     agent_url:    string,
//   }
//
// Phase 2: swap for <oidc-gate> that resolves ready with a JWT — api-client unchanged.

const SESSION_KEY = 'two:credentials'
const CSS         = new URL('./login-gate.css', import.meta.url).href

class LoginGate extends HTMLElement {
  #resolve = null

  get credentials() {
    const raw = sessionStorage.getItem(SESSION_KEY)
    return raw ? JSON.parse(raw) : null
  }

  logout() {
    sessionStorage.removeItem(SESSION_KEY)
    window.location.reload()
  }

  async connectedCallback() {
    this.style.display = 'none'

    this.ready = new Promise(resolve => { this.#resolve = resolve })

    const stored = this.credentials
    if (stored) {
      this.#resolve(stored)
      return
    }

    // Load config.json defaults to pre-fill the form
    let defaults = {}
    try {
      const r = await fetch('./config.json')
      if (r.ok) defaults = await r.json()
    } catch { /* no defaults */ }

    this.#renderOverlay(defaults)
  }

  #renderOverlay(defaults) {
    const overlay = document.createElement('div')
    overlay.attachShadow({ mode: 'open' })
    overlay.shadowRoot.innerHTML = `
      <link rel="stylesheet" href="${CSS}">
      <div class="overlay">
        <div class="card">
          <div class="header">
            <h1>two — connect</h1>
            <p>Enter your Netbox and agent details to continue.</p>
          </div>

          <form class="fields" id="form" autocomplete="on">
            <div class="field">
              <label>Netbox URL</label>
              <input id="netbox_url" type="url" placeholder="http://netbox.local"
                     value="${defaults.netbox_url ?? ''}" required />
            </div>
            <div class="field">
              <label>Netbox Token</label>
              <input id="netbox_token" type="password" placeholder="your-api-token"
                     value="${defaults.netbox_token ?? ''}" required />
            </div>
            <div class="field">
              <label>Agent URL</label>
              <input id="agent_url" type="url" placeholder="http://127.0.0.1:8080"
                     value="${defaults.agent_url ?? ''}" required />
            </div>

            <button type="submit">Connect</button>
          </form>
        </div>
      </div>
    `

    document.body.appendChild(overlay)

    const form = overlay.shadowRoot.getElementById('form')

    form.addEventListener('submit', e => {
      e.preventDefault()

      const credentials = {
        netbox_url:   overlay.shadowRoot.getElementById('netbox_url').value.replace(/\/$/, ''),
        netbox_token: overlay.shadowRoot.getElementById('netbox_token').value.trim(),
        agent_url:    overlay.shadowRoot.getElementById('agent_url').value.replace(/\/$/, ''),
      }

      sessionStorage.setItem(SESSION_KEY, JSON.stringify(credentials))
      overlay.remove()
      this.#resolve(credentials)
    })
  }
}

customElements.define('login-gate', LoginGate)
