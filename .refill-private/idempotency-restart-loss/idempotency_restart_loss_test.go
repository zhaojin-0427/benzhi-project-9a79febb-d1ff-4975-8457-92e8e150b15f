package idempotencyrestartloss_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"stageguard/internal/httpapi"
	"stageguard/internal/storage"
	"stageguard/internal/workflow"
	"strings"
	"testing"
)

func post(handler http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/dossiers", strings.NewReader(body))
	req.Header.Set("Idempotency-Key", "create-once")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestIdempotentCreateSurvivesServerRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"showName":"测试演出","venue":"一号厅","createdBy":"工程师","scheduledAt":"2030-01-01T10:00:00Z","equipmentBoundary":[{"id":"e1","name":"吊杆","ratedLoadKg":100,"isolationBoundary":"A区"}]}`
	first := post(httpapi.New(workflow.New(store)).Handler(), body)
	if first.Code != http.StatusCreated {
		t.Fatalf("首次创建失败: status=%d body=%s", first.Code, first.Body.String())
	}
	reopened, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	second := post(httpapi.New(workflow.New(reopened)).Handler(), body)
	if second.Code != first.Code || second.Body.String() != first.Body.String() {
		t.Fatalf("服务重启后相同幂等请求未重放原响应: first=%s second=%s", first.Body.String(), second.Body.String())
	}
	if got := len(reopened.Snapshot().Dossiers); got != 1 {
		t.Fatalf("服务重启后相同幂等创建产生重复档案: got=%d want=1", got)
	}
}
