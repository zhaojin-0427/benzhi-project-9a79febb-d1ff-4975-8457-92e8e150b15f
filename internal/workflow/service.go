package workflow

import (
	"errors"
	"fmt"
	"stageguard/internal/domain"
	"stageguard/internal/storage"
	"strconv"
	"strings"
	"time"
)

type Service struct{ store *storage.Store }

func New(store *storage.Store) *Service      { return &Service{store: store} }
func (s *Service) Snapshot() domain.Snapshot { return s.store.Snapshot() }

type CreateDossierCommand struct {
	ShowName, Venue, CreatedBy string
	ScheduledAt                time.Time
	EquipmentBoundary          []domain.Equipment
	IdempotencyKey             string
}

func (s *Service) CreateDossier(c CreateDossierCommand) (domain.SafetyDossier, error) {
	if err := domain.ValidateDossierInput(c.ShowName, c.Venue, c.CreatedBy, c.ScheduledAt, c.EquipmentBoundary); err != nil {
		return domain.SafetyDossier{}, err
	}
	now := time.Now().UTC()
	d := domain.SafetyDossier{ID: newID("dos"), ShowName: strings.TrimSpace(c.ShowName), Venue: strings.TrimSpace(c.Venue), ScheduledAt: c.ScheduledAt, EquipmentBoundary: c.EquipmentBoundary, Status: domain.StatusDraft, Version: 1, CreatedBy: strings.TrimSpace(c.CreatedBy), UpdatedAt: now, Revisions: []domain.DossierRevision{}}
	key := strings.TrimSpace(c.IdempotencyKey)
	fp := ""
	if key != "" {
		fp = createDossierFingerprint(c)
	}
	dossier, _, err := s.store.CreateDossierWithReceipt(d, c.CreatedBy, key, fp)
	return dossier, err
}

type ReviseDossierCommand struct {
	DossierID, ShowName, Venue, Actor string
	ScheduledAt                       time.Time
	EquipmentBoundary                 []domain.Equipment
	ExpectedVersion                   int
}

func (s *Service) ReviseDossier(c ReviseDossierCommand) (domain.SafetyDossier, error) {
	if strings.TrimSpace(c.Actor) == "" {
		return domain.SafetyDossier{}, errors.New("操作者不能为空")
	}
	if err := domain.ValidateDossierInput(c.ShowName, c.Venue, c.Actor, c.ScheduledAt, c.EquipmentBoundary); err != nil {
		return domain.SafetyDossier{}, err
	}
	d, _, err := s.store.Mutate(c.ExpectedVersion, c.DossierID, c.Actor, "dossier.revised", "修订演出场次与设备隔离边界", func(st *domain.Snapshot) error {
		old, ok := st.Dossiers[c.DossierID]
		if !ok {
			return errors.New("档案不存在")
		}
		if old.Status != domain.StatusDraft {
			return fmt.Errorf("仅草稿状态允许修订，当前状态为%s", old.Status)
		}
		for _, item := range st.Inspections {
			if item.DossierID == c.DossierID {
				return errors.New("档案已有检查记录，不能修订设备边界")
			}
		}
		before := domain.DossierSummary(old)
		old.ShowName, old.Venue, old.ScheduledAt = strings.TrimSpace(c.ShowName), strings.TrimSpace(c.Venue), c.ScheduledAt
		old.EquipmentBoundary = append([]domain.Equipment(nil), c.EquipmentBoundary...)
		now := time.Now().UTC()
		old.Revisions = append(old.Revisions, domain.DossierRevision{Version: old.Version + 1, Actor: c.Actor, At: now, BeforeSummary: before, AfterSummary: domain.DossierSummary(old)})
		st.Dossiers[c.DossierID] = old
		return nil
	})
	return d, err
}

type InspectionCommand struct {
	ID, DossierID, EquipmentID, CheckCode, ObservedValue, Result, Inspector, Notes, EmergencyStopResult string
	MeasuredLoadKg                                                                                      float64
	LimitResponseMs                                                                                     int
	ExpectedVersion                                                                                     int
}

type BatchInspectionCommand struct {
	DossierID, Actor, IdempotencyKey string
	ExpectedVersion                  int
	Items                            []InspectionCommand
}
type BatchRowError struct {
	Row         int    `json:"row"`
	EquipmentID string `json:"equipmentID"`
	CheckCode   string `json:"checkCode"`
	Reason      string `json:"reason"`
}
type BatchValidationError struct {
	Errors []BatchRowError `json:"errors"`
}

