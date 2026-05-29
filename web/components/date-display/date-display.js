import { config } from '../../core/config.js'

class DateDisplay extends HTMLElement {
  #interval = null

  connectedCallback() {
    this.attachShadow({ mode: 'open' })
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
          background: #1e1e2e;
          border: 1px solid #313244;
          border-radius: 8px;
          padding: 16px 20px;
          font-family: monospace;
          color: #cdd6f4;
          min-width: 260px;
        }
        .label {
          font-size: 11px;
          text-transform: uppercase;
          letter-spacing: 0.08em;
          color: #6c7086;
          margin-bottom: 6px;
        }
        .time {
          font-size: 28px;
          font-weight: 600;
          color: #89b4fa;
        }
        .date {
          font-size: 13px;
          color: #a6adc8;
          margin-top: 4px;
        }
        .agent {
          margin-top: 12px;
          font-size: 11px;
          color: #585b70;
          border-top: 1px solid #313244;
          padding-top: 10px;
        }
      </style>
      <div class="label">Current time</div>
      <div class="time" id="time"></div>
      <div class="date" id="date"></div>
      <div class="agent">agent → <span id="agent-url"></span></div>
    `

    this.shadowRoot.getElementById('agent-url').textContent = config.agent_url
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
