// API service component.
//
// Declare after login-gate in the shell:
//   <login-gate></login-gate>
//   <api-client></api-client>
//
// Use from any other component:
//   const api = document.querySelector('api-client')
//   const vrfs = await api.netbox.list('/ipam/vrfs/')
//   const vpc  = await api.agent.get('/vpcs/vp-admin')
//   await api.agent.post('/vpcs', { name: 'vp-admin', cidr: '10.0.0.0/8' })
//   await api.agent.delete('/vpcs/vp-admin')
//   await api.agent.waitFor('/vms/i-test1', 'started')
//
// Phase 2: swap login-gate for oidc-gate → this component unchanged.

export class ApiError extends Error {
  constructor(origin, status, message) {
    super(`[${origin}] HTTP ${status}: ${message}`)
    this.origin = origin  // 'netbox' | 'agent'
    this.status = status
  }
}

class ApiClient extends HTMLElement {
  // Resolves once login-gate is ready (or immediately if no gate in DOM).
  #credentials = null

  async connectedCallback() {
    this.style.display = 'none'

    // Support login-gate (static token) and biscuit-gate (attenuated token).
    const gate = document.querySelector('login-gate')
    this.#credentials = gate ? await gate.ready : null

    this.netbox = this.#buildNetbox()
    this.agent  = this.#buildAgent()
  }


  get creds() {
    return this.#credentials ?? {}
  }

  // ── Internal ──────────────────────────────────────────────────────────────

  async #request(origin, url, options = {}) {
    let response
    try {
      response = await fetch(url, options)
    } catch (e) {
      throw new ApiError(origin, 0, `Network error: ${e.message}`)
    }

    if (response.status === 204) return null

    const text = await response.text()
    let json
    try { json = JSON.parse(text) } catch { json = null }

    if (!response.ok) {
      const detail = json?.detail ?? json?.message ?? text
      throw new ApiError(origin, response.status, detail)
    }

    return json
  }

  // ── Netbox ────────────────────────────────────────────────────────────────

  #buildNetbox() {
    const headers = () => ({
      'Authorization': `Token ${this.creds.netbox_token}`,
      'Accept': 'application/json',
    })

    const url = (path, params = {}) => {
      const u = new URL(path, this.creds.netbox_url + '/api/')
      Object.entries(params).forEach(([k, v]) => {
        if (v != null) u.searchParams.set(k, v)
      })
      return u.toString()
    }

    return {
      list: (path, params = {}) =>
        this.#request('netbox', url(path, { limit: 1000, ...params }), { headers: headers() })
            .then(d => d?.results ?? []),

      get: (path, params = {}) =>
        this.#request('netbox', url(path, params), { headers: headers() }),
    }
  }

  // ── Agent ─────────────────────────────────────────────────────────────────

  #buildAgent() {
    const headers = (body = false) => ({
      'Accept': 'application/json',
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    })

    const url = path => new URL(path, this.creds.agent_url + '/').toString()

    const req = (method, path, body) =>
      this.#request('agent', url(path), {
        method,
        headers: headers(body != null),
        ...(body != null ? { body: JSON.stringify(body) } : {}),
      })

    return {
      get:    path      => req('GET',    path),
      list:   path      => req('GET',    path),
      post:   (path, b) => req('POST',   path, b),
      delete: path      => req('DELETE', path),

      waitFor: async (path, desired, { timeout = 120_000, interval = 2_000 } = {}) => {
        const deadline = Date.now() + timeout
        while (Date.now() < deadline) {
          const r = await req('GET', path)
          if (r?.state === desired) return r
          if (r?.state === 'error') throw new ApiError('agent', 'error', `${path} reached error state`)
          await new Promise(ok => setTimeout(ok, interval))
        }
        throw new ApiError('agent', 'timeout', `${path} did not reach '${desired}' in ${timeout}ms`)
      },
    }
  }
}

customElements.define('api-client', ApiClient)
