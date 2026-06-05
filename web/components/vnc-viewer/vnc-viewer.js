// VNC viewer component — wraps noVNC RFB.
//
// Usage (in a side-panel or page):
//   <vnc-viewer vm="i-test1"></vnc-viewer>
//
// Connects to ws://<agent-host>:9000/vnc/<vm-name> via the VNC gateway (cmd/vncd).
// Agent host is read from login-gate credentials; falls back to location.hostname.
//
// Attributes:
//   vm       — VM name (required)
//   port     — gateway port (default: 9000)

import RFB from '../../vendor/novnc/core/rfb.js'

const RECONNECT_DELAY = 3000  // ms between auto-reconnect attempts

class VncViewer extends HTMLElement {
  #rfb      = null
  #retryTimer = null

  connectedCallback() {
    this.attachShadow({ mode: 'open' })
    this.#render()
    this.#connect()
  }

  disconnectedCallback() {
    clearTimeout(this.#retryTimer)
    this.#rfb?.disconnect()
    this.#rfb = null
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  #render() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: flex;
          flex-direction: column;
          width: 100%;
          height: 100%;
          min-height: 400px;
          background: #000;
          border-radius: 6px;
          overflow: hidden;
        }
        #screen { flex: 1; overflow: hidden; }
        #statusbar {
          display: flex;
          align-items: center;
          gap: 8px;
          padding: 6px 12px;
          background: #1e1e2e;
          border-top: 1px solid #313244;
          flex-shrink: 0;
        }
        .dot {
          width: 8px; height: 8px;
          border-radius: 50%;
          background: #6c7086;
          flex-shrink: 0;
        }
        .dot.connected    { background: #a6e3a1; }
        .dot.disconnected { background: #f38ba8; }
        .dot.connecting   { background: #f9e2af; }
        #status-text {
          font-size: 12px;
          font-family: monospace;
          color: #6c7086;
        }
        .spacer { flex: 1; }
        button {
          background: transparent;
          border: 1px solid #45475a;
          border-radius: 4px;
          padding: 3px 8px;
          color: #a6adc8;
          font-size: 11px;
          cursor: pointer;
        }
        button:hover { background: #313244; }
      </style>

      <div id="screen"></div>
      <div id="statusbar">
        <span class="dot connecting" id="dot"></span>
        <span id="status-text">Connexion…</span>
        <span class="spacer"></span>
        <button id="fullscreen-btn" title="Plein écran">⛶</button>
        <button id="reconnect-btn" title="Reconnecter">↺</button>
      </div>
    `

    this.shadowRoot.getElementById('fullscreen-btn')
      .addEventListener('click', () => this.#toggleFullscreen())

    this.shadowRoot.getElementById('reconnect-btn')
      .addEventListener('click', () => { this.#rfb?.disconnect(); this.#connect() })
  }

  // ── Connection ───────────────────────────────────────────────────────────────

  #wsUrl() {
    const vm   = this.getAttribute('vm')
    const port = this.getAttribute('port') ?? '9000'
    const creds = document.querySelector('login-gate')?.credentials
    const host  = creds?.agent_url
      ? new URL(creds.agent_url).hostname
      : location.hostname
    return `ws://${host}:${port}/vnc/${vm}`
  }

  #connect() {
    const vm = this.getAttribute('vm')
    if (!vm) { this.#setStatus('disconnected', 'Attribut vm manquant'); return }

    const url = this.#wsUrl()
    this.#setStatus('connecting', `Connexion à ${url}…`)

    this.#rfb = new RFB(this.shadowRoot.getElementById('screen'), url)
    this.#rfb.scaleViewport = true
    this.#rfb.resizeSession = false

    this.#rfb.addEventListener('connect', () => {
      this.#setStatus('connected', 'Connecté')
    })

    this.#rfb.addEventListener('disconnect', e => {
      const clean = e.detail?.clean ?? false
      if (clean) {
        this.#setStatus('disconnected', 'Déconnecté')
      } else {
        this.#setStatus('disconnected', 'Déconnecté — reconnexion dans 3s…')
        this.#retryTimer = setTimeout(() => this.#connect(), RECONNECT_DELAY)
      }
    })

    this.#rfb.addEventListener('credentialsrequired', () => {
      this.#rfb.sendCredentials({ password: '' })
    })
  }

  // ── Helpers ──────────────────────────────────────────────────────────────────

  #setStatus(state, msg) {
    const dot  = this.shadowRoot.getElementById('dot')
    const text = this.shadowRoot.getElementById('status-text')
    if (!dot || !text) return
    dot.className  = `dot ${state}`
    text.textContent = msg
  }

  #toggleFullscreen() {
    const screen = this.shadowRoot.getElementById('screen')
    if (!document.fullscreenElement) {
      screen.requestFullscreen?.()
    } else {
      document.exitFullscreen?.()
    }
  }
}

customElements.define('vnc-viewer', VncViewer)
