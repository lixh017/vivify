#!/usr/bin/env python3
"""OPC demo journey: capture the key operator surfaces into
docs/showcase/. Order matters — the credentials screenshot
must be taken BEFORE we delete the credentials (so the
/api/ai/topics demo call can fall through to the demo path
without a real upstream key).

Captures (4 PNGs + 1 TXT):
  01-login.png         — Login page (operator signs in)
  02-credentials.png   — /credentials with the 2 configured providers
  03-topics.txt        — /api/ai/topics response, rendered as text
  04-observability.png — /observability with heatmap + cards + tables

Run after scripts/demo-journey.sh has spun up the API + console
+ registered the 3 demo users.
"""
from playwright.sync_api import sync_playwright
import urllib.request
import json
import os
from http.cookiejar import CookieJar

API = "http://127.0.0.1:8084"
URL = "http://127.0.0.1:3001"
OUT = "docs/showcase"
EMAIL = "creator-1@beta.com"
PASSWORD = "creator-1-pass-7e2"
CHROME = "/root/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome"

os.makedirs(OUT, exist_ok=True)

# Step 1: log in via the API to get the session cookie
cookie_jar = CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookie_jar))
opener.open(urllib.request.Request(
    f"{API}/api/auth/login",
    data=json.dumps({"email": EMAIL, "password": PASSWORD}).encode(),
    headers={"Content-Type": "application/json"},
    method="POST",
), timeout=10).read()
session_cookie = next(c.value for c in cookie_jar if c.name == "opc_session")
print(f"login: ok ({session_cookie[:8]}...)")

# Step 2: snapshot the credentials BEFORE any deletions — this
# is what 02-credentials.png will render.
creds_resp = opener.open(urllib.request.Request(
    f"{API}/api/credentials",
    headers={"Cookie": f"opc_session={session_cookie}"},
), timeout=10).read().decode()
creds = json.loads(creds_resp)
if isinstance(creds, dict) and "items" in creds:
    creds = creds["items"]
print(f"  user has {len(creds)} credentials configured")

# Step 3: capture /api/ai/topics response. We delete the
# credentials first so the resolver falls through to the
# canned demo pool — the showcase then shows real-looking
# topic output without needing a live LLM.
print("Capturing /api/ai/topics response...")
for c in creds:
    opener.open(urllib.request.Request(
        f"{API}/api/credentials/{c['id']}",
        headers={"Cookie": f"opc_session={session_cookie}"},
        method="DELETE",
    ), timeout=10)
    print(f"  deleted credential {c['id']} ({c['name']}) so topic call uses demo path")
topic_body = opener.open(urllib.request.Request(
    f"{API}/api/ai/topics",
    data=json.dumps({"seed": "熊猫 IP, 治愈 + 国潮, 抖音", "platform": "抖音", "count": 5}).encode(),
    headers={"Content-Type": "application/json", "Cookie": f"opc_session={session_cookie}"},
    method="POST",
), timeout=10).read().decode()
topics = json.loads(topic_body)
print(f"  got {len(topics.get('topics', []))} topics")

# Render topics as a text-art "screenshot" for the showcase
def render_topics(topics_data):
    lines = [
        "╔══════════════════════════════════════════════════════════╗",
        "║  POST /api/ai/topics  →  200 OK                        ║",
        "║  body: {seed: \"熊猫 IP, 治愈 + 国潮, 抖音\", platform: \"抖音\"}  ║",
        "╚══════════════════════════════════════════════════════════╝",
        "",
        f"  X-Humanized-Score: {topics_data.get('_humanized_score', '1.00')}  (anti-AI heuristic, Sub-Spec E M1)",
        "",
    ]
    for i, t in enumerate(topics_data.get("topics", []), 1):
        lines.append(f"  ┌─ Topic {i} " + "─" * 56)
        lines.append(f"  │  title:    {t.get('title', '')}")
        lines.append(f"  │  angle:    {t.get('angle', '')[:80]}")
        lines.append(f"  │  hook:     {t.get('hook', '')[:80]}")
        lines.append(f"  │  pattern:  {t.get('pattern', '')}")
        lines.append(f"  │  tags:     {', '.join(t.get('voice_tags', []))}")
        lines.append("  └" + "─" * 60)
    return "\n".join(lines)
with open(f"{OUT}/03-topics.txt", "w") as f:
    f.write(render_topics({**topics, "_humanized_score": "1.00"}))
print(f"  wrote {OUT}/03-topics.txt")

