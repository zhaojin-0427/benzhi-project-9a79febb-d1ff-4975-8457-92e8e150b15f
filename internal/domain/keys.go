package domain

import (
	"fmt"
	"strings"
	"time"
)

func InspectionKey(dossierID, equipmentID, checkCode string) string {
	return dossierID + "|" + equipmentID + "|" + checkCode
}

func IssueKey(dossierID, equipmentID, checkCode, ruleCode string) string {
	return dossierID + "|" + equipmentID + "|" + checkCode + "|" + ruleCode
}

func DossierSummary(d SafetyDossier) string {
	parts := make([]string, 0, len(d.EquipmentBoundary))
	for _, e := range d.EquipmentBoundary {
		parts = append(parts, fmt.Sprintf("%s:%s:%.2fkg:%s", e.ID, e.Name, e.RatedLoadKg, e.IsolationBoundary))
	}
	return fmt.Sprintf("演出=%s；场地=%s；时间=%s；设备=[%s]", d.ShowName, d.Venue, d.ScheduledAt.UTC().Format(time.RFC3339), strings.Join(parts, ","))
}
