package dossier_list_stale_cache

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"stageguard/internal/domain"
	"stageguard/internal/httpapi"
	"stageguard/internal/storage"
	"stageguard/internal/workflow"
	"testing"
	"time"
)

func TestDossierListRefreshesAfterCreate(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(workflow.New(store)).Handler()

	assertListTotal(t, handler, 0)
	payload, err := json.Marshal(map[string]any{
		"showName":    "缓存后创建的演出",
		"venue":       "实验剧场",
		"createdBy":   "工程师",
		"scheduledAt": time.Now().UTC().Add(time.Hour),
		"equipmentBoundary": []domain.Equipment{{
			ID: "lift-01", Name: "升降台", RatedLoadKg: 800, IsolationBoundary: "北侧隔离线",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRequest(http.MethodPost, "/api/dossiers", bytes.NewReader(payload))
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("创建档案失败: status=%d body=%s", created.Code, created.Body.String())
	}

	assertListTotal(t, handler, 1)
}

func assertListTotal(t *testing.T, handler http.Handler, want int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/dossiers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询档案失败: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Dossiers []domain.SafetyDossier `json:"dossiers"`
		Totals   struct {
			Total int `json:"total"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Dossiers) != want || response.Totals.Total != want {
		t.Fatalf("TestDossierListRefreshesAfterCreate: 写入成功后列表未刷新: dossiers=%d total=%d want=%d", len(response.Dossiers), response.Totals.Total, want)
	}
}
