package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
