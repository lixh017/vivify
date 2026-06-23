-- Migration 001: Initial schema for 点睛 / Vivify platform DB.
-- This is the baseline. All future migrations build on top of this.
--
-- Schema source of truth: vivify/db.py SCHEMA constant.
-- Idempotent: uses CREATE TABLE IF NOT EXISTS, so safe to re-run on a fresh DB.

CREATE TABLE IF NOT EXISTS characters (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    english_name    TEXT,
    species         TEXT,
    dir_path        TEXT NOT NULL,
    character_yaml_path TEXT,
    canonical_dir   TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS episodes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    character_id    TEXT NOT NULL,
    episode_id      TEXT NOT NULL,
    season          TEXT,
    tone            TEXT,
    platform        TEXT,
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
    status          TEXT,
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
    qa_passed       INTEGER,
    qa_issues       TEXT,
    cost_yuan       REAL,
    model_used      TEXT,
    FOREIGN KEY (episode_id) REFERENCES episodes(id)
);

CREATE TABLE IF NOT EXISTS assets (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    asset_type      TEXT NOT NULL,
    character_id    TEXT,
    asset_path      TEXT NOT NULL,
    url             TEXT,
    file_size_bytes INTEGER,
    duration_sec    REAL,
    width           INTEGER,
    height          INTEGER,
    metadata_json   TEXT,
    tags            TEXT,
    used_in_episodes TEXT,
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
    character_id    TEXT,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    category        TEXT,
    status          TEXT,
    source          TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_episodes_character ON episodes(character_id);
CREATE INDEX IF NOT EXISTS idx_shots_episode ON shots(episode_id);
CREATE INDEX IF NOT EXISTS idx_assets_character ON assets(character_id);
CREATE INDEX IF NOT EXISTS idx_lessons_character ON lessons(character_id);
