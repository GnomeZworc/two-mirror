// VM create / edit form.
//
// Usage:
//   <form-vm></form-vm>                    → create mode
//   <form-vm vm-name="i-test1"></form-vm>  → edit mode (loads VM from agent)
//
// Events dispatched on the element:
//   vm-saved  → { detail: { name, mode: 'create'|'edit' } }
//   vm-error  → { detail: { message } }
//
// Edit mode stops the VM then recreates it with the new parameters.
// The name field is read-only in edit mode.

const CSS = new URL('./form-vm.css', import.meta.url).href

const PLUS_ICON   = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>`
const TRASH_ICON  = `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6M14 11v6"/></svg>`

class FormVm extends HTMLElement {
  #api    = null
  #mode   = 'create'
  #vmData = null   // loaded in edit mode

  async connectedCallback() {
    this.attachShadow({ mode: 'open' })
    this.shadowRoot.innerHTML = `<link rel="stylesheet" href="${CSS}"><div id="root"></div>`

    this.#api  = document.querySelector('api-client')
    this.#mode = this.hasAttribute('vm-name') ? 'edit' : 'create'

    if (this.#mode === 'edit') {
      this.#setStatus('loading', 'Chargement…')
      try {
        this.#vmData = await this.#api.agent.get(`/vms/${this.getAttribute('vm-name')}`)
      } catch (e) {
        this.#setStatus('error', `Impossible de charger la VM : ${e.message}`)
        return
      }
    }

    this.#render()
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  #render() {
    const d    = this.#vmData
    const edit = this.#mode === 'edit'

    this.#root().innerHTML = `
      <form id="vm-form">

        <div class="section">
          <div class="section-title">Général</div>

          <div class="field">
            <label>Nom</label>
            <input name="name" placeholder="i-mon-vm" value="${d?.name ?? ''}"
              ${edit ? 'readonly' : 'required'} />
          </div>

          <div class="row">
            <div class="field">
              <label>Mémoire (MB)</label>
              <input name="memory" type="number" min="128" step="128"
                placeholder="1024" value="${d?.memory ?? ''}" required />
            </div>
            <div class="field">
              <label>vCPUs</label>
              <input name="cpus" type="number" min="1" max="32"
                placeholder="2" value="${d?.cpus ?? ''}" required />
            </div>
          </div>

          <div class="field toggle-row">
            <input name="uefi" type="checkbox" id="uefi-cb"
              ${d?.uefi ? 'checked' : ''} />
            <label for="uefi-cb" class="toggle-label">UEFI (OVMF)</label>
          </div>
        </div>

        <div class="section">
          <div class="section-title">Authentification</div>
          <div class="field">
            <label>Clé SSH</label>
            <input name="sshkey" placeholder="ssh-ed25519 AAAA…"
              value="${d?.sshkey ?? ''}" />
          </div>
          <div class="field">
            <label>Mot de passe (optionnel)</label>
            <input name="password" type="password" placeholder="laisser vide = désactivé" />
          </div>
        </div>

        <div class="section">
          <div class="section-title">Interface réseau</div>
          <div class="row">
            <div class="field">
              <label>Subnet</label>
              <input name="subnet" placeholder="sn-000000"
                value="${d?.interfaces?.[0]?.subnet ?? ''}" required />
            </div>
            <div class="field">
              <label>IP</label>
              <input name="ip" placeholder="192.168.14.x"
                value="${d?.interfaces?.[0]?.ip ?? ''}" required />
            </div>
          </div>
        </div>

        <div class="section">
          <div class="section-title">Stockage</div>
          <div id="disks">
            ${this.#renderDisks(d?.storage ?? [{ dev: 'vda', path: '' }])}
          </div>
          <button type="button" class="btn btn-ghost" id="add-disk">
            ${PLUS_ICON} Ajouter un disque
          </button>
        </div>

        <div class="status" id="status"></div>

        <div class="actions">
          <button type="submit" class="btn btn-primary" id="submit-btn">
            ${edit ? 'Mettre à jour' : 'Créer'}
          </button>
          ${edit ? `<button type="button" class="btn btn-danger" id="delete-btn">Supprimer</button>` : ''}
        </div>

      </form>
    `

    this.#bindEvents()
  }

  #renderDisks(disks) {
    return disks.map((disk, i) => `
      <div class="disk-row" data-disk="${i}">
        <input name="dev_${i}"  placeholder="vda" value="${disk.dev  ?? ''}" required />
        <input name="path_${i}" placeholder="/vm/nom.qcow2" value="${disk.path ?? ''}" required />
        <button type="button" class="btn btn-ghost" data-remove-disk="${i}">${TRASH_ICON}</button>
      </div>
    `).join('')
  }

  // ── Events ──────────────────────────────────────────────────────────────────

  #bindEvents() {
    const form = this.#root().querySelector('#vm-form')

    form.addEventListener('submit', e => { e.preventDefault(); this.#submit() })

    this.#root().querySelector('#add-disk')?.addEventListener('click', () => {
      this.#addDisk()
    })

    this.#root().querySelector('#delete-btn')?.addEventListener('click', () => {
      this.#delete()
    })

    this.#root().addEventListener('click', e => {
      const btn = e.target.closest('[data-remove-disk]')
      if (btn) this.#removeDisk(Number(btn.dataset.removeDisk))
    })
  }

  // ── Disk list helpers ────────────────────────────────────────────────────────

  #currentDisks() {
    return [...this.#root().querySelectorAll('[data-disk]')].map(row => {
      const i = row.dataset.disk
      return {
        dev:  row.querySelector(`[name="dev_${i}"]`).value.trim(),
        path: row.querySelector(`[name="path_${i}"]`).value.trim(),
      }
    })
  }

  #addDisk() {
    const container = this.#root().querySelector('#disks')
    const idx       = container.querySelectorAll('[data-disk]').length
    const row       = document.createElement('div')
    row.dataset.disk = idx
    row.className   = 'disk-row'
    row.innerHTML   = `
      <input name="dev_${idx}"  placeholder="vda" required />
      <input name="path_${idx}" placeholder="/vm/nom.qcow2" required />
      <button type="button" class="btn btn-ghost" data-remove-disk="${idx}">${TRASH_ICON}</button>
    `
    container.appendChild(row)
  }

  #removeDisk(idx) {
    this.#root().querySelector(`[data-disk="${idx}"]`)?.remove()
    this.#reindexDisks()
  }

  #reindexDisks() {
    this.#root().querySelectorAll('[data-disk]').forEach((row, i) => {
      row.dataset.disk = i
      row.querySelector('[name^="dev_"]').name  = `dev_${i}`
      row.querySelector('[name^="path_"]').name = `path_${i}`
      const trash = row.querySelector('[data-remove-disk]')
      if (trash) trash.dataset.removeDisk = i
    })
  }

  // ── Build payload ────────────────────────────────────────────────────────────

  #buildPayload() {
    const f    = this.#root().querySelector('#vm-form')
    const data = new FormData(f)
    const val  = k => data.get(k)?.trim() ?? ''

    return {
      name:       val('name'),
      memory:     Number(val('memory')),
      cpus:       Number(val('cpus')),
      uefi:       f.querySelector('[name="uefi"]').checked,
      sshkey:     val('sshkey'),
      password:   val('password'),
      interfaces: [{ subnet: val('subnet'), ip: val('ip'), primary: true }],
      storage:    this.#currentDisks(),
    }
  }

  // ── Submit ───────────────────────────────────────────────────────────────────

  async #submit() {
    const btn = this.#root().querySelector('#submit-btn')
    btn.disabled = true
    this.#clearStatus()

    try {
      const payload = this.#buildPayload()

      if (this.#mode === 'edit') {
        // Stop existing VM, then recreate with new params
        this.#setStatus('loading', 'Arrêt de la VM…')
        await this.#api.agent.delete(`/vms/${payload.name}`)
        await this.#api.agent.waitFor(`/vms/${payload.name}`, 'stopped')
        this.#setStatus('loading', 'Recréation…')
      } else {
        this.#setStatus('loading', 'Création…')
      }

      await this.#api.agent.post('/vms', payload)
      await this.#api.agent.waitFor(`/vms/${payload.name}`, 'started')

      this.#setStatus('success', this.#mode === 'edit' ? 'VM mise à jour.' : 'VM créée.')
      this.dispatchEvent(new CustomEvent('vm-saved', {
        bubbles: true,
        detail: { name: payload.name, mode: this.#mode },
      }))

      // Auto-close parent side-panel after 1s
      setTimeout(() => this.closest('side-panel')?.close(), 1000)

    } catch (e) {
      this.#setStatus('error', e.message)
      this.dispatchEvent(new CustomEvent('vm-error', {
        bubbles: true,
        detail: { message: e.message },
      }))
    } finally {
      btn.disabled = false
    }
  }

  // ── Delete ───────────────────────────────────────────────────────────────────

  async #delete() {
    if (!confirm(`Supprimer la VM ${this.getAttribute('vm-name')} ?`)) return
    const btn = this.#root().querySelector('#delete-btn')
    btn.disabled = true
    this.#setStatus('loading', 'Suppression…')

    try {
      const name = this.getAttribute('vm-name')
      await this.#api.agent.delete(`/vms/${name}`)
      await this.#api.agent.waitFor(`/vms/${name}`, 'stopped')
      this.#setStatus('success', 'VM supprimée.')
      this.dispatchEvent(new CustomEvent('vm-saved', {
        bubbles: true,
        detail: { name, mode: 'delete' },
      }))
      setTimeout(() => this.closest('side-panel')?.close(), 1000)
    } catch (e) {
      this.#setStatus('error', e.message)
      btn.disabled = false
    }
  }

  // ── Helpers ──────────────────────────────────────────────────────────────────

  #root()   { return this.shadowRoot.getElementById('root') }

  #setStatus(type, msg) {
    const el = this.#root().querySelector('#status')
    if (!el) return
    el.className = `status ${type}`
    el.textContent = msg
  }

  #clearStatus() {
    const el = this.#root().querySelector('#status')
    if (el) { el.className = 'status'; el.textContent = '' }
  }
}

customElements.define('form-vm', FormVm)
