package idempotencypayloadcollision_test

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

func TestIdempotencyKeyRejectsDifferentPayload(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(workflow.New(store)).Handler()
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/dossiers", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", "same-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	first := post(`{"showName":"演出甲","venue":"一号厅","createdBy":"工程师","scheduledAt":"2030-01-01T10:00:00Z","equipmentBoundary":[{"id":"e1","name":"吊杆","ratedLoadKg":100,"isolationBoundary":"A区"}]}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("首次请求失败: status=%d body=%s", first.Code, first.Body.String())
	}
	second := post(`{"showName":"演出乙","venue":"二号厅","createdBy":"工程师","scheduledAt":"2030-01-02T10:00:00Z","equipmentBoundary":[{"id":"e2","name":"升降台","ratedLoadKg":200,"isolationBoundary":"B区"}]}`)
	if second.Code >= 200 && second.Code < 300 {
		t.Fatalf("相同 Idempotency-Key 携带不同请求体被静默当作成功重放: status=%d body=%s", second.Code, second.Body.String())
	}
}
