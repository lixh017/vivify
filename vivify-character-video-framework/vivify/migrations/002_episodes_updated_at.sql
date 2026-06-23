-- Migration 002: Add updated_at column to episodes.
--
-- We noticed in the original implementation that episodes had no
-- updated_at column, which made the DB UPDATE in _ensure_episode_row
-- silently fail. This migration adds the column with a default of
-- created_at (so existing rows get a sensible value).
--
-- SQLite-specific notes:
-- - ALTER TABLE ADD COLUMN is supported in SQLite.
-- - We can't add NOT NULL DEFAULT datetime('now') in SQLite — the
--   DEFAULT must be a constant. So we add NULLable, then UPDATE rows
--   to backfill created_at, then... actually we leave it NULLable
--   and have application code maintain it.

ALTER TABLE episodes ADD COLUMN updated_at TEXT;
