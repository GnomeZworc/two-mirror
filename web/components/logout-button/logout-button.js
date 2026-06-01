const CSS = new URL('./logout-button.css', import.meta.url).href

const ICON = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>`

class LogoutButton extends HTMLElement {
  connectedCallback() {
    this.attachShadow({ mode: 'open' })
    this.shadowRoot.innerHTML = `
      <link rel="stylesheet" href="${CSS}">
      <button type="button" title="Log out">
        <span class="icon">${ICON}</span>
        <span class="label">Log out</span>
      </button>
    `
    this.shadowRoot.querySelector('button')
      .addEventListener('click', () => this.#logout())
  }

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
