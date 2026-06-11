package agents

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// CoverRequest is the user-facing input to the cover-image
// pipeline. All fields except Title are optional; missing
// fields are filled in by sensible defaults (style, platform)
// or ignored (topic_id is only used as a cache key hint).
//
// Kept as a shared type because the handler builds the prompt
// directly from the request and the demo-path SVG fallback
// reads the same fields.
type CoverRequest struct {
	Title    string `json:"title"`
	Style    string `json:"style,omitempty"`
	Platform string `json:"platform,omitempty"`
	TopicID  uint   `json:"topic_id,omitempty"`
}

// BuildCoverPrompt turns a CoverRequest into a stable Chinese
// prompt MiniMax can render against. We do NOT echo the title
// verbatim — providers do a better job when the prompt carries
// the IP voice + style + format intent in addition to the
// subject. Missing style/platform fall back to canonical
// defaults so a half-filled request still produces a sensible
// image.
func BuildCoverPrompt(req CoverRequest) string {
	style := strings.TrimSpace(req.Style)
	if style == "" {
		style = "治愈系"
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "抖音"
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "熊猫"
	}
	// Aspect ratio hint by platform. 抖音/小红书 are 9:16
	// vertical, 哔哩哔哩 is 16:9 widescreen. We embed the hint
	// in the prompt because the providers differ in how they
	// accept ratio params and a text hint works on both.
	ratioHint := "vertical 9:16"
	switch platform {
	case "哔哩哔哩", "YouTube":
		ratioHint = "widescreen 16:9"
	}
	return fmt.Sprintf(
		"%s 风格封面,主体:%s,平台:%s,画幅:%s,高质量,精细插画",
		style, title, platform, ratioHint,
	)
}

// MockCoverImageURL returns a deterministic data: URL that
// renders as a tiny SVG with the topic title burned in. The
// data: URL is preferred over an external placeholder because
// it requires no outbound network from the API server and the
// frontend can render it in <img> tags as-is.
//
// The URL embeds a hash of (title+style+platform) so two
// callers asking for the same input get a stable image — the
// demo flow caches nicely, and the frontend can show "we
// would have asked the model for ..." without round-tripping
// the same SVG twice.
func MockCoverImageURL(req CoverRequest) string {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "未命名选题"
	}
	style := strings.TrimSpace(req.Style)
	if style == "" {
		style = "治愈系"
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "通用"
	}

	h := sha256.Sum256([]byte(title + "|" + style + "|" + platform))
	digest := hex.EncodeToString(h[:8])
	// Color is derived from the digest so the placeholder is
	// visually distinct per topic. We stick to a #RRGGBB pair
	// because not every <img>-style renderer supports oklch.
	r := h[0]
	g := h[1]
	b := h[2]
	bg := fmt.Sprintf("#%02x%02x%02x", r, g, b)

	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="720" height="1280" viewBox="0 0 720 1280">`+
			`<rect width="720" height="1280" fill="%s"/>`+
			`<text x="40" y="120" font-family="sans-serif" font-size="48" fill="#fff" font-weight="bold">%s · %s</text>`+
			`<text x="40" y="200" font-family="sans-serif" font-size="36" fill="#fff">%s</text>`+
			`<text x="40" y="1240" font-family="sans-serif" font-size="24" fill="#fff" opacity="0.7">mock:%s · set MINIMAX_API_KEY for real images</text>`+
			`</svg>`,
		bg, platform, style, escapeXML(title), digest,
	)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

// escapeXML escapes the four characters that would otherwise
// break an SVG text element. title is user-supplied so we
// cannot trust it to be entity-safe.
func escapeXML(s string) string {
	r := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	)
	return r.Replace(s)
}
