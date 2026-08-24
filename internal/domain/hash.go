package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

func HashSnapshot(s Snapshot) string {
	dossiers := make(map[string]SafetyDossier, len(s.Dossiers))
	for id, dossier := range s.Dossiers {
		dossier.FrozenHash = ""
		dossiers[id] = dossier
	}
	b, _ := json.Marshal(struct {
		D map[string]SafetyDossier    `json:"d"`
		I map[string]InspectionItem   `json:"i"`
		X map[string]SafetyIssue      `json:"x"`
		P map[string]ActivationPermit `json:"p"`
	}{dossiers, s.Inspections, s.Issues, s.Permits})
	return hashBytes(b)
}

func HashDossierContent(s Snapshot, dossierID string, frozenVersion int) string {
	d := s.Dossiers[dossierID]
	d.Status = StatusFrozen
	d.Version = frozenVersion
	d.FrozenHash = ""
	d.UpdatedAt = time.Time{}
	inspections := map[string]InspectionItem{}
	issues := map[string]SafetyIssue{}
	for id, item := range s.Inspections {
		if item.DossierID == dossierID {
			inspections[id] = item
		}
	}
	for id, issue := range s.Issues {
		if issue.DossierID == dossierID {
			issues[id] = issue
		}
	}
	b, _ := json.Marshal(struct {
		D SafetyDossier             `json:"d"`
		I map[string]InspectionItem `json:"i"`
		X map[string]SafetyIssue    `json:"x"`
	}{d, inspections, issues})
	return hashBytes(b)
}

func HashPersistedSnapshot(s Snapshot) string {
	s.IntegrityHash = ""
	b, _ := json.Marshal(s)
	return hashBytes(b)
}

func HashEvent(e AuditEvent) string {
	return hashBytes([]byte(fmt.Sprintf("%s|%s|%s|%s|%s|%d|%s|%s", e.ID, e.DossierID, e.Type, e.Actor, e.At.UTC().Format(time.RFC3339Nano), e.Version, e.Detail, e.PrevHash)))
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
