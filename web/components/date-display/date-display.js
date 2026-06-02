const CSS = new URL('./date-display.css', import.meta.url).href

class DateDisplay extends HTMLElement {
  #interval = null

  connectedCallback() {
    this.attachShadow({ mode: 'open' })
    this.shadowRoot.innerHTML = `
      <link rel="stylesheet" href="${CSS}">
      <div class="label">Current time</div>
      <div class="time" id="time"></div>
      <div class="date" id="date"></div>
      <div class="agent">agent → <span id="agent-url"></span></div>
    `
    this.#tick()
    this.#interval = setInterval(() => this.#tick(), 1000)
  }

  disconnectedCallback() {
    clearInterval(this.#interval)
  }

  #tick() {
    const now = new Date()
    this.shadowRoot.getElementById('time').textContent = now.toLocaleTimeString()
    this.shadowRoot.getElementById('date').textContent = now.toLocaleDateString(undefined, {
      weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'
    })
  }
}

customElements.define('date-display', DateDisplay)
