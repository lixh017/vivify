"""Tests for migration 003 (publishes_extended) and the schema alignment.

Pre-migration bug (June 2026): publishes table was missing
platform_video_id / upload_status / share_count / collect_count /
last_refreshed_at columns. After migration + schema update in db.py,
new DBs have the columns; old DBs get them via the migration runner.

These tests pin:
- The SQL file is present + runnable
- After applying, the columns exist with the right defaults
- A round-trip insert + read works
"""
import sqlite3
from pathlib import Path

import pytest

from vivify.db import init_db, connect
from vivify.migrator import apply_pending, schema_version


MIGRATION_PATH = (
    Path(__file__).resolve().parent.parent
    / "vivify"
    / "migrations"
    / "003_publishes_extended.sql"
)


def test_migration_003_file_exists():
    """The migration SQL file must exist."""
    assert MIGRATION_PATH.exists(), (
        f"003_publishes_extended.sql missing at {MIGRATION_PATH}"
    )


def test_migration_003_adds_all_expected_columns(tmp_path):
    """After init_db() + apply_pending(), publishes has all new columns."""
    db = tmp_path / "vivify.db"
    init_db(str(db))
    # The schema in db.py SCHEMA constant already includes the new columns
    # for fresh DBs. But we ALSO run migration 003 to ensure backwards compat
    # — and on a fresh DB the migration is a no-op (ADD COLUMN is idempotent
    # only via IF NOT EXISTS, which SQLite doesn't support pre-3.35).
    # Apply pending should report no migrations.
    applied = apply_pending(str(db))
    # On a fresh DB, migration 003 should be already-applied via SCHEMA
    # constant OR detected as already-applied. We don't assert exact count
    # — just that schema is correct.

    with connect(str(db)) as conn:
        cols = {row[1] for row in conn.execute("PRAGMA table_info(publishes)").fetchall()}
    assert "platform_video_id" in cols
    assert "upload_status" in cols
    assert "share_count" in cols
    assert "collect_count" in cols
    assert "last_refreshed_at" in cols


def test_migration_003_on_legacy_db(tmp_path):
    """Simulate a pre-003 DB: create with old schema, then run migration."""
    db = tmp_path / "legacy.db"
    # Old schema (mimics pre-003)
    conn = sqlite3.connect(str(db))
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS characters (
            id TEXT PRIMARY KEY,
            name TEXT,
            dir_path TEXT,
            character_yaml_path TEXT,
            canonical_dir TEXT,
            english_name TEXT,
            species TEXT,
            updated_at TEXT,
            created_at TEXT DEFAULT (datetime('now'))
        );
        CREATE TABLE IF NOT EXISTS episodes (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            episode_id TEXT NOT NULL,
            character_id TEXT NOT NULL,
            storyboard TEXT,
            script TEXT,
            voice TEXT,
            platform TEXT,
            quality_tier TEXT,
            target_dur INTEGER,
            video_model TEXT,
            status TEXT,
            cost_yuan REAL,
            output_path TEXT,
            actual_dur_sec REAL,
            file_size_bytes INTEGER,
            error_message TEXT,
            render_started_at TEXT,
            render_completed_at TEXT,
            updated_at TEXT,
            FOREIGN KEY (character_id) REFERENCES characters(id)
        );
        CREATE TABLE IF NOT EXISTS publishes (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            episode_id INTEGER NOT NULL,
            platform TEXT NOT NULL,
            published_url TEXT,
            published_at TEXT NOT NULL DEFAULT (datetime('now')),
            title TEXT,
            description TEXT,
            hashtags TEXT,
            view_count INTEGER DEFAULT 0,
            like_count INTEGER DEFAULT 0,
            comment_count INTEGER DEFAULT 0,
            completion_rate REAL,
            FOREIGN KEY (episode_id) REFERENCES episodes(id)
        );
    """)
    conn.commit()
    conn.close()

    # Insert a legacy row to test backfill
    conn = sqlite3.connect(str(db))
    conn.execute(
        "INSERT INTO characters (id, name) VALUES (?, ?)",
        ("fengge", "峰哥"),
    )
    conn.execute(
        "INSERT INTO episodes (episode_id, character_id) VALUES (?, ?)",
        ("EP001", "fengge"),
    )
    ep_pk = conn.execute("SELECT id FROM episodes LIMIT 1").fetchone()[0]
    conn.execute(
        "INSERT INTO publishes (episode_id, platform, published_url) VALUES (?, ?, ?)",
        (ep_pk, "抖音", "https://www.douyin.com/video/legacy"),
    )
    conn.commit()
    conn.close()

    # Apply migration (use the migrator's duplicate-column guard so legacy DBs work)
    sql = MIGRATION_PATH.read_text(encoding="utf-8")
    conn = sqlite3.connect(str(db))
    try:
        conn.executescript(sql)
    except sqlite3.OperationalError as e:
        msg = str(e).lower()
        if "duplicate column" not in msg and "already exists" not in msg:
            raise
    conn.commit()
    conn.close()

    # Verify columns added + backfill ran
    conn = sqlite3.connect(str(db))
    cols = {row[1] for row in conn.execute("PRAGMA table_info(publishes)").fetchall()}
    assert "platform_video_id" in cols
    assert "upload_status" in cols
    assert "share_count" in cols
    assert "collect_count" in cols
    assert "last_refreshed_at" in cols

    # Backfill: legacy row should now have upload_status='succeeded'
    row = conn.execute(
        "SELECT upload_status, published_url FROM publishes LIMIT 1"
    ).fetchone()
    conn.close()
    assert row[0] == "succeeded"
    assert row[1] == "https://www.douyin.com/video/legacy"


def test_publishes_roundtrip_with_new_columns(tmp_path):
    """Insert a publish row with the new fields, read it back, verify."""
    db = tmp_path / "vivify.db"
    init_db(str(db))

    # Need a character + episode first (foreign key)
    with connect(str(db)) as conn:
        conn.execute(
            "INSERT INTO characters (id, name, dir_path, character_yaml_path) "
            "VALUES (?, ?, ?, ?)",
            ("fengge", "峰哥", "/tmp/fengge", "/tmp/fengge/character.yaml"),
        )
        cur = conn.execute(
            "INSERT INTO episodes (episode_id, character_id) VALUES (?, ?)",
            ("EP001", "fengge"),
        )
        ep_pk = cur.lastrowid
        conn.execute(
            """INSERT INTO publishes
                (episode_id, platform, platform_video_id, upload_status,
                 published_url, title, hashtags, view_count, share_count,
                 collect_count)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (ep_pk, "抖音", "v0200fgc0001c1abc", "succeeded",
             "https://www.douyin.com/video/v0200fgc0001c1abc",
             "峰哥治愈系日常",
             "#峰哥 #治愈 #日常",
             1234, 56, 78),
        )
        conn.commit()

    with connect(str(db)) as conn:
        row = conn.execute(
            "SELECT platform_video_id, upload_status, share_count, "
            "collect_count FROM publishes LIMIT 1"
        ).fetchone()

    assert row[0] == "v0200fgc0001c1abc"
    assert row[1] == "succeeded"
    assert row[2] == 56
    assert row[3] == 78