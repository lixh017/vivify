"""vivify.commands.publish — publish a completed episode + track analytics.

Commands:
  publish   <character> <episode> --platform {抖音,小红书,B站}
              Stub: writes a publishes row + simulated URL. Real upload
              would call 抖音开放平台 / 小红书 API. Currently the API
              integration is a stub — the DB record is the source of truth.

  list      [--character X] [--platform Y]
              Show all published episodes.

  show      <publish-id>
              Show details of one publish.

  analytics <character> <episode>
              Show aggregated analytics across all platforms for the episode.

  refresh   <publish-id> [--simulate]
              Fetch latest view/like/comment from platform API. Stub mode
              by default — increments the existing counts by a small
              random amount so you can see the UI working.

The publish flow today is intentionally simple:

  1. `vivify episode render <char> <ep>` → writes episode row
  2. `vivify publish publish <char> <ep> --platform 抖音` → writes publish row
     (no real upload — we don't have 抖音 API keys)
  3. `vivify publish refresh <id>` → updates counts (stub: random delta)

When real 抖音/小红书 API access is set up, swap `upload_to_platform()`
below with the actual HTTP call. The DB schema and command surface
stay the same.
"""

import json
import random
import uuid
from datetime import datetime, timezone

import click

from ..db import connect, init_db


# Per-platform URL templates (stub). Real upload would return a real URL.
_PLATFORM_URL_TEMPLATES = {
    "抖音": "https://www.douyin.com/video/{stub_id}",
    "小红书": "https://www.xiaohongshu.com/explore/{stub_id}",
    "哔哩哔哩": "https://www.bilibili.com/video/{stub_id}",
}


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def _upload_to_platform(platform: str, video_path: str, title: str,
                        description: str, hashtags: str) -> dict:
    """STUB: simulate a platform upload. Returns dict with published_url.

    Real implementation would:
      抖音:   POST https://open.douyin.com/video/create/  (with openapi token)
      小红书: POST https://edith.xiaohongshu.com/api/sns/v1/web/notes
      哔哩哔哩: POST https://member.bilibili.com/v2/upload/video

    We don't have those credentials wired, so the stub returns a fake URL
    and the DB row is the only artifact. This lets the rest of the
    platform (analytics, list, show) be exercised end-to-end.
    """
    stub_id = uuid.uuid4().hex[:18]
    template = _PLATFORM_URL_TEMPLATES.get(platform, "https://stub/{stub_id}")
    return {
        "published_url": template.format(stub_id=stub_id),
        "platform": platform,
    }


def _fetch_analytics(platform: str, published_url: str) -> dict:
    """STUB: simulate an analytics refresh. Returns fake-but-plausible deltas.

    Real implementation would call:
      抖音:   /video/data/ (needs /oauth/access_token + openid)
      小红书: /api/sns/v1/web/notes/{note_id}/metrics
      B站:    /x/web-interface/view?bvid=...

    We simulate with random deltas so the user can see analytics move.
    """
    return {
        "view_count_delta": random.randint(10, 500),
        "like_count_delta": random.randint(1, 50),
        "comment_count_delta": random.randint(0, 10),
        "completion_rate": round(random.uniform(0.4, 0.9), 3),
    }


@click.group("publish")
def cli():
    """Publish completed episodes + track analytics."""
    pass


@cli.command("publish")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--platform", "-p", required=True,
              type=click.Choice(["抖音", "小红书", "哔哩哔哩"]))
@click.option("--title", default=None,
              help="Post title (defaults to '{character}短剧 EP{id}').")
@click.option("--description", "-d", default="",
              help="Post description / caption.")
@click.option("--hashtags", default=None,
              help="Comma-separated hashtags (e.g. 'panda,治愈,凌晨').")
