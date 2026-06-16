#!/usr/bin/env python3
"""Seed call_log with a realistic-looking 7-day activity pattern:
- Weekday business hours (Mon-Fri 09:00-18:00) are HEAVY
- Weekday evening + weekend morning is MEDIUM
- Late night + early morning is BARE
- Tuesday 14:00 is the PEAK (a "team standup demo" pattern)

Inserts via SQLite directly because the API's /api/ai/topics
endpoint is rate-limited and demo-mode; SQL is the fastest
way to get a heatmap-worthy distribution.
"""
import sqlite3
import random
import datetime

DB = "/tmp/obs-demo.db"
USER_ID = 1
PROVIDERS = ["anthropic", "openai", "minimax"]
SKILLS = ["ai_topics", "ai_humanize", "ai_score", "cover_generate", "platform_adapt"]

now = datetime.datetime(2026, 6, 16, 18, 0, 0)  # Monday evening
conn = sqlite3.connect(DB)
cur = conn.cursor()

# Wipe existing rows for a clean screenshot
cur.execute("DELETE FROM call_logs")

random.seed(42)  # deterministic
rows = []
N = 0
# Iterate 7 days back from "now"
for day_offset in range(7):
    day = now - datetime.timedelta(days=day_offset)
    # day.weekday(): Mon=0, Sun=6
    dow = day.weekday()
    is_weekend = dow >= 5
    for hour in range(24):
        # Density: business hours heavy, nights bare
        if is_weekend:
            base = 3 if 10 <= hour <= 20 else 1
        else:
            if 9 <= hour <= 18:
                base = 18
            elif 19 <= hour <= 22:
                base = 8
            elif 7 <= hour <= 8:
                base = 5
            else:
                base = 1
        # Special: Tuesday 14:00 spike (team demo)
        if dow == 1 and hour == 14:
            base = 45
        # Wednesday 10:00 medium spike (recurring standup)
        if dow == 2 and hour == 10:
            base = 25
        # Random jitter
        n = max(0, base + random.randint(-2, 2))
        for _ in range(n):
            minute = random.randint(0, 59)
            second = random.randint(0, 59)
            ts = day.replace(hour=hour, minute=minute, second=second)
            # 5% error rate on heavy hours, 0% on light
            status = 2 if (n > 5 and random.random() < 0.05) else 0
            provider = random.choice(PROVIDERS)
            skill = random.choice(SKILLS)
            cost = random.choice([3, 5, 7, 10]) if status == 0 else 0
            latency = random.randint(80, 2500) if status == 0 else random.randint(10, 200)
            rows.append((USER_ID, skill, provider, status, latency, cost, ts.isoformat() + "Z"))

# Use the actual column names from call_logs table
cur.executemany(
    """INSERT INTO call_logs
       (user_id, skill, provider, status, latency_ms, cost_cents, created_at)
       VALUES (?, ?, ?, ?, ?, ?, ?)""",
    rows,
)
conn.commit()
print(f"Seeded {len(rows)} call_log rows")
# Quick sanity check
cur.execute("SELECT strftime('%w', created_at) AS dow, strftime('%H', created_at) AS hour, COUNT(*) FROM call_logs GROUP BY dow, hour ORDER BY dow, hour")
buckets = cur.fetchall()
print(f"Active (dow, hour) buckets: {len(buckets)}")
cur.execute("SELECT MIN(created_at), MAX(created_at) FROM call_logs")
print(f"Time range: {cur.fetchone()}")
conn.close()
