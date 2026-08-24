package domain

import (
	"fmt"
	"strings"
	"time"
)

func ValidateEvidence(items []Evidence, now time.Time) error {
	seen := map[string]bool{}
	for n, item := range items {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Reference) == "" || strings.TrimSpace(item.Digest) == "" {
			return fmt.Errorf("第%d条证据的名称、引用地址和内容摘要不能为空", n+1)
		}
		if item.CollectedAt.IsZero() || item.CollectedAt.After(now) {
			return fmt.Errorf("第%d条证据采集时间无效或晚于当前时间", n+1)
		}
		switch item.Type {
		case EvidencePhoto, EvidenceRetest, EvidenceDocument, EvidenceVideo:
		default:
			return fmt.Errorf("第%d条证据类型不受支持", n+1)
		}
		ref := strings.TrimSpace(item.Reference)
		if seen[ref] {
			return fmt.Errorf("证据引用重复: %s", ref)
		}
		seen[ref] = true
	}
	return nil
}

func RequiredEvidence(severity IssueSeverity, evidence []Evidence) []string {
	if severity != SeverityHigh && severity != SeverityCritical {
		return nil
	}
	found := map[string]bool{}
	for _, item := range evidence {
		found[item.Type] = true
	}
	missing := []string{}
	for _, kind := range []string{EvidencePhoto, EvidenceRetest} {
		if !found[kind] {
			missing = append(missing, kind)
		}
	}
	return missing
}
