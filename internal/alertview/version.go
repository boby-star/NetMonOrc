package alertview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Version fingerprints the complete public representation for a strong ETag.
func Version(v Alert) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Cycle identifies the firing represented by this row; a later firing differs.
func Cycle(v Alert) string {
	data, _ := json.Marshal(struct {
		ID      string
		FiredAt *time.Time
	}{v.ID, v.FiredAt})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
