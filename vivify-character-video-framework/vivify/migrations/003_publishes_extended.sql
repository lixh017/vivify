-- Migration 003: Extend publishes table for platform-agnostic analytics.
--
-- Context: the original publishes table was minimal (just published_url
-- + 3 counters). After publishing-api-spec research (June 2026), we
-- identified gaps that block:
--   1. Idempotency on retry — no platform_video_id to look up the
--      post on the platform after upload
--   2. Status tracking — no upload_status to distinguish pending/uploading/
--      failed/succeeded
--   3. Cross-platform analytics — share_count + collect_count are
--      standard fields on 抖音 / 小红书 / B站 but missing here
--
-- Fields added (all nullable for backwards compat):
--   platform_video_id  — the platform-assigned ID (e.g., Douyin aweme_id)
--   upload_status      — 'pending' | 'uploading' | 'moderating' |
--                         'succeeded' | 'failed' | 'rejected'
--   share_count        — platform-reported share count
--   collect_count      — 小红书收藏 / B站收藏
--   last_refreshed_at  — when analytics were last fetched
--
-- Note: published_url stays the canonical "human-facing URL" column.
-- platform_video_id is for API lookups (we look up by ID, not URL).
--
-- This migration is forward-only (no DOWN). To undo: write 004.

ALTER TABLE publishes ADD COLUMN platform_video_id TEXT;
ALTER TABLE publishes ADD COLUMN upload_status TEXT DEFAULT 'pending';
ALTER TABLE publishes ADD COLUMN share_count INTEGER DEFAULT 0;
ALTER TABLE publishes ADD COLUMN collect_count INTEGER DEFAULT 0;
ALTER TABLE publishes ADD COLUMN last_refreshed_at TEXT;

-- Backfill upload_status for any rows that pre-date this migration.
-- The column default is 'pending', so we override to 'succeeded' for rows
-- that already have a published_url (i.e. the upload must have completed).
UPDATE publishes SET upload_status = 'succeeded'
  WHERE upload_status = 'pending' AND published_url IS NOT NULL;