#!/usr/bin/env python3
"""Multi-persona seed for the OPC demo showcase.

Creates a realistic operator + 2 creators + 1 agent identity,
configures credentials, and seeds a week of call_log that
tells the "Panda IP" content workflow story:

  - 1 operator (admin): panda-admin
  - 2 creators: creator-1, creator-2 (onboarded by the operator)
  - 2 agent API keys (one anthropic, one openai) used by
    creator-1 to drive external automation

Plus a week of call_log distributed across:
  - 5 skills (ai_topics, ai_humanize, ai_score, cover_generate,
    platform_adapt)
  - 3 providers (anthropic, openai, minimax)
  - creator-1 + creator-2 (operator has 0 calls — admin should
    not be the busy actor on the dashboard)
  - weekday business hours heavy, Tue 14:00 spike, weekend light
  - ~3% error rate on heavy hours

Run via:  python3 scripts/demo-seed.py /tmp/demo.db
"""
import sqlite3
import sys
import os
import random
import datetime
import hashlib
import base64

# Args
if len(sys.argv) < 2:
    print("usage: demo-seed.py <db-path>")
    sys.exit(1)
DB = sys.argv[1]

# Operators, creators, agent
USERS = [
    # (email, password, name, role)
    ("panda-admin@e2e.local",  "panda-admin-pass",   "Panda Admin",   "operator"),
    ("creator-1@beta.com",     "creator-1-pass-7e2", "Creator One",   "creator"),
    ("creator-2@beta.com",     "creator-2-pass-9b1", "Creator Two",   "creator"),
]
# ENCRYPTION_KEY from the smoke scripts. Used to encrypt
# credentials' plaintext_key the same way the Go auth package
# does. The Go side uses AES-GCM with a 32-byte key; for the
# seed we just write plaintext (Go will treat ciphertext as
# opaque) and let the demo flow rewrite if needed. Actually
# for a working demo we DO need the credentials to round-trip,
# so we use the Go auth.Encrypt function via subprocess? No,
# let me just write the credentials as the Go API would, then
# insert them via the API after seed.
# Simplest: write users + sessions + call_log here, then a
# second pass (scripts/demo-journey.sh) registers the users
# via the API and stores the session cookie.

random.seed(2026)  # deterministic demo

now = datetime.datetime(2026, 6, 16, 18, 0, 0)  # Mon evening

conn = sqlite3.connect(DB)
cur = conn.cursor()

# Wipe ONLY call_log + auxiliary tables. Users are created by
# the API (POST /api/auth/register) so the bcrypt hash is
# correct and the demo journey can actually log in.
for table in ("call_logs", "credentials", "sessions",
              "agents", "topics", "scripts", "content_items"):
    cur.execute(f"DELETE FROM {table}")

# Look up user IDs the API already created.
user_ids = {}
for email, _pw, _name, _role in USERS:
    cur.execute("SELECT id FROM users WHERE email = ?", (email,))
    row = cur.fetchone()
    if not row:
        print(f"WARN: user {email} not found; run scripts/demo-journey.sh first to register")
        sys.exit(1)
    user_ids[email] = row[0]
print(f"Users (from API): {user_ids}")

# We skip credentials + call_log insert here — the demo
# journey script registers the user via the API (so the
# bcrypt hash is correct) and creates credentials + call_log
# via real HTTP calls. The journey script then captures
# screenshots of the result.

# But for the observability screenshot to look interesting,
# we need call_log data. Add it here for the 2 creators.
PROVIDERS = ["anthropic", "openai", "minimax"]
SKILLS = ["ai_topics", "ai_humanize", "ai_score", "cover_generate", "platform_adapt"]
creator_ids = [user_ids["creator-1@beta.com"], user_ids["creator-2@beta.com"]]

rows = []
for day_offset in range(7):
    day = now - datetime.timedelta(days=day_offset)
    dow = day.weekday()
    is_weekend = dow >= 5
    for hour in range(24):
        # Density: business hours heavy, nights bare
        if is_weekend:
            base = 2 if 10 <= hour <= 20 else 0
        else:
            if 9 <= hour <= 18:
                base = 12
            elif 19 <= hour <= 22:
                base = 5
            elif 7 <= hour <= 8:
                base = 3
            else:
                base = 0
        # Special: Tuesday 14:00 spike (team demo + content
        # planning block — 2 creators both running)
        if dow == 1 and hour == 14:
            base = 32
        # Wednesday 10:00 medium spike (recurring standup)
        if dow == 2 and hour == 10:
            base = 18
        # Friday 16:00 wrap-up burst
        if dow == 4 and hour == 16:
            base = 22
        for _ in range(base):
            minute = random.randint(0, 59)
            second = random.randint(0, 59)
            ts = day.replace(hour=hour, minute=minute, second=second)
            uid = random.choice(creator_ids)
            provider = random.choice(PROVIDERS)
            skill = random.choice(SKILLS)
            # Mostly OK; 3% error rate on heavy hours
            status = 2 if (base > 5 and random.random() < 0.03) else 0
            cost = random.choice([3, 5, 7, 10, 12]) if status == 0 else 0
            latency = random.randint(80, 2500) if status == 0 else random.randint(10, 200)
            # actor_type: 70% human (creator session), 30% agent (X-API-Key)
            actor_type = "agent" if random.random() < 0.30 else "human"
            agent_id = random.randint(1, 2) if actor_type == "agent" else 0
            rows.append((
                uid, agent_id, actor_type, skill, provider,
                0 if status == 0 else None,  # credential_id
                status, latency, cost, 0,  # cost_units
                "CNY", None,  # error_message
                ts.isoformat() + "Z",
            ))

cur.executemany(
    """INSERT INTO call_logs
       (user_id, agent_id, actor_type, skill, provider,
        credential_id, status, latency_ms, cost_cents, cost_units,
        cost_currency, error_message, created_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
    rows,
)
conn.commit()
print(f"call_log rows: {len(rows)}")

# Summary
cur.execute("SELECT COUNT(*) FROM call_logs WHERE status=0")
ok = cur.fetchone()[0]
cur.execute("SELECT COUNT(*) FROM call_logs WHERE status=2")
err = cur.fetchone()[0]
cur.execute("SELECT COALESCE(SUM(cost_cents), 0) FROM call_logs")
cost = cur.fetchone()[0]
print(f"  ok: {ok}, errors: {err}, total cost: {cost} cents (¥{cost/100:.2f})")
conn.close()
print("OK")
