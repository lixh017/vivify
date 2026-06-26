"""vivify.commands.character — manage IP characters.

Migrates existing characters/<id>/ character.yaml configs into the
vivify state DB. Each character is registered once and becomes
queryable via `vivify character list` / `show`.

Also provides `vivify character new <name>` for scaffolding a brand-new
IP from `characters/_template/` (interactive or --non-interactive).
"""

import re
import string
import click
import yaml
from pathlib import Path
from datetime import datetime
from ..db import connect


# Default project root — used to locate the `characters/` and
# `characters/_template/` directories from wherever the CLI is invoked.
# This is repo-relative (the project ships from this layout) so we
# resolve it once at module import.
def _project_root() -> Path:
    """Return the project root (the directory containing `characters/`)."""
    return Path(__file__).resolve().parent.parent.parent


def _template_dir() -> Path:
    """Return the path to `characters/_template/`."""
    return _project_root() / "characters" / "_template"


# Placeholder names the generator knows how to fill. Anything else
# (e.g. {{opening_a}} used in the experiments README) is left untouched
# for the user to fill in manually.
GENERATED_PLACEHOLDERS = {
    "name",
    "english_name",
    "species",
    "palette_primary",
    "palette_secondary",
    "personality_1",
    "personality_2",
    "personality_3",
    "default_voice",
    "scenes",
    "scenes_split_0",
    "scenes_split_1",
    "scenes_split_2",
    "scenes_split_3",
    "scenes_split_4",
    "expression_default",
    "idempotency_seed",
}


def _slugify(s: str) -> str:
    """Make a filesystem-safe slug from a free-form name.

    Strips whitespace and replaces separators with underscores. CJK
    characters are preserved (they are valid on modern filesystems);
    only directory separators and control characters are stripped.
    """
    s = s.strip()
    s = re.sub(r"[\s/\\]+", "_", s)
    s = re.sub(r"[\x00-\x1f]", "", s)  # strip control chars
    return s


def _seed_for(name: str) -> str:
    """Deterministic 8-char seed for idempotency tracking."""
    import hashlib
    h = hashlib.sha1(f"vivify-character-new:{name}".encode("utf-8")).hexdigest()
    return h[:8]


def _expression_for(personalities: list[str]) -> str:
    """Build a default expression register from 3 personality tags."""
    if not personalities:
        return "半阖眼, 不直视镜头"
    return "半阖眼 + 微微笑意, 透出 " + " / ".join(personalities)


def _safe_substitute(content: str, mapping: dict[str, str]) -> str:
    """Run string.Template.safe_substitute on the given content.

    Uses `$name` and `${name}` forms (default for string.Template).
    For the more common `{{name}}` form (used in our templates), we
    pre-convert `{{name}}` → `${name}` before substituting.
    """

    # Convert {{name}} → ${name} for string.Template
    converted = re.sub(r"\{\{\s*([a-zA-Z0-9_]+)\s*\}\}", r"${\1}", content)
    return string.Template(converted).safe_substitute(mapping)


