package workflow

import (
	"errors"

	"stageguard/internal/domain"
)

type MatrixCell struct {
	EquipmentID string                 `json:"equipmentID"`
	Check       domain.CheckDefinition `json:"check"`
	Inspection  *domain.InspectionItem `json:"inspection,omitempty"`
	Missing     bool                   `json:"missing"`
}

type InspectionMatrix struct {
	Required        int          `json:"required"`
	Completed       int          `json:"completed"`
	Passed          int          `json:"passed"`
	Failed          int          `json:"failed"`
	Missing         int          `json:"missing"`
	CompletionRatio float64      `json:"completionRatio"`
	Cells           []MatrixCell `json:"cells"`
	MissingItems    []MatrixCell `json:"missingItems"`
}

func MatrixFor(st domain.Snapshot, dossierID string) (InspectionMatrix, error) {
	d, ok := st.Dossiers[dossierID]
	if !ok {
		return InspectionMatrix{}, errors.New("档案不存在")
	}
	byKey := map[string]domain.InspectionItem{}
	for _, item := range st.Inspections {
		if item.DossierID == dossierID {
			byKey[domain.InspectionKey(dossierID, item.EquipmentID, item.CheckCode)] = item
		}
	}
	m := InspectionMatrix{Required: len(d.EquipmentBoundary) * len(domain.InspectionTemplate())}
	for _, e := range d.EquipmentBoundary {
		for _, check := range domain.InspectionTemplate() {
			cell := MatrixCell{EquipmentID: e.ID, Check: check}
			item, found := byKey[domain.InspectionKey(dossierID, e.ID, check.Code)]
			if found {
				copy := item
				cell.Inspection = &copy
				m.Completed++
				if item.Result == "通过" {
					m.Passed++
				} else {
					m.Failed++
				}
			} else {
				cell.Missing = true
				m.Missing++
				m.MissingItems = append(m.MissingItems, cell)
			}
			m.Cells = append(m.Cells, cell)
		}
	}
	if m.Required > 0 {
		m.CompletionRatio = float64(m.Completed) / float64(m.Required)
	}
	return m, nil
}
