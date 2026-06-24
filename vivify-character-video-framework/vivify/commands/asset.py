"""vivify.commands.asset — manage asset library.

The asset library is a queryable index of all generated artifacts:
images, videos, audio, reference files. Each asset has metadata and
tags, and tracks which episodes used it.

L1 backing for the L2 `vivify-asset-orchestrator` skill. Adds three
subcommands on top of the existing list/register/show/register-canonical:
  generate   — call the asset_orchestrator for one or many scenes
  ledger     — read the JSONL ledger (cost + status history)
  router     — inspect / dry-run the provider-selection router
"""

import os
import click
import json
from pathlib import Path
from datetime import datetime
from ..db import connect
from ..asset_orchestrator import generate_asset
from ..asset_router import load_config as _load_config, pick as _router_pick, _model_cost_yuan as _model_cost_yuan
from ..providers.base import GenerateRequest as _GenerateRequest


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


# ============================================================================
# L1 backing for vivify-asset-orchestrator SKILL.md
# ============================================================================
#
# Three subcommands drive the new pipeline:
#
#   vivify asset generate   — single or batch call to asset_orchestrator
#   vivify asset ledger     — read the append-only JSONL cost/status ledger
#   vivify asset router     — inspect router config + dry-run provider pick
#
# These live alongside the existing list/register/show/register-canonical
# commands without changing their behavior.
# ============================================================================


def _vendor_env() -> dict:
    """Read vendor credentials from os.environ. Returns {} if any are missing.

    The orchestrator needs ARK_API_KEY + ARK_BASE_URL for the ark adapter,
    and the local mmx CLI for minimax. We forward everything from the
    current process env so scripts can set them in one place.
    """
    return {
        k: v for k, v in os.environ.items()
        if k.startswith(("ARK_", "MINIMAX_", "MMS_"))
    }


@cli.command("generate", help="Generate one asset via the asset_orchestrator pipeline.")
@click.option("--scene", "scene_json", default=None,
              help="Scene JSON: {scene_id, type, prompt, [outfit], [tone], [options]}.")
@click.option("--batch", "batch_path", default=None,
              help="Path to scenes JSONL file (one scene JSON per line).")
@click.option("--type", "asset_type",
              type=click.Choice(["image", "video"], case_sensitive=False),
              required=True, help="Asset type to generate.")
@click.option("--provider", "provider_filter", default=None,
              help="Force a specific provider (e.g. ark, minimax). Skip the router.")
@click.option("--out", "out_path", default=None,
              help="Override the output path the adapter writes to.")
@click.option("--dry-run", is_flag=True, default=False,
              help="Show which provider would be picked; do not call any adapter.")
@click.option("--config-path", "config_path", default=None,
              help="Override the router YAML path.")
