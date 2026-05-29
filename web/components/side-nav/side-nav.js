// Side navigation component.
// Reads items from navigation.json (path relative to web root via data-base attribute,
// defaults to ./navigation.json).
// Highlights the active entry by matching href against window.location.pathname.

const ICONS = {
  dashboard: `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>`,
  vpc:       `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.66 4.03 3 9 3s9-1.34 9-3V5"/><path d="M3 12c0 1.66 4.03 3 9 3s9-1.34 9-3"/></svg>`,
  subnet:    `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="20" rx="2"/><path d="M8 12h8M12 8v8"/></svg>`,
  vm:        `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/></svg>`,
}

function resolveIcon(name) {
  return ICONS[name] ?? `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/></svg>`
}

class SideNav extends HTMLElement {
  async connectedCallback() {
    this.attachShadow({ mode: 'open' })

    const navPath = this.dataset.src ?? './navigation.json'
    const response = await fetch(navPath)
    if (!response.ok) throw new Error(`side-nav: failed to load ${navPath}`)
    const items = await response.json()

    this.#render(items)
  }

  #render(items) {
    const currentPage = window.location.pathname.split('/').pop() || 'index.html'

    const links = items.map(item => {
      const isActive = item.href === currentPage
      return `
        <a href="${item.href}" class="nav-item ${isActive ? 'active' : ''}">
          <span class="icon">${resolveIcon(item.icon)}</span>
          <span class="label">${item.label}</span>
        </a>
      `
    }).join('')

    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: flex;
          flex-direction: column;
          width: 220px;
          min-width: 220px;
          background: #1e1e2e;
          border-right: 1px solid #313244;
          padding: 16px 12px;
          gap: 4px;
          height: 100%;
        }

        .nav-item {
          display: flex;
          align-items: center;
          gap: 10px;
          padding: 9px 12px;
          border-radius: 6px;
          text-decoration: none;
          color: #a6adc8;
          font-size: 14px;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
          transition: background 0.1s, color 0.1s;
        }

        .nav-item:hover {
          background: #313244;
          color: #cdd6f4;
        }

        .nav-item.active {
          background: #313244;
          color: #89b4fa;
        }

        .nav-item.active .icon {
          color: #89b4fa;
        }

        .icon {
          display: flex;
          align-items: center;
          color: #6c7086;
          flex-shrink: 0;
        }

        .nav-item:hover .icon {
          color: #cdd6f4;
        }
      </style>

      ${links}
    `
  }
}

customElements.define('side-nav', SideNav)
