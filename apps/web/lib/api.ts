// Typed API client for the OPC Go backend.
//
// Transport: relative `/api/...` paths that Next.js rewrites to
// NEXT_PUBLIC_API_URL. Server-side fetches and client-side fetches both
// resolve through the same rewrite, so we never need absolute URLs here.
//
// Error handling: non-2xx responses throw an Error with a useful message
// (status + body excerpt). Callers can catch and surface to the UI.

import type {
  Topic,
  Script,
  ContentItem,
  KnowledgeDoc,
  ListResponse,
} from './types'

const BASE = process.env.NEXT_PUBLIC_API_URL || ''

class ApiError extends Error {
  constructor(public status: number, public body: string, public path: string) {
    super(`API ${path} failed: ${status} ${body}`)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}/api${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new ApiError(res.status, body, path)
  }
  // 204 No Content (Topic DELETE) — return undefined cast to T.
  if (res.status === 204) {
    return undefined as unknown as T
  }
  return res.json() as Promise<T>
}

export type TopicListParams = {
  platform?: string
  status?: string
  limit?: number
  offset?: number
}

export type ScriptListParams = {
  topic_id?: number
  platform?: string
  limit?: number
  offset?: number
}

export type ContentItemListParams = {
  script_id?: number
  platform?: string
  limit?: number
  offset?: number
}

export type KnowledgeListParams = {
  doc_type?: string
  path?: string
}

export const api = {
  topics: {
    list: (params?: TopicListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<Topic>>(`/topics${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<Topic>(`/topics/${id}`),
    create: (data: Partial<Topic>) =>
      request<Topic>('/topics', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<Topic>) =>
      request<Topic>(`/topics/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: number) =>
      request<void>(`/topics/${id}`, { method: 'DELETE' }),
  },
  scripts: {
    list: (params?: ScriptListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<Script>>(`/scripts${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<Script>(`/scripts/${id}`),
    create: (data: Partial<Script>) =>
      request<Script>('/scripts', { method: 'POST', body: JSON.stringify(data) }),
    update: (id: number, data: Partial<Script>) =>
      request<Script>(`/scripts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/scripts/${id}`, { method: 'DELETE' }),
  },
  contentItems: {
    list: (params?: ContentItemListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<ContentItem>>(`/content-items${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<ContentItem>(`/content-items/${id}`),
    create: (data: Partial<ContentItem>) =>
      request<ContentItem>('/content-items', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: number, data: Partial<ContentItem>) =>
      request<ContentItem>(`/content-items/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/content-items/${id}`, { method: 'DELETE' }),
  },
  knowledge: {
    list: (params?: KnowledgeListParams) => {
      const q = params ? new URLSearchParams(params as Record<string, string>).toString() : ''
      return request<ListResponse<KnowledgeDoc>>(`/knowledge${q ? `?${q}` : ''}`)
    },
    get: (id: number) => request<KnowledgeDoc>(`/knowledge/${id}`),
    create: (data: Partial<KnowledgeDoc>) =>
      request<KnowledgeDoc>('/knowledge', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id: number, data: Partial<KnowledgeDoc>) =>
      request<KnowledgeDoc>(`/knowledge/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id: number) =>
      request<{ deleted: number }>(`/knowledge/${id}`, { method: 'DELETE' }),
  },
  series: {
    list: () => request<{ items: { id: number; name: string }[] }>('/series'),
  },
  ai: {
    generateTopics: (data: {
      seed: string
      platform: string
      count: number
    }) =>
      request<{ topics: GeneratedTopic[] }>('/ai/topics', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    humanize: (data: { script: string }) =>
      request<{ humanized: string }>('/ai/humanize', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    postmortem: (contentItemId: number) =>
      request<{ report: string; structured: PostmortemStructured }>(
        '/ai/postmortem',
        { method: 'POST', body: JSON.stringify({ content_item_id: contentItemId }) },
      ),
  },
}

export type GeneratedTopic = {
  title: string
  angle: string
  expected_performance: string
  hook: string
}

export type PostmortemStructured = {
  success_factors: string[]
  reusable_patterns: string[]
  insights: string[]
  suggestions: string[]
}

export { ApiError }
