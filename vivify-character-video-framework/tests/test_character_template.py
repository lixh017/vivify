"""Tests for the multi-IP character template + `vivify character new`.

Two layers of tests:

1. Template integrity — every file in `characters/_template/` is present
   and has placeholders the generator knows how to fill (or, for
   intentionally-unfilled ones like `{{opening_a}}`, no GENERATED_* set).

2. Generator behavior — `vivify character new <name> --non-interactive`
   scaffolds a fresh IP from the template, substitutes placeholders
   correctly, and produces a character.yaml that passes the linter
   (after dropping a stub canonical jpg ≥ 50KB).
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml
from click.testing import CliRunner

from vivify.cli import main as cli
from vivify.commands.character import (
    GENERATED_PLACEHOLDERS,
    scaffold_character,
)


# ── paths ─────────────────────────────────────────────────────────────

# Project root is one level above this tests/ dir.
_PROJECT_ROOT = Path(__file__).resolve().parent.parent
TEMPLATE_DIR = _PROJECT_ROOT / "characters" / "_template"


# Required files (relative to template dir). All are part of the
# contract the generator must keep filling.
REQUIRED_TEMPLATE_FILES = [
    "character.yaml",
    "README.md",
    "lessons.md",
    "gotchas.md",
    "canonical/.gitkeep",
    "overrides/README.md",
    "experiments/README.md",
    "analytics/README.md",
    "examples/README.md",
]


# ── fixtures ──────────────────────────────────────────────────────────


@pytest.fixture
def fresh_template_copy(tmp_path: Path) -> Path:
    """Copy the real _template/ to a throwaway location, then return it.

    This isolates each test from the on-disk template state and lets us
    drop stub jpgs into the copy to test lint pass-through.
    """
    import shutil
    copy = tmp_path / "_template_copy"
    shutil.copytree(TEMPLATE_DIR, copy)
    return copy


@pytest.fixture
def stub_canonical(tmp_path: Path) -> Path:
    """Create a directory with 3 stub jpgs ≥ 50KB each.

    Tests use this to satisfy `lint_character.py`'s canonical image
    size requirement when validating a generated character.yaml.
    """
    canon = tmp_path / "canonical"
    canon.mkdir()
    for name in ("primary.jpg", "alternate-1.jpg", "alternate-2.jpg"):
        (canon / name).write_bytes(b"\xff\xd8\xff\xe0" + b"X" * (60 * 1024))
    return canon


# ── 1. Template integrity ─────────────────────────────────────────────


def test_template_directory_exists():
    """The template directory must exist (this is the source of truth)."""
    assert TEMPLATE_DIR.is_dir(), f"missing template dir: {TEMPLATE_DIR}"


@pytest.mark.parametrize("rel_path", REQUIRED_TEMPLATE_FILES)
def test_template_file_present(rel_path: str):
    """Every required template file is present and non-empty."""
    p = TEMPLATE_DIR / rel_path
    assert p.is_file(), f"missing template file: {p}"
    assert p.stat().st_size > 0, f"empty template file: {p}"


def test_template_has_no_unknown_placeholders():
    """All {{...}} placeholders in the template are in GENERATED_PLACEHOLDERS
    OR are intentionally un-filled (we know about these two: opening_a,
    opening_b — used in the experiments README example)."""
    # Whitelist of placeholders that are intentionally not auto-filled.
    # The user is expected to fill these by hand.
    known_unfilled = {"opening_a", "opening_b"}
    unknown: list[tuple[str, str]] = []
    for f in TEMPLATE_DIR.rglob("*"):
        if not f.is_file():
            continue
        text = f.read_text(encoding="utf-8")
        for match in re.findall(r"\{\{\s*([a-zA-Z0-9_]+)\s*\}\}", text):
            if match in GENERATED_PLACEHOLDERS:
                continue
            if match in known_unfilled:
                continue
            unknown.append((str(f.relative_to(TEMPLATE_DIR)), match))
    assert not unknown, (
        "template contains placeholders neither auto-filled nor "
        f"whitelisted:\n  " + "\n  ".join(f"{p}: {m}" for p, m in unknown)
    )


def test_template_character_yaml_has_generated_metadata():
    """The template character.yaml includes a `_generated:` block that
    records the idempotency_seed + default_voice + personality tags.
    Agents inspecting a generated IP need this metadata to know how
    the file was produced."""
    p = TEMPLATE_DIR / "character.yaml"
    data = yaml.safe_load(p.read_text(encoding="utf-8"))
    assert "_generated" in data, "template must declare _generated metadata"
    gen = data["_generated"]
    assert "idempotency_seed" in gen
    assert "default_voice" in gen
    assert "personality_tags" in gen
    assert len(gen["personality_tags"]) == 3


# ── 2. Generator behavior (scaffold_character) ────────────────────────


def test_scaffold_creates_directory_with_all_template_files(tmp_path: Path):
    """scaffold_character creates characters/<slug>/ and copies all
    template files (including README.md, lessons.md, gotchas.md,
    overrides/, experiments/, analytics/, examples/, canonical/.gitkeep)."""
    target = scaffold_character(
        "测试熊猫",
        species="成年熊猫",
        palette_primary="C73E1D",
        palette_secondary="F5F0E1",
        personalities=["内敛", "幽默", "治愈"],
        scenes=["竹林小院", "月下窗棂", "围炉夜话"],
        default_voice="治愈",
        project_root=_PROJECT_ROOT,
    )
    try:
        # The dir is created under _PROJECT_ROOT / "characters" / <slug>.
        assert target.is_dir()
        for rel in REQUIRED_TEMPLATE_FILES:
            assert (target / rel).is_file(), f"missing {rel} in {target}"
    finally:
        # Cleanup — scaffold_character writes to the real project root,
        # so we must remove it after the test.
        import shutil
        if target.exists():
            shutil.rmtree(target)


def test_scaffold_substitutes_placeholders(tmp_path: Path):
    """Every GENERATED_PLACEHOLDER is replaced in the output character.yaml.
    No {{...}} placeholders remain in the generated files (the only
    exceptions are the intentionally-unfilled {{opening_a}} / {{opening_b}}
    in the experiments README example)."""
    import shutil
    target = scaffold_character(
        "烟烟罗",
        species="小猫",
        palette_primary="112233",
        palette_secondary="FFEEDD",
        personalities=["傲娇", "聪明", "嘴硬"],
        scenes=["暖炉旁", "古道边", "雨巷", "围炉夜话"],
        default_voice="哲学",
        project_root=_PROJECT_ROOT,
    )
    try:
        # ── character.yaml: name, species, palette, voice all replaced ──
        yaml_text = (target / "character.yaml").read_text(encoding="utf-8")
        data = yaml.safe_load(yaml_text)
        char = data["character"]
        assert char["name"] == "烟烟罗"
        assert char["species"] == "小猫"
        assert char["fur_face"] == "112233"
        assert char["fur_extremity"] == "FFEEDD"
        # Voice profiles keep their 4 tones; the seeded _generated
        # block records the default tone.
        gen = data["_generated"]
        assert gen["default_voice"] == "哲学"
        assert gen["personality_tags"] == ["傲娇", "聪明", "嘴硬"]
        # All generated placeholders are gone from character.yaml
        remaining = re.findall(r"\{\{\s*([a-zA-Z0-9_]+)\s*\}\}", yaml_text)
        # Filter out the experiment-block's intentionally unfilled placeholders
        # (which are in README.md, not character.yaml) — character.yaml
        # should have zero.
        assert not remaining, f"unfilled placeholders in character.yaml: {remaining}"

        # ── README.md: name, species, palette, voice all replaced ──
        readme = (target / "README.md").read_text(encoding="utf-8")
        assert "烟烟罗" in readme
        assert "小猫" in readme
        assert "112233" in readme
        assert "哲学" in readme
        # opening_a / opening_b are intentionally NOT filled
        assert "${opening_a}" in readme or "{{opening_a}}" in readme
    finally:
        if target.exists():
            shutil.rmtree(target)


def test_scaffold_rejects_invalid_palette():
    """Non-hex palette is rejected with a clear error."""
    with pytest.raises(Exception) as exc:
        scaffold_character(
            "test",
            species="x",
            palette_primary="not-a-hex",
            palette_secondary="F5F0E1",
            personalities=["a", "b", "c"],
            scenes=["s1", "s2", "s3"],
            default_voice="治愈",
            project_root=_PROJECT_ROOT,
        )
    assert "hex" in str(exc.value).lower()


def test_scaffold_rejects_wrong_number_of_personalities():
    """Exactly 3 personality tags are required."""
    with pytest.raises(Exception) as exc:
        scaffold_character(
            "test",
            species="x",
            palette_primary="AABBCC",
            palette_secondary="F5F0E1",
            personalities=["only-one"],
            scenes=["s1", "s2", "s3"],
            default_voice="治愈",
            project_root=_PROJECT_ROOT,
        )
    assert "3" in str(exc.value)


def test_scaffold_rejects_invalid_voice():
    """Voice must be one of the 4 standard tones."""
    with pytest.raises(Exception) as exc:
        scaffold_character(
            "test",
            species="x",
            palette_primary="AABBCC",
            palette_secondary="F5F0E1",
            personalities=["a", "b", "c"],
            scenes=["s1", "s2", "s3"],
            default_voice="摇滚",  # not in the standard set
            project_root=_PROJECT_ROOT,
        )
    assert "voice" in str(exc.value).lower() or "国潮" in str(exc.value)


def test_scaffold_refuses_to_overwrite(tmp_path: Path):
    """If characters/<slug>/ already exists, scaffold_character refuses
    rather than clobbering it (safety net for accidental re-runs)."""
    # First call: succeeds, leaves dir on disk
    target = scaffold_character(
        "duptest",
        species="x",
        palette_primary="AABBCC",
        palette_secondary="F5F0E1",
        personalities=["a", "b", "c"],
        scenes=["s1", "s2", "s3"],
        default_voice="治愈",
        project_root=_PROJECT_ROOT,
    )
    try:
        # Second call: must fail
        with pytest.raises(Exception) as exc:
            scaffold_character(
                "duptest",
                species="x",
                palette_primary="AABBCC",
                palette_secondary="F5F0E1",
                personalities=["a", "b", "c"],
                scenes=["s1", "s2", "s3"],
                default_voice="治愈",
                project_root=_PROJECT_ROOT,
            )
        assert "exists" in str(exc.value).lower()
    finally:
        import shutil
        if target.exists():
            shutil.rmtree(target)


# ── 3. CLI integration: `vivify character new --non-interactive` ──────


def _invoke(*args, db_path=None, project_root=None) -> "click.testing.Result":
    """Invoke the CLI as if from a real shell.

    Note: `vivify character new` does NOT need a DB to scaffold, but the
    main `vivify` group auto-runs `init_db` and `apply_pending` on every
    invocation, so we always pass --db to a tmp path.
    """
    argv = []
    if db_path is not None:
        argv += ["--db", str(db_path), "--quiet-init"]
    argv += list(args)
    return CliRunner().invoke(cli, argv)


def test_cli_new_noninteractive_scaffolds_directory(
    tmp_path: Path, db_path_setup
):
    """`vivify character new <name> --non-interactive --...` creates the
    IP directory under characters/ and registers the IP in the DB."""
    # Note: real CLI scaffolds under _PROJECT_ROOT/characters/<slug>;
    # we use a fresh slug per test run and clean up.
    import shutil
    slug = "smoketest"
    target = _PROJECT_ROOT / "characters" / slug
    if target.exists():
        shutil.rmtree(target)

    result = _invoke(
        "character", "new", slug,
        "--species", "测试小狗",
        "--palette-primary", "334455",
        "--palette-secondary", "FFEEDD",
        "--personality", "活泼,好奇,机灵",
        "--scenes", "公园,河边,城市街角",
        "--voice", "御宅",
        "--english-name", "Smoketest",
        "--non-interactive",
        db_path=str(db_path_setup),
    )
    try:
        assert result.exit_code == 0, (
            f"CLI failed (exit {result.exit_code}):\n{result.output}"
        )
        assert target.is_dir(), f"expected dir at {target}"
        assert (target / "character.yaml").is_file()
        assert (target / "README.md").is_file()
        assert (target / "lessons.md").is_file()
        assert (target / "gotchas.md").is_file()
        assert (target / "canonical" / ".gitkeep").is_file()
        # The CLI should also print next-steps
        assert "Next steps" in result.output
        assert "lint" in result.output.lower()
    finally:
        if target.exists():
            shutil.rmtree(target)


def test_cli_new_noninteractive_missing_args_fails(db_path_setup):
    """If --non-interactive is set and any of the 5 answers is missing,
    the CLI exits non-zero with a clear error."""
    result = _invoke(
        "character", "new", "x",
        "--non-interactive",
        # --species, --palette-primary, etc. all missing
        db_path=str(db_path_setup),
    )
    assert result.exit_code != 0
    assert "--species" in result.output or "non-interactive" in result.output.lower()


# ── 4. End-to-end: generated character.yaml passes lint_character ─────


def test_generated_character_passes_lint_character(
    tmp_path: Path, db_path_setup, stub_canonical
):
    """The whole point of the generator: a freshly-scaffolded IP, once
    the user drops 3 jpgs into canonical/, should pass
    `validators/lint_character.py`.

    This catches drift between the template's structure and the
    linter's expectations (e.g. if we rename `outfit_*` or change
    the `anti_patterns` format, this test will break first)."""
    import shutil
    import subprocess

    # Scaffold
    slug = "lintcheck"
    target = _PROJECT_ROOT / "characters" / slug
    if target.exists():
        shutil.rmtree(target)
    scaffold_character(
        slug,
        species="成年熊猫",
        palette_primary="C73E1D",
        palette_secondary="F5F0E1",
        personalities=["内敛", "幽默", "治愈"],
        scenes=["竹林小院", "月下窗棂", "围炉夜话"],
        default_voice="治愈",
        project_root=_PROJECT_ROOT,
    )
    try:
        # Drop stub jpgs (the linter requires 3 jpgs, each ≥ 50KB)
        canon = target / "canonical"
        for name in ("primary.jpg", "alternate-1.jpg", "alternate-2.jpg"):
            shutil.copy(stub_canonical / name, canon / name)

        # Run the linter as a subprocess (it's a script, not a module)
        result = subprocess.run(
            ["python3", "validators/lint_character.py", str(target)],
            capture_output=True, text=True,
            cwd=str(_PROJECT_ROOT),
        )
        assert result.returncode == 0, (
            f"lint_character failed (rc={result.returncode}):\n"
            f"STDOUT:\n{result.stdout}\nSTDERR:\n{result.stderr}"
        )
        assert "ALL CHECKS PASSED" in result.stdout
    finally:
        if target.exists():
            shutil.rmtree(target)


# ── helpers ───────────────────────────────────────────────────────────


@pytest.fixture
def db_path_setup(tmp_path: Path) -> Path:
    """Init a throwaway DB for tests that need the auto-migrate side effect."""
    from vivify.db import init_db
    db = tmp_path / "test.db"
    init_db(str(db))
    return db
