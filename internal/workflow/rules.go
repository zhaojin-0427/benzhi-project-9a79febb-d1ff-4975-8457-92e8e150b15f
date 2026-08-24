package workflow

import "stageguard/internal/domain"

type issueHit struct {
	item    domain.InspectionItem
	finding finding
}

type finding struct {
	severity          domain.IssueSeverity
	code, description string
}

func rulesFor(in domain.InspectionItem, d domain.SafetyDossier) []finding {
	rated := 0.0
	for _, e := range d.EquipmentBoundary {
		if e.ID == in.EquipmentID {
			rated = e.RatedLoadKg
		}
	}
	switch in.CheckCode {
	case "LOAD_LIMIT":
		if in.MeasuredLoadKg > rated {
			return []finding{{domain.SeverityCritical, "LOAD_LIMIT", "实测载荷超过设备额定载荷"}}
		}
	case "LIMIT_RESPONSE":
		if in.LimitResponseMs > 500 {
			return []finding{{domain.SeverityHigh, "LIMIT_RESPONSE", "限位响应超过500毫秒"}}
		}
	case "EMERGENCY_STOP":
		if in.EmergencyStopResult == "失败" || in.Result == "不通过" {
			return []finding{{domain.SeverityCritical, "EMERGENCY_STOP", "急停测试未通过"}}
		}
	}
	return nil
}
