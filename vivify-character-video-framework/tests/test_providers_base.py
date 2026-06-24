"""Tests for vivify.providers.base — the ProviderAdapter ABC + dataclasses.

These are pure data-class / interface tests. No I/O, no mocking.
"""

from dataclasses import fields
from pathlib import Path

import pytest

from vivify.providers.base import (
    AssetType,
    GenerateRequest,
    GenerateResult,
    ProviderAdapter,
)


def test_asset_type_literal_includes_expected_values():
    """AssetType is a Literal of {video, image, tts, bgm}."""
    from typing import get_args
    assert set(get_args(AssetType)) == {"video", "image", "tts", "bgm"}


def test_generate_request_required_and_optional_fields():
    """GenerateRequest has all the SKILL.md-spec'd fields, with sensible defaults."""
    req = GenerateRequest(
        scene_id="panda-st05-sh02-v1",
        asset_type="image",
        prompt="a red circle on white",
    )
    assert req.scene_id == "panda-st05-sh02-v1"
    assert req.asset_type == "image"
    assert req.prompt == "a red circle on white"
    assert req.reference_image is None
    assert req.duration_sec is None
    assert req.size is None
    assert req.outfit is None
    assert req.tone is None
    assert req.options == {}  # default factory


def test_generate_request_full_construction():
    """All fields can be set; round-trips through dataclass repr cleanly."""
    req = GenerateRequest(
        scene_id="panda-st05-sh02-v1",
        asset_type="video",
        prompt="a panda walks",
        reference_image="/tmp/face.jpg",
        duration_sec=5,
        size="1024x1792",
        outfit="hufu_red",
        tone="国潮",
        options={"ratio": "9:16", "resolution": "720p"},
    )
    assert req.reference_image == "/tmp/face.jpg"
    assert req.duration_sec == 5
    assert req.options["ratio"] == "9:16"


def test_generate_result_defaults_to_failure():
    """GenerateResult defaults to ok=False, no asset, no path, zero cost."""
    r = GenerateResult(ok=False)
    assert r.ok is False
    assert r.asset_id is None
    assert r.local_path is None
    assert r.public_url is None
    assert r.cost_yuan == 0.0
    assert r.duration_ms == 0
    assert r.provider == ""
    assert r.model == ""
    assert r.error is None


def test_generate_result_success_construction():
    """Success result carries vendor IDs, local path, cost, duration."""
    r = GenerateResult(
        ok=True,
        asset_id="cgt-20260608140817-l579p",
        local_path=Path("/tmp/clip.mp4"),
        public_url="https://ark-content-generation...mp4",
        cost_yuan=2.0,
        duration_ms=45000,
        provider="ark",
        model="doubao-seedance-1-5-pro-251215",
    )
    assert r.ok is True
    assert r.local_path == Path("/tmp/clip.mp4")
    assert r.provider == "ark"
    assert r.cost_yuan == 2.0


def test_provider_adapter_is_abstract():
    """Cannot instantiate ProviderAdapter directly — its methods are abstract."""
    with pytest.raises(TypeError):
        ProviderAdapter()  # type: ignore[abstract]


def test_provider_adapter_subclass_must_implement_methods():
    """Subclass that doesn't implement generate/cost_estimate is still abstract."""
    class HalfBaked(ProviderAdapter):
        name = "half"
        asset_type = "image"
        def generate(self, req):  # missing cost_estimate
            return GenerateResult(ok=True)
    with pytest.raises(TypeError):
        HalfBaked()  # type: ignore[abstract]


def test_provider_adapter_subclass_works_when_complete():
    """A complete subclass can be instantiated."""
    class StubProvider(ProviderAdapter):
        name = "stub"
        asset_type = "image"
        def generate(self, req):
            return GenerateResult(ok=True, provider="stub", model="m")
        def cost_estimate(self, req):
            return 0.1
    p = StubProvider()
    assert p.name == "stub"
    assert p.cost_estimate(GenerateRequest(scene_id="x", asset_type="image", prompt="y")) == 0.1
    assert p.generate(GenerateRequest(scene_id="x", asset_type="image", prompt="y")).ok is True
