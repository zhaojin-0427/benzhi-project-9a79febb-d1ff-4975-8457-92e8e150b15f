package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"stageguard/internal/domain"
	"stageguard/internal/web"
	"stageguard/internal/workflow"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	service *workflow.Service
	idemMu  sync.Mutex
	idem    map[string]savedResponse
}

func New(service *workflow.Service) *Server {
	return &Server{service: service, idem: map[string]savedResponse{}}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/check-template", s.checkTemplate)
	mux.HandleFunc("/api/dossiers", s.dossiers)
	mux.HandleFunc("/api/dossiers/", s.dossierRoute)
	mux.HandleFunc("/api/issues/", s.issueRoute)
	mux.HandleFunc("/api/permits/", s.permitRoute)
	mux.HandleFunc("/api/audit", s.audit)
	mux.Handle("/", web.Handler())
	return limit(s.idempotentHandler(mux))
}
func (s *Server) checkTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		method(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": domain.InspectionTemplate()})
}

type savedResponse struct {
	status int
	header http.Header
	body   []byte
}
type captureWriter struct {
	header http.Header
	status int
	body   []byte
}

func (w *captureWriter) Header() http.Header { return w.header }
func (w *captureWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}
func (w *captureWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.body = append(w.body, b...)
	return len(b), nil
}
func (s *Server) idempotentHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			next.ServeHTTP(w, r)
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		cacheKey := r.URL.Path + "|" + key
		s.idemMu.Lock()
		defer s.idemMu.Unlock()
		saved, ok := s.idem[cacheKey]
		if ok {
			for k, v := range saved.header {
				w.Header()[k] = append([]string(nil), v...)
			}
			w.WriteHeader(saved.status)
			_, _ = w.Write(saved.body)
			return
		}
		cw := &captureWriter{header: make(http.Header)}
		next.ServeHTTP(cw, r)
		if cw.status >= 200 && cw.status < 300 {
			s.idem[cacheKey] = savedResponse{status: cw.status, header: cw.header, body: append([]byte(nil), cw.body...)}
		}
		for k, v := range cw.header {
			w.Header()[k] = v
		}
		w.WriteHeader(cw.status)
		_, _ = w.Write(cw.body)
	})
}
func limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}
func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		method(w)
		return
	}
	snap := s.service.Snapshot()
	writeJSON(w, http.StatusOK, snap)
}
func (s *Server) dossiers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		filter, err := dossierFilter(r)
		if err != nil {
			bad(w, err)
			return
		}
		out, totals, err := s.service.ListDossiers(filter)
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dossiers": out, "totals": totals})
	case "POST":
		var in struct {
			ShowName, Venue, CreatedBy string
			ScheduledAt                time.Time
			EquipmentBoundary          []domain.Equipment `json:"equipmentBoundary"`
		}
		if err := decode(r, &in); err != nil {
			bad(w, err)
			return
		}
		d, err := s.service.CreateDossier(workflow.CreateDossierCommand{ShowName: in.ShowName, Venue: in.Venue, CreatedBy: in.CreatedBy, IdempotencyKey: r.Header.Get("Idempotency-Key"), ScheduledAt: in.ScheduledAt, EquipmentBoundary: in.EquipmentBoundary})
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, d)
	default:
		method(w)
	}
}
func (s *Server) dossierRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/dossiers/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == "GET" {
		snap := s.service.Snapshot()
		d, ok := snap.Dossiers[id]
		if !ok {
			notFound(w)
			return
		}
		writeJSON(w, http.StatusOK, d)
		return
	}
	if len(parts) == 1 && (r.Method == "PATCH" || r.Method == "PUT") {
		var in workflow.ReviseDossierCommand
		if err := decode(r, &in); err != nil {
			bad(w, err)
			return
		}
		in.DossierID = id
		d, err := s.service.ReviseDossier(in)
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
		return
	}
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "inspections":
		s.inspections(w, r, id)
	case "issues":
		if len(parts) > 2 && parts[2] == "check" {
			s.detect(w, r, id)
		} else {
			s.issues(w, r, id)
		}
	case "review":
		s.review(w, r, id)
	case "freeze":
		s.freeze(w, r, id)
	case "permit":
		s.permit(w, r, id)
	case "audit":
		s.dossierAudit(w, r, id)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) inspections(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method == "GET" {
		snap := s.service.Snapshot()
		out := []domain.InspectionItem{}
		for _, item := range snap.Inspections {
			if item.DossierID == id {
				out = append(out, item)
			}
		}
		matrix, err := workflow.MatrixFor(snap, id)
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"inspections": out, "matrix": matrix})
		return
	}
	if r.Method != "POST" {
		method(w)
		return
	}
	var in struct {
		workflow.InspectionCommand
		Items []workflow.InspectionCommand `json:"items"`
		Actor string                       `json:"actor"`
	}
	if err := decode(r, &in); err != nil {
		bad(w, err)
		return
	}
	if len(in.Items) > 0 {
		result, err := s.service.RecordInspectionBatch(workflow.BatchInspectionCommand{DossierID: id, Actor: in.Actor, IdempotencyKey: r.Header.Get("Idempotency-Key"), ExpectedVersion: in.ExpectedVersion, Items: in.Items})
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
		return
	}
	in.InspectionCommand.DossierID = id
	i, err := s.service.RecordInspection(in.InspectionCommand)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, i)
}
func (s *Server) issues(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		method(w)
		return
	}
	out, err := s.service.IssuesForDossier(id)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": out})
}
func (s *Server) detect(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var in workflow.DetectionCommand
	if err := decode(r, &in); err != nil {
		bad(w, err)
		return
	}
	in.DossierID = id
	result, err := s.service.ReconcileIssues(in)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) review(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var in workflow.ReviewCommand
	if err := decode(r, &in); err != nil {
		bad(w, err)
		return
	}
	in.DossierID = id
	d, err := s.service.Review(in)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
func (s *Server) freeze(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "POST" {
		method(w)
		return
	}
	var in struct {
		Actor           string `json:"actor"`
		ExpectedVersion int    `json:"expectedVersion"`
	}
	if err := decode(r, &in); err != nil {
		bad(w, err)
		return
	}
	d, err := s.service.Freeze(id, in.Actor, in.ExpectedVersion)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
func (s *Server) permit(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method == "GET" {
		snap := s.service.Snapshot()
		var p []domain.ActivationPermit
		for _, x := range snap.Permits {
			if x.DossierID == id {
				p = append(p, x)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"permits": p})
		return
	}
	if r.Method != "POST" {
		method(w)
		return
	}
	var in struct {
		Actor           string `json:"actor"`
		ExpectedVersion int    `json:"expectedVersion"`
	}
	if err := decode(r, &in); err != nil {
		bad(w, err)
		return
	}
	p, err := s.service.IssuePermit(id, in.Actor, in.ExpectedVersion)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}
func (s *Server) issueRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/issues/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method == "GET" {
		snap := s.service.Snapshot()
		issue, ok := snap.Issues[parts[0]]
		if !ok {
			notFound(w)
			return
		}
		if len(parts) == 1 {
			writeJSON(w, http.StatusOK, issue)
			return
		}
		if len(parts) == 2 && parts[1] == "revisions" {
			writeJSON(w, http.StatusOK, map[string]any{"revisions": issue.Revisions})
			return
		}
		if len(parts) == 3 && parts[1] == "revisions" {
			revision, err := strconv.Atoi(parts[2])
			if err != nil {
				bad(w, errors.New("修订号无效"))
				return
			}
			item, diff, err := s.service.IssueRevision(parts[0], revision)
			if err != nil {
				bad(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"revision": item, "diff": diff})
			return
		}
		http.NotFound(w, r)
		return
	}
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" {
		method(w)
		return
	}
	if parts[1] == "remediation" {
		var in workflow.RemediationCommand
		if err := decode(r, &in); err != nil {
			bad(w, err)
			return
		}
		in.IssueID = parts[0]
		i, err := s.service.SubmitRemediation(in)
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusOK, i)
		return
	}
	if parts[1] == "review" {
		var in struct {
			Actor, Decision, Note string
			ExpectedVersion       int `json:"expectedVersion"`
		}
		if err := decode(r, &in); err != nil {
			bad(w, err)
			return
		}
		snap := s.service.Snapshot()
		_, ok := snap.Issues[parts[0]]
		if !ok {
			notFound(w)
			return
		}
		out, err := s.service.ReviewIssue(parts[0], in.Actor, in.Decision, in.Note, in.ExpectedVersion)
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	http.NotFound(w, r)
}
func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		method(w)
		return
	}
	page, pageSize, err := pageParams(r)
	if err != nil {
		bad(w, err)
		return
	}
	events, total, err := s.service.AuditPage(r.URL.Query().Get("dossierID"), r.URL.Query().Get("type"), page, pageSize)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "total": total, "page": page, "pageSize": pageSize})
}
func (s *Server) dossierAudit(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		method(w)
		return
	}
	page, pageSize, err := pageParams(r)
	if err != nil {
		bad(w, err)
		return
	}
	events, total, err := s.service.AuditPage(id, r.URL.Query().Get("type"), page, pageSize)
	if err != nil {
		bad(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "total": total, "page": page, "pageSize": pageSize})
}

