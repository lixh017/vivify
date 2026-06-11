// Package info implements the consistency check for info IPs
// (资讯 — news/information host shows). The 6 anchors and their
// weights are defined in
// docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md
// §7.2.4.
package info

import (
	"strings"

	"github.com/opc/api/internal/assetgen"
)

// Check scores a prompt against the profile's 6 brand anchors.
// Pure function; no IO. Public wrapper around checkInfo.
func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
	if p == nil {
		return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
	}
	ip, ok := p.(*Profile)
	if !ok {
		return assetgen.ConsistencyResult{
			Score: 0,
			Fails: []string{"profile is not *info.Profile"},
		}
	}
	return checkInfo(ip, prompt)
}

func checkInfo(p *Profile, prompt string) assetgen.ConsistencyResult {
	var score float64
	var fails []string

	add := func(needle, failName string, weight float64) {
		if strings.Contains(prompt, needle) {
			score += weight
		} else {
			fails = append(fails, failName)
		}
	}

	add(p.HostPersona, "no host persona", 0.20)
	add(p.NewsroomStyle, "no newsroom style", 0.20)
	add(p.FontTone, "no font tone", 0.15)
	add(p.AccentColor, "no accent color", 0.15)
	add("无 AI 播报感", "no anti-AI-broadcast", 0.20)
	add(p.BackgroundColor, "no background color", 0.10)

	return assetgen.ConsistencyResult{
		Score: assetgen.Round2Public(score),
		Fails: fails,
	}
}
