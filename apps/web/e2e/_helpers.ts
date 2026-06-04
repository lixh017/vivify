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

import { APIRequestContext, APIResponse, request } from "@playwright/test";

export const API_BASE = "http://localhost:48080";
export const WEB_BASE = "http://localhost:3000";

export interface TopicCreate {
  title: string;
  angle: string;
  platform?: string;
  status?: string;
}

export interface ScriptCreate {
  topic_id: number;
  title: string;
  content: string;
  platform?: string;
}

export interface ContentItemCreate {
  script_id: number;
  platform: string;
  platform_url?: string;
  performance_metrics?: string;
  scheduled_at?: string;
  published_at?: string;
}

export interface KnowledgeDocCreate {
  title: string;
  path: string;
  content: string;
  doc_type?: string;
}

// Response envelopes the API returns from List endpoints. All four
// CRUD surfaces share this shape (`{ items, total, limit, offset }`).
// Keeping the type narrow makes any future envelope change surface
// as a TypeError at the listX call site, not a flaky teardown.
interface ListEnvelope<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

// assertOk throws if the response is not in the 2xx range. We treat
// 2xx as the only acceptable outcome for non-404 deletes here: the
// backend uses 200 (with body) for scripts/content-items and 204
// (no body) for topics/knowledge. The helper centralizes the
// status-code contract so callers don't have to remember which is
// which.
function assertOk(res: APIResponse, context: string): void {
  if (!res.ok()) {
    const status = res.status();
    // 404 is fine on a delete: a previous test may have wiped the
    // row already. Anything else is a real failure.
    if (status === 404) return;
    throw new Error(
      `${context} failed: ${status} ${res.statusText()} (url=${res.url()})`,
    );
  }
}

// parseListEnvelope validates the envelope shape before the test
// reads `.items`. Without this, a future API change (e.g. wrapping
// items inside `data.items`) would surface as `TypeError: cannot
// read property 'items' of undefined` deep inside `wipeAll`, which
// looks like a teardown flake.
async function parseListEnvelope<T>(
  res: APIResponse,
  context: string,
): Promise<ListEnvelope<T>> {
  assertOk(res, context);
  const body = (await res.json()) as { items?: T[] } | null;
  if (!body || !Array.isArray(body.items)) {
    const preview =
      body === null
        ? "null"
        : typeof body === "object"
          ? Object.keys(body).join(",")
          : typeof body;
    throw new Error(
      `${context}: response did not contain an items array (keys=${preview}, url=${res.url()})`,
    );
  }
  return body as ListEnvelope<T>;
}

// runWithConcurrency fans `tasks` out across at most `limit`
// concurrent workers. We use this in `wipeAll` so a runaway row
// count can't open thousands of in-flight HTTP requests against the
// API. 8 is a comfortable ceiling: it's well under the typical
// per-process connection limit but high enough to keep teardown
// fast for fixtures in the hundreds of rows.
async function runWithConcurrency<T>(
  tasks: Array<() => Promise<T>>,
  limit: number,
): Promise<T[]> {
  const results: T[] = [];
  let cursor = 0;
  const workers = Array.from(
    { length: Math.min(limit, tasks.length) },
    async () => {
      while (cursor < tasks.length) {
        const idx = cursor++;
        results[idx] = await tasks[idx]();
      }
    },
  );
  await Promise.all(workers);
  return results;
}

// ApiClient is a thin wrapper over Playwright's request fixture that
// targets the OPC backend directly. We use it to seed the DB with
// test data and to clean up between tests, which is faster and more
// deterministic than driving the UI for every setup step.
export class ApiClient {
  constructor(private readonly ctx: APIRequestContext) {}

  static async create(): Promise<ApiClient> {
    const ctx = await request.newContext({ baseURL: API_BASE });
    return new ApiClient(ctx);
  }

  async listTopics() {
    const res = await this.ctx.get("/topics");
    return parseListEnvelope<{ id: number }>(res, "listTopics");
  }

  async createTopic(data: TopicCreate) {
    const res = await this.ctx.post("/topics", { data });
    assertOk(res, "createTopic");
    return (await res.json()) as { id: number };
  }

  async deleteTopic(id: number) {
    const res = await this.ctx.delete(`/topics/${id}`);
    assertOk(res, `deleteTopic(${id})`);
  }

  async listScripts() {
    const res = await this.ctx.get("/scripts");
    return parseListEnvelope<{ id: number }>(res, "listScripts");
  }

  async createScript(data: ScriptCreate) {
    const res = await this.ctx.post("/scripts", { data });
    assertOk(res, "createScript");
    return (await res.json()) as { id: number };
  }

  async deleteScript(id: number) {
    const res = await this.ctx.delete(`/scripts/${id}`);
    assertOk(res, `deleteScript(${id})`);
  }

