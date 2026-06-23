"""vivify.commands.memory — migrate markdown knowledge to the DB.

Replaces dual-track (memory/*.md on disk + lessons table in DB) with
single-track (DB only). After migration, agents should:

  - READ from: vivify lesson list / vivify lesson search
  - WRITE to: vivify lesson add
  - NOT read:  memory/*.md directly (the files are kept for reference
              until you're ready to delete them)

The migration is one-shot and idempotent (re-running skips files
that are already migrated, identified by source = relative path).

Commands:
  list                       List all migrated memory files (cross-IP lessons)
  migrate [--dry-run]        Bulk-import all memory/**/*.md into the lessons table
  show <id>                  Show full body of a migrated lesson
  unlink <id> [--delete-md]  Remove a migrated lesson from DB (and optionally the .md)
"""

import json
import re
from pathlib import Path

import click

from ..db import connect, init_db


# Status marker parsing (✅ validated / 🟡 hypothesis / 🔴 draft)
STATUS_RE = re.compile(
    r"Status:\s*([✅🟡🔴\s]*?(?:validated|hypothesis|draft))", re.IGNORECASE)
TITLE_RE = re.compile(r"^#\s+(.+?)$", re.MULTILINE)
H1_LINE_RE = re.compile(r"^#\s+.+?$", re.MULTILINE)


def _parse_md_file(path: Path) -> dict:
    """Parse a memory .md file into a lesson dict.

    Returns dict with: title, body, status, category, source
    """
    text = path.read_text(encoding="utf-8")

    # Title = first H1 line
    title_m = TITLE_RE.search(text)
    title = title_m.group(1).strip() if title_m else path.stem

    # Status = first match of `> Status: ✅ validated` etc.
    status_m = STATUS_RE.search(text)
    if status_m:
        raw = status_m.group(1).lower()
        if "validated" in raw:
            status = "validated"
        elif "hypothesis" in raw:
            status = "hypothesis"
        elif "draft" in raw:
            status = "draft"
        else:
            status = "draft"
    else:
        status = "draft"

    # Body = entire text minus the first H1 (we store title separately)
    body = H1_LINE_RE.sub("", text, count=1).strip()

    # Category = parent directory name (prompt-engineering, model-capabilities)
    category = path.parent.name if path.parent.name else "uncategorized"

    return {
        "title": title,
        "body": body,
        "status": status,
        "category": category,
        "source": str(path),
    }


@click.group("memory")
def cli():
    """Cross-IP knowledge base (migrated from memory/*.md)."""
    pass


@cli.command("list")
@click.option("--category", "-c", default=None, help="Filter by category.")
@click.option("--status", "-s", default=None,
              type=click.Choice(["validated", "hypothesis", "draft"]))
@click.option("--as-json", "as_json", is_flag=True)
@click.pass_obj
def list_cmd(obj, category, status, as_json):
    """List migrated cross-IP lessons."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        q = """SELECT id, title, category, status, source, created_at
               FROM lessons
               WHERE character_id IS NULL"""
        params: list = []
        if category:
            q += " AND category = ?"; params.append(category)
        if status:
            q += " AND status = ?"; params.append(status)
        q += " ORDER BY category, title"
        rows = [dict(r) for r in conn.execute(q, params).fetchall()]
    if as_json:
        click.echo(json.dumps(rows, indent=2, ensure_ascii=False, default=str))
        return
    if not rows:
        click.echo("(no cross-IP lessons — run `vivify memory migrate` to import memory/*.md)")
        return
    click.echo(f"{'ID':<4} {'CAT':<22} {'STATUS':<12} TITLE")
    click.echo("─" * 80)
    for r in rows:
        click.echo(f"{r['id']:<4} {(r['category'] or '—'):<22} "
                   f"{(r['status'] or '—'):<12} {r['title']}")


@cli.command("show")
@click.argument("lesson_id", type=int)
@click.pass_obj
def show_cmd(obj, lesson_id):
    """Show full body of a migrated lesson."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        row = conn.execute(
            "SELECT * FROM lessons WHERE id = ? AND character_id IS NULL",
            (lesson_id,),
        ).fetchone()
    if not row:
        raise click.ClickException(f"no cross-IP lesson #{lesson_id}")
    click.echo(f"═══ #{row['id']} — {row['title']} ═══")
    click.echo(f"  category: {row['category']}")
    click.echo(f"  status:   {row['status']}")
    click.echo(f"  source:   {row['source']}")
    click.echo(f"  added:    {row['created_at']}")
    click.echo("")
    click.echo(row["body"])


