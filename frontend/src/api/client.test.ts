import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiRequestError, setCsrfToken } from './client'

function mockFetch(impl: typeof fetch): void {
  vi.stubGlobal('fetch', vi.fn(impl))
}

afterEach(() => {
  vi.unstubAllGlobals()
  setCsrfToken('')
})

describe('api client', () => {
  it('builds GET url with snake_case query params, no body/csrf header', async () => {
    let calledUrl = ''
    mockFetch(async (input) => {
      calledUrl = typeof input === 'string' ? input : input.toString()
      return new Response(JSON.stringify({ items: [] }), { status: 200 })
    })

    await api.get('/projects', { query: { page_size: 10, page_token: 'abc' } })

    expect(calledUrl).toContain('/v1/projects?')
    expect(calledUrl).toContain('page_size=10')
    expect(calledUrl).toContain('page_token=abc')
  })

  it('throws ApiRequestError with parsed body and reason on non-2xx', async () => {
    mockFetch(
      async () =>
        new Response(
          JSON.stringify({
            code: 412,
            message: '结果超行数上限',
            details: [{ reason: 'QUERY_ROW_LIMIT_EXCEEDED' }],
          }),
          { status: 412 },
        ),
    )

    await expect(api.get('/x')).rejects.toBeInstanceOf(ApiRequestError)
    try {
      await api.get('/x')
    } catch (e) {
      const err = e as ApiRequestError
      expect(err.status).toBe(412)
      expect(err.reason).toBe('QUERY_ROW_LIMIT_EXCEEDED')
      expect(err.message).toBe('结果超行数上限')
    }
  })

  it('adds X-Csrf-Token and Content-Type on POST when token is set', async () => {
    setCsrfToken('tok-123')
    let captured: Record<string, string> = {}
    mockFetch(async (_input, init) => {
      captured = (init?.headers ?? {}) as Record<string, string>
      return new Response(null, { status: 204 })
    })

    await api.post('/x', { a: 1 })

    expect(captured['X-Csrf-Token']).toBe('tok-123')
    expect(captured['Content-Type']).toBe('application/json')
  })

  it('omits X-Csrf-Token on GET', async () => {
    setCsrfToken('tok-123')
    let captured: Record<string, string> = {}
    mockFetch(async (_input, init) => {
      captured = (init?.headers ?? {}) as Record<string, string>
      return new Response(JSON.stringify({ items: [] }), { status: 200 })
    })

    await api.get('/projects')

    expect(captured['X-Csrf-Token']).toBeUndefined()
  })
})
