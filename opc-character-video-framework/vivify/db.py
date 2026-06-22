"""vivify.db — SQLite state database.

The single source of truth for characters, episodes, assets, lessons,
publishes, and render jobs. All CLI commands read/write through this layer.
"""

import sqlite3
from pathlib import Path
from contextlib import contextmanager
from typing import Iterator

DEFAULT_DB_PATH = ".tmp/data/vivify.db"

SCHEMA = """
CREATE TABLE IF NOT EXISTS characters (
    id              TEXT PRIMARY KEY,           -- 'fengge', 'xiaohu'
    name            TEXT NOT NULL,             -- '峰哥'
    english_name    TEXT,
    species         TEXT,
    dir_path        TEXT NOT NULL,             -- absolute path to characters/<id>/
    character_yaml_path TEXT,
    canonical_dir   TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS episodes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    character_id    TEXT NOT NULL,
    episode_id      TEXT NOT NULL,             -- 'EP004'
    season          TEXT,
    tone            TEXT,                      -- 治愈/御宅/哲学/国潮
    platform        TEXT,                      -- 抖音/小红书/B站
    storyboard_path TEXT,
    script_path     TEXT,
    output_path     TEXT,
    title_card      TEXT,
    end_card        TEXT,
    quality_tier    TEXT,
    model_used      TEXT,
    target_dur_sec  INTEGER,
    actual_dur_sec  REAL,
    file_size_bytes INTEGER,
    cost_yuan       REAL,
    status          TEXT,                       -- 'pending', 'rendering', 'completed', 'failed'
    error_message   TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    render_started_at  TEXT,
    render_completed_at TEXT,
    UNIQUE(character_id, episode_id),
    FOREIGN KEY (character_id) REFERENCES characters(id)
);

CREATE TABLE IF NOT EXISTS shots (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    episode_id      INTEGER NOT NULL,
    shot_number     INTEGER NOT NULL,
    duration_sec    INTEGER,
    outfit_id       TEXT,
    scene_id        TEXT,
    image_path      TEXT,
    image_url       TEXT,
    video_path      TEXT,
    video_url       TEXT,
    kling_prompt    TEXT,
    voiceover_text  TEXT,
    qa_passed       INTEGER,                    -- 0 or 1
    qa_issues       TEXT,                       -- JSON list
    cost_yuan       REAL,
    model_used      TEXT,
    FOREIGN KEY (episode_id) REFERENCES episodes(id)
);

CREATE TABLE IF NOT EXISTS assets (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    asset_type      TEXT NOT NULL,             -- 'image', 'video', 'audio'
    character_id    TEXT,
    asset_path      TEXT NOT NULL,
    url             TEXT,
    file_size_bytes INTEGER,
    duration_sec    REAL,
    width           INTEGER,
    height          INTEGER,
    metadata_json   TEXT,
    tags            TEXT,                        -- comma-separated
    used_in_episodes TEXT,                       -- JSON list of "char-ep" ids
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS publishes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    episode_id      INTEGER NOT NULL,
    platform        TEXT NOT NULL,
    published_url   TEXT,
    published_at    TEXT NOT NULL DEFAULT (datetime('now')),
    title           TEXT,
    description     TEXT,
    hashtags        TEXT,
    view_count      INTEGER DEFAULT 0,
    like_count      INTEGER DEFAULT 0,
    comment_count   INTEGER DEFAULT 0,
    completion_rate REAL,
    FOREIGN KEY (episode_id) REFERENCES episodes(id)
);

CREATE TABLE IF NOT EXISTS render_jobs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    episode_id      INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued',
    started_at      TEXT,
    completed_at    TEXT,
    error_message   TEXT,
    retries         INTEGER DEFAULT 0,
    FOREIGN KEY (episode_id) REFERENCES episodes(id)
);

CREATE TABLE IF NOT EXISTS lessons (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    character_id    TEXT,                       -- NULL = cross-IP
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    category        TEXT,                        -- 'prompt-engineering', 'ip-specific', etc.
    status          TEXT,                        -- 'validated', 'hypothesis', 'draft'
    source          TEXT,                        -- 'EP004', 'manual', etc.
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_episodes_character ON episodes(character_id);
CREATE INDEX IF NOT EXISTS idx_shots_episode ON shots(episode_id);
CREATE INDEX IF NOT EXISTS idx_assets_character ON assets(character_id);
CREATE INDEX IF NOT EXISTS idx_lessons_character ON lessons(character_id);
"""


def get_db_path(db_path: str = None) -> Path:
    """Resolve DB path. Defaults to .tmp/data/vivify.db in cwd (gitignored)."""
    if db_path:
        return Path(db_path)
    return Path.cwd() / DEFAULT_DB_PATH


def init_db(db_path: str = None) -> Path:
    """Create DB if not exists, run schema. Returns resolved path."""
    path = get_db_path(db_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    with sqlite3.connect(path) as conn:
        conn.executescript(SCHEMA)
        conn.commit()
    return path


@contextmanager
def connect(db_path: str = None) -> Iterator[sqlite3.Connection]:
    """Context manager for DB connection. Auto-commits on success."""
    path = get_db_path(db_path)
    conn = sqlite3.connect(path)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys = ON")
    try:
        yield conn
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()