func (e *BatchValidationError) Error() string {
	return fmt.Sprintf("批量检查有%d行无效", len(e.Errors))
}

type BatchInspectionResult struct {
	Version          int                     `json:"version"`
	Added            int                     `json:"added"`
	Corrected        int                     `json:"corrected"`
	Inspections      []domain.InspectionItem `json:"inspections"`
	IdempotentReplay bool                    `json:"idempotentReplay"`
}

func (s *Service) RecordInspection(c InspectionCommand) (domain.InspectionItem, error) {
	result, err := s.RecordInspectionBatch(BatchInspectionCommand{DossierID: c.DossierID, Actor: c.Inspector, ExpectedVersion: c.ExpectedVersion, Items: []InspectionCommand{c}})
	if err != nil {
		return domain.InspectionItem{}, err
	}
	return result.Inspections[0], nil
}

func (s *Service) RecordInspectionBatch(c BatchInspectionCommand) (BatchInspectionResult, error) {
	if len(c.Items) == 0 {
		return BatchInspectionResult{}, errors.New("批次至少包含一条检查结果")
	}
	if len(c.Items) > 200 {
		return BatchInspectionResult{}, errors.New("单批检查不能超过200条")
	}
	snap := s.store.Snapshot()
	d, ok := snap.Dossiers[c.DossierID]
	if !ok {
		return BatchInspectionResult{}, errors.New("档案不存在")
	}
	if d.Status != domain.StatusDraft && d.Status != domain.StatusInspecting {
		return BatchInspectionResult{}, errors.New("当前状态不允许录入检查")
	}
	fingerprint := fingerprint(c.Items)
	receiptKey := c.DossierID + "|" + strings.TrimSpace(c.IdempotencyKey)
	if c.IdempotencyKey != "" {
		if receipt, found := snap.BatchReceipts[receiptKey]; found {
			if receipt.Fingerprint != fingerprint {
				return BatchInspectionResult{}, errors.New("幂等键已用于不同的批量请求")
			}
			return resultFromReceipt(snap, receipt, true), nil
		}
	}
	prepared, rowErrors := validateBatch(d, c.Items)
	if len(rowErrors) > 0 {
		return BatchInspectionResult{}, &BatchValidationError{Errors: rowErrors}
	}
	actor := strings.TrimSpace(c.Actor)
	if actor == "" {
		actor = prepared[0].Inspector
	}
	added, corrected := 0, 0
	ids := make([]string, 0, len(prepared))
	previewAdded, previewCorrected := 0, 0
	for _, item := range prepared {
		if inspectionIDByKey(snap, domain.InspectionKey(c.DossierID, item.EquipmentID, item.CheckCode)) == "" {
			previewAdded++
		} else {
			previewCorrected++
		}
	}
	detail := fmt.Sprintf("批量检查新增%d项、更正%d项", previewAdded, previewCorrected)
	_, _, err := s.store.Mutate(c.ExpectedVersion, c.DossierID, actor, "inspection.batch_recorded", detail, func(st *domain.Snapshot) error {
		current := st.Dossiers[c.DossierID]
		if current.Status != domain.StatusDraft && current.Status != domain.StatusInspecting {
			return errors.New("当前状态不允许录入检查")
		}
		for _, item := range prepared {
			oldID := inspectionIDByKey(*st, domain.InspectionKey(c.DossierID, item.EquipmentID, item.CheckCode))
			if oldID != "" {
				item.ID = oldID
				corrected++
			} else {
				added++
			}
			item.RecordedAt = time.Now().UTC()
			st.Inspections[item.ID] = item
			ids = append(ids, item.ID)
		}
		current.Status = domain.StatusInspecting
		st.Dossiers[c.DossierID] = current
		if c.IdempotencyKey != "" {
			st.BatchReceipts[receiptKey] = domain.BatchReceipt{DossierID: c.DossierID, Fingerprint: fingerprint, Version: current.Version + 1, Added: added, Corrected: corrected, InspectionIDs: append([]string(nil), ids...)}
		}
		return nil
	})
	if err != nil {
		return BatchInspectionResult{}, err
	}
	out := s.store.Snapshot()
	result := BatchInspectionResult{Version: out.Dossiers[c.DossierID].Version, Added: added, Corrected: corrected}
	for _, id := range ids {
		result.Inspections = append(result.Inspections, out.Inspections[id])
	}
	return result, nil
}

