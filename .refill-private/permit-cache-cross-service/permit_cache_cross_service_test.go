package permit_cache_cross_service_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"stageguard/internal/domain"
	"stageguard/internal/httpapi"
	"stageguard/internal/storage"
	"stageguard/internal/workflow"
)

const sharedPermitCode = "SG-CACHE-ISOLATION-2026"

type permitResponse struct {
	Permit  domain.ActivationPermit `json:"permit"`
	Dossier domain.SafetyDossier    `json:"dossier"`
}

func TestPermitLookupCacheIsIsolatedAcrossServices(t *testing.T) {
	firstService, firstDossier := serviceWithPermit(t, "first", "第一账本值班经理")
	secondService, secondDossier := serviceWithPermit(t, "second", "第二账本值班经理")

	first := getPermit(t, firstService)
	if first.Permit.DossierID != firstDossier {
		t.Fatalf("第一服务返回了错误许可: got %q want %q", first.Permit.DossierID, firstDossier)
	}

	second := getPermit(t, secondService)
	if second.Permit.DossierID != secondDossier || second.Dossier.ID != secondDossier {
		t.Fatalf("第二服务受到第一服务缓存污染: permit dossier %q, response dossier %q, want %q", second.Permit.DossierID, second.Dossier.ID, secondDossier)
	}
	if second.Permit.IssuedBy != "第二账本值班经理" {
		t.Fatalf("第二服务泄漏了第一服务许可: issuedBy %q", second.Permit.IssuedBy)
	}
}

func serviceWithPermit(t *testing.T, suffix, issuer string) (*workflow.Service, string) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	dossierID := "dossier-" + suffix
	dossier := domain.SafetyDossier{
		ID:                dossierID,
		ShowName:          "缓存隔离演出-" + suffix,
		Venue:             "独立剧场-" + suffix,
		ScheduledAt:       time.Date(2026, 8, 26, 20, 0, 0, 0, time.UTC),
		EquipmentBoundary: []domain.Equipment{{ID: "hoist-" + suffix, Name: "升降台", RatedLoadKg: 1000, IsolationBoundary: "隔离区"}},
		Status:            domain.StatusFrozen,
		Version:           1,
		CreatedBy:         "工程师",
		UpdatedAt:         time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC),
		FrozenHash:        "frozen-hash-" + suffix,
	}
	if _, err = store.CreateDossier(dossier, "工程师"); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.Mutate(1, dossierID, issuer, "permit.seeded", "为缓存隔离测试签发许可", func(snapshot *domain.Snapshot) error {
		permit := domain.ActivationPermit{
			ID:            "permit-" + suffix,
			DossierID:     dossierID,
			FrozenVersion: 1,
			IssuedBy:      issuer,
			IssuedAt:      time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC),
			ContentHash:   "frozen-hash-" + suffix,
			PermitCode:    sharedPermitCode,
		}
		snapshot.Permits[permit.ID] = permit
		current := snapshot.Dossiers[dossierID]
		current.Status = domain.StatusIssued
		snapshot.Dossiers[dossierID] = current
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return workflow.New(store), dossierID
}

func getPermit(t *testing.T, service *workflow.Service) permitResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/permits/"+sharedPermitCode, nil)
	response := httptest.NewRecorder()
	httpapi.New(service).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("许可查询失败: status %d body %s", response.Code, response.Body.String())
	}
	var body permitResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