def generate_cmd(scene_json, batch_path, asset_type, provider_filter,
                  out_path, dry_run, config_path):
    """Generate one asset (--scene) or many (--batch) via the orchestrator.

    The orchestrator handles router + cost-cap + retry + ledger; this
    command is a thin wrapper that builds the GenerateRequest and prints
    the GenerateResult.

    Notes:
      - TTS and BGM are not yet supported by this entry point. For TTS,
        use `vivify episode render`. For BGM, also use `episode render`.
    """
    if not scene_json and not batch_path:
        raise click.UsageError("either --scene or --batch is required")
    if scene_json and batch_path:
        raise click.UsageError("--scene and --batch are mutually exclusive")

    # Lazy imports — keeps `vivify --help` fast and lets tests patch
    # the orchestrator symbol without polluting the module load path.
    from ..providers.base import GenerateRequest
    from ..asset_orchestrator import generate_asset as orch_generate
    from ..asset_router import load_config, pick, _model_cost_yuan

    cfg = load_config(path=Path(config_path) if config_path else None)

    if dry_run:
        # Just show the pick + estimated cost; no adapter call.
        # Build a minimal requirements dict to feed pick().
        req_data = json.loads(scene_json) if scene_json else {}
        requirements = {"duration_sec": req_data.get("duration_sec", 0)}
        provider_id = pick(cfg, asset_type, requirements=requirements,
                            provider_filter=provider_filter)
        if provider_id == "manual-pending":
            click.echo("[dry-run] no provider satisfies the requirements "
                       "(filter={!r}, asset_type={!r})".format(provider_filter, asset_type))
            raise click.exceptions.Exit(1)
        estimate = _model_cost_yuan(provider_id, requirements.get("duration_sec", 0))
        click.echo(f"[dry-run] would pick provider: {provider_id}")
        click.echo(f"[dry-run] estimate: ¥{estimate:.4f}")
        per_asset = cfg.cost_caps.get("per_asset_cny")
        if per_asset:
            click.echo(f"[dry-run] per_asset cap: ¥{per_asset:.2f}  "
                       f"({'OK' if estimate <= per_asset else 'OVER CAP'})")
        return

    # Real call path — parse the scene JSON once
    if scene_json:
        try:
            data = json.loads(scene_json)
        except json.JSONDecodeError as e:
            raise click.UsageError(f"--scene is not valid JSON: {e}")
        results = [_run_one(data, asset_type, provider_filter, out_path,
                            cfg, generate_asset)]
    else:
        # Batch mode — one scene per line
        results = []
        with open(batch_path, "r", encoding="utf-8") as f:
            for lineno, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue
                try:
                    data = json.loads(line)
                except json.JSONDecodeError as e:
                    click.echo(f"  line {lineno}: SKIPPED ({e})", err=True)
                    results.append(None)
                    continue
                results.append(_run_one(data, asset_type, provider_filter,
                                         out_path, cfg, generate_asset))

    # Summary
    ok = sum(1 for r in results if r and r.ok)
    fail = len(results) - ok
    click.echo(f"\n[generate] {ok} succeeded, {fail} failed (out of {len(results)})")
    if fail:
        raise click.exceptions.Exit(1)


def _run_one(scene_data: dict, asset_type: str, provider_filter, out_path,
             cfg, orch_generate):
    """Run one orchestrator call. Returns the GenerateResult (or None on parse error)."""
    from ..providers.base import GenerateRequest

    req = GenerateRequest(
        scene_id=scene_data.get("scene_id", ""),
        asset_type=asset_type,
        prompt=scene_data.get("prompt", ""),
        reference_image=scene_data.get("reference_image"),
        duration_sec=scene_data.get("duration_sec"),
        size=scene_data.get("size"),
        outfit=scene_data.get("outfit"),
        tone=scene_data.get("tone"),
        options=scene_data.get("options", {}),
    )
    result = orch_generate(req, env=_vendor_env(),
                            provider_filter=provider_filter)
    status = "✓" if result.ok else "✗"
    cost = result.cost_yuan
    err = f" — {result.error}" if not result.ok and result.error else ""
    click.echo(f"  {status} {req.scene_id or '(no-id)':<24} "
               f"provider={result.provider or '-'!s:<10} "
               f"¥{cost:.4f}{err}")
    return result


@cli.command("ledger", help="Read the JSONL asset-generation ledger.")
@click.option("--last", "last_n", type=int, default=20,
              help="Show the last N entries (default 20).")
@click.option("--tail", is_flag=True, default=False,
              help="Stream mode — useful with `| jq`.")
@click.option("--filter", "filter_str", default=None,
              help="Filter rows by field=value (e.g. status=success).")
@click.option("--as-json", "as_json", is_flag=True, default=False,
              help="Print rows as a JSON array instead of a table.")
