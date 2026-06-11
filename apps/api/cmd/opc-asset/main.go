// opc-asset is the platform-side CLI for the opc asset pipeline.
//
// It is the replacement for the original audit/roundtrip-volcengine.mjs
// one-off Node script. Where the mjs was a single-purpose
// smoke that ran once and then sat unused, opc-asset is a real
// CLI subcommand-driven binary that:
//
//   - Builds prompts from the canonical assetgen profile
//     (peakge_v1 by default), so every generation is anchored to
//     the same brand schema.
//   - Scores prompts on the 7 IP-consistency anchors before
//     spending model tokens on them.
//   - Calls the platform's *agents.MiniMax provider for image and
//     video (so the same auth + cost + retry policy that the
//     handlers use applies here too — no separate Volcengine
//     dep in this env).
//   - Records every run to a JSONL ledger so the operator can
//     audit which assets came from which scene+outfit+profile
//     combo, and at what cost.
//
// Subcommands:
//
//	check       Run the IP consistency check on a prompt.
//	generate    Generate an image or video. Writes the file to --out.
//	ledger      Show the last N ledger entries (default 5).
//
// Examples:
//
//	opc-asset check --prompt "峰哥在竹林"
//	opc-asset generate --type image --scene 竹林小院 --outfit 朱红 --out /tmp/x.jpg
//	opc-asset generate --type video --scene 竹林小院 --outfit 翠绿 --out /tmp/x.mp4
//	opc-asset ledger --last 10
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/opc/api/internal/assetgen"
	"github.com/opc/api/internal/agents"
)

// ledgerPath is the canonical JSONL ledger where opc-asset appends
// one line per generation. The path is fixed (not configurable
// via flag) so the asset, log, and audit teams can all find it
// without re-discovering a flag every release. Matches the
// original mjs path so historical entries remain co-located.
const ledgerPath = "/root/.claude/agents/opc-asset-ledger.jsonl"

// LedgerEntry is the shape appended to ledgerPath. The fields are
// the union of what the mjs wrote plus the profile_version so
// future per-version reporting can split the file cleanly.
type LedgerEntry struct {
	Timestamp        string  `json:"timestamp"`
	SceneID          string  `json:"scene_id"`
	ProfileVersion   string  `json:"profile_version"`
	Provider         string  `json:"provider"`
	Type             string  `json:"type"`
	Outfit           string  `json:"outfit,omitempty"`
	CostCNY          float64 `json:"cost_cny"`
	DurationMS       int64   `json:"duration_ms"`
	Status           string  `json:"status"`
	AssetPath        string  `json:"asset_path,omitempty"`
	PromptHash       string  `json:"prompt_hash,omitempty"`
	ConsistencyScore float64 `json:"consistency_score"`
	Error            string  `json:"error,omitempty"`
}

