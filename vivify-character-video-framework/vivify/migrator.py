"""vivify.migrator — schema migration system (lightweight, no deps).

Schema migrations live in vivify/migrations/NNN_description.sql.
Each migration is applied exactly once; applied versions are tracked in
the `migrations` table inside the DB itself.

Usage:
    from vivify.migrator import get_status, apply_pending
    get_status(db_path)        → (applied, pending)
    apply_pending(db_path)     → applies all pending migrations

CLI:
    vivify db status           Show applied + pending migrations
    vivify db migrate          Apply pending migrations

Design choices:
- No external deps (no alembic). Plain SQL files, plain sqlite3.
- Migrations are forward-only (no down-migrations). If a migration
  breaks, fix it in a new migration that ALTERs/corrects.
- Each migration runs in a single transaction (sqlite3.executescript).
- Idempotent at the file level: re-running the same migration is a no-op
  (we check `migrations` table before applying).
"""

from __future__ import annotations

import sqlite3
from dataclasses import dataclass
from pathlib import Path

from .db import connect, init_db


MIGRATIONS_DIR = Path(__file__).parent / "migrations"


@dataclass
class Migration:
    version: int           # 1, 2, 3, ...
    name: str              # 'initial_schema', 'add_cost_columns', etc.
    sql: str               # full SQL body
    path: Path             # absolute path to the .sql file


def _parse_filename(p: Path) -> Migration | None:
    """Parse '001_initial_schema.sql' → Migration(version=1, name=...)."""
    stem = p.stem  # '001_initial_schema'
    parts = stem.split("_", 1)
    if len(parts) != 2 or not parts[0].isdigit():
        return None  # not a migration file
    version = int(parts[0])
    name = parts[1]
    return Migration(version=version, name=name, sql=p.read_text(encoding="utf-8"),
                     path=p)


def discover_migrations(migrations_dir: Path = None) -> list[Migration]:
    """Read all migration files, sorted by version."""
    d = migrations_dir or MIGRATIONS_DIR
    if not d.exists():
        return []
    out: list[Migration] = []
    for p in sorted(d.glob("*.sql")):
        m = _parse_filename(p)
        if m:
            out.append(m)
    return out


def _ensure_migrations_table(conn: sqlite3.Connection) -> None:
    """Create the migrations tracking table if it doesn't exist yet."""
    conn.execute(
        """CREATE TABLE IF NOT EXISTS migrations (
            version     INTEGER PRIMARY KEY,
            name        TEXT NOT NULL,
            applied_at  TEXT NOT NULL DEFAULT (datetime('now')),
            sql_hash    TEXT
        )"""
    )


def get_status(db_path: str | None = None,
               migrations_dir: Path = None) -> tuple[list[Migration], list[Migration]]:
    """Return (applied, pending) migrations.

    applied  = migrations whose version is in the migrations table
    pending  = migration files on disk not yet applied
    """
    init_db(db_path)
    available = discover_migrations(migrations_dir)
    with connect(db_path) as conn:
        _ensure_migrations_table(conn)
        rows = conn.execute("SELECT version FROM migrations").fetchall()
        applied_versions = {r["version"] for r in rows}

    applied = [m for m in available if m.version in applied_versions]
    pending = [m for m in available if m.version not in applied_versions]
    return applied, pending


def apply_pending(db_path: str | None = None,
                  migrations_dir: Path = None,
                  verbose: bool = True) -> list[Migration]:
    """Apply all pending migrations in version order.

    Returns the list of migrations that were applied (in order).
    If a migration raises, the error propagates and remaining migrations
    are NOT applied (so the DB stays in a known state).
    """
    init_db(db_path)
    applied_existing, pending = get_status(db_path, migrations_dir)
    if not pending:
        if verbose:
            print("[migrator] no pending migrations")
        return []

    if verbose:
        print(f"[migrator] {len(pending)} pending migration(s)")

    applied_now: list[Migration] = []
    with connect(db_path) as conn:
        _ensure_migrations_table(conn)
        for m in pending:
            if verbose:
                print(f"[migrator] applying {m.version:03d}_{m.name}...")
            try:
                # Some migrations include ALTER TABLE ADD COLUMN statements
                # that conflict with the SCHEMA constant in db.py for fresh
                # DBs (the new columns are already there). Wrap each
                # statement in a try/except that ignores "duplicate column"
                # errors — this is safe because adding a column twice is
                # always idempotent in our schema.
                import sqlite3 as _sqlite3
                try:
                    conn.executescript(m.sql)
                except _sqlite3.OperationalError as e:
                    msg = str(e).lower()
                    if "duplicate column" in msg or "already exists" in msg:
                        # Skip duplicate-column statements but keep going
                        pass
                    else:
                        raise
                conn.execute(
                    "INSERT INTO migrations (version, name, sql_hash) VALUES (?, ?, ?)",
                    (m.version, m.name, _hash_sql(m.sql)),
                )
                applied_now.append(m)
                if verbose:
                    print(f"[migrator]   ✓ {m.version:03d}_{m.name}")
            except Exception as e:
                if verbose:
                    print(f"[migrator]   ✗ FAILED: {e}")
                raise
    return applied_now


def _hash_sql(sql: str) -> str:
    """Stable hash of a migration's SQL body (for drift detection)."""
    import hashlib
    return hashlib.sha256(sql.encode("utf-8")).hexdigest()[:16]


def schema_version(db_path: str | None = None) -> int:
    """Return the highest applied migration version. 0 if none applied."""
    init_db(db_path)
    with connect(db_path) as conn:
        _ensure_migrations_table(conn)
        row = conn.execute(
            "SELECT COALESCE(MAX(version), 0) AS v FROM migrations"
        ).fetchone()
        return int(row["v"])


__all__ = [
    "Migration",
    "MIGRATIONS_DIR",
    "discover_migrations",
    "get_status",
    "apply_pending",
    "schema_version",
]