func validateBatch(d domain.SafetyDossier, commands []InspectionCommand) ([]domain.InspectionItem, []BatchRowError) {
	items := make([]domain.InspectionItem, 0, len(commands))
	errs := []BatchRowError{}
	seen := map[string]bool{}
	for n, c := range commands {
		i := domain.InspectionItem{ID: c.ID, DossierID: d.ID, EquipmentID: strings.TrimSpace(c.EquipmentID), CheckCode: strings.TrimSpace(c.CheckCode), ObservedValue: c.ObservedValue, MeasuredLoadKg: c.MeasuredLoadKg, LimitResponseMs: c.LimitResponseMs, EmergencyStopResult: c.EmergencyStopResult, Result: c.Result, Inspector: strings.TrimSpace(c.Inspector), Notes: c.Notes, RecordedAt: time.Now().UTC()}
		if i.ID == "" {
			i.ID = newID("ins")
		}
		reason := ""
		if !equipmentExists(d, i.EquipmentID) {
			reason = "设备不属于该档案"
		} else if err := domain.ValidateInspection(i); err != nil {
			reason = err.Error()
		}
		key := domain.InspectionKey(d.ID, i.EquipmentID, i.CheckCode)
		if seen[key] {
			reason = "批次内设备与检查代码重复"
		}
		seen[key] = true
		if reason != "" {
			errs = append(errs, BatchRowError{Row: n + 1, EquipmentID: i.EquipmentID, CheckCode: i.CheckCode, Reason: reason})
		}
		items = append(items, i)
	}
	return items, errs
}

func equipmentExists(d domain.SafetyDossier, id string) bool {
	for _, e := range d.EquipmentBoundary {
		if e.ID == id {
			return true
		}
	}
	return false
}
func inspectionIDByKey(st domain.Snapshot, key string) string {
	for id, x := range st.Inspections {
		if domain.InspectionKey(x.DossierID, x.EquipmentID, x.CheckCode) == key {
			return id
		}
	}
	return ""
}
func resultFromReceipt(st domain.Snapshot, r domain.BatchReceipt, replay bool) BatchInspectionResult {
	out := BatchInspectionResult{Version: r.Version, Added: r.Added, Corrected: r.Corrected, IdempotentReplay: replay}
	for _, id := range r.InspectionIDs {
		if item, ok := st.Inspections[id]; ok {
			out.Inspections = append(out.Inspections, item)
		}
	}
	return out
}

type RetestValue struct {
	IssueID             string   `json:"issueID"`
	EquipmentID         string   `json:"equipmentID"`
	CheckCode           string   `json:"checkCode"`
	MeasuredLoadKg      *float64 `json:"measuredLoadKg,omitempty"`
	LimitResponseMs     *int     `json:"limitResponseMs,omitempty"`
	EmergencyStopResult string   `json:"emergencyStopResult,omitempty"`
}
type DetectionCommand struct {
	DossierID, Actor string
	ExpectedVersion  int
	RetestValues     []RetestValue
}
type DetectionResult struct {
	New                int                  `json:"new"`
	Continuing         int                  `json:"continuing"`
	PendingElimination int                  `json:"pendingElimination"`
	Reopened           int                  `json:"reopened"`
	Issues             []domain.SafetyIssue `json:"issues"`
	DetectedAt         time.Time            `json:"detectedAt"`
}
type IncompleteInspectionError struct {
	Missing []MatrixCell `json:"missingItems"`
}

func (e *IncompleteInspectionError) Error() string {
	return fmt.Sprintf("检查矩阵不完整，仍缺少%d项", len(e.Missing))
}