func (s *Server) permitRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/permits/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != "GET" {
		method(w)
		return
	}
	if len(parts) == 2 && parts[1] == "verify" {
		result, err := s.service.VerifyPermit(parts[0])
		if err != nil {
			bad(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if len(parts) == 1 {
		permit, err := s.service.PermitByCode(parts[0])
		if err != nil {
			bad(w, err)
			return
		}
		snap := s.service.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{"permit": permit, "dossier": snap.Dossiers[permit.DossierID]})
		return
	}
	http.NotFound(w, r)
}

func dossierFilter(r *http.Request) (workflow.DossierFilter, error) {
	q := r.URL.Query()
	f := workflow.DossierFilter{Venue: q.Get("venue"), Keyword: q.Get("keyword"), Unissued: q.Get("unissued") == "true"}
	for _, raw := range q["status"] {
		for _, status := range strings.Split(raw, ",") {
			if strings.TrimSpace(status) != "" {
				f.Statuses = append(f.Statuses, domain.DossierStatus(strings.TrimSpace(status)))
			}
		}
	}
	if raw := q.Get("from"); raw != "" {
		v, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, errors.New("演出时间开始值格式无效")
		}
		f.From = &v
	}
	if raw := q.Get("to"); raw != "" {
		v, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return f, errors.New("演出时间结束值格式无效")
		}
		f.To = &v
	}
	return f, nil
}
func pageParams(r *http.Request) (int, int, error) {
	page, pageSize := 1, 20
	var err error
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.New("页码无效")
		}
	}
	if raw := r.URL.Query().Get("pageSize"); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil {
			return 0, 0, errors.New("每页条数无效")
		}
	}
	if page < 1 {
		return 0, 0, errors.New("页码必须大于等于1")
	}
	if pageSize < 1 || pageSize > 100 {
		return 0, 0, errors.New("每页条数必须在1到100之间")
	}
	return page, pageSize, nil
}
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return errors.New("请求 JSON 无效")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func bad(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if strings.HasPrefix(err.Error(), "版本冲突") {
		status = http.StatusConflict
	}
	if strings.Contains(err.Error(), "不存在") {
		status = http.StatusNotFound
	}
	var batch *workflow.BatchValidationError
	if errors.As(err, &batch) {
		writeJSON(w, status, map[string]any{"error": err.Error(), "rows": batch.Errors})
		return
	}
	var incomplete *workflow.IncompleteInspectionError
	if errors.As(err, &incomplete) {
		writeJSON(w, status, map[string]any{"error": err.Error(), "missingItems": incomplete.Missing})
		return
	}
	var blocked *workflow.FreezeBlockedError
	if errors.As(err, &blocked) {
		writeJSON(w, status, map[string]any{"error": err.Error(), "blockingIssues": blocked.Issues})
		return
	}
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "资源不存在"})
}
func method(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "请求方法不支持"})
}
