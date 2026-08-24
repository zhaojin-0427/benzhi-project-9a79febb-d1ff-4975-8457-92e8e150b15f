package workflow

import (
	"path/filepath"
	"stageguard/internal/domain"
	"stageguard/internal/storage"
	"testing"
	"time"
)

func TestEndToEndPermit(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(st)
	d, err := s.CreateDossier(CreateDossierCommand{ShowName: "测试演出", Venue: "剧场", CreatedBy: "工程师", ScheduledAt: time.Now().Add(time.Hour), EquipmentBoundary: []domain.Equipment{{ID: "e1", Name: "吊杆", RatedLoadKg: 100, IsolationBoundary: "后台"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordInspectionBatch(BatchInspectionCommand{DossierID: d.ID, Actor: "工程师", ExpectedVersion: d.Version, Items: []InspectionCommand{
		{EquipmentID: "e1", CheckCode: "LOAD_LIMIT", MeasuredLoadKg: 120, ObservedValue: "120kg", Result: "通过", Inspector: "工程师"},
		{EquipmentID: "e1", CheckCode: "LIMIT_RESPONSE", LimitResponseMs: 200, Result: "通过", Inspector: "工程师"},
		{EquipmentID: "e1", CheckCode: "EMERGENCY_STOP", EmergencyStopResult: "通过", Result: "通过", Inspector: "工程师"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	d = s.Snapshot().Dossiers[d.ID]
	issues, err := s.DetectIssues(d.ID, "工程师", d.Version)
	if err != nil || len(issues) != 1 {
		t.Fatalf("检测问题: %v, %d", err, len(issues))
	}
	d = s.Snapshot().Dossiers[d.ID]
	now := time.Now().UTC()
	evidence := []domain.Evidence{{Name: "现场照片", Type: domain.EvidencePhoto, CollectedAt: now, Reference: "photo://1", Digest: "整改现场"}, {Name: "复测记录", Type: domain.EvidenceRetest, CollectedAt: now, Reference: "retest://1", Digest: "90kg"}}
	if _, err = s.SubmitRemediation(RemediationCommand{IssueID: issues[0].ID, Remediation: "降低载荷", RetestData: "90kg", Evidence: evidence, Actor: "工程师", ExpectedVersion: d.Version}); err != nil {
		t.Fatal(err)
	}
	d = s.Snapshot().Dossiers[d.ID]
	if _, err = s.ReviewIssue(issues[0].ID, "安全员", "通过", "复核通过", d.Version); err != nil {
		t.Fatal(err)
	}
	d = s.Snapshot().Dossiers[d.ID]
	d, err = s.Freeze(d.ID, "安全员", d.Version)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.IssuePermit(d.ID, "值班经理", d.Version)
	if err != nil {
		t.Fatal(err)
	}
	if p.ContentHash == "" || s.Snapshot().Dossiers[d.ID].Status != domain.StatusIssued {
		t.Fatal("许可未签发")
	}
	verification, err := s.VerifyPermit(p.PermitCode)
	if err != nil || !verification.Valid {
		t.Fatalf("许可核验失败: %+v, %v", verification, err)
	}
}

func TestRevisionAndBatchAreAtomic(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := New(st)
	d, err := s.CreateDossier(CreateDossierCommand{ShowName: "初版", Venue: "一号厅", CreatedBy: "工程师", ScheduledAt: time.Now().Add(time.Hour), EquipmentBoundary: []domain.Equipment{{ID: "e1", Name: "吊杆", RatedLoadKg: 100, IsolationBoundary: "A区"}}})
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.ReviseDossier(ReviseDossierCommand{DossierID: d.ID, ShowName: "修订版", Venue: "二号厅", Actor: "工程师", ScheduledAt: d.ScheduledAt, EquipmentBoundary: []domain.Equipment{{ID: "e1", Name: "吊杆", RatedLoadKg: 120, IsolationBoundary: "B区"}}, ExpectedVersion: d.Version})
	if err != nil || d.Version != 2 || len(d.Revisions) != 1 {
		t.Fatalf("修订失败: %+v %v", d, err)
	}
	beforeEvents := len(s.Snapshot().Events)
	_, err = s.RecordInspectionBatch(BatchInspectionCommand{DossierID: d.ID, Actor: "工程师", ExpectedVersion: d.Version, Items: []InspectionCommand{{EquipmentID: "e1", CheckCode: "LOAD_LIMIT", MeasuredLoadKg: 100, Result: "通过", Inspector: "工程师"}, {EquipmentID: "outside", CheckCode: "LIMIT_RESPONSE", LimitResponseMs: 100, Result: "通过", Inspector: "工程师"}}})
	if err == nil {
		t.Fatal("无效批次未被拒绝")
	}
	snap := s.Snapshot()
	if len(snap.Inspections) != 0 || snap.Dossiers[d.ID].Version != d.Version || len(snap.Events) != beforeEvents {
		t.Fatal("无效批次产生了部分写入")
	}
	items := []InspectionCommand{{EquipmentID: "e1", CheckCode: "LOAD_LIMIT", MeasuredLoadKg: 100, Result: "通过", Inspector: "工程师"}, {EquipmentID: "e1", CheckCode: "LIMIT_RESPONSE", LimitResponseMs: 100, Result: "通过", Inspector: "工程师"}, {EquipmentID: "e1", CheckCode: "EMERGENCY_STOP", EmergencyStopResult: "通过", Result: "通过", Inspector: "工程师"}}
	result, err := s.RecordInspectionBatch(BatchInspectionCommand{DossierID: d.ID, Actor: "工程师", IdempotencyKey: "batch-1", ExpectedVersion: d.Version, Items: items})
	if err != nil || result.Added != 3 {
		t.Fatalf("合法批次失败: %+v %v", result, err)
	}
	replay, err := s.RecordInspectionBatch(BatchInspectionCommand{DossierID: d.ID, Actor: "工程师", IdempotencyKey: "batch-1", ExpectedVersion: d.Version, Items: items})
	if err != nil || !replay.IdempotentReplay || s.Snapshot().Dossiers[d.ID].Version != result.Version {
		t.Fatal("幂等重试重复变更")
	}
}
