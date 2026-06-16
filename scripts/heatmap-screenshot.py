#!/usr/bin/env python3
"""Playwright: log into console, mock the /api/observability/summary
fetch with real data fetched from the API, screenshot /observability.

The console's /api → API rewrite is now fixed, but cookies don't
share across :3001 ↔ :8083 origins. Easiest reliable approach:
route.fulfill() to intercept the console's fetch and return the
real API response verbatim, so the page renders exactly the same
data it would in production (modulo the cookie hop).
"""
from playwright.sync_api import sync_playwright
import sys
import urllib.request
import urllib.error
import json
from http.cookiejar import CookieJar

EMAIL = "demo-heatmap@e2e.local"
PASSWORD = "demo-heatmap-pass-123"
API = "http://127.0.0.1:8083"
URL = "http://127.0.0.1:3001"

# Pre-fetch the session cookie (used by the console's /api/auth/me check)
# AND the summary data (mocked into the page's fetch).
cookie_jar = CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookie_jar))
login_body = json.dumps({"email": EMAIL, "password": PASSWORD}).encode()
req = urllib.request.Request(
    f"{API}/api/auth/login",
    data=login_body,
    headers={"Content-Type": "application/json"},
    method="POST",
)
try:
    resp = opener.open(req, timeout=10)
    print(f"login: {resp.status}")
except urllib.error.HTTPError as e:
    print(f"login failed: {e.code} {e.reason}")
    sys.exit(1)
session_cookie = None
for c in cookie_jar:
    if c.name == "opc_session":
        session_cookie = c.value
        break
if not session_cookie:
    print("opc_session cookie not found in response")
    sys.exit(1)

# Pre-fetch the summary data (the page uses ?range=7d by default).
# We mock this back to the page; the mock is identical to what the
# real rewrite would return, so the page is functionally the same
# as a logged-in user fetching live.
summary_req = urllib.request.Request(
    f"{API}/api/observability/summary?range=7d",
    headers={"Cookie": f"opc_session={session_cookie}"},
)
summary_data = opener.open(summary_req, timeout=10).read().decode()
print(f"summary: {len(summary_data)} bytes, by_hour_dow cells: {json.loads(summary_data).get('by_hour_dow', []).__len__()}")

with sync_playwright() as p:
    browser = p.chromium.launch(
        headless=True,
        executable_path="/root/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome",
    )
    ctx = browser.new_context(viewport={"width": 1440, "height": 900}, device_scale_factor=2)
    # Add the session cookie to the console's origin so the
    # /api/auth/me check on page load sees a logged-in user.
    ctx.add_cookies([
        {"name": "opc_session", "value": session_cookie, "url": URL},
    ])
    page = ctx.new_page()

    # Intercept the console's /api/observability/summary fetch
    # and return the pre-fetched real data. The console then
    # renders the heatmap from the same payload it would have
    # gotten in production.
    def fulfill_observability(route, request):
        if "/api/observability/summary" in request.url:
            route.fulfill(
                status=200,
                content_type="application/json",
                body=summary_data,
            )
        else:
            route.continue_()
    page.route("**/*", fulfill_observability)

    page.goto(f"{URL}/observability", wait_until="domcontentloaded", timeout=15000)
    page.wait_for_timeout(3500)  # let the heatmap render
    print(f"URL after navigation: {page.url}")

    # Full-page screenshot
    page.screenshot(path="/tmp/heatmap-screenshot.png", full_page=True)
    print("OK: /tmp/heatmap-screenshot.png")

    # Tight crop of just the heatmap card
    try:
        card = page.locator("h2:has-text('活跃时段热力图')").locator(
            "xpath=ancestor::div[contains(@class,'rounded-lg')][1]"
        )
        if card.count() > 0:
            card.screenshot(path="/tmp/heatmap-card.png")
            print("OK: /tmp/heatmap-card.png")
    except Exception as e:
        print(f"WARN: card crop failed: {e}", file=sys.stderr)

    browser.close()