func (s *Service) DetectIssues(dossierID, actor string, expected int) ([]domain.SafetyIssue, error) {
	result, err := s.ReconcileIssues(DetectionCommand{DossierID: dossierID, Actor: actor, ExpectedVersion: expected})
	return result.Issues, err
}
func (s *Service) ReconcileIssues(c DetectionCommand) (DetectionResult, error) {
	snap := s.store.Snapshot()
	d, ok := snap.Dossiers[c.DossierID]
	if !ok {
		return DetectionResult{}, errors.New("档案不存在")
	}
	if d.Status != domain.StatusInspecting && d.Status != domain.StatusRemediation {
		return DetectionResult{}, errors.New("当前状态不允许自动检测")
	}
	matrix, _ := MatrixFor(snap, c.DossierID)
	if matrix.Missing > 0 {
		return DetectionResult{}, &IncompleteInspectionError{Missing: matrix.MissingItems}
	}
	effective := map[string]domain.InspectionItem{}
	for _, in := range snap.Inspections {
		if in.DossierID == c.DossierID {
			effective[domain.InspectionKey(c.DossierID, in.EquipmentID, in.CheckCode)] = in
		}
	}
	for n, r := range c.RetestValues {
		key := ""
		if r.IssueID != "" {
			issue, found := snap.Issues[r.IssueID]
			if !found || issue.DossierID != c.DossierID {
				return DetectionResult{}, fmt.Errorf("第%d条复测值关联问题无效", n+1)
			}
			key = domain.InspectionKey(c.DossierID, issue.EquipmentID, issue.CheckCode)
		} else {
			key = domain.InspectionKey(c.DossierID, r.EquipmentID, r.CheckCode)
		}
		item, found := effective[key]
		if !found {
			return DetectionResult{}, fmt.Errorf("第%d条复测值没有对应检查项", n+1)
		}
		switch item.CheckCode {
		case "LOAD_LIMIT":
			if r.MeasuredLoadKg == nil || *r.MeasuredLoadKg <= 0 {
				return DetectionResult{}, fmt.Errorf("第%d条载荷复测值无效", n+1)
			}
			item.MeasuredLoadKg = *r.MeasuredLoadKg
		case "LIMIT_RESPONSE":
			if r.LimitResponseMs == nil || *r.LimitResponseMs <= 0 {
				return DetectionResult{}, fmt.Errorf("第%d条限位复测值无效", n+1)
			}
			item.LimitResponseMs = *r.LimitResponseMs
		case "EMERGENCY_STOP":
			if r.EmergencyStopResult != "通过" && r.EmergencyStopResult != "失败" {
				return DetectionResult{}, fmt.Errorf("第%d条急停复测值无效", n+1)
			}
			item.EmergencyStopResult = r.EmergencyStopResult
		}
		effective[key] = item
	}
	now := time.Now().UTC()
	hits := map[string]issueHit{}
	for _, in := range effective {
		for _, f := range rulesFor(in, d) {
			key := domain.IssueKey(c.DossierID, in.EquipmentID, in.CheckCode, f.code)
			hits[key] = issueHit{item: in, finding: f}
		}
	}
	result := DetectionResult{DetectedAt: now}
	_, _, err := s.store.Mutate(c.ExpectedVersion, c.DossierID, c.Actor, "issues.reconciled", "风险规则对账", func(st *domain.Snapshot) error {
		byKey := map[string]string{}
		for id, x := range st.Issues {
			if x.DossierID == c.DossierID {
				if x.StableKey == "" {
					x.StableKey = domain.IssueKey(x.DossierID, x.EquipmentID, x.CheckCode, x.RuleCode)
					st.Issues[id] = x
				}
				byKey[x.StableKey] = id
			}
		}
		for key, hit := range hits {
			if id, found := byKey[key]; found {
				x := st.Issues[id]
				wasElimination := x.PendingElimination
				wasClosed := x.ReviewDecision == domain.ReviewPassed
				x.Severity = hit.finding.severity
				x.Description = hit.finding.description
				x.LastDetectedAt = &now
				x.PendingElimination = false
				x.UpdatedAt = now
				if wasElimination || wasClosed {
					x.ReopenCount++
					x.ReviewDecision = domain.ReviewPending
					x.ResolvedAt = nil
					result.Reopened++
				} else {
					result.Continuing++
				}
				st.Issues[id] = x
			} else {
				x := domain.SafetyIssue{ID: newID("iss"), DossierID: c.DossierID, InspectionItemID: hit.item.ID, EquipmentID: hit.item.EquipmentID, CheckCode: hit.item.CheckCode, StableKey: key, Severity: hit.finding.severity, RuleCode: hit.finding.code, Description: hit.finding.description, ReviewDecision: domain.ReviewPending, UpdatedAt: now, LastDetectedAt: &now, Revisions: []domain.RemediationRevision{}}
				st.Issues[x.ID] = x
				result.New++
			}
		}
		for id, x := range st.Issues {
			if x.DossierID != c.DossierID {
				continue
			}
			if _, hit := hits[x.StableKey]; !hit && x.ReviewDecision != domain.ReviewPassed {
				if !x.PendingElimination {
					result.PendingElimination++
				}
				x.PendingElimination = true
				x.ReviewDecision = domain.ReviewPending
				x.UpdatedAt = now
				st.Issues[id] = x
			}
		}
		dd := st.Dossiers[c.DossierID]
		dd.LastDetectedAt = &now
		if len(hits) > 0 {
			dd.Status = domain.StatusRemediation
		} else {
			dd.Status = domain.StatusReview
		}
		st.Dossiers[c.DossierID] = dd
		return nil
	})
	if err != nil {
		return DetectionResult{}, err
	}
	out := s.store.Snapshot()
	for _, x := range out.Issues {
		if x.DossierID == c.DossierID {
			result.Issues = append(result.Issues, x)
		}
	}
	sortIssues(result.Issues)
	return result, nil
}

