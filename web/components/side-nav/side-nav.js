// Side navigation component.
// Desktop : sidebar fixe à gauche dans le flux flex.
// Mobile (≤768px) : overlay depuis la gauche, déclenché par un bouton hamburger
//                   inséré dans <header>. Géométrie gérée en inline style pour
//                   éviter les conflits de spécificité avec le shadow DOM.

const HAMBURGER_ICON = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none"
  stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
  <line x1="3" y1="6"  x2="21" y2="6"/>
  <line x1="3" y1="12" x2="21" y2="12"/>
  <line x1="3" y1="18" x2="21" y2="18"/>
</svg>`

const ICONS = {
  dashboard: `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>`,
  vpc:       `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.66 4.03 3 9 3s9-1.34 9-3V5"/><path d="M3 12c0 1.66 4.03 3 9 3s9-1.34 9-3"/></svg>`,
  subnet:    `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="20" height="20" rx="2"/><path d="M8 12h8M12 8v8"/></svg>`,
  vm:        `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/></svg>`,
}

const CSS = new URL('./side-nav.css', import.meta.url).href
const MQ  = window.matchMedia('(max-width: 768px)')
const NAV_W   = 220   // px

function resolveIcon(name) {
  return ICONS[name] ?? `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/></svg>`
}

class SideNav extends HTMLElement {
  #items     = []
  #hamburger = null
  #backdrop  = null
  #mqHandler = null
  #isMobile  = false

  async connectedCallback() {
    this.attachShadow({ mode: 'open' })

    const navPath  = this.dataset.src ?? './navigation.json'
    const response = await fetch(navPath)
    if (!response.ok) throw new Error(`side-nav: failed to load ${navPath}`)
    this.#items = await response.json()

    this.#render()

    this.#mqHandler = () => this.#applyMode()
    MQ.addEventListener('change', this.#mqHandler)
    this.#applyMode()
  }

  disconnectedCallback() {
    MQ.removeEventListener('change', this.#mqHandler)
    this.#removeHamburger()
    this.#removeBackdrop()
  }

  // ── Rendu des items ─────────────────────────────────────────────────────────

  #render() {
    const current = window.location.pathname.split('/').pop() || 'index.html'

    const links = this.#items.map(item => {
      const active = item.href === current
      return `<a href="${item.href}" class="item ${active ? 'active' : ''}">
        <span class="icon">${resolveIcon(item.icon)}</span>
        <span class="label">${item.label}</span>
      </a>`
    }).join('')

    this.shadowRoot.innerHTML = `
      <link rel="stylesheet" href="${CSS}">
      ${links}
    `

    // Fermer l'overlay sur tap d'un lien (mobile)
    this.shadowRoot.querySelectorAll('.item').forEach(a =>
      a.addEventListener('click', () => { if (this.#isMobile) this.#close() })
    )
  }

  // ── Basculement desktop / mobile ────────────────────────────────────────────

  #applyMode() {
    if (MQ.matches) this.#toMobile()
    else            this.#toDesktop()
  }

  #toMobile() {
    this.#isMobile = true

    // inline style = spécificité maximale, pas de conflit avec le shadow DOM
    Object.assign(this.style, {
      position:   'fixed',
      top:        '0',
      left:       '0',
      width:      `${NAV_W}px`,
      minWidth:   `${NAV_W}px`,
      height:     '100vh',
      zIndex:     '500',
      transform:  'translateX(-100%)',
      transition: 'transform 0.25s cubic-bezier(0.4,0,0.2,1)',
      overflow:   'hidden',
      boxShadow:  '4px 0 24px rgba(0,0,0,0.45)',
    })

    this.#addHamburger()
  }

  #toDesktop() {
    this.#isMobile = false
    // Réinitialise tous les styles inline → shadow DOM reprend le contrôle
    this.style.cssText = ''
    this.#removeHamburger()
    this.#removeBackdrop()
  }

  // ── Hamburger ───────────────────────────────────────────────────────────────

  #addHamburger() {
    if (this.#hamburger) return

    const btn = document.createElement('button')
    btn.innerHTML = HAMBURGER_ICON
    btn.setAttribute('aria-label', 'Menu')
    btn.setAttribute('data-sidenav-toggle', '')
    Object.assign(btn.style, {
      display:         'inline-flex',
      alignItems:      'center',
      justifyContent:  'center',
      background:      'transparent',
      border:          '1px solid #313244',
      borderRadius:    '6px',
      padding:         '7px',
      color:           '#a6adc8',
      cursor:          'pointer',
      minWidth:        '36px',
      minHeight:       '36px',
      flexShrink:      '0',
    })
    btn.addEventListener('click', () => this.#toggle())
    this.#hamburger = btn

    // Insère après le h1 dans le header, ou en premier si pas de h1
    const header = document.querySelector('header')
    if (!header) { document.body.prepend(btn); return }
    const h1 = header.querySelector('h1')
    h1 ? h1.after(btn) : header.prepend(btn)
  }

  #removeHamburger() {
    this.#hamburger?.remove()
    this.#hamburger = null
  }

  // ── Open / Close (mobile) ───────────────────────────────────────────────────

  #toggle() { this.#isMobile && (this.style.transform === 'translateX(0px)'
    ? this.#close() : this.#open()) }

  #open() {
    this.style.transform = 'translateX(0)'

    const bd = document.createElement('div')
    Object.assign(bd.style, {
      position:   'fixed',
      inset:      '0',
      background: 'rgba(0,0,0,0)',
      zIndex:     '499',
      transition: 'background 0.25s',
    })
    bd.addEventListener('click', () => this.#close())
    document.body.appendChild(bd)
    this.#backdrop = bd
    requestAnimationFrame(() => { bd.style.background = 'rgba(0,0,0,0.45)' })
  }

  #close() {
    this.style.transform = 'translateX(-100%)'
    this.#removeBackdrop()
  }

  #removeBackdrop() {
    if (!this.#backdrop) return
    const bd = this.#backdrop
    this.#backdrop = null
    bd.style.background = 'rgba(0,0,0,0)'
    bd.addEventListener('transitionend', () => bd.remove(), { once: true })
  }
}

customElements.define('side-nav', SideNav)
