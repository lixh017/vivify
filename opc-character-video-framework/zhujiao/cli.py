"""zhujiao.cli — main CLI entry point.

铸角 (ZhuJiao) is the engineering platform CLI for the OPC
character video framework. Run as `zhujiao` (after install) or
`python -m zhujiao`.
"""

import click
import sys

from .db import init_db
from .commands import character as character_cmd
from .commands import lesson as lesson_cmd
from .commands import asset as asset_cmd


@click.group()
@click.version_option(package_name="zhujiao")
@click.option("--db", "db_path", default=None,
              help="Path to SQLite DB (default: data/zhujiao.db).")
def main(db_path):
    """铸角 / ZhuJiao — character video platform CLI.

    Run `zhujiao <command> --help` for command-specific help.
    """
    init_db(db_path)


main.add_command(character_cmd.cli)
main.add_command(lesson_cmd.cli)
main.add_command(asset_cmd.cli)


if __name__ == "__main__":
    main(obj={})