type RemediationCommand struct {
	IssueID, Remediation, RetestData, Actor string
	EvidenceRefs                            []string
	Evidence                                []domain.Evidence `json:"evidence"`
	ExpectedVersion                         int
}

func (s *Service) SubmitRemediation(c RemediationCommand) (domain.SafetyIssue, error) {
	if strings.TrimSpace(c.Remediation) == "" || strings.TrimSpace(c.RetestData) == "" {
		return domain.SafetyIssue{}, errors.New("整改说明和复测数据不能为空")
	}
	snap := s.store.Snapshot()
	issue, ok := snap.Issues[c.IssueID]
	if !ok {
		return domain.SafetyIssue{}, errors.New("问题不存在")
	}
	if issue.PendingElimination {
		return domain.SafetyIssue{}, errors.New("待确认消除问题无需提交整改，请由安全员复核")
	}
	if len(c.Evidence) == 0 && len(c.EvidenceRefs) > 0 {
		for _, ref := range c.EvidenceRefs {
			c.Evidence = append(c.Evidence, domain.Evidence{Name: "兼容证据", Type: domain.EvidencePhoto, CollectedAt: time.Now().UTC(), Reference: ref, Digest: "旧版证据引用"})
		}
	}
	if err := domain.ValidateEvidence(c.Evidence, time.Now().UTC()); err != nil {
		return domain.SafetyIssue{}, err
	}
	if missing := domain.RequiredEvidence(issue.Severity, c.Evidence); len(missing) > 0 {
		return domain.SafetyIssue{}, fmt.Errorf("缺少必需证据类型: %s", strings.Join(missing, "、"))
	}
	_, _, err := s.store.Mutate(c.ExpectedVersion, issue.DossierID, c.Actor, "issue.remediated", fmt.Sprintf("问题%s提交第%d次整改修订", issue.ID, issue.Revision+1), func(st *domain.Snapshot) error {
		x := st.Issues[c.IssueID]
		x.Remediation = c.Remediation
		x.RetestData = c.RetestData
		x.Evidence = append([]domain.Evidence(nil), c.Evidence...)
		x.EvidenceRefs = nil
		for _, e := range c.Evidence {
			x.EvidenceRefs = append(x.EvidenceRefs, e.Reference)
		}
		x.Revision++
		x.ReviewDecision = domain.ReviewPending
		x.ReviewNote = ""
		x.ReviewedRevision = 0
		x.ReviewedBy = ""
		x.ReviewedAt = nil
		x.ResolvedAt = nil
		x.UpdatedAt = time.Now().UTC()
		x.Revisions = append(x.Revisions, domain.RemediationRevision{Revision: x.Revision, Remediation: c.Remediation, RetestData: c.RetestData, EvidenceRefs: append([]string(nil), x.EvidenceRefs...), Evidence: append([]domain.Evidence(nil), c.Evidence...), SubmittedBy: c.Actor, SubmittedAt: x.UpdatedAt, Decision: domain.ReviewPending})
		st.Issues[c.IssueID] = x
		dd := st.Dossiers[x.DossierID]
		if allEvidenceReady(*st, x.DossierID) {
			dd.Status = domain.StatusReview
		} else {
			dd.Status = domain.StatusRemediation
		}
		st.Dossiers[x.DossierID] = dd
		return nil
	})
	if err != nil {
		return domain.SafetyIssue{}, err
	}
	return s.store.Snapshot().Issues[c.IssueID], nil
}
func allEvidenceReady(st domain.Snapshot, dossierID string) bool {
	found := false
	for _, x := range st.Issues {
		if x.DossierID != dossierID || x.ReviewDecision == domain.ReviewPassed {
			continue
		}
		found = true
		if x.PendingElimination {
			continue
		}
		if x.Revision == 0 || len(domain.RequiredEvidence(x.Severity, x.Evidence)) > 0 {
			return false
		}
	}
	return found
}

