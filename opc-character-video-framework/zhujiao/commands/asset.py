"""zhujiao.commands.asset — manage asset library.

The asset library is a queryable index of all generated artifacts:
images, videos, audio, reference files. Each asset has metadata and
tags, and tracks which episodes used it.
"""

import click
import json
from pathlib import Path
from datetime import datetime
from ..db import connect


@click.group(name="asset", help="Manage asset library (素材库).")
def cli():
    pass


@cli.command("list", help="List assets in the library.")
@click.option("--type", "asset_type", type=click.Choice(["image", "video", "audio"]),
              default=None)
@click.option("--character", "character_id", default=None)
@click.option("--tag", default=None, help="Filter by tag.")
@click.option("--json", "as_json", is_flag=True)
def list_cmd(asset_type, character_id, tag, as_json):
    q = "SELECT id, asset_type, character_id, asset_path, file_size_bytes, tags, created_at FROM assets WHERE 1=1"
    params = []
    if asset_type:
        q += " AND asset_type = ?"
        params.append(asset_type)
    if character_id:
        q += " AND character_id = ?"
        params.append(character_id)
    if tag:
        q += " AND tags LIKE ?"
        params.append(f"%{tag}%")
    q += " ORDER BY created_at DESC"
    with connect() as conn:
        rows = conn.execute(q, params).fetchall()
    if as_json:
        click.echo(json.dumps([dict(r) for r in rows], indent=2, ensure_ascii=False))
        return
    if not rows:
        click.echo("(no assets registered)")
        return
    for r in rows:
        size_kb = (r["file_size_bytes"] or 0) / 1024
        click.echo(
            f"  [{r['id']:>4}] {r['asset_type']:<6} {r['character_id'] or '-':<10} "
            f"{size_kb:>7.1f} KB  {Path(r['asset_path']).name}"
        )


@cli.command("register", help="Register a file in the asset library.")
@click.argument("path", type=click.Path(exists=True, dir_okay=False))
@click.option("--type", "asset_type", type=click.Choice(["image", "video", "audio"]),
              required=True)
@click.option("--character", "character_id", default=None)
@click.option("--tag", "tags", default="", help="Comma-separated tags.")
@click.option("--json", "as_json", is_flag=True)
def register_cmd(path: str, asset_type: str, character_id: str, tags: str, as_json):
    p = Path(path)
    size = p.stat().st_size
    record = {
        "asset_type": asset_type,
        "character_id": character_id,
        "asset_path": str(p.resolve()),
        "file_size_bytes": size,
        "tags": tags,
        "created_at": datetime.utcnow().isoformat(),
    }
    # Best-effort width/height/duration
    if asset_type == "image":
        try:
            from PIL import Image
            with Image.open(p) as img:
                record["width"], record["height"] = img.size
        except Exception:
            pass
    elif asset_type == "video":
        try:
            import subprocess
            r = subprocess.run(
                ["ffprobe", "-v", "error", "-show_entries",
                 "format=duration:stream=width,height", "-of", "default=nw=1", str(p)],
                capture_output=True, text=True,
            )
            for line in r.stdout.splitlines():
                if "=" in line:
                    k, v = line.split("=", 1)
                    if k == "duration":
                        record["duration_sec"] = float(v)
                    elif k == "width":
                        record["width"] = int(v)
                    elif k == "height":
                        record["height"] = int(v)
        except Exception:
            pass
    with connect() as conn:
        cur = conn.execute(
            """INSERT INTO assets
                (asset_type, character_id, asset_path, file_size_bytes,
                 duration_sec, width, height, tags, created_at)
               VALUES
                (:asset_type, :character_id, :asset_path, :file_size_bytes,
                 :duration_sec, :width, :height, :tags, :created_at)""",
            {**record, "duration_sec": record.get("duration_sec"),
             "width": record.get("width"), "height": record.get("height")},
        )
        asset_id = cur.lastrowid
    if as_json:
        click.echo(json.dumps({**record, "id": asset_id}, indent=2, ensure_ascii=False))
    else:
        click.echo(f"✓ asset #{asset_id} registered: {p.name} ({size/1024:.1f} KB)")


@cli.command("show", help="Show asset details.")
@click.argument("asset_id", type=int)
@click.option("--json", "as_json", is_flag=True)
def show_cmd(asset_id: int, as_json):
    with connect() as conn:
        row = conn.execute("SELECT * FROM assets WHERE id = ?", (asset_id,)).fetchone()
        if not row:
            raise click.UsageError(f"asset #{asset_id} not found")
    if as_json:
        click.echo(json.dumps(dict(row), indent=2, ensure_ascii=False, default=str))
        return
    click.echo(f"#{row['id']}  {row['asset_type']}  {Path(row['asset_path']).name}")
    click.echo(f"character:  {row['character_id'] or '-'}")
    click.echo(f"path:       {row['asset_path']}")
    click.echo(f"size:       {(row['file_size_bytes'] or 0)/1024:.1f} KB")
    if row["width"] and row["height"]:
        click.echo(f"dimensions: {row['width']}x{row['height']}")
    if row["duration_sec"]:
        click.echo(f"duration:   {row['duration_sec']:.1f} sec")
    if row["tags"]:
        click.echo(f"tags:       {row['tags']}")
    if row["used_in_episodes"]:
        click.echo(f"used in:    {row['used_in_episodes']}")


@cli.command("register-canonical", help="Auto-register all canonical images for a character.")
@click.argument("character_id")
def register_canonical_cmd(character_id: str):
    """Scan characters/<id>/canonical/ and register every jpg/png."""
    from .character import register_character
    with connect() as conn:
        char = conn.execute(
            "SELECT dir_path FROM characters WHERE id = ?", (character_id,)
        ).fetchone()
    if not char:
        # Auto-register
        register_character(character_id, f"characters/{character_id}")
        with connect() as conn:
            char = conn.execute(
                "SELECT dir_path FROM characters WHERE id = ?", (character_id,)
            ).fetchone()
    if not char:
        raise click.UsageError(f"character '{character_id}' not found")
    canonical_dir = Path(char["dir_path"]) / "canonical"
    if not canonical_dir.exists():
        raise click.UsageError(f"no canonical/ dir for {character_id}")
    count = 0
    for f in sorted(canonical_dir.glob("*")):
        if f.suffix.lower() in (".jpg", ".jpeg", ".png"):
            # Register as image asset with tag 'canonical'
            from datetime import datetime
            with connect() as conn:
                conn.execute(
                    """INSERT INTO assets
                        (asset_type, character_id, asset_path,
                         file_size_bytes, tags, created_at)
                       VALUES (?, ?, ?, ?, ?, ?)""",
                    ("image", character_id, str(f.resolve()),
                     f.stat().st_size, "canonical,reference", datetime.utcnow().isoformat()),
                )
                count += 1
    click.echo(f"✓ registered {count} canonical image(s) for {character_id}")