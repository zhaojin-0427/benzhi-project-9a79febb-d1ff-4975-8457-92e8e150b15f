package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func ValidateEquipment(e Equipment) error {
	if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Name) == "" {
		return errors.New("设备编号和名称不能为空")
	}
	if e.RatedLoadKg <= 0 || e.RatedLoadKg > 100000 {
		return errors.New("额定载荷必须在0到100000千克之间")
	}
	if strings.TrimSpace(e.IsolationBoundary) == "" {
		return errors.New("隔离边界不能为空")
	}
	return nil
}

func ValidateDossierInput(show, venue, creator string, scheduled time.Time, equipment []Equipment) error {
	if strings.TrimSpace(show) == "" || strings.TrimSpace(venue) == "" || strings.TrimSpace(creator) == "" {
		return errors.New("演出、场地和创建人不能为空")
	}
	if scheduled.IsZero() {
		return errors.New("演出时间不能为空")
	}
	if len(equipment) == 0 {
		return errors.New("至少登记一台舞台机械")
	}
	seen := map[string]bool{}
	for _, e := range equipment {
		if err := ValidateEquipment(e); err != nil {
			return err
		}
		if seen[e.ID] {
			return fmt.Errorf("设备编号重复: %s", e.ID)
		}
		seen[e.ID] = true
	}
	return nil
}

func ValidateInspection(i InspectionItem) error {
	if strings.TrimSpace(i.DossierID) == "" || strings.TrimSpace(i.EquipmentID) == "" || strings.TrimSpace(i.CheckCode) == "" || strings.TrimSpace(i.Inspector) == "" {
		return errors.New("检查档案、设备、检查项和检查人不能为空")
	}
	if i.Result != "通过" && i.Result != "不通过" {
		return errors.New("检查结果只能是通过或不通过")
	}
	if i.MeasuredLoadKg < 0 || i.LimitResponseMs < 0 {
		return errors.New("测量值不能为负数")
	}
	if !ValidCheckCode(i.CheckCode) {
		return errors.New("检查项不属于安全检查模板")
	}
	switch i.CheckCode {
	case "LOAD_LIMIT":
		if i.MeasuredLoadKg <= 0 || i.MeasuredLoadKg > 100000 {
			return errors.New("载荷检查要求0到100000千克之间的有效实测载荷")
		}
	case "LIMIT_RESPONSE":
		if i.LimitResponseMs <= 0 || i.LimitResponseMs > 600000 {
			return errors.New("限位检查要求1到600000毫秒之间的响应时间")
		}
	case "EMERGENCY_STOP":
		if i.EmergencyStopResult != "通过" && i.EmergencyStopResult != "失败" {
			return errors.New("急停检查要求明确的通过或失败动作结果")
		}
	}
	return nil
}