type ReviewCommand struct {
	DossierID, Actor, Decision, Note string
	ExpectedVersion                  int
}

func (s *Service) Review(c ReviewCommand) (domain.SafetyDossier, error) {
	if c.Decision == string(domain.ReviewReturned) {
		return domain.SafetyDossier{}, errors.New("整档退回已停用，请逐项退回问题")
	}
	if c.Decision != string(domain.ReviewPassed) {
		return domain.SafetyDossier{}, errors.New("复核决定只能是通过或退回")
	}
	return s.Freeze(c.DossierID, c.Actor, c.ExpectedVersion)
}
func (s *Service) ReviewIssue(issueID, actor, decision, note string, expected int) (domain.SafetyIssue, error) {
	if decision != string(domain.ReviewPassed) && decision != string(domain.ReviewReturned) {
		return domain.SafetyIssue{}, errors.New("问题复核决定无效")
	}
	if strings.TrimSpace(actor) == "" {
		return domain.SafetyIssue{}, errors.New("复核人不能为空")
	}
	if decision == string(domain.ReviewReturned) && strings.TrimSpace(note) == "" {
		return domain.SafetyIssue{}, errors.New("退回复核必须填写原因")
	}
	snap := s.store.Snapshot()
	i, ok := snap.Issues[issueID]
	if !ok {
		return domain.SafetyIssue{}, errors.New("问题不存在")
	}
	if !i.PendingElimination && i.Revision == 0 {
		return domain.SafetyIssue{}, errors.New("问题尚无完整整改修订，不能复核")
	}
	if !i.PendingElimination && len(domain.RequiredEvidence(i.Severity, i.Evidence)) > 0 {
		return domain.SafetyIssue{}, errors.New("问题整改证据不完整，不能复核")
	}
	now := time.Now().UTC()
	_, _, err := s.store.Mutate(expected, i.DossierID, actor, "issue.reviewed", fmt.Sprintf("问题%s逐项复核：%s", issueID, decision), func(st *domain.Snapshot) error {
		x := st.Issues[issueID]
		x.ReviewDecision = domain.ReviewDecision(decision)
		x.ReviewNote = note
		x.ReviewedBy = actor
		x.ReviewedAt = &now
		x.ReviewedRevision = x.Revision
		if decision == string(domain.ReviewPassed) {
			x.ResolvedAt = &now
		} else {
			x.ResolvedAt = nil
			x.PendingElimination = false
		}
		if len(x.Revisions) > 0 {
			n := len(x.Revisions) - 1
			x.Revisions[n].Decision = domain.ReviewDecision(decision)
			x.Revisions[n].ReviewNote = note
			x.Revisions[n].ReviewedBy = actor
			x.Revisions[n].ReviewedAt = &now
		}
		x.UpdatedAt = now
		st.Issues[issueID] = x
		dd := st.Dossiers[x.DossierID]
		if decision == string(domain.ReviewReturned) {
			dd.Status = domain.StatusRemediation
		} else if allIssuesReviewed(*st, x.DossierID) {
			dd.Status = domain.StatusReview
		}
		st.Dossiers[x.DossierID] = dd
		return nil
	})
	if err != nil {
		return domain.SafetyIssue{}, err
	}
	return s.store.Snapshot().Issues[issueID], nil
}
func allIssuesReviewed(st domain.Snapshot, dossierID string) bool {
	for _, x := range st.Issues {
		if x.DossierID == dossierID && (x.ReviewDecision != domain.ReviewPassed || x.ReviewedRevision != x.Revision) {
			return false
		}
	}
	return true
}

