package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"stageguard/internal/domain"
	"time"
)

func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func createDossierFingerprint(c CreateDossierCommand) string {
	return fingerprint(struct {
		ShowName, Venue, CreatedBy string
		ScheduledAt                time.Time
		EquipmentBoundary          []domain.Equipment
	}{c.ShowName, c.Venue, c.CreatedBy, c.ScheduledAt, c.EquipmentBoundary})
}
