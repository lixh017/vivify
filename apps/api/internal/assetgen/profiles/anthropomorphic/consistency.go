// Package anthropomorphic implements the consistency check for
// anthropomorphic animal IPs (panda, fox, etc.). The 7 anchors
// and their weights are defined in
// docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md
// §7.2.1.
package anthropomorphic

import (
	"strings"

	"github.com/opc/api/internal/assetgen"
)

// Check scores a prompt against the profile's 7 brand anchors.
// Pure function; no IO. Public wrapper around checkAnthropomorphic.
func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
	if p == nil {
		return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
	}
	ap, ok := p.(*Profile)
	if !ok {
		return assetgen.ConsistencyResult{
			Score: 0,
			Fails: []string{"profile is not *anthropomorphic.Profile"},
		}
	}
	return checkAnthropomorphic(ap, prompt)
}

func checkAnthropomorphic(p *Profile, prompt string) assetgen.ConsistencyResult {
	var score float64
	var fails []string

	add := func(needle, failName string, weight float64) {
		if strings.Contains(prompt, needle) {
			score += weight
		} else {
			fails = append(fails, failName)
		}
	}

	add(p.Name_, "no "+p.Name_+" name", 0.20)
	add(p.Species, "no panda species", 0.20)
	add(p.BodyColor, "no body white color", 0.20)
	add(p.EyeColor, "no eye black color", 0.20)
	add("国潮", "no guofeng theme", 0.10)
	add("不露爪", "no anti-claws", 0.05)
	add("不要 AI 生成感", "no anti-AI tokens", 0.05)

	return assetgen.ConsistencyResult{
		Score: assetgen.Round2Public(score),
		Fails: fails,
	}
}