type FreezeBlockedError struct {
	Issues []FreezeBlock `json:"issues"`
}
type FreezeBlock struct {
	IssueID string `json:"issueID"`
	Reason  string `json:"reason"`
}

func (e *FreezeBlockedError) Error() string {
	return fmt.Sprintf("仍有%d个问题阻塞冻结", len(e.Issues))
}
func (s *Service) Freeze(dossierID, actor string, expected int) (domain.SafetyDossier, error) {
	snap := s.store.Snapshot()
	d, ok := snap.Dossiers[dossierID]
	if !ok {
		return domain.SafetyDossier{}, errors.New("档案不存在")
	}
	if d.Status != domain.StatusReview {
		return domain.SafetyDossier{}, errors.New("档案必须处于待复核状态")
	}
	blocks := []FreezeBlock{}
	for _, x := range snap.Issues {
		if x.DossierID != dossierID {
			continue
		}
		reason := ""
		if x.ReviewDecision != domain.ReviewPassed {
			reason = "问题尚未通过逐项复核"
		} else if x.ReviewedRevision != x.Revision {
			reason = "通过决定对应的整改修订已过期"
		}
		if reason != "" {
			blocks = append(blocks, FreezeBlock{IssueID: x.ID, Reason: reason})
		}
	}
	if len(blocks) > 0 {
		return domain.SafetyDossier{}, &FreezeBlockedError{Issues: blocks}
	}
	out, _, err := s.store.Mutate(expected, dossierID, actor, "dossier.frozen", "全部当前有效问题已逐项通过，冻结候选安全版本", func(st *domain.Snapshot) error {
		dd := st.Dossiers[dossierID]
		dd.Status = domain.StatusFrozen
		dd.Version++
		st.Dossiers[dossierID] = dd
		dd.FrozenHash = domain.HashDossierContent(*st, dossierID, dd.Version)
		dd.Version--
		st.Dossiers[dossierID] = dd
		return nil
	})
	return out, err
}

type existingPermitError struct{ permit domain.ActivationPermit }

func (e *existingPermitError) Error() string { return "档案已有许可" }

func (s *Service) IssuePermit(dossierID, actor string, expected int) (domain.ActivationPermit, error) {
	snap := s.store.Snapshot()
	for _, p := range snap.Permits {
		if p.DossierID == dossierID {
			return p, nil
		}
	}
	d, ok := snap.Dossiers[dossierID]
	if !ok {
		return domain.ActivationPermit{}, errors.New("档案不存在")
	}
	if d.Status != domain.StatusFrozen {
		return domain.ActivationPermit{}, errors.New("档案必须冻结后才能签发许可")
	}
	if d.FrozenHash == "" {
		return domain.ActivationPermit{}, errors.New("冻结版本缺少校验哈希")
	}
	eventIDs := []string{}
	for _, e := range snap.Events {
		if e.DossierID == dossierID {
			eventIDs = append(eventIDs, e.ID)
		}
	}
	p := domain.ActivationPermit{ID: newID("permit"), DossierID: dossierID, FrozenVersion: d.Version, IssuedBy: actor, IssuedAt: time.Now().UTC(), ContentHash: d.FrozenHash, PermitCode: "SG-" + strconv.FormatInt(time.Now().UnixNano(), 10), AuditEventIDs: eventIDs}
	_, _, err := s.store.Mutate(expected, dossierID, actor, "permit.issued", "签发不可变启用许可", func(st *domain.Snapshot) error {
		for _, old := range st.Permits {
			if old.DossierID == dossierID {
				return &existingPermitError{permit: old}
			}
		}
		st.Permits[p.ID] = p
		dd := st.Dossiers[dossierID]
		if dd.Status != domain.StatusFrozen {
			return errors.New("档案状态已变化，不能签发许可")
		}
		dd.Status = domain.StatusIssued
		st.Dossiers[dossierID] = dd
		return nil
	})
	if err != nil {
		var existing *existingPermitError
		if errors.As(err, &existing) {
			return existing.permit, nil
		}
		for _, existing := range s.store.Snapshot().Permits {
			if existing.DossierID == dossierID {
				return existing, nil
			}
		}
		return domain.ActivationPermit{}, err
	}
	return p, nil
}
