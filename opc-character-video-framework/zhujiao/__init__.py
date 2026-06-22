"""铸角 / ZhuJiao — character video platform CLI.

A engineering platform for producing AI character short drama videos.
Replaces the loose scripts + skills approach with a proper CLI backed
by a SQLite state database, an asset library, and per-IP data layers.

Quick start:
    $ zhujiao character list
    $ zhujiao character show fengge
    $ zhujiao character validate fengge
    $ zhujiao episode list --character fengge
    $ zhujiao asset list --type image
    $ zhujiao lesson list
    $ zhujiao lesson add --title "..." --body "..." --character fengge
"""

__version__ = "0.1.0"
__all__ = ["cli", "db", "models"]