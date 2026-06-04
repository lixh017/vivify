// Shared test helpers for the OPC Playwright E2E suite.
//
// Every spec resets the backend DB at the start of its run via the
// `/api/_test/reset` admin route (mounted by the API only when
// GIN_MODE is not "release" — see apps/api/cmd/server/main.go).
// Resetting per spec keeps tests isolated: a topic created in one
// file does not bleed into another.
//
// We never call the production reset route from a real deployment:
// the route is only registered in dev/test modes. The webServer in
// playwright.config.ts starts the API with GIN_MODE=release, so the
// reset route is NOT available. The helpers below therefore use
// direct REST deletes of any leftover data from previous runs.

import { APIRequestContext, request } from '@playwright/test'

export const API_BASE = 'http://localhost:48080'
export const WEB_BASE = 'http://localhost:3000'

export interface TopicCreate {
  title: string
  angle: string
  platform?: string
  status?: string
}

export interface ScriptCreate {
  topic_id: number
  title: string
  content: string
  platform?: string
}

export interface ContentItemCreate {
  script_id: number
  platform: string
  platform_url?: string
  performance_metrics?: string
  scheduled_at?: string
  published_at?: string
}

export interface KnowledgeDocCreate {
  title: string
  path: string
  content: string
  doc_type?: string
}

// ApiClient is a thin wrapper over Playwright's request fixture that
// targets the OPC backend directly. We use it to seed the DB with
// test data and to clean up between tests, which is faster and more
// deterministic than driving the UI for every setup step.
export class ApiClient {
  constructor(private readonly ctx: APIRequestContext) {}

  static async create(): Promise<ApiClient> {
    const ctx = await request.newContext({ baseURL: API_BASE })
    return new ApiClient(ctx)
  }

  async listTopics() {
    const res = await this.ctx.get('/api/topics')
    return (await res.json()) as { items: { id: number }[]; total: number }
  }

  async createTopic(data: TopicCreate) {
    const res = await this.ctx.post('/api/topics', { data })
    return (await res.json()) as { id: number }
  }

  async deleteTopic(id: number) {
    await this.ctx.delete(`/api/topics/${id}`)
  }

  async listScripts() {
    const res = await this.ctx.get('/api/scripts')
    return (await res.json()) as { items: { id: number }[] }
  }

  async createScript(data: ScriptCreate) {
    const res = await this.ctx.post('/api/scripts', { data })
    return (await res.json()) as { id: number }
  }

  async deleteScript(id: number) {
    await this.ctx.delete(`/api/scripts/${id}`)
  }

  async listContentItems() {
    const res = await this.ctx.get('/api/content-items')
    return (await res.json()) as { items: { id: number }[] }
  }

  async createContentItem(data: ContentItemCreate) {
    const res = await this.ctx.post('/api/content-items', { data })
    return (await res.json()) as { id: number }
  }

  async deleteContentItem(id: number) {
    await this.ctx.delete(`/api/content-items/${id}`)
  }

  async listKnowledge() {
    const res = await this.ctx.get('/api/knowledge')
    return (await res.json()) as { items: { id: number }[] }
  }

  async createKnowledgeDoc(data: KnowledgeDocCreate) {
    const res = await this.ctx.post('/api/knowledge', { data })
    return (await res.json()) as { id: number }
  }

  async deleteKnowledgeDoc(id: number) {
    await this.ctx.delete(`/api/knowledge/${id}`)
  }

  // wipeAll deletes every record in every collection. Used between
  // specs so the seed fixtures are deterministic.
  async wipeAll() {
    const [topics, scripts, items, docs] = await Promise.all([
      this.listTopics(),
      this.listScripts(),
      this.listContentItems(),
      this.listKnowledge(),
    ])
    await Promise.all([
      ...items.items.map((it) => this.deleteContentItem(it.id)),
      ...docs.items.map((d) => this.deleteKnowledgeDoc(d.id)),
      ...scripts.items.map((s) => this.deleteScript(s.id)),
      ...topics.items.map((t) => this.deleteTopic(t.id)),
    ])
  }

  async dispose() {
    await this.ctx.dispose()
  }
}
