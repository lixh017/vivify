"""character_loader.py — load character.yaml into a runtime dict.

The character.yaml is the heart of the framework — it defines:
  - Identity (canonical reference images)
  - Outfit / scene / prop whitelists
  - Voice profiles per tone
  - Visual style anchor
  - Voice rules + hook formulas

The pipeline reads this once at startup and uses the values to:
  - Send canonical reference image to Seedream (identity preservation)
  - Lock visual style across all shots (style_anchor appended to prompts)
  - Pick the right TTS voice_id + speed + emotion per tone
  - Validate outfits/scenes used in storyboards

This makes the pipeline character-agnostic. To add a new character:
  1. Drop 3 reference jpgs in characters/<name>/canonical/
  2. Copy characters/_template/character.yaml to characters/<name>/
  3. Fill in the values (see characters/fengge/character.yaml for reference)
  4. Run: --character-dir characters/<name>
"""

import sys
from pathlib import Path


def load_character(character_dir: str) -> dict:
    """Load character.yaml from the given directory.

    Returns a dict with the full character profile. Raises a clear
    error if the file is missing or malformed.
    """
    char_path = Path(character_dir) / "character.yaml"
    if not char_path.exists():
        raise SystemExit(
            f"character.yaml not found at {char_path}\n"
            f"  See characters/_template/character.yaml for the schema."
        )

    try:
        import yaml
    except ImportError:
        raise SystemExit(
            "PyYAML is required: pip install pyyaml"
        )

    with open(char_path, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    if "character" not in data:
        raise SystemExit(
            f"{char_path} missing top-level 'character:' key"
        )

    char = data["character"]

    # Resolve canonical paths relative to character_dir
    if "canonical" in char:
        canon = char["canonical"]
        if "primary" in canon:
            canon["primary"] = str(Path(character_dir) / canon["primary"])
        if "alternates" in canon:
            canon["alternates"] = [
                str(Path(character_dir) / p) for p in canon["alternates"]
            ]

    # Required fields
    required = ["name", "canonical", "outfits", "scenes", "voice_profiles"]
    missing = [r for r in required if r not in char]
    if missing:
        raise SystemExit(
            f"{char_path} missing required fields: {', '.join(missing)}"
        )

    # Default style_anchor if not provided
    if "style_anchor" not in char:
        char["style_anchor"] = ""

    # Default hook_formulas to empty list
    if "hook_formulas" not in char:
        char["hook_formulas"] = []

    # Default voice_rules
    if "voice_rules" not in char:
        char["voice_rules"] = {"sentence_style": [], "signature_words": [], "banned_words": []}

    # Build outfit lookup
    char["_outfit_by_id"] = {o["id"]: o for o in char["outfits"]}
    char["_scene_by_id"] = {s["id"]: s for s in char["scenes"]}
    char["_voice_by_tone"] = char["voice_profiles"]

    return char


def validate_outfit(char: dict, outfit_id: str) -> dict:
    """Look up an outfit by ID, or return a fallback descriptor."""
    return char["_outfit_by_id"].get(outfit_id, {
        "id": outfit_id,
        "name": outfit_id,
        "description": "[unknown outfit — please add to character.yaml outfits list]",
        "anti_patterns": "",
    })


def format_outfit_desc(char: dict, outfit_id: str) -> str:
    """Format an outfit as a Kling prompt fragment."""
    o = validate_outfit(char, outfit_id)
    desc = o.get("description", "")
    anti = o.get("anti_patterns", "")
    parts = [f"{o['name']} ({desc})"]
    if anti:
        parts.append(f"**{anti}**")
    return ", ".join(parts)


def get_style_anchor(char: dict) -> str:
    """Return the style anchor string for prompt augmentation."""
    return char.get("style_anchor", "").replace("\n", " ").strip()


def get_voice_profile(char: dict, tone: str) -> dict:
    """Return the TTS voice profile for a tone, or a default."""
    return char["_voice_by_tone"].get(tone, {
        "voice_id": "male-qn-jingying",
        "speed": 0.85,
        "pitch": 0,
        "emotion": "neutral",
        "vol": 1.0,
        "guide": "",
    })


if __name__ == "__main__":
    # Smoke test
    if len(sys.argv) < 2:
        print("usage: character_loader.py <character-dir>")
        sys.exit(1)
    char = load_character(sys.argv[1])
    print(f"loaded: {char['name']} ({char.get('english_name', '')})")
    print(f"  outfits: {len(char['outfits'])}")
    print(f"  scenes:  {len(char['scenes'])}")
    print(f"  tones:   {', '.join(char['voice_profiles'].keys())}")
    print(f"  primary canonical: {char['canonical']['primary']}")