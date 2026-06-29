"""vivify.cli — main CLI entry point.

点睛 / Vivify is the engineering platform CLI for the OPC
character video framework. Run as `vivify` (after install) or
`python -m vivify`.

点睛 (Diǎn Jīng) = "dot the eyes of the dragon" — the final touch
that brings a static painting to life. Our framework does the same
for IP characters: takes a static character design and animates it
into a living video short drama.

Vivify = Latin "to make alive" — the same meaning in international form.
"""

import click
import sys

from .db import init_db
from .migrator import apply_pending, schema_version
from .commands import character as character_cmd
from .commands import lesson as lesson_cmd
from .commands import asset as asset_cmd
from .commands import episode as episode_cmd
from .commands import cost as cost_cmd
from .commands import memory as memory_cmd
from .commands import publish as publish_cmd
from .commands import workflow as workflow_cmd
from .commands import db as db_cmd
from .commands import doctor as doctor_cmd


@click.group()
@click.version_option("0.1.0", "--version", "-V",
                     message="%(version)s 点睛 / Vivify")
@click.option("--db", "db_path", default=None,
              help="Path to SQLite DB (default: .tmp/data/vivify.db).")
@click.option("--quiet-init", is_flag=True, default=False,
              help="Suppress auto-migration messages on first run.")
@click.pass_context
def main(ctx, db_path, quiet_init):
    """点睛 / Vivify — character video platform CLI.

    画龙点睛,把静态 IP 角色点活成短剧。

    Run `vivify <command> --help` for command-specific help.
    """
    init_db(db_path)
    # Auto-apply pending migrations on every invocation. This keeps the DB
    # schema up to date without requiring a separate `vivify db migrate`
    # step on first install. apply_pending() is idempotent.
    applied = apply_pending(db_path, verbose=not quiet_init)
    if applied and not quiet_init:
        click.echo(
            f"[vivify] auto-applied {len(applied)} migration(s); "
            f"schema now v{schema_version(db_path)}",
            err=True,
        )
    ctx.ensure_object(dict)
    ctx.obj["db_path"] = db_path


main.add_command(character_cmd.cli)
main.add_command(lesson_cmd.cli)
main.add_command(asset_cmd.cli)
main.add_command(episode_cmd.cli)
main.add_command(cost_cmd.cli)
main.add_command(memory_cmd.cli)
main.add_command(publish_cmd.cli)
main.add_command(workflow_cmd.cli)
main.add_command(db_cmd.cli)
main.add_command(doctor_cmd.cli)


if __name__ == "__main__":
    main(obj={})