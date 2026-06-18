# Releasing opc-panda-plugin

This document describes how to release a new version of opc-panda-plugin. Release management is split into two parts:

1. **The plugin repo** (this repo, `opc-panda-plugin`) — source + manifest + scripts
2. **The opc-mcp binary** (built from the main opc repo) — uploaded as a GitHub Release asset

## Versioning

We follow semver:
- `v0.1.0` — first public preview (initial plugin manifest)
- `v0.2.0` — multi-agent install (Codex/Cursor/Windsurf support) + openclaw TS shim
- `v1.0.0` — production release with full docs, CI, and tested install on all 5 agents

## Release process (TL;DR)

```bash
# 1. Make sure plugin repo is pushed
cd /path/to/opc-panda-plugin
git push origin main

# 2. Build opc-mcp + upload to GitHub Releases
./scripts/release.sh v0.1.0
# (auto-builds from /root/workspace/opc, uploads, tags)

# 3. Verify install URL works
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/scripts/install.sh | bash
```

## Detailed steps

### 1. Build opc-mcp binary

The opc-mcp binary is built from the main opc monorepo at `apps/api/cmd/mcp-server/`. The plugin's `release.sh` does this automatically:

```bash
cd /path/to/opc-panda-plugin
./scripts/release.sh v0.1.0
```

This:
- Goes to `$OPC_REPO/apps/api` (default: `/root/workspace/opc`)
- Runs `go build -tags fts5 -trimpath -ldflags="-s -w" -o /tmp/opc-mcp-XXXXX ./cmd/mcp-server`
- Uploads the resulting binary to GitHub Releases as `opc-mcp`

To use a pre-built binary instead of building:

```bash
./scripts/release.sh v0.1.0 /path/to/opc-mcp
```

### 2. Verify the release

```bash
# Check the release exists
gh release list

# Check the binary downloads
curl -fsSL -o /tmp/test-opc-mcp https://github.com/<owner>/<repo>/releases/latest/download/opc-mcp
chmod +x /tmp/test-opc-mcp
DB_PATH=/tmp/test.db MINIMAX_API_KEY= /tmp/test-opc-mcp &  # should print "opc-mcp ready"
kill %1
```

### 3. Test the install script end-to-end

```bash
# Test on a fresh container
docker run --rm -it ubuntu:latest bash -c "
  curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/scripts/install.sh | bash
"
# (requires ARK_API_KEY + MINIMAX_API_KEY to be set in the env)
```

## Install path reference (for users)

After release, the user-facing install is always:

```bash
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/scripts/install.sh | bash
```

This single command:
1. Downloads the latest `opc-mcp` binary from GitHub Releases
2. Detects which agent is installed (Claude Code, Codex, Cursor, Windsurf, openclaw)
3. Writes the correct MCP config
4. Copies the 2 skills
5. Asks the user to restart their agent

## License

MIT
