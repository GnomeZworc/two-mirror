// Logout button — drop anywhere in the DOM.
//
//   <logout-button></logout-button>            full button with label
//   <logout-button compact></logout-button>    icon only (for tight spaces)
//
// Delegates to <login-gate>.logout(). Works regardless of where it sits
// (header, members panel, dropdown…) since it resolves the gate from the document.
//
// Phase 2: if login-gate is swapped for oidc-gate, this still works as long as
// the auth component exposes a logout() method (see #gate()).

const ICON = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>`

class LogoutButton extends HTMLElement {
  connectedCallback() {
    this.attachShadow({ mode: 'open' })

    const compact = this.hasAttribute('compact')

    this.shadowRoot.innerHTML = `
      <style>
        :host { display: inline-flex; }

        button {
          display: inline-flex;
          align-items: center;
          gap: 7px;
          background: transparent;
          border: 1px solid #313244;
          border-radius: 6px;
          padding: ${compact ? '7px' : '7px 12px'};
          color: #a6adc8;
          font-size: 13px;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
          cursor: pointer;
          transition: background 0.12s, color 0.12s, border-color 0.12s;
        }

        button:hover {
          background: #313244;
          color: #f38ba8;
          border-color: #f38ba8;
        }

        .icon { display: flex; align-items: center; }
        .label { ${compact ? 'display: none;' : ''} }
      </style>

      <button type="button" title="Log out">
        <span class="icon">${ICON}</span>
        <span class="label">Log out</span>
      </button>
    `

    this.shadowRoot.querySelector('button')
      .addEventListener('click', () => this.#logout())
  }

  // Resolves the auth component. Today: login-gate. Tomorrow: any element
  // exposing logout() (oidc-gate, oauth-gate…).
  #gate() {
    return document.querySelector('login-gate, [data-auth-gate]')
  }

  #logout() {
    const gate = this.#gate()
    if (gate?.logout) {
      gate.logout()
    } else {
      console.warn('logout-button: no auth gate with logout() found in document')
    }
  }
}

customElements.define('logout-button', LogoutButton)
