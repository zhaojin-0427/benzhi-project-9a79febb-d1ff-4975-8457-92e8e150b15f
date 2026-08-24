package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"stageguard/internal/domain"
	"testing"
	"time"
)

func TestPersistAndRestoreHashChain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d := domain.SafetyDossier{ID: "d1", ShowName: "测试", Venue: "剧场", ScheduledAt: time.Now(), EquipmentBoundary: []domain.Equipment{{ID: "e1", Name: "吊杆", RatedLoadKg: 100, IsolationBoundary: "后台"}}, Status: domain.StatusDraft, Version: 1, CreatedBy: "工程师", UpdatedAt: time.Now()}
	if _, err = s.CreateDossier(d, "工程师"); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snap domain.Snapshot
	if err = json.Unmarshal(b, &snap); err != nil {
		t.Fatal(err)
	}
	snap.Events[0].Detail = "被篡改"
	b, err = json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(path); err == nil {
		t.Fatal("损坏账本应拒绝加载")
	}
}
