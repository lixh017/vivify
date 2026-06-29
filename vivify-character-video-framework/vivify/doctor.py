"""vivify.doctor — environment diagnostics for the vivify CLI.

Pure logic, NO Click dependency. Each `check_*` returns a CheckResult;
the CLI command in commands/doctor.py renders them. All checks are
best-effort and never raise — they catch their own exceptions and return
ok=False with a fix_hint.
"""

from __future__ import annotations

import os
import platform
import shutil
import sqlite3
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Optional


REQ_PY = (3, 10)
DISK_MIN = 1 * 1024 ** 3
ARK_MIN = 20

_FFMPEG_HINT = {
    "darwin":  "Install:\n  macOS:         brew install ffmpeg",
    "windows": ("Install:\n  Windows:       download from https://ffmpeg.org/download.html\n"
                "                (or `choco install ffmpeg` / `winget install ffmpeg`)"),
}.get(platform.system().lower(), (
    "Install:\n"
    "  Ubuntu/Debian: sudo apt install ffmpeg\n"
    "  Fedora/RHEL:   sudo dnf install ffmpeg\n"
    "  Arch:          sudo pacman -S ffmpeg"))


def _auto_install_ffmpeg_cmd() -> str:
    """Pick the right shell command to install ffmpeg on this OS, for
    `vivify doctor --self-install`. Best-effort: returns the first
    command that we *think* will work; the actual `attempt_install` may
    still fail (needs sudo, network, package manager).
    """
    sysname = platform.system().lower()
    if sysname == "darwin":
        return "brew install ffmpeg"
    if sysname == "windows":
        return "choco install ffmpeg"  # may not be on PATH; user can fix
    # Linux: try to detect the family
    apt = shutil.which("apt")
    dnf = shutil.which("dnf")
    pacman = shutil.which("pacman")
    if apt:
        return "sudo apt install -y ffmpeg"
    if dnf:
        return "sudo dnf install -y ffmpeg"
    if pacman:
        return "sudo pacman -S --noconfirm ffmpeg"
    return "sudo apt install -y ffmpeg"  # best guess


@dataclass
class CheckResult:
    name: str
    ok: bool
    detail: str
    fix_hint: Optional[str] = None
    critical: bool = False  # True if failure should produce exit code 1
    install_cmd: Optional[str] = None  # shell command for `vivify doctor --self-install`


def _safe(name: str, fn: Callable[[], CheckResult], critical: bool = False) -> CheckResult:
    try:
        return fn()
    except Exception as e:  # pragma: no cover - defensive
        return CheckResult(name=name, ok=False, detail=f"check crashed: {e}",
                           fix_hint="Report this bug — doctor should never crash.",
                           critical=critical)


def check_python() -> CheckResult:
    v = sys.version_info
    ver = f"{v.major}.{v.minor}.{v.micro}"
    ok = (v.major, v.minor) >= REQ_PY
    detail = f"Python {ver} (>= {'.'.join(map(str, REQ_PY))} required)"
    return (CheckResult(name="python", ok=True, detail=detail) if ok else
            CheckResult(name="python", ok=False, detail=detail,
                        fix_hint=f"Upgrade Python to >={'.'.join(map(str, REQ_PY))}",
                        critical=True))


def _ffmpeg_paths() -> list[Path]:
    p: list[Path] = []
    if os.environ.get("FFMPEG"):
        p.append(Path(os.environ["FFMPEG"]))
    p += [Path("/usr/bin/ffmpeg"), Path("/usr/local/bin/ffmpeg"),
          Path("/opt/homebrew/bin/ffmpeg"),
          Path("/root/.openclaw/extensions/dingtalk-connector/node_modules/"
               "@ffmpeg-installer/linux-x64/ffmpeg")]
    s = platform.system().lower()
    if s == "darwin":
        p += sorted(Path("/usr/local/Cellar/ffmpeg").glob("*/bin/ffmpeg"))
    elif s == "windows":
        p += [Path(r"C:\ffmpeg\bin\ffmpeg.exe"),
              Path(r"C:\Program Files\ffmpeg\bin\ffmpeg.exe")]
    return p


def check_ffmpeg() -> CheckResult:
    on_path = shutil.which("ffmpeg")
    cands = ([Path(on_path)] if on_path else []) + [x for x in _ffmpeg_paths() if x.exists()]
    seen: set[str] = set()
    for p in cands:
        k = str(p.resolve())
        if k in seen:
            continue
        seen.add(k)
        try:
            r = subprocess.run([str(p), "-version"], capture_output=True, text=True, timeout=3)
            if r.returncode == 0 and r.stdout:
                return CheckResult(name="ffmpeg", ok=True,
                                   detail=f"{r.stdout.splitlines()[0]}  ({p})")
        except (subprocess.TimeoutExpired, OSError, FileNotFoundError):
            continue
    return CheckResult(name="ffmpeg", ok=False, detail="NOT FOUND",
                       fix_hint=_FFMPEG_HINT, critical=True,
                       install_cmd=_auto_install_ffmpeg_cmd())


