package domain

type CheckDefinition struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Unit        string `json:"unit,omitempty"`
}

func InspectionTemplate() []CheckDefinition {
	return []CheckDefinition{
		{Code: "LOAD_LIMIT", Name: "额定载荷", Description: "核对实测载荷不得超过设备额定载荷", Unit: "kg"},
		{Code: "LIMIT_RESPONSE", Name: "限位响应", Description: "测量限位保护动作响应时间", Unit: "ms"},
		{Code: "EMERGENCY_STOP", Name: "急停功能", Description: "验证急停装置动作及复位状态"},
	}
}

func ValidCheckCode(code string) bool {
	for _, item := range InspectionTemplate() {
		if item.Code == code {
			return true
		}
	}
	return false
}
