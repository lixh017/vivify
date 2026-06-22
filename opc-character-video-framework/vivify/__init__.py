"""点睛 / Vivify — character video platform CLI.

点睛 (Diǎn Jīng) = "dot the eyes of the dragon" — the final touch
that brings a static painting to life. Vivify is the international form
of the same idea: Latin "to make alive".

An engineering platform for producing AI character short drama videos.
Replaces loose scripts + markdown skills with a proper CLI backed
by a SQLite state database, an asset library, and per-IP data layers.

Quick start:
    $ vivify character list
    $ vivify character show fengge
    $ vivify character validate fengge
    $ vivify episode list --character fengge
    $ vivify asset list --type image
    $ vivify lesson list
    $ vivify lesson add --title "..." --body "..." --character fengge
"""

__version__ = "0.1.0"
__all__ = ["cli", "db", "models"]