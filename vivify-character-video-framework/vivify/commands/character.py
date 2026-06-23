"""vivify.commands.character — manage IP characters.

Migrates existing characters/<id>/ character.yaml configs into the
vivify state DB. Each character is registered once and becomes
queryable via `vivify character list` / `show`.
"""

import click
import yaml
from pathlib import Path
from datetime import datetime
from ..db import connect


def register_character(character_id: str, dir_path: str) -> dict:
    """Register or refresh a character in the DB from character.yaml.

    Reads characters/<id>/character.yaml and stores metadata in the
    characters table. Idempotent — safe to call repeatedly.
    """
    char_dir = Path(dir_path)
    yaml_path = char_dir / "character.yaml"
    if not yaml_path.exists():
        raise click.UsageError(f"character.yaml not found at {yaml_path}")

    with open(yaml_path, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)
    char = data.get("character", {})

    canonical_dir = char_dir / "canonical"
    record = {
        "id": character_id,
        "name": char.get("name", character_id),
        "english_name": char.get("english_name"),
        "species": char.get("species"),
        "dir_path": str(char_dir.resolve()),
        "character_yaml_path": str(yaml_path.resolve()),
        "canonical_dir": str(canonical_dir.resolve()) if canonical_dir.exists() else None,
        "updated_at": datetime.utcnow().isoformat(),
    }

    with connect() as conn:
        # Upsert
        existing = conn.execute(
            "SELECT id FROM characters WHERE id = ?", (character_id,)
        ).fetchone()
        if existing:
            conn.execute(
                """UPDATE characters SET
                    name = :name,
                    english_name = :english_name,
                    species = :species,
                    dir_path = :dir_path,
                    character_yaml_path = :character_yaml_path,
                    canonical_dir = :canonical_dir,
                    updated_at = :updated_at
                WHERE id = :id""",
                record,
            )
        else:
            conn.execute(
                """INSERT INTO characters
                    (id, name, english_name, species, dir_path,
                     character_yaml_path, canonical_dir, created_at, updated_at)
                VALUES
                    (:id, :name, :english_name, :species, :dir_path,
                     :character_yaml_path, :canonical_dir, :updated_at, :updated_at)""",
                record,
            )
    return record


@click.group(name="character", help="Manage IP characters (注册/查询/校验角色).")
def cli():
    pass


@cli.command("list", help="List all registered characters.")
@click.option("--json", "as_json", is_flag=True, help="Output as JSON.")
def list_cmd(as_json: bool):
    with connect() as conn:
        rows = conn.execute(
            """SELECT id, name, english_name, species, canonical_dir, updated_at
               FROM characters ORDER BY id"""
        ).fetchall()
    if as_json:
        import json
        click.echo(json.dumps([dict(r) for r in rows], indent=2, ensure_ascii=False))
        return
    if not rows:
        click.echo("(no characters registered yet — use 'vivify character add <id>')")
        return
    click.echo(f"{'ID':<14} {'NAME':<10} {'EN':<10} {'SPECIES':<14} {'UPDATED':<20}")
    click.echo("─" * 70)
    for r in rows:
        en = r["english_name"] or ""
        sp = r["species"] or ""
        click.echo(f"{r['id']:<14} {r['name']:<10} {en:<10} {sp:<14} {r['updated_at']:<20}")


@cli.command("add", help="Register a character from a directory.")
@click.argument("character_id")
@click.argument("dir_path", type=click.Path(exists=True, file_okay=False))
@click.option("--json", "as_json", is_flag=True)
def add_cmd(character_id: str, dir_path: str, as_json: bool):
    record = register_character(character_id, dir_path)
    if as_json:
        import json
        click.echo(json.dumps(record, indent=2, ensure_ascii=False))
    else:
        click.echo(f"✓ registered: {record['id']} ({record['name']} / {record['english_name']})")
        click.echo(f"  dir: {record['dir_path']}")
        if record["canonical_dir"]:
            click.echo(f"  canonical: {record['canonical_dir']}")


@cli.command("show", help="Show character details.")
@click.argument("character_id")
@click.option("--json", "as_json", is_flag=True)
def show_cmd(character_id: str, as_json: bool):
    with connect() as conn:
        char = conn.execute(
            "SELECT * FROM characters WHERE id = ?", (character_id,)
        ).fetchone()
        if not char:
            raise click.UsageError(f"character '{character_id}' not registered")
        # Count episodes
        ep_count = conn.execute(
            "SELECT COUNT(*) AS n FROM episodes WHERE character_id = ?",
            (character_id,),
        ).fetchone()["n"]
        # Count assets
        asset_count = conn.execute(
            "SELECT COUNT(*) AS n FROM assets WHERE character_id = ?",
            (character_id,),
        ).fetchone()["n"]
        # Count lessons
        lesson_count = conn.execute(
            "SELECT COUNT(*) AS n FROM lessons WHERE character_id = ?",
            (character_id,),
        ).fetchone()["n"]
    if as_json:
        import json
        click.echo(json.dumps({
            **dict(char),
            "episode_count": ep_count,
            "asset_count": asset_count,
            "lesson_count": lesson_count,
        }, indent=2, ensure_ascii=False))
        return
    click.echo(f"id:           {char['id']}")
    click.echo(f"name:         {char['name']} ({char['english_name']})")
    click.echo(f"species:      {char['species']}")
    click.echo(f"dir:          {char['dir_path']}")
    click.echo(f"yaml:         {char['character_yaml_path']}")
    if char["canonical_dir"]:
        click.echo(f"canonical:    {char['canonical_dir']}")
    click.echo(f"created:      {char['created_at']}")
    click.echo(f"updated:      {char['updated_at']}")
    click.echo(f"")
    click.echo(f"episodes:     {ep_count}")
    click.echo(f"assets:       {asset_count}")
    click.echo(f"lessons:      {lesson_count}")


@cli.command("validate", help="Run all validators on a character.")
@click.argument("character_id")
def validate_cmd(character_id: str):
    """Run lint_character + lint_constraints on the character's dir."""
    import subprocess
    with connect() as conn:
        char = conn.execute(
            "SELECT dir_path FROM characters WHERE id = ?", (character_id,)
        ).fetchone()
        if not char:
            raise click.UsageError(f"character '{character_id}' not registered")
    dir_path = char["dir_path"]
    click.echo(f"validating {character_id} at {dir_path} ...")
    validators = [
        ("lint_character",   "validators/lint_character.py"),
        ("lint_constraints", "validators/lint_constraints.py"),
    ]
    failed = []
    for name, script in validators:
        click.echo(f"\n── {name} ──")
        result = subprocess.run(
            ["python3", script, dir_path],
            capture_output=True, text=True, cwd=Path(__file__).parent.parent.parent,
        )
        click.echo(result.stdout)
        if result.returncode != 0:
            click.echo(result.stderr, err=True)
            failed.append(name)
    if failed:
        click.echo(f"\n✗ {len(failed)} validator(s) failed: {', '.join(failed)}")
        raise click.exceptions.Exit(1)
    click.echo(f"\n✓ all validators passed for {character_id}")


@cli.command("refresh", help="Re-read character.yaml and update the registry.")
@click.argument("character_id")
def refresh_cmd(character_id: str):
    with connect() as conn:
        char = conn.execute(
            "SELECT dir_path FROM characters WHERE id = ?", (character_id,)
        ).fetchone()
        if not char:
            raise click.UsageError(f"character '{character_id}' not registered")
    record = register_character(character_id, char["dir_path"])
    click.echo(f"✓ refreshed {record['id']} ({record['name']})")