package snapshot_file_rotation_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"stageguard/internal/domain"
	"stageguard/internal/storage"
)

func TestSnapshotRotationDoesNotDetachFutureWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	dossier := domain.SafetyDossier{
		ID:          "rotation-dossier",
		ShowName:    "轮换前演出",
		Venue:       "一号剧场",
		ScheduledAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		EquipmentBoundary: []domain.Equipment{{
			ID: "lift-1", Name: "升降台", RatedLoadKg: 1000, IsolationBoundary: "主舞台东侧",
		}},
		Status:    domain.StatusDraft,
		Version:   1,
		CreatedBy: "工程师",
		UpdatedAt: time.Date(2030, 1, 1, 1, 0, 0, 0, time.UTC),
	}
	if _, err = store.CreateDossier(dossier, "工程师"); err != nil {
		t.Fatal(err)
	}

	oldSnapshot, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".rotated"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, oldSnapshot, 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err = store.Mutate(dossier.Version, dossier.ID, "工程师", "dossier.revised", "轮换后修订", func(s *domain.Snapshot) error {
		current := s.Dossiers[dossier.ID]
		current.ShowName = "轮换后演出"
		s.Dossiers[dossier.ID] = current
		return nil
	})
	if err != nil {
		t.Fatalf("轮换后的合法写入失败: %v", err)
	}

	reopened, err := storage.Open(path)
	if err != nil {
		t.Fatalf("从配置路径重启失败: %v", err)
	}
	got, ok := reopened.Snapshot().Dossiers[dossier.ID]
	if !ok || got.ShowName != "轮换后演出" {
		t.Fatalf("重启未恢复轮换后的写入: %#v", got)
	}
}
