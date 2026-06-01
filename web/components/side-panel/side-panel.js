// Configurable right-side drawer panel.
//
// ── Declarative ──────────────────────────────────────────────────────────────
//
//   <side-panel title="VPC details" width="520px">
//     <vpc-detail vpc="vp-admin"></vpc-detail>
//   </side-panel>
//   panel.open()
//   panel.close()
//
// ── Programmatic (from any component) ────────────────────────────────────────
//
//   import { SidePanel } from '../side-panel/side-panel.js'
//
//   const panel = SidePanel.open({
//     title:   'VPC — vp-admin',
//     width:   '520px',
//     content: '<vpc-detail vpc="vp-admin"></vpc-detail>',
//   })
//   panel.close()
//
// ── Multiple panels ───────────────────────────────────────────────────────────
//
//   Each open panel stacks with a slight depth offset.
//   Clicking the backdrop closes only the topmost panel.
//   ESC closes the topmost panel.

const CSS = new URL('./side-panel.css', import.meta.url).href

const CLOSE_ICON = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none"
  stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
</svg>`

const STACK_OFFSET  = 12   // px shift per stacked panel
const ANIM_DURATION = 220  // ms

function openCount() {
  return document.querySelectorAll('side-panel[data-open]').length
}

function topPanel() {
  const panels = [...document.querySelectorAll('side-panel[data-open]')]
  return panels[panels.length - 1] ?? null
}

// Global ESC handler — closes topmost panel only.
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') topPanel()?.close()
})

export class SidePanel extends HTMLElement {
  // ── Static factory ──────────────────────────────────────────────────────────

  static open({ title = '', content = '', width = '480px' } = {}) {
    const panel = document.createElement('side-panel')
    if (title) panel.setAttribute('title',  title)
    if (width) panel.setAttribute('width',  width)
    if (content) panel.innerHTML = content
    document.body.appendChild(panel)
    panel.open()
    return panel
  }

  // ── Lifecycle ───────────────────────────────────────────────────────────────

  connectedCallback() {
    this.attachShadow({ mode: 'open' })
    this.#render()
  }

  // ── Public API ──────────────────────────────────────────────────────────────

  open() {
    const depth  = openCount()
    const offset = depth * STACK_OFFSET

    // Shift panel left based on stack depth
    this.style.setProperty('--offset', `${offset}px`)
    this.setAttribute('data-open', '')

    // Backdrop: only show/darken the shared one
    this.#ensureBackdrop()

    this.dispatchEvent(new CustomEvent('panel-open', { bubbles: true }))
  }

  close() {
    if (!this.hasAttribute('data-open')) return

    this.removeAttribute('data-open')
    this.#updateBackdrop()

    this.addEventListener('transitionend', () => {
      // If created programmatically (appended to body by factory), remove from DOM
      if (this.dataset.programmatic) this.remove()
    }, { once: true })

    this.dispatchEvent(new CustomEvent('panel-close', { bubbles: true }))
  }

  // ── Rendering ───────────────────────────────────────────────────────────────

  #render() {
    const title = this.getAttribute('title') ?? ''
    const width = this.getAttribute('width') ?? '480px'

    // Passe la largeur comme custom property — référencée dans le CSS via var(--width)
    this.style.setProperty('--width', width)

    this.shadowRoot.innerHTML = `
      <link rel="stylesheet" href="${CSS}">
      <div class="header">
        <span class="title">${title}</span>
        <button class="close" aria-label="Close">${CLOSE_ICON}</button>
      </div>
      <div class="body">
        <slot></slot>
      </div>
    `

    this.shadowRoot.querySelector('.close').addEventListener('click', () => this.close())

    // Mark programmatic panels so they self-remove on close
    if (!this.parentElement || this.parentElement === document.body) {
      this.dataset.programmatic = 'true'
    }
  }

  // ── Backdrop ─────────────────────────────────────────────────────────────────

  #ensureBackdrop() {
    let bd = document.getElementById('__side-panel-backdrop__')
    if (!bd) {
      bd = document.createElement('div')
      bd.id = '__side-panel-backdrop__'
      Object.assign(bd.style, {
        position: 'fixed', inset: '0',
        background: 'rgba(0,0,0,0)',
        transition: `background ${ANIM_DURATION}ms`,
        zIndex: '999',
      })
      bd.addEventListener('click', () => topPanel()?.close())
      document.body.appendChild(bd)
    }
    // Opacity scales with stack depth (max 0.5)
    const opacity = Math.min(0.5, openCount() * 0.15)
    requestAnimationFrame(() => { bd.style.background = `rgba(0,0,0,${opacity})` })
  }

  #updateBackdrop() {
    const bd = document.getElementById('__side-panel-backdrop__')
    if (!bd) return
    const remaining = openCount()
    if (remaining === 0) {
      bd.style.background = 'rgba(0,0,0,0)'
      bd.addEventListener('transitionend', () => bd.remove(), { once: true })
    } else {
      const opacity = Math.min(0.5, remaining * 0.15)
      bd.style.background = `rgba(0,0,0,${opacity})`
    }
  }
}

customElements.define('side-panel', SidePanel)
