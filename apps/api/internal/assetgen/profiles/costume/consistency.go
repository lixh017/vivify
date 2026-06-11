// Package costume implements the consistency check for costume
// IPs (古装 — historical period drama characters). The 7 anchors
// and their weights are defined in
// docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md
// §7.2.3.
package costume

import (
	"strings"

	"github.com/opc/api/internal/assetgen"
)

// Check scores a prompt against the profile's 7 brand anchors.
// Pure function; no IO. Public wrapper around checkCostume.
func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
	if p == nil {
		return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
	}
	cp, ok := p.(*Profile)
	if !ok {
		return assetgen.ConsistencyResult{
			Score: 0,
			Fails: []string{"profile is not *costume.Profile"},
		}
	}
	return checkCostume(cp, prompt)
}

func checkCostume(p *Profile, prompt string) assetgen.ConsistencyResult {
	var score float64
	var fails []string

	add := func(needle, failName string, weight float64) {
		if strings.Contains(prompt, needle) {
			score += weight
		} else {
			fails = append(fails, failName)
		}
	}

	add(p.Name_, "no character name", 0.15)
	add(p.Era, "no era", 0.20)
	add(p.Role, "no role", 0.10)

	layerFound := false
	for _, layer := range p.CostumeLayer {
		if strings.Contains(prompt, layer) {
			layerFound = true
			break
		}
	}
	if layerFound {
		score += 0.15
	} else {
		fails = append(fails, "no costume layer keyword")
	}

	propFound := false
	for _, prop := range p.PropKit {
		if strings.Contains(prompt, prop) {
			propFound = true
			break
		}
	}
	if propFound {
		score += 0.10
	} else {
		fails = append(fails, "no prop keyword")
	}

	add("无穿越", "no anti-anachronism", 0.15)

	colorFound := false
	for _, c := range p.Palette {
		if strings.Contains(prompt, c) {
			colorFound = true
			break
		}
	}
	if colorFound {
		score += 0.15
	} else {
		fails = append(fails, "no palette color")
	}

	return assetgen.ConsistencyResult{
		Score: assetgen.Round2Public(score),
		Fails: fails,
	}
}