@click.option("--dry-run", is_flag=True)
@click.pass_obj
def publish_cmd(obj, character_id, episode_id, platform, title, description,
                hashtags, dry_run):
    """Publish a completed episode to a platform.

    STUB: writes a DB row + returns a fake URL. No real upload happens.
    """
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        ep_row = conn.execute(
            "SELECT * FROM episodes WHERE character_id = ? AND episode_id = ?",
            (character_id, episode_id),
        ).fetchone()
        if not ep_row:
            raise click.ClickException(
                f"episode {character_id}/{episode_id} not found "
                f"(run `vivify episode render` first)")
        if ep_row["status"] != "completed":
            click.echo(f"[warn] episode status is '{ep_row['status']}', "
                       f"not 'completed'. Publishing anyway (intentional?).",
                       err=True)

    title = title or f"{character_id}短剧 {episode_id}"
    hashtags = hashtags or ""

    click.echo(f"Publishing {character_id}/{episode_id} to {platform}...")
    if dry_run:
        click.echo(f"  title:      {title}")
        click.echo(f"  description: {description[:80]}{'...' if len(description) > 80 else ''}")
        click.echo(f"  hashtags:   {hashtags}")
        click.echo(f"  video:      {ep_row['output_path'] or '—'}")
        click.echo("(dry-run: nothing written)")
        return

    upload_result = _upload_to_platform(
        platform, ep_row["output_path"] or "", title, description, hashtags)
    published_url = upload_result["published_url"]

    with connect(db_path) as conn:
        cur = conn.execute(
            """INSERT INTO publishes (
                episode_id, platform, published_url, published_at,
                title, description, hashtags
            ) VALUES (?, ?, ?, ?, ?, ?, ?)""",
            (ep_row["id"], platform, published_url, _now_iso(),
             title, description, hashtags),
        )
        pub_id = cur.lastrowid
    click.echo(f"✓ published (stub): {published_url}")
    click.echo(f"  publish id: {pub_id}")
    click.echo(f"  → run `vivify publish refresh {pub_id}` to update analytics")


@cli.command("list")
@click.option("--character", "-c", default=None)
@click.option("--platform", "-p", default=None,
              type=click.Choice(["抖音", "小红书", "哔哩哔哩"]))
@click.option("--as-json", "as_json", is_flag=True)
@click.pass_obj
def list_cmd(obj, character, platform, as_json):
    """List all publishes (optionally filtered)."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        # NOTE: avoid p.* / e.episode_id collision — publishes.episode_id is
        # the FK integer, episodes.episode_id is the user-visible string.
        # We alias the FK to fk_episode_id to keep both disambiguated.
        q = """SELECT p.id AS id, p.platform, p.published_url, p.published_at,
                      p.title, p.description, p.hashtags,
                      p.view_count, p.like_count, p.comment_count,
                      p.completion_rate,
                      p.episode_id AS fk_episode_id,
                      e.character_id, e.episode_id AS ep_code
               FROM publishes p
               JOIN episodes e ON e.id = p.episode_id
               WHERE 1=1"""
        params: list = []
        if character:
            q += " AND e.character_id = ?"; params.append(character)
        if platform:
            q += " AND p.platform = ?"; params.append(platform)
        q += " ORDER BY p.published_at DESC"
        rows = [dict(r) for r in conn.execute(q, params).fetchall()]
    if as_json:
        click.echo(json.dumps(rows, indent=2, ensure_ascii=False, default=str))
        return
    if not rows:
        click.echo("(no publishes — try `vivify publish publish <char> <ep> --platform 抖音`)")
        return
    click.echo(f"{'ID':<4} {'CHAR':<10} {'EP':<8} {'PLATFORM':<8} "
               f"{'VIEWS':<8} {'LIKES':<8} {'COMMENTS':<8} PUBLISHED")
    click.echo("─" * 90)
    for r in rows:
        click.echo(
            f"{r['id']:<4} {r['character_id']:<10} {r['ep_code']:<8} "
            f"{r['platform']:<8} {r['view_count']:<8} {r['like_count']:<8} "
            f"{r['comment_count']:<8} {r['published_at']}"
        )


@cli.command("show")
@click.argument("publish_id", type=int)
@click.pass_obj
def show_cmd(obj, publish_id):
    """Show details of one publish."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        # Same FK-vs-code disambiguation as list_cmd
        row = conn.execute(
            """SELECT p.id, p.platform, p.published_url, p.published_at,
                      p.title, p.description, p.hashtags,
                      p.view_count, p.like_count, p.comment_count,
                      p.completion_rate, p.episode_id AS fk_episode_id,
                      e.character_id, e.episode_id AS ep_code
               FROM publishes p
               JOIN episodes e ON e.id = p.episode_id
               WHERE p.id = ?""",
            (publish_id,),
        ).fetchone()
    if not row:
        raise click.ClickException(f"publish #{publish_id} not found")
    click.echo(f"═══ Publish #{row['id']} — {row['character_id']}/{row['ep_code']} ═══")
    click.echo(f"  platform:      {row['platform']}")
    click.echo(f"  url:           {row['published_url']}")
    click.echo(f"  published_at:  {row['published_at']}")
    click.echo(f"  title:         {row['title']}")
    if row["description"]:
        click.echo(f"  description:   {row['description']}")
    if row["hashtags"]:
        click.echo(f"  hashtags:      {row['hashtags']}")
    click.echo("")
    click.echo(f"  view_count:     {row['view_count']}")
    click.echo(f"  like_count:     {row['like_count']}")
    click.echo(f"  comment_count:  {row['comment_count']}")
    click.echo(f"  completion_rate: {row['completion_rate'] or '—'}")


