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
from .commands import character as character_cmd
from .commands import lesson as lesson_cmd
from .commands import asset as asset_cmd
from .commands import episode as episode_cmd
from .commands import cost as cost_cmd
from .commands import memory as memory_cmd
from .commands import publish as publish_cmd


@click.group()
@click.version_option("0.1.0", "--version", "-V",
                     message="%(version)s 点睛 / Vivify")
@click.option("--db", "db_path", default=None,
              help="Path to SQLite DB (default: .tmp/data/vivify.db).")
@click.pass_context
def main(ctx, db_path):
    """点睛 / Vivify — character video platform CLI.

    画龙点睛,把静态 IP 角色点活成短剧。

    Run `vivify <command> --help` for command-specific help.
    """
    init_db(db_path)
    ctx.ensure_object(dict)
    ctx.obj["db_path"] = db_path


main.add_command(character_cmd.cli)
main.add_command(lesson_cmd.cli)
main.add_command(asset_cmd.cli)
main.add_command(episode_cmd.cli)
main.add_command(cost_cmd.cli)
main.add_command(memory_cmd.cli)
main.add_command(publish_cmd.cli)


if __name__ == "__main__":
    main(obj={})