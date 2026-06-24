"""Tests for pyproject.toml — package metadata + tool config.

Validates that the pyproject.toml is parseable, declares the expected
runtime + test dependencies, and configures pytest correctly.
"""

from pathlib import Path
import sys

try:
    import tomllib  # Python 3.11+
except ImportError:
    import tomli as tomllib  # type: ignore

import pytest


PYPROJECT = Path(__file__).parent.parent / "pyproject.toml"


def _load():
    assert PYPROJECT.exists(), f"{PYPROJECT} missing"
    return tomllib.loads(PYPROJECT.read_text(encoding="utf-8"))


def test_pyproject_is_valid_toml():
    """tomllib can parse the file without errors."""
    data = _load()
    assert "project" in data
    assert "build-system" in data


def test_pyproject_declares_vivify_package():
    data = _load()
    assert data["project"]["name"] == "vivify"
    assert data["project"]["version"]


def test_pyproject_runtime_deps_include_click_and_yaml():
    """The CLI imports click and PyYAML at module-load time."""
    data = _load()
    deps = data["project"]["dependencies"]
    assert any(d.startswith("click") for d in deps), deps
    assert any(d.lower().startswith("pyyaml") for d in deps), deps


def test_pyproject_pytest_testpaths():
    """pytest is configured to run from the tests/ dir."""
    data = _load()
    assert "tool" in data
    assert "pytest" in data["tool"]
    ini = data["tool"]["pytest"]["ini_options"]
    assert ini["testpaths"] == ["tests"]


def test_pyproject_excludes_non_package_dirs():
    """The package finder should NOT include tests/, characters/, etc."""
    data = _load()
    opts = data["tool"]["setuptools"]["packages"]["find"]
    excludes = set(opts.get("exclude", []))
    expected = {"tests*", "characters*", "examples*", "prompt_library*",
                "validators*", "docs*", "data*", "scripts*", "memory*"}
    assert expected.issubset(excludes), f"missing excludes: {expected - excludes}"


def test_pyproject_python_requires_at_least_310():
    """Click 8.1 + Python 3.10 f-string syntax require 3.10+."""
    data = _load()
    assert data["project"]["requires-python"] == ">=3.10"


def test_pyproject_optional_deps_include_image():
    """Pillow is optional — only needed for image dimension probing."""
    data = _load()
    optional = data["project"]["optional-dependencies"]
    assert "image" in optional, "missing [image] optional dep for Pillow"