def scaffold_character(
    name: str,
    *,
    species: str,
    palette_primary: str,
    palette_secondary: str,
    personalities: list[str],
    scenes: list[str],
    default_voice: str,
    english_name: str | None = None,
    project_root: Path | None = None,
) -> Path:
    """Scaffold a new IP character directory from `characters/_template/`.

    Parameters
    ----------
    name : str
        Chinese (or free-form) display name. Used as both `character.name`
        and as the directory name slugified.
    species, palette_primary, palette_secondary, default_voice : str
        Answers to questions 1, 2, 5.
    personalities : list[str]
        Exactly 3 short tags (question 3).
    scenes : list[str]
        3-5 scene tags (question 4). Padded/truncated to 5 internally.
    english_name : str | None
        Optional English/Pinyin name; defaults to a slugified ascii form
        of `name`.
    project_root : Path | None
        Override the project root (used by tests).

    Returns
    -------
    Path
        The path to the newly-created character directory.

    Raises
    ------
    click.UsageError
        If validation fails or the target directory already exists.
    click.ClickException
        If the template directory is missing.
    """
    # ── input validation ──────────────────────────────────────────────
    name = name.strip()
    if not name:
        raise click.UsageError("name must not be empty")
    if not species.strip():
        raise click.UsageError("--species must not be empty")
    if len(personalities) != 3 or not all(p.strip() for p in personalities):
        raise click.UsageError("exactly 3 personality tags are required")
    if len(scenes) < 3 or len(scenes) > 5:
        raise click.UsageError("--scenes must have between 3 and 5 entries")
    if default_voice not in ("国潮", "治愈", "哲学", "御宅"):
        raise click.UsageError(
            f"--voice must be one of 国潮/治愈/哲学/御宅, got '{default_voice}'"
        )
    # hex colors (no leading #)
    for label, value in (("palette_primary", palette_primary),
                         ("palette_secondary", palette_secondary)):
        v = value.strip().lstrip("#").upper()
        if not re.fullmatch(r"[0-9A-F]{6}", v):
            raise click.UsageError(
                f"--{label.replace('_', '-')} must be a 6-char hex (no #), got '{value}'"
            )
        if label == "palette_primary":
            palette_primary = v
        else:
            palette_secondary = v

    slug = _slugify(name)
    if not slug:
        raise click.UsageError(f"name '{name}' has no alphanumeric chars to slugify")

    # ── paths ─────────────────────────────────────────────────────────
    root = project_root or _project_root()
    template = root / "characters" / "_template"
    target = root / "characters" / slug
    if not template.is_dir():
        raise click.ClickException(f"template directory not found: {template}")
    if target.exists():
        raise click.UsageError(
            f"character directory already exists: {target}\n"
            f"  pick a different name, or remove the existing dir first"
        )

    # ── pad scenes to 5 (template has 5 scene slots) ──────────────────
    scenes_padded = list(scenes) + ["(add another scene here)"] * (5 - len(scenes))
    if not english_name:
        # Auto-derive: if the name is all CJK, use the slug itself
        # (CJK is safe on disk); if it has ASCII, lowercase + underscore it.
        if slug and all(ord(c) > 127 for c in slug if c != "_"):
            english_name = slug
        else:
            english_name = slug.lower().replace(" ", "_")

    # ── placeholder mapping ──────────────────────────────────────────
    mapping = {
        "name": name,
        "english_name": english_name,
        "species": species.strip(),
        "palette_primary": palette_primary,
        "palette_secondary": palette_secondary,
        "personality_1": personalities[0].strip(),
        "personality_2": personalities[1].strip(),
        "personality_3": personalities[2].strip(),
        "default_voice": default_voice,
        "scenes": ", ".join(scenes),
        "scenes_split_0": scenes_padded[0],
        "scenes_split_1": scenes_padded[1],
        "scenes_split_2": scenes_padded[2],
        "scenes_split_3": scenes_padded[3],
        "scenes_split_4": scenes_padded[4],
        "expression_default": _expression_for(personalities),
        "idempotency_seed": _seed_for(slug),
    }

    # ── copy + render template tree ──────────────────────────────────
    # We walk the template directory and re-emit every file through
    # _safe_substitute. .gitkeep files get special handling (they hold
    # the canonical/ drop instructions and should keep the body).
    for src in sorted(template.rglob("*")):
        if src.is_dir():
            continue
        rel = src.relative_to(template)
        dest = target / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        if src.suffix == ".gitkeep":
            # Keep the contents but still run substitution so the
            # instructions reflect the actual character name.
            rendered = _safe_substitute(src.read_text(encoding="utf-8"), mapping)
        else:
            rendered = _safe_substitute(src.read_text(encoding="utf-8"), mapping)
        dest.write_text(rendered, encoding="utf-8")

    return target


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


# ── `vivify character new` ──────────────────────────────────────────────
# Scaffold a brand-new IP from `characters/_template/`. The interactive
# path asks the 5 questions from CLAUDE.md; the non-interactive path
# (for CI / scripted use) accepts them as CLI flags.

_VALID_VOICES = ("国潮", "治愈", "哲学", "御宅")


@cli.command("new", help="Scaffold a new IP character from the template.")
@click.argument("name")
@click.option("--species", default=None,
              help="Species (e.g. '成年熊猫', '小猫', '小男孩').")
@click.option("--palette-primary", default=None,
              help="Primary palette hex WITHOUT '#' (e.g. 'C73E1D').")
@click.option("--palette-secondary", default=None,
              help="Secondary palette hex WITHOUT '#' (e.g. 'F5F0E1').")
@click.option("--personality", "personalities", default=None,
              help='Three personality tags, comma-separated '
                   '(e.g. "内敛,幽默,治愈").')
@click.option("--scenes", "scenes", default=None,
              help="3-5 scene tags, comma-separated "
                   '(e.g. "竹林小院,月下窗棂,围炉夜话").')
