package agents

import (
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
)

// writeBase64File decodes b64 and writes the bytes to path.
// Creates parent directories as needed.
func writeBase64File(path, b64 string, mode os.FileMode) error {
	if path == "" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		// Try URL-safe without padding as a fallback.
		if raw, err = base64.URLEncoding.DecodeString(b64); err != nil {
			return err
		}
	}
	return os.WriteFile(path, raw, mode)
}

// hexDecode decodes a hex string to bytes. The MiniMax speech
// endpoint returns audio as a hex string (not base64) — this
// helper is the inverse. Lenient about whitespace and case.
func hexDecode(s string) ([]byte, error) {
	cleaned := ""
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		cleaned += string(r)
	}
	return hex.DecodeString(cleaned)
}