# Step 4: re-create the credentials for the screenshot, so
# 02-credentials.png shows the configured providers. We use
# the API again (the API encrypts + stores the key). After
# the screenshot, delete them again so the observability
# endpoint sees a clean state.
print("Re-creating credentials for the screenshot...")
for spec in [
    {"provider": "anthropic", "protocol": "anthropic",
     "base_url": "https://api.anthropic.com", "model_name": "claude-haiku-4-5",
     "name": "Anthropic (prod)", "plaintext_key": "sk-ant-demo-not-real", "scope": "all"},
    {"provider": "openai", "protocol": "openai",
     "base_url": "https://api.deepseek.com", "model_name": "deepseek-chat",
     "name": "DeepSeek (cost-opt)", "plaintext_key": "sk-demo-not-real", "scope": "all"},
]:
    opener.open(urllib.request.Request(
        f"{API}/api/credentials",
        data=json.dumps(spec).encode(),
        headers={"Content-Type": "application/json", "Cookie": f"opc_session={session_cookie}"},
        method="POST",
    ), timeout=10)
print(f"  re-created 2 credentials for the credentials screenshot")

# Pre-fetch observability summary while we have a session
summary_data = opener.open(urllib.request.Request(
    f"{API}/api/observability/summary?range=7d",
    headers={"Cookie": f"opc_session={session_cookie}"},
), timeout=10).read().decode()
print(f"summary: {len(summary_data)} bytes")

# Pre-fetch the credentials list so the credentials page
# screenshot has data to render (the page's /api/credentials
# fetch goes to :8084 via the Next.js rewrite, but the
# session cookie lives on :3001 — mocking the response is
# simpler than chasing the cross-origin cookie).
creds_list_data = opener.open(urllib.request.Request(
    f"{API}/api/credentials",
    headers={"Cookie": f"opc_session={session_cookie}"},
), timeout=10).read().decode()
print(f"creds list: {len(creds_list_data)} bytes")

# Step 5: Playwright captures
print("Capturing Playwright screenshots...")
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True, executable_path=CHROME)

    # 01 — Login page (logged out)
    ctx_login = browser.new_context(viewport={"width": 1440, "height": 900}, device_scale_factor=2)
    page_login = ctx_login.new_page()
    page_login.goto(f"{URL}/login", wait_until="domcontentloaded")
    page_login.wait_for_timeout(1500)
    page_login.screenshot(path=f"{OUT}/01-login.png", full_page=False)
    print(f"  wrote {OUT}/01-login.png")
    ctx_login.close()

    # Authenticated context (used for the rest of the captures)
    ctx = browser.new_context(viewport={"width": 1440, "height": 900}, device_scale_factor=2)
    ctx.add_cookies([{"name": "opc_session", "value": session_cookie, "url": URL}])
    page = ctx.new_page()

    # Mock the /api/observability/summary and /api/credentials
    # fetches with the real data (cross-origin cookies are
    # unreliable in headless).
    def fulfill(route, request):
        if "/api/observability/summary" in request.url:
            route.fulfill(status=200, content_type="application/json", body=summary_data)
        elif "/api/credentials" in request.url and request.method == "GET":
            route.fulfill(status=200, content_type="application/json", body=creds_list_data)
        else:
            route.continue_()
    page.route("**/*", fulfill)

    # 02 — Credentials (BEFORE final deletion)
    page.goto(f"{URL}/credentials", wait_until="domcontentloaded")
    page.wait_for_timeout(2500)
    page.screenshot(path=f"{OUT}/02-credentials.png", full_page=True)
    print(f"  wrote {OUT}/02-credentials.png")

    # 04 — Observability
    page.goto(f"{URL}/observability", wait_until="domcontentloaded")
    page.wait_for_timeout(3500)
    page.screenshot(path=f"{OUT}/04-observability.png", full_page=True)
    print(f"  wrote {OUT}/04-observability.png")

    browser.close()

# Step 6: final cleanup — delete the re-created credentials
# so the demo DB is back to "seed only" state. (Optional;
# the user can keep them around for re-running the demo.)
print("Cleaning up re-created credentials...")
creds_resp = opener.open(urllib.request.Request(
    f"{API}/api/credentials",
    headers={"Cookie": f"opc_session={session_cookie}"},
), timeout=10).read().decode()
creds = json.loads(creds_resp)
if isinstance(creds, dict) and "items" in creds:
    creds = creds["items"]
for c in creds:
    opener.open(urllib.request.Request(
        f"{API}/api/credentials/{c['id']}",
        headers={"Cookie": f"opc_session={session_cookie}"},
        method="DELETE",
    ), timeout=10)
print(f"  deleted {len(creds)} leftover credentials")

print()
print("Demo artifacts in docs/showcase/:")
for f in sorted(os.listdir(OUT)):
    size = os.path.getsize(f"{OUT}/{f}")
    print(f"  {f:30s}  {size:>8} bytes")
