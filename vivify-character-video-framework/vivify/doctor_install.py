"""vivify.doctor_install — best-effort installer for missing deps.

Used by `vivify doctor --self-install`. We try to install:
- ffmpeg (via apt / dnf / pacman / brew / choco)
- mmx (via pip)

We do NOT auto-install:
- ARK_API_KEY (manual — user must generate at volcengine.com)
- Python itself
- System packages that need admin (sudo) on locked-down machines

Returns: (ok: bool, message: str) per attempt.
"""
from __future__ import annotations

import shutil
import subprocess
import sys


def attempt_install(cmd: str, *, timeout: int = 180) -> tuple[bool, str]:
    """Run `cmd` in a subprocess. Return (ok, message).

    The command is split on whitespace; arguments with spaces are not
    supported (use a one-shot script file if you need them). Streams
    output to /dev/null — doctor doesn't show the install transcript.
    """
    if not cmd or not cmd.strip():
        return False, "empty command"

    args = cmd.split()
    if not args:
        return False, "no args after split"

    try:
        # Some install commands need sudo; some don't (e.g. `pip install
        # --user` or `brew install` if the user is already root). We
        # just try the command as-is.
        result = subprocess.run(
            args,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired:
        return False, f"timed out after {timeout}s"
    except FileNotFoundError as e:
        return False, f"command not found: {e}"
    except OSError as e:
        return False, f"OS error: {e}"

    if result.returncode == 0:
        return True, "ok"
    err = (result.stderr or b"").decode("utf-8", errors="replace").strip()
    return False, f"exit={result.returncode}: {err[:200]}"


__all__ = ["attempt_install"]