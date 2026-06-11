// Package digital_human implements the consistency check for
// digital human IPs (virtual anchors, virtual idols). The 8
// anchors and their weights are defined in
// docs/superpowers/specs/2026-06-11-opc-phase2-ip-template-design.md
// §7.2.2.
package digital_human

import (
	"fmt"
	"strings"

	"github.com/opc/api/internal/assetgen"
)

// Check scores a prompt against the profile's 8 brand anchors.
// Pure function; no IO. Public wrapper around checkDigitalHuman.
func Check(p assetgen.Profile, prompt string) assetgen.ConsistencyResult {
	if p == nil {
		return assetgen.ConsistencyResult{Score: 0, Fails: []string{"nil profile"}}
	}
	dh, ok := p.(*Profile)
	if !ok {
		return assetgen.ConsistencyResult{
			Score: 0,
			Fails: []string{"profile is not *digital_human.Profile"},
		}
	}
	return checkDigitalHuman(dh, prompt)
}

func checkDigitalHuman(p *Profile, prompt string) assetgen.ConsistencyResult {
	var score float64
	var fails []string

	add := func(needle, failName string, weight float64) {
		if strings.Contains(prompt, needle) {
			score += weight
		} else {
			fails = append(fails, failName)
		}
	}

	add(p.Name_, "no host name", 0.15)
	add(fmt.Sprintf("%d 岁", p.Age), "no age", 0.10)
	add(p.Gender, "no gender", 0.10)
	add(p.Ethnicity, "no ethnicity", 0.10)
	add(p.SkinTone, "no skin tone", 0.15)
	add(p.VoiceTone, "no voice tone", 0.10)
	add("表情自然", "no natural expression", 0.15)
	add("无恐怖谷", "no anti-uncanny-valley", 0.15)

	return assetgen.ConsistencyResult{
		Score: assetgen.Round2Public(score),
		Fails: fails,
	}
}
