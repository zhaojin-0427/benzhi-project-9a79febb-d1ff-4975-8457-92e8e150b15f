package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"stageguard/internal/domain"
)

func verifyLedger(path string, events []domain.AuditEvent) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取审计账本: %w", err)
	}
	lines := bytes.Split(bytes.TrimSpace(b), []byte{'\n'})
	if len(events) == 0 && len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if len(lines) != len(events) {
		return errors.New("审计账本与快照事件数量不一致")
	}
	for i, line := range lines {
		var event domain.AuditEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("解析审计账本第%d行: %w", i+1, err)
		}
		if event.ID != events[i].ID || event.Hash != events[i].Hash {
			return fmt.Errorf("审计账本第%d行与快照不一致", i+1)
		}
	}
	return verifyEvents(events)
}

func verifyEvents(events []domain.AuditEvent) error {
	prev := ""
	for _, e := range events {
		if e.PrevHash != prev || e.Hash != domain.HashEvent(e) {
			return errors.New("审计哈希链校验失败")
		}
		prev = e.Hash
	}
	return nil
}
