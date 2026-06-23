"""vivify.commands.lesson — manage IP/cross-IP lessons.

Lessons are first-class database records. The CLI replaces the
previous approach of scattered markdown files in memory/ and
characters/<ip>/.
"""

import click
import json
from ..db import connect


@click.group(name="lesson", help="Manage lessons (沉淀教训).")
def cli():
    pass


@cli.command("list", help="List lessons.")
@click.option("--character", "character_id", default=None,
              help="Filter by character ID (omit for cross-IP).")
@click.option("--category", default=None, help="Filter by category.")
@click.option("--status", default=None, help="Filter by status (validated/hypothesis/draft).")
@click.option("--json", "as_json", is_flag=True)
def list_cmd(character_id, category, status, as_json):
    q = "SELECT id, character_id, title, category, status, source, created_at FROM lessons WHERE 1=1"
    params = []
    if character_id is not None:
        q += " AND character_id = ?"
        params.append(character_id)
    if category:
        q += " AND category = ?"
        params.append(category)
    if status:
        q += " AND status = ?"
        params.append(status)
    q += " ORDER BY created_at DESC"
    with connect() as conn:
        rows = conn.execute(q, params).fetchall()
    if as_json:
        click.echo(json.dumps([dict(r) for r in rows], indent=2, ensure_ascii=False))
        return
    if not rows:
        click.echo("(no lessons)")
        return
    for r in rows:
        char = r["character_id"] or "CROSS-IP"
        click.echo(f"  [{r['id']:>4}] {char:<10} [{r['status'] or '-':<10}] {r['title']}")


@cli.command("add", help="Record a new lesson.")
@click.option("--title", required=True)
@click.option("--body", required=True)
@click.option("--character", "character_id", default=None,
              help="Character ID (omit for cross-IP).")
@click.option("--category", default=None)
@click.option("--status", default="hypothesis",
              type=click.Choice(["validated", "hypothesis", "draft"]))
@click.option("--source", default="manual")
def add_cmd(title, body, character_id, category, status, source):
    with connect() as conn:
        cur = conn.execute(
            """INSERT INTO lessons (character_id, title, body, category, status, source)
               VALUES (?, ?, ?, ?, ?, ?)""",
            (character_id, title, body, category, status, source),
        )
        lesson_id = cur.lastrowid
    click.echo(f"✓ lesson #{lesson_id} added")
    if character_id:
        click.echo(f"  character: {character_id}")
    else:
        click.echo(f"  scope:     cross-IP")
    click.echo(f"  status:    {status}")


@cli.command("show", help="Show full lesson body.")
@click.argument("lesson_id", type=int)
def show_cmd(lesson_id: int):
    with connect() as conn:
        row = conn.execute("SELECT * FROM lessons WHERE id = ?", (lesson_id,)).fetchone()
        if not row:
            raise click.UsageError(f"lesson #{lesson_id} not found")
    click.echo(f"#{row['id']}  {row['title']}")
    click.echo(f"character: {row['character_id'] or 'CROSS-IP'}")
    click.echo(f"category:  {row['category'] or '-'}")
    click.echo(f"status:    {row['status'] or '-'}")
    click.echo(f"source:    {row['source'] or '-'}")
    click.echo(f"created:   {row['created_at']}")
    click.echo("")
    click.echo(row["body"])


@cli.command("search", help="Full-text search lesson bodies.")
@click.argument("query")
@click.option("--character", "character_id", default=None)
def search_cmd(query: str, character_id):
    q = "SELECT id, character_id, title, body, status FROM lessons WHERE (title LIKE ? OR body LIKE ?)"
    params = [f"%{query}%", f"%{query}%"]
    if character_id:
        q += " AND character_id = ?"
        params.append(character_id)
    q += " ORDER BY created_at DESC"
    with connect() as conn:
        rows = conn.execute(q, params).fetchall()
    if not rows:
        click.echo(f"(no lessons matching '{query}')")
        return
    click.echo(f"matched {len(rows)} lesson(s):")
    for r in rows:
        char = r["character_id"] or "CROSS-IP"
        # Show 80-char snippet
        body = r["body"] or ""
        snippet = body.replace("\n", " ")[:120]
        click.echo(f"  [{r['id']:>4}] {char:<10} {r['title']}")
        click.echo(f"        {snippet}{'...' if len(body) > 120 else ''}")