@cli.command("migrate")
@click.argument("memory_dir", default="memory", type=click.Path(exists=True))
@click.option("--dry-run", is_flag=True,
              help="Show what would be imported, but don't write to DB.")
@click.pass_obj
def migrate_cmd(obj, memory_dir, dry_run):
    """Bulk-import memory/**/*.md into the lessons table.

    Idempotent: re-running skips files already imported (matched by source path).
    Use this once to populate the DB, then re-run only if you add new files.
    """
    db_path = obj.get("db_path")
    init_db(db_path)
    md_dir = Path(memory_dir)
    md_files = sorted(md_dir.rglob("*.md"))
    if not md_files:
        raise click.ClickException(f"no .md files under {md_dir}")

    # Exclude INDEX.md (it's a manifest, not a lesson)
    md_files = [f for f in md_files if f.name != "INDEX.md"]

    imported, skipped = [], []
    with connect(db_path) as conn:
        existing_sources = {
            r["source"] for r in conn.execute(
                "SELECT source FROM lessons WHERE character_id IS NULL"
            ).fetchall()
        }
        for md in md_files:
            parsed = _parse_md_file(md)
            if parsed["source"] in existing_sources:
                skipped.append(md.name)
                continue
            if dry_run:
                imported.append((md, parsed))
            else:
                conn.execute(
                    """INSERT INTO lessons (
                        character_id, title, body, category, status, source
                    ) VALUES (NULL, ?, ?, ?, ?, ?)""",
                    (parsed["title"], parsed["body"],
                     parsed["category"], parsed["status"], parsed["source"]),
                )
                imported.append((md, parsed))

    click.echo(f"═══ Memory migration {'(DRY RUN) ' if dry_run else ''}═══")
    click.echo(f"  scanned:   {len(md_files)} files")
    click.echo(f"  to import: {len(imported)}")
    click.echo(f"  skipped:   {len(skipped)} (already in DB)")
    click.echo("")
    if imported:
        click.echo("  Importing:")
        for md, parsed in imported:
            click.echo(f"    [{parsed['category']:<22}] [{parsed['status']:<11}] "
                       f"{md.name} — {parsed['title']}")
    if skipped:
        click.echo("")
        click.echo("  Skipped (idempotency):")
        for s in skipped:
            click.echo(f"    {s}")
    if not dry_run and imported:
        click.echo("")
        click.echo("✓ migration complete. Run `vivify memory list` to verify.")


@cli.command("unlink")
@click.argument("lesson_id", type=int)
@click.option("--delete-md", is_flag=True,
              help="Also delete the source .md file (use with care).")
@click.option("--yes", "-y", is_flag=True)
@click.pass_obj
def unlink_cmd(obj, lesson_id, delete_md, yes):
    """Remove a migrated lesson from DB (and optionally its .md)."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        row = conn.execute(
            "SELECT * FROM lessons WHERE id = ? AND character_id IS NULL",
            (lesson_id,),
        ).fetchone()
        if not row:
            raise click.ClickException(f"no cross-IP lesson #{lesson_id}")
        if not yes:
            md_path = row["source"]
            extra = f" + DELETE {md_path}" if delete_md else ""
            click.confirm(f"Delete lesson #{lesson_id} ({row['title']}){extra}?",
                          abort=True)
        if delete_md and row["source"]:
            md = Path(row["source"])
            if md.exists():
                md.unlink()
                click.echo(f"  deleted file: {md}")
        conn.execute("DELETE FROM lessons WHERE id = ?", (lesson_id,))
    click.echo(f"✓ unlinked lesson #{lesson_id}")


__all__ = ["cli"]