def ledger_cmd(last_n, tail, filter_str, as_json):
    """Print recent rows from ~/.claude/agents/vivify-asset-ledger.jsonl.

    Each row is one asset-generation call (success or failure). The
    ledger is append-only — use this for cost reconciliation, debugging
    provider failures, and the monthly-cap audit.
    """
    from ..asset_ledger import read_recent, stream

    # Parse --filter (key=value)
    filter_key = None
    filter_val = None
    if filter_str:
        if "=" not in filter_str:
            raise click.UsageError(f"--filter expects key=value, got {filter_str!r}")
        filter_key, filter_val = filter_str.split("=", 1)

    def _matches(row: dict) -> bool:
        if filter_key is None:
            return True
        return str(row.get(filter_key, "")) == filter_val

    if tail:
        # Stream mode — print rows as they are read
        rows = (e.to_dict() for e in stream())
        if as_json:
            click.echo("[")
            first = True
            for row in rows:
                if not _matches(row):
                    continue
                if not first:
                    click.echo(",")
                click.echo(json.dumps(row, ensure_ascii=False), nl=False)
                first = False
            click.echo("]")
        else:
            for row in rows:
                if _matches(row):
                    click.echo(_format_row(row))
        return

    rows = [e.to_dict() for e in read_recent(last_n)]
    rows = [r for r in rows if _matches(r)]

    if not rows:
        if as_json:
            click.echo("[]")
        else:
            click.echo("(ledger empty or no rows match filter)")
        return

    if as_json:
        click.echo(json.dumps(rows, indent=2, ensure_ascii=False))
    else:
        click.echo(f"--- last {len(rows)} ledger row(s) ---")
        for row in rows:
            click.echo(_format_row(row))


def _format_row(row: dict) -> str:
    """Format one ledger row for human display."""
    cost = row.get("cost_cny", 0.0)
    status = row.get("status", "?")
    err = row.get("error") or ""
    err_short = (err[:60] + "…") if len(err) > 60 else err
    return (f"  {row.get('timestamp', '')[:19]}Z  "
            f"{row.get('provider', '-'):<10}  "
            f"{row.get('type', '-'):<6}  "
            f"¥{cost:>7.4f}  "
            f"{status:<18}  "
            f"{row.get('scene_id', '-')}"
            f"{('  ⚠ ' + err_short) if err_short else ''}")


@cli.command("router", help="Inspect the provider-selection router.")
@click.option("--show-config", is_flag=True, default=False,
              help="Print the router YAML config (write default if missing).")
@click.option("--pick", "do_pick", is_flag=True, default=False,
              help="Dry-run: show which provider would be picked for given args.")
@click.option("--type", "asset_type",
              type=click.Choice(["video", "image", "tts", "bgm"], case_sensitive=False),
              default=None, help="Asset type (for --pick).")
@click.option("--duration-sec", "duration_sec", type=int, default=0,
              help="Video duration in seconds (for --pick cost estimate).")
@click.option("--provider", "provider_filter", default=None,
              help="Force one provider id (for --pick).")
@click.option("--config-path", "config_path", default=None,
              help="Override the router YAML path.")
def router_cmd(show_config, do_pick, asset_type, duration_sec,
                provider_filter, config_path):
    """Inspect or dry-run the provider-selection router.

    `--show-config` prints the active YAML (creates the default in
    ~/.claude/config/vivify-providers.yaml if missing).
    `--pick` walks the router algorithm and prints the chosen provider
    + estimated cost without calling any adapter.
    """
    from ..asset_router import load_config, pick, _model_cost_yuan, DEFAULT_CONFIG_PATH

    cfg_path = Path(config_path) if config_path else DEFAULT_CONFIG_PATH

    if show_config:
        # load_config() writes the default if missing, so just call it.
        load_config(path=cfg_path)
        click.echo(f"# Router config: {cfg_path}")
        click.echo(cfg_path.read_text(encoding="utf-8"))
        return

    if do_pick:
        if not asset_type:
            raise click.UsageError("--pick requires --type")
        cfg = load_config(path=cfg_path)
        requirements = {"duration_sec": duration_sec}
        provider_id = pick(cfg, asset_type, requirements=requirements,
                            provider_filter=provider_filter)
        estimate = _model_cost_yuan(provider_id, duration_sec)
        per_asset = cfg.cost_caps.get("per_asset_cny")
        click.echo(f"asset_type:        {asset_type}")
        click.echo(f"duration_sec:      {duration_sec}")
        click.echo(f"provider_filter:   {provider_filter or '(none)'}")
        click.echo(f"chosen provider:   {provider_id}")
        click.echo(f"estimate (¥):      {estimate:.4f}")
        click.echo(f"per_asset cap (¥): {per_asset:.2f}" if per_asset else "per_asset cap: (none)")
        if per_asset and estimate > per_asset:
            click.echo("⚠ estimate exceeds per_asset cap — call will be refused by orchestrator")
        return

    # No flag → show help
    ctx = click.get_current_context()
    click.echo(ctx.get_help())