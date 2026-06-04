#!/usr/bin/env bash
#
# scripts/backup.sh — daily SQLite backup with 7-day retention.
#
# - Copies the live SQLite database out of the `opc-data` volume mount
#   (or wherever $DB_PATH points) to $BACKUP_DIR.
# - Gzips the copy (sqlite3 is append-friendly; gzip is fine for archives).
# - Deletes any backup older than 7 days.
#
# Environment overrides:
#   DB_PATH         Path to the live database (default: /data/opc.db)
#   BACKUP_DIR      Where to write backups (default: /backups)
#   RETENTION_DAYS  How many days of backups to keep (default: 7)
#   USE_SQLITE3     If set to 1, use `sqlite3 .backup` for a consistent,
#                   atomic snapshot. Requires the sqlite3 CLI to be on
#                   the host. Otherwise the script falls back to cp.
#
# Recommended cron entry (runs at 03:00 every day):
#   0 3 * * *  /opt/opc/scripts/backup.sh >> /var/log/opc-backup.log 2>&1
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
DB_PATH="${DB_PATH:-/data/opc.db}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"
USE_SQLITE3="${USE_SQLITE3:-}"
DATE_STAMP="$(date -u +%Y-%m-%d)"
TIMESTAMP="$(date -u +%Y-%m-%dT%H%M%SZ)"

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log()  { printf '[backup %s] %s\n' "$TIMESTAMP" "$*"; }
warn() { printf '[backup %s] WARN: %s\n' "$TIMESTAMP" "$*" >&2; }
die()  { printf '[backup %s] ERROR: %s\n' "$TIMESTAMP" "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
if [[ ! -f "$DB_PATH" ]]; then
  die "Database file not found: $DB_PATH"
fi

mkdir -p "$BACKUP_DIR"
if [[ ! -w "$BACKUP_DIR" ]]; then
  die "Backup directory not writable: $BACKUP_DIR"
fi

# ---------------------------------------------------------------------------
# Snapshot
# ---------------------------------------------------------------------------
BACKUP_FILE="$BACKUP_DIR/opc-${DATE_STAMP}.db"
GZ_FILE="$BACKUP_FILE.gz"

log "Backing up $DB_PATH -> $GZ_FILE"

if [[ -n "$USE_SQLITE3" ]] && command -v sqlite3 >/dev/null 2>&1; then
  # Atomic, consistent snapshot using SQLite's online backup API.
  sqlite3 "$DB_PATH" ".backup '$BACKUP_FILE'"
else
  # Fallback: simple copy. SQLite's WAL is safely recoverable, but a
  # warm backup may include unflushed WAL pages. Acceptable for a daily
  # snapshot of a low-traffic single-writer DB.
  if ! cp -f "$DB_PATH" "$BACKUP_FILE"; then
    die "cp failed for $DB_PATH -> $BACKUP_FILE"
  fi
fi

# Compress (best-effort; the gzipped file is what we keep).
gzip -f "$BACKUP_FILE"

# Verify the archive is non-empty.
if [[ ! -s "$GZ_FILE" ]]; then
  die "Backup file is empty: $GZ_FILE"
fi

log "Created $(basename "$GZ_FILE") ($(stat -c%s "$GZ_FILE" 2>/dev/null || stat -f%z "$GZ_FILE") bytes)"

# ---------------------------------------------------------------------------
# Retention
# ---------------------------------------------------------------------------
# Use -mtime +N to find files modified MORE than N days ago, then delete.
# We rely on the date in the filename (YYYY-MM-DD) which matches the mtime.
log "Pruning backups older than $RETENTION_DAYS days in $BACKUP_DIR"
DELETED=0
while IFS= read -r -d '' f; do
  rm -f -- "$f"
  DELETED=$((DELETED + 1))
  log "  pruned: $(basename "$f")"
done < <(find "$BACKUP_DIR" -maxdepth 1 -type f \
            -name 'opc-*.db.gz' \
            -mtime "+${RETENTION_DAYS}" -print0)

log "Retention cleanup complete (deleted $DELETED file(s))"
log "Done."