  async listContentItems() {
    const res = await this.ctx.get("/content-items");
    return parseListEnvelope<{ id: number }>(res, "listContentItems");
  }

  async createContentItem(data: ContentItemCreate) {
    const res = await this.ctx.post("/content-items", { data });
    assertOk(res, "createContentItem");
    return (await res.json()) as { id: number };
  }

  async deleteContentItem(id: number) {
    const res = await this.ctx.delete(`/content-items/${id}`);
    assertOk(res, `deleteContentItem(${id})`);
  }

  async listKnowledge() {
    const res = await this.ctx.get("/knowledge");
    return parseListEnvelope<{ id: number }>(res, "listKnowledge");
  }

  async createKnowledgeDoc(data: KnowledgeDocCreate) {
    const res = await this.ctx.post("/knowledge", { data });
    assertOk(res, "createKnowledgeDoc");
    return (await res.json()) as { id: number };
  }

  async deleteKnowledgeDoc(id: number) {
    const res = await this.ctx.delete(`/knowledge/${id}`);
    assertOk(res, `deleteKnowledgeDoc(${id})`);
  }

  // wipeAll deletes every record in every collection. Used between
  // specs so the seed fixtures are deterministic.
  //
  // The delete order is the reverse of the create order: child rows
  // first, parents last. If a future schema change introduces FK
  // constraints, deleting a parent that still has children will
  // 500; doing it in the right order keeps the teardown resilient
  // without a schema-level ON DELETE CASCADE.
  //
  // The four lists are fetched in parallel (independent GETs), but
  // the deletes themselves run in three sequential phases
  // (content-items → knowledge docs → scripts → topics) with
  // bounded concurrency inside each phase. Sequential phases
  // remove the non-determinism that a single Promise.all caused:
  // two specs running in parallel could each delete a parent's id
  // before the child referenced it, producing 500s that looked
  // like flakes.
  async wipeAll() {
    const [topics, scripts, items, docs] = await Promise.all([
      this.listTopics(),
      this.listScripts(),
      this.listContentItems(),
      this.listKnowledge(),
    ]);

    const phase1 = items.items.map((it) => () => this.deleteContentItem(it.id));
    await runWithConcurrency(phase1, 8);

    const phase2 = docs.items.map((d) => () => this.deleteKnowledgeDoc(d.id));
    await runWithConcurrency(phase2, 8);

    const phase3 = scripts.items.map((s) => () => this.deleteScript(s.id));
    await runWithConcurrency(phase3, 8);

    const phase4 = topics.items.map((t) => () => this.deleteTopic(t.id));
    await runWithConcurrency(phase4, 8);
  }

  async dispose() {
    await this.ctx.dispose();
  }

  // register hits the public /api/auth/register endpoint and is
  // gated by the API's REGISTRATION_ENABLED env var. The Playwright
  // config sets that env var to 1 so the cross-tenant test can
  // create two users without poking at the DB directly. Other
  // specs don't call this; they reset via wipeAll and use the
  // shared "anonymous" request context.
  async register(email: string, password: string, name: string) {
    const res = await this.ctx.post("/auth/register", {
      data: { email, password, name },
    });
    // 409 on duplicate email is fine — the spec can detect that
    // case via the status and treat it as "already seeded".
    if (!res.ok() && res.status() !== 409) {
      throw new Error(
        `register(${email}) failed: ${res.status()} ${res.statusText()}`,
      );
    }
    return res.status();
  }

  // login posts to /api/auth/login and returns the session cookie
  // value. We extract it from the Set-Cookie header because that's
  // the source of truth the browser will use in the real flow.
  async login(email: string, password: string): Promise<string> {
    const res = await this.ctx.post("/auth/login", {
      data: { email, password },
    });
    if (!res.ok()) {
      throw new Error(
        `login(${email}) failed: ${res.status()} ${res.statusText()}`,
      );
    }
    const setCookie = res.headers()["set-cookie"] || "";
    const m = setCookie.match(/opc_session=([^;]+)/);
    if (!m) {
      throw new Error(
        `login(${email}): no opc_session cookie in response (set-cookie=${setCookie})`,
      );
    }
    return m[1];
  }
}

// BrowserContext helper for two-user flows. The MultiTenant
// spec uses two contexts so a topic created by userA's cookies
// is invisible to userB's cookies — exactly the regression we
// want to pin. A single shared context would defeat the test
// because the cookie jar would always be the most-recent login.
export async function newAuthedContext(
  browser: import("@playwright/test").Browser,
  cookieValue: string,
) {
  const ctx = await browser.newContext();
  await ctx.addCookies([
    {
      name: "opc_session",
      value: cookieValue,
      domain: "localhost",
      path: "/api",
      httpOnly: true,
      secure: false,
      sameSite: "Lax",
    },
  ]);
  return ctx;
}