@cli.command("analytics")
@click.argument("character_id")
@click.argument("episode_id")
@click.option("--as-json", "as_json", is_flag=True)
@click.pass_obj
def analytics_cmd(obj, character_id, episode_id, as_json):
    """Aggregated analytics across all platforms for one episode."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        ep = conn.execute(
            "SELECT * FROM episodes WHERE character_id = ? AND episode_id = ?",
            (character_id, episode_id),
        ).fetchone()
        if not ep:
            raise click.ClickException(f"episode {character_id}/{episode_id} not found")
        pubs = [dict(r) for r in conn.execute(
            "SELECT * FROM publishes WHERE episode_id = ? ORDER BY platform",
            (ep["id"],),
        ).fetchall()]

    if as_json:
        click.echo(json.dumps({"episode": dict(ep), "publishes": pubs},
                              indent=2, ensure_ascii=False, default=str))
        return

    if not pubs:
        click.echo(f"(no publishes for {character_id}/{episode_id})")
        return

    totals = {
        "view_count": sum(p["view_count"] for p in pubs),
        "like_count": sum(p["like_count"] for p in pubs),
        "comment_count": sum(p["comment_count"] for p in pubs),
    }
    avg_completion = None
    completed = [p["completion_rate"] for p in pubs if p["completion_rate"] is not None]
    if completed:
        avg_completion = sum(completed) / len(completed)

    click.echo(f"═══ Analytics — {character_id}/{episode_id} ═══")
    click.echo(f"  publishes:     {len(pubs)}")
    click.echo(f"  totals: views={totals['view_count']}  "
               f"likes={totals['like_count']}  comments={totals['comment_count']}")
    if avg_completion is not None:
        click.echo(f"  avg completion: {avg_completion:.1%}")
    click.echo("")
    click.echo(f"  {'PLATFORM':<10} {'VIEWS':<8} {'LIKES':<8} {'COMMENTS':<8} "
               f"{'COMPLETION':<12} URL")
    click.echo("  " + "─" * 80)
    for p in pubs:
        comp = f"{p['completion_rate']:.1%}" if p["completion_rate"] else "—"
        click.echo(f"  {p['platform']:<10} {p['view_count']:<8} {p['like_count']:<8} "
                   f"{p['comment_count']:<8} {comp:<12} {p['published_url']}")


@cli.command("refresh")
@click.argument("publish_id", type=int)
@click.pass_obj
def refresh_cmd(obj, publish_id):
    """Fetch latest analytics for a publish (STUB: random deltas)."""
    db_path = obj.get("db_path")
    init_db(db_path)
    with connect(db_path) as conn:
        row = conn.execute(
            "SELECT * FROM publishes WHERE id = ?", (publish_id,),
        ).fetchone()
        if not row:
            raise click.ClickException(f"publish #{publish_id} not found")
        delta = _fetch_analytics(row["platform"], row["published_url"])
        conn.execute(
            """UPDATE publishes SET
                view_count = view_count + ?,
                like_count = like_count + ?,
                comment_count = comment_count + ?,
                completion_rate = ?
               WHERE id = ?""",
            (delta["view_count_delta"],
             delta["like_count_delta"],
             delta["comment_count_delta"],
             delta["completion_rate"],
             publish_id),
        )
    click.echo(f"✓ refreshed publish #{publish_id} (stub delta):")
    click.echo(f"  +{delta['view_count_delta']:>4} views")
    click.echo(f"  +{delta['like_count_delta']:>4} likes")
    click.echo(f"  +{delta['comment_count_delta']:>4} comments")
    click.echo(f"   completion_rate: {delta['completion_rate']:.1%}")


__all__ = ["cli"]