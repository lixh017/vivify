package assetgen

import (
	"math"
	"strings"
)

// ConsistencyResult is the output of ConsistencyCheck. Score is in
// [0, 1]; Fails lists the human-readable names of the anchors the
// prompt is missing, in the order they were checked (most
// brand-critical first).
type ConsistencyResult struct {
	Score float64
	Fails []string
}

// ConsistencyCheck scores a prompt against the profile's brand
// anchors. The same rules apply to both image and video prompts;
// the score is brand-only, not generation quality.
//
// Anchors and their weights (sum to 1.0):
//   - name           (peakge name "峰哥")            0.20
//   - species        ("成年熊猫")                     0.20
//   - body white     ("rgb(245,240,225)")            0.20
//   - eye black      ("rgb(26,26,26)")               0.20
//   - theme          ("国潮")                         0.10
//   - no claws       ("不露爪")                       0.05
//   - no AI look     ("不要 AI 生成感")               0.05
//
// Score is rounded to 2 decimals to match the JSONL ledger format
// the original mjs used.
func ConsistencyCheck(p Profile, prompt string) ConsistencyResult {
	var score float64
	var fails []string

	check := func(needle, failName string, weight float64) {
		if strings.Contains(prompt, needle) {
			score += weight
		} else {
			fails = append(fails, failName)
		}
	}

	check(p.Name, "no "+p.Name+" name", 0.20)
	check(p.Species, "no panda species", 0.20)
	check(p.Colors.BodyWhite, "no body white color", 0.20)
	check(p.Colors.EyeBlack, "no eye black color", 0.20)
	check("国潮", "no guofeng theme", 0.10)
	check("不露爪", "no anti-claws", 0.05)
	check("不要 AI 生成感", "no anti-AI tokens", 0.05)

	return ConsistencyResult{
		Score: math.Round(score*100) / 100,
		Fails: fails,
	}
}