def check_mmx() -> CheckResult:
    path = shutil.which("mmx")
    if not path:
        return CheckResult(name="mmx", ok=False, detail="NOT FOUND on PATH",
                           fix_hint="Install: `pip install minimax-media` (provides `mmx`).",
                           install_cmd="pip install minimax-media")
    try:
        r = subprocess.run(["mmx", "--version"], capture_output=True, text=True, timeout=3)
    except (subprocess.TimeoutExpired, OSError) as e:
        return CheckResult(name="mmx", ok=False, detail=f"mmx at {path} but failed: {e}",
                           fix_hint="Reinstall: `pip install --upgrade minimax-media`")
    ver = ""
    if r.returncode == 0:
        for line in (r.stdout + r.stderr).splitlines():
            line = line.strip()
            if line and not line.lower().startswith("usage"):
                ver = line
                break
    if r.returncode == 0:
        return CheckResult(name="mmx", ok=True, detail=f"mmx: {ver or path}")
    return CheckResult(name="mmx", ok=False, detail=f"mmx: {ver or 'failed'}",
                       fix_hint=f"mmx at {path} but `--version` failed; "
                                "try `pip install --upgrade minimax-media`")


def _ark_key_files() -> list[Path]:
    h = Path.home()
    return [h / ".claude" / "config" / "vivify-volcengine.env",
            h / ".claude" / "config" / "opc-volcengine.env"]


def _read_key(p: Path) -> Optional[str]:
    try:
        if not p.exists():
            return None
        for raw in p.read_text(encoding="utf-8", errors="ignore").splitlines():
            line = raw.strip()
            if line.startswith("ARK_API_KEY"):
                _, _, v = line.partition("=")
                v = v.strip().strip('"').strip("'")
                if v:
                    return v
    except Exception:
        return None
    return None


def check_ark_key() -> CheckResult:
    key, source = os.environ.get("ARK_API_KEY"), "env"
    if not key:
        for fp in _ark_key_files():
            k = _read_key(fp)
            if k:
                key, source = k, str(fp)
                break
    if not key:
        return CheckResult(name="ARK_API_KEY", ok=False, detail="NOT FOUND",
                           fix_hint=("Set ARK_API_KEY in env, or write to "
                                     "~/.claude/config/vivify-volcengine.env "
                                     "as `ARK_API_KEY=<key>`"),
                           critical=True)
    if len(key) < ARK_MIN:
        return CheckResult(name="ARK_API_KEY", ok=False,
                           detail=f"too short ({len(key)} chars, expect >{ARK_MIN})",
                           fix_hint=f"Key in {source or 'env'} looks truncated.",
                           critical=True)
    d = f"{key[:8]}... (length {len(key)})"
    if source != "env":
        d += f" loaded from {source}"
    return CheckResult(name="ARK_API_KEY", ok=True, detail=d)


def check_db(db_path: Optional[str] = None) -> CheckResult:
    from .db import init_db
    from .migrator import schema_version
    path = init_db(db_path)
    v = schema_version(str(path))
    ep = sh = 0
    try:
        with sqlite3.connect(path) as c:
            ep = c.execute("SELECT COUNT(*) FROM episodes").fetchone()[0]
            sh = c.execute("SELECT COUNT(*) FROM shots").fetchone()[0]
    except sqlite3.OperationalError:
        pass
    counts = f", {ep} episodes, {sh} shots" if ep or sh else ""
    return CheckResult(name="db", ok=True, detail=f"{path}  (schema v{v}{counts})")


def check_disk_space(out_dir: Optional[Path] = None) -> CheckResult:
    target = Path(out_dir) if out_dir else Path("/tmp")
    if not target.exists():
        target.mkdir(parents=True, exist_ok=True)
    free = shutil.disk_usage(str(target)).free
    free_gb = free / 1024 ** 3
    ok = free >= DISK_MIN
    return CheckResult(
        name="disk", ok=ok, detail=f"{free_gb:.1f}G free at {target}",
        fix_hint=(f"Need >={DISK_MIN // (1024**3)}G free at {target}. "
                  "Clean up old renders or change --out-dir.") if not ok else None)


def check_models_activated(provider: str = "ark") -> CheckResult:
    key_ok = bool(os.environ.get("ARK_API_KEY"))
    src: Optional[Path] = None
    if not key_ok:
        for fp in _ark_key_files():
            if fp.exists() and _read_key(fp):
                key_ok, src = True, fp
                break
    if not key_ok:
        return CheckResult(name=f"models[{provider}]", ok=False,
                           detail=f"no {provider} key (see ARK_API_KEY above)",
                           fix_hint=f"Set ARK_API_KEY to activate {provider} models.")
    if src is None:
        for fp in _ark_key_files():
            if fp.exists():
                src = fp
                break
    has_model = False
    if src and src.exists():
        has_model = any(
            line.startswith(("MODEL", "ARK_MODEL", "VIDEO_MODEL")) and "=" in line
            for line in src.read_text(encoding="utf-8", errors="ignore").splitlines())
    extra = " (env file has model name)" if has_model else " (no model in env file — using defaults)"
    return CheckResult(name=f"models[{provider}]", ok=True,
                       detail=f"{provider} key present{extra}")


def run_all_checks(out_dir: Optional[Path] = None,
                   db_path: Optional[str] = None) -> list[CheckResult]:
    """Run every check in stable display order. Each check is exception-safe."""
    return [
        _safe("python",  check_python,                  critical=True),
        _safe("ffmpeg",  check_ffmpeg,                  critical=True),
        _safe("mmx",     check_mmx),
        _safe("ark_key", check_ark_key,                 critical=True),
        _safe("db",      lambda: check_db(db_path),     critical=True),
        _safe("disk",    lambda: check_disk_space(out_dir)),
        _safe("models",  lambda: check_models_activated("ark")),
    ]


__all__ = [
    "CheckResult",
    "check_python", "check_ffmpeg", "check_mmx", "check_ark_key",
    "check_db", "check_disk_space", "check_models_activated", "run_all_checks",
]