@click.option("--voice", "default_voice", default=None,
              type=click.Choice(_VALID_VOICES, case_sensitive=False),
              help="Default voice/tone (国潮/治愈/哲学/御宅).")
@click.option("--english-name", default=None,
              help="Optional English/Pinyin name (default: slugified).")
@click.option("--non-interactive", is_flag=True, default=False,
              help="Skip prompts; require all 5 answers as flags. "
                   "Exits non-zero if any are missing.")
@click.option("--register/--no-register", default=True,
              help="After scaffolding, run 'character add' to register "
                   "the new IP in the DB (default: register).")
def new_cmd(
    name: str,
    species: str | None,
    palette_primary: str | None,
    palette_secondary: str | None,
    personalities: str | None,
    scenes: str | None,
    default_voice: str | None,
    english_name: str | None,
    non_interactive: bool,
    register: bool,
):
    """Scaffold characters/<slug>/ from characters/_template/.

    In interactive mode (default) you will be asked 5 questions:
      1. Species
      2. Primary palette (hex)
      3. Personality (3 short tags)
      4. Default scenes (3-5 scene tags)
      5. Default voice/tone

    In --non-interactive mode, all 5 must be supplied as flags (use this
    for CI / scripted use).
    """
    # ── collect answers ──────────────────────────────────────────────
    if non_interactive:
        missing = []
        if not species: missing.append("--species")
        if not palette_primary: missing.append("--palette-primary")
        if not palette_secondary: missing.append("--palette-secondary")
        if not personalities: missing.append("--personality")
        if not scenes: missing.append("--scenes")
        if not default_voice: missing.append("--voice")
        if missing:
            raise click.UsageError(
                "--non-interactive requires: " + ", ".join(missing)
            )
        personality_list = [p.strip() for p in personalities.split(",") if p.strip()]
        scene_list = [s.strip() for s in scenes.split(",") if s.strip()]
    else:
        species = species or click.prompt("1/5  Species", type=str)
        palette_primary = palette_primary or click.prompt(
            "2a/5 Primary palette (hex, no #)", type=str
        )
        palette_secondary = palette_secondary or click.prompt(
            "2b/5 Secondary palette (hex, no #)", type=str
        )
        personality_raw = personalities or click.prompt(
            "3/5  Personality (3 short tags, comma-separated)", type=str
        )
        personality_list = [p.strip() for p in personality_raw.split(",") if p.strip()]
        scenes_raw = scenes or click.prompt(
            "4/5  Default scenes (3-5 tags, comma-separated)", type=str
        )
        scene_list = [s.strip() for s in scenes_raw.split(",") if s.strip()]
        default_voice = default_voice or click.prompt(
            "5/5  Default voice/tone", type=click.Choice(_VALID_VOICES)
        )

    # ── scaffold ─────────────────────────────────────────────────────
    target = scaffold_character(
        name,
        species=species,
        palette_primary=palette_primary,
        palette_secondary=palette_secondary,
        personalities=personality_list,
        scenes=scene_list,
        default_voice=default_voice,
        english_name=english_name,
    )

    click.echo(f"\n✓ scaffolded: {target}")
    click.echo(f"  name:    {name}")
    click.echo(f"  species: {species}")
    click.echo(f"  voice:   {default_voice}")
    click.echo(f"  palette: #{palette_primary} / #{palette_secondary}")
    click.echo(f"  personality: {', '.join(personality_list)}")
    click.echo(f"  scenes:  {', '.join(scene_list)}")
    click.echo("")

    # ── optionally register in the DB ───────────────────────────────
    if register:
        try:
            slug = target.name
            record = register_character(slug, str(target))
            click.echo(f"✓ registered in DB: {record['id']}")
        except Exception as e:  # noqa: BLE001 — DB errors shouldn't fail scaffold
            click.echo(f"  (skipping DB register: {e})", err=True)

    # ── next-steps checklist ─────────────────────────────────────────
    slug = target.name
    click.echo("")
    click.echo("Next steps:")
    click.echo(f"  1. Review:   cat characters/{slug}/character.yaml")
    click.echo(f"  2. Lint:     python3 validators/lint_character.py characters/{slug}")
    click.echo(f"  3. Drop 3 jpgs into characters/{slug}/canonical/  (face shot MUST be separate)")
    click.echo(f"  4. Check:    python3 validators/canonical_image_check.py characters/{slug}/canonical")
    click.echo(f"  5. Render 1 test episode: see vivify episode render --help")