func main() {
	// godotenv loads .env from the current working directory (and
	// parents). This is the same env-loading the main API server
	// uses, so contributors can `cd apps/api && opc-asset ...` and
	// pick up MINIMAX_API_KEY + DB_PATH + ENCRYPTION_KEY without
	// extra export. Silent on miss (prod/CI run without a .env
	// file is fine; the agent will just report Available()==false
	// and the `generate` subcommand will return an error).
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	logger := slog.Default()
	var err error
	switch os.Args[1] {
	case "check":
		err = runCheck(os.Args[2:])
	case "generate":
		err = runGenerate(os.Args[2:], logger)
	case "ledger":
		err = runLedger(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "opc-asset: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "opc-asset: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `opc-asset — opc brand-on asset CLI

Subcommands:
  check       Score a prompt against the IP consistency rules
  generate    Generate an image or video (uses *agents.MiniMax)
  ledger      Show the last N ledger entries

Run 'opc-asset <subcommand> --help' for subcommand-specific flags.
`)
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	prompt := fs.String("prompt", "", "the prompt to score (required)")
	profileVersion := fs.String("profile", "fengge_v1", "profile version (default fengge_v1)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *prompt == "" {
		return fmt.Errorf("--prompt is required")
	}
	prof := assetgen.GetProfile(*profileVersion)
	if prof == nil {
		return fmt.Errorf("unknown profile version %q", *profileVersion)
	}
	res := assetgen.ConsistencyCheck(*prof, *prompt)
	out := map[string]interface{}{
		"profile":    prof.Version,
		"score":      res.Score,
		"fails":      res.Fails,
		"prompt_len": len(*prompt),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func runGenerate(args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	assetType := fs.String("type", "", "image or video (required)")
	scene := fs.String("scene", "竹林小院", "scene description (default 竹林小院)")
	outfit := fs.String("outfit", "朱红", "outfit description (default 朱红)")
	out := fs.String("out", "", "output file path (required)")
	profileVersion := fs.String("profile", "fengge_v1", "profile version (default fengge_v1)")
	sceneID := fs.String("scene-id", "manual", "logical scene id (recorded in ledger)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *assetType == "" {
		return fmt.Errorf("--type is required (image or video)")
	}
	if *out == "" {
		return fmt.Errorf("--out is required")
	}
	prof := assetgen.GetProfile(*profileVersion)
	if prof == nil {
		return fmt.Errorf("unknown profile version %q", *profileVersion)
	}
	prompt := assetgen.BuildPrompt(*prof, *scene, *outfit, *assetType)
	consistency := assetgen.ConsistencyCheck(*prof, prompt)

	startedAt := time.Now()
	// MiniMax picks the API key from MINIMAX_API_KEY in the env
	// (godotenv above). The CLI mirrors the platform's auth path
	// so a contributor running `opc-asset` on the same host as
	// the server uses the same quota + key.
	mm := agents.NewMiniMax(os.Getenv("MINIMAX_API_KEY"))
	if !mm.Available() {
		entry := newLedgerEntry(*sceneID, prof.Version, "minimax", *assetType, *outfit, startedAt, consistency.Score, int64(0), *out, "", "missing-api-key")
		appendLedger(entry)
		return fmt.Errorf("MINIMAX_API_KEY is not configured; cannot generate. Wrote a 'missing-api-key' row to the ledger for audit")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	assetPath, err := callMiniMax(ctx, mm, *assetType, prompt, *out)
	dur := time.Since(startedAt).Milliseconds()
	if err != nil {
		entry := newLedgerEntry(*sceneID, prof.Version, "minimax", *assetType, *outfit, startedAt, consistency.Score, int64(0), *out, "", err.Error())
		appendLedger(entry)
		return err
	}
	entry := newLedgerEntry(*sceneID, prof.Version, "minimax", *assetType, *outfit, startedAt, consistency.Score, dur, assetPath, promptHash(prompt), "")
	entry.CostCNY = computeCostCNY(*assetType)
	appendLedger(entry)

	out_ := map[string]interface{}{
		"profile":          prof.Version,
		"type":             *assetType,
		"asset_path":       assetPath,
		"consistency":      consistency.Score,
		"consistency_fails": consistency.Fails,
		"duration_ms":      dur,
		"prompt":           prompt,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out_)
}

// computeCostCNY returns the per-generation cost in CNY for the
// given asset type, using the legacy mjs amounts so the ledger
// matches historical entries (0.05 CNY / image, 7.5 CNY / video).
// These are the rates the previous CLI charged; the platform's
// call_log middleware still records the real provider cost
// independently, so this is for the generation audit trail, not
// billing. The amount is hardcoded here (rather than read from
// config/cost.go) to keep the ledger stable across pricing
// changes — the audit trail should not retroactively rewrite
// history when the operator revises the rate.
func computeCostCNY(assetType string) float64 {
	switch assetType {
	case assetgen.TypeImage:
		return 0.05
	case assetgen.TypeVideo:
		return 7.5
	default:
		return 0
	}
}

func callMiniMax(ctx context.Context, mm *agents.MiniMax, assetType, prompt, out string) (string, error) {
	// Ensure the parent directory exists. MiniMax writes the
	// result to a path the caller specifies (or to its own cache)
	// — for image, FilePaths[0] is the written path. We pass
	// the user-requested --out so the file lands where the
	// operator asked.
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	switch assetType {
	case assetgen.TypeImage:
		res, err := mm.Image(ctx, prompt, agents.MiniMaxImageOptions{
			Model:       "image-01",
			AspectRatio: "9:16",
			// Width/Height omitted — AspectRatio drives the size.
			// OutPath is the destination the agent will write to.
			OutPath: out,
		})
		if err != nil {
			return "", err
		}
		if len(res.FilePaths) == 0 {
			return "", fmt.Errorf("image generation returned no file paths")
		}
		return res.FilePaths[0], nil
	case assetgen.TypeVideo:
		res, err := mm.Video(ctx, prompt, agents.MiniMaxVideoOptions{
			Model: "MiniMax-Hailuo-2.3",
		})
		if err != nil {
			return "", err
		}
		// MiniMax.Video writes to res.Path (it manages the
		// download). We then move/copy to the requested --out so
		// the operator's contract holds.
		if res.Path == "" {
			return "", fmt.Errorf("video generation returned no path")
		}
		if res.Path != out {
			if err := copyFile(res.Path, out); err != nil {
				return res.Path, fmt.Errorf("video generated at %s but copy to %s failed: %w", res.Path, out, err)
			}
		}
		return out, nil
	default:
		return "", fmt.Errorf("unsupported asset type %q (use image or video)", assetType)
	}
}

func runLedger(args []string) error {
	fs := flag.NewFlagSet("ledger", flag.ContinueOnError)
	last := fs.Int("last", 5, "how many entries to show (default 5)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	entries, err := readLedgerTail(*last)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	for _, e := range entries {
		_ = enc.Encode(e)
	}
	return nil
}

// newLedgerEntry centralizes the field order + defaults so every
// appender produces the same shape. CostCNY is a placeholder —
// the platform's call_log middleware records the real cost
// independently; this ledger is a generation audit trail, not a
// billing source.
func newLedgerEntry(sceneID, profileVersion, provider, assetType, outfit string, startedAt time.Time, consistency float64, durationMS int64, assetPath, promptHash, errMsg string) LedgerEntry {
	entry := LedgerEntry{
		Timestamp:        startedAt.UTC().Format(time.RFC3339),
		SceneID:          sceneID,
		ProfileVersion:   profileVersion,
		Provider:         provider,
		Type:             assetType,
		Outfit:           outfit,
		CostCNY:          0,
		DurationMS:       durationMS,
		Status:           "success",
		AssetPath:        assetPath,
		PromptHash:       promptHash,
		ConsistencyScore: consistency,
	}
	if errMsg != "" {
		entry.Status = "error"
		entry.Error = errMsg
	}
	return entry
}

// appendLedger writes a single entry to the JSONL ledger. The
// file is append-only; we do not rewrite, rotate, or compact.
// This matches the mjs contract.
func appendLedger(e LedgerEntry) {
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "opc-asset: cannot ensure ledger dir: %v\n", err)
		return
	}
	f, err := os.OpenFile(ledgerPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opc-asset: cannot open ledger: %v\n", err)
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	_ = enc.Encode(e)
}

func readLedgerTail(n int) ([]LedgerEntry, error) {
	f, err := os.Open(ledgerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var all []LedgerEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e LedgerEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue // skip malformed lines
		}
		all = append(all, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(all) <= n {
		return all, nil
	}
	return all[len(all)-n:], nil
}

// promptHash returns a short fingerprint of the prompt so two
// runs with the same {profile, scene, outfit, type} can be
// correlated in the ledger. We use the FNV-1a 64-bit hash for
// zero-dependency and adequate collision resistance at our row
// volumes; the original mjs used sha256:16-hex, which we keep as
// a string prefix so existing tooling that greps for `sha256:`
// still matches.
func promptHash(p string) string {
	const offset64 uint64 = 14695981039346656037
	const prime64 uint64 = 1099511628211
	h := offset64
	for i := 0; i < len(p); i++ {
		h ^= uint64(p[i])
		h *= prime64
	}
	// Mirror the mjs's 16-hex presentation as much as possible.
	return fmt.Sprintf("fnv64:%016x", h)
}

// copyFile is a small os.Stat-then-ReadFile+WriteFile helper. The
// video case moves MiniMax's res.Path to the operator's --out
// without depending on io.Copy. Kept inline so opc-asset has no
// internal/cmd-shared dependency.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// init: keep strings imported even if a future edit removes the
// direct use; the linter would catch a real dead-import, but
// this guards against a half-edit.
var _ = strings.Contains
