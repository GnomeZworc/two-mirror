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
      <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }

        .overlay {
          position: fixed;
          inset: 0;
          background: #181825;
          display: flex;
          align-items: center;
          justify-content: center;
          z-index: 9999;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
        }

        .card {
          background: #1e1e2e;
          border: 1px solid #313244;
          border-radius: 12px;
          padding: 36px 40px;
          width: 100%;
          max-width: 420px;
          display: flex;
          flex-direction: column;
          gap: 24px;
        }

        .header {
          display: flex;
          flex-direction: column;
          gap: 6px;
        }

        .header h1 {
          font-size: 20px;
          font-weight: 600;
          color: #89b4fa;
          letter-spacing: 0.02em;
        }

        .header p {
          font-size: 13px;
          color: #6c7086;
        }

        .fields {
          display: flex;
          flex-direction: column;
          gap: 14px;
        }

        .field {
          display: flex;
          flex-direction: column;
          gap: 6px;
        }

        label {
          font-size: 12px;
          font-weight: 500;
          color: #a6adc8;
          text-transform: uppercase;
          letter-spacing: 0.06em;
        }

        input {
          background: #181825;
          border: 1px solid #313244;
          border-radius: 6px;
          padding: 9px 12px;
          color: #cdd6f4;
          font-size: 14px;
          font-family: monospace;
          outline: none;
          transition: border-color 0.15s;
          width: 100%;
        }

        input:focus {
          border-color: #89b4fa;
        }

        input::placeholder {
          color: #45475a;
        }

        button {
          background: #89b4fa;
          color: #1e1e2e;
          border: none;
          border-radius: 6px;
          padding: 10px 16px;
          font-size: 14px;
          font-weight: 600;
          cursor: pointer;
          transition: background 0.15s, opacity 0.15s;
          width: 100%;
        }

        button:hover { background: #b4d0fa; }
      </style>

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
