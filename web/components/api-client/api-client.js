// API service component.
//
// Declare once in the shell:
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
// Phase 2: swap this component for one that points to the orchestrator.
// No other component changes.

import { config } from '../../core/config.js'

export class ApiError extends Error {
  constructor(origin, status, message) {
    super(`[${origin}] HTTP ${status}: ${message}`)
    this.origin = origin  // 'netbox' | 'agent'
    this.status = status
  }
}

class ApiClient extends HTMLElement {
  connectedCallback() {
    this.style.display = 'none'
    this.netbox = this.#buildNetbox()
    this.agent  = this.#buildAgent()
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
      'Authorization': `Token ${config.netbox_token}`,
      'Accept': 'application/json',
    })

    const url = (path, params = {}) => {
      const u = new URL(path, config.netbox_url + '/api/')
      Object.entries(params).forEach(([k, v]) => {
        if (v != null) u.searchParams.set(k, v)
      })
      return u.toString()
    }

    return {
      // Returns results array. Netbox paginates; limit=1000 covers most cases.
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

    const url = path => new URL(path, config.agent_url + '/').toString()

    const req = (method, path, body) =>
      this.#request('agent', url(path), {
        method,
        headers: headers(body != null),
        ...(body != null ? { body: JSON.stringify(body) } : {}),
      })

    return {
      get:    path       => req('GET',    path),
      list:   path       => req('GET',    path),
      post:   (path, b)  => req('POST',   path, b),
      delete: path       => req('DELETE', path),

      // Poll path until resource.state === desired or timeout.
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
