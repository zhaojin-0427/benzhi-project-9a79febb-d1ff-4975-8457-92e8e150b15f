package workflow

import (
	"errors"
	"sort"
	"strings"
	"time"

	"stageguard/internal/domain"
)

type DossierFilter struct {
	Statuses       []domain.DossierStatus
	Venue, Keyword string
	From, To       *time.Time
	Unissued       bool
}

type DossierListItem struct {
	Dossier              domain.SafetyDossier         `json:"dossier"`
	InspectionRatio      float64                      `json:"inspectionRatio"`
	UnresolvedBySeverity map[domain.IssueSeverity]int `json:"unresolvedBySeverity"`
	PendingReview        int                          `json:"pendingReview"`
	PermitStatus         string                       `json:"permitStatus"`
}

type DashboardTotals struct {
	ByStatus            map[domain.DossierStatus]int `json:"byStatus"`
	Next24HoursUnissued int                          `json:"next24HoursUnissued"`
	CriticalIssues      int                          `json:"criticalIssues"`
	Total               int                          `json:"total"`
}

func (s *Service) ListDossiers(f DossierFilter) ([]DossierListItem, DashboardTotals, error) {
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return nil, DashboardTotals{}, errors.New("演出时间开始值不能晚于结束值")
	}
	snap := s.store.Snapshot()
	statusSet := map[domain.DossierStatus]bool{}
	for _, x := range f.Statuses {
		statusSet[x] = true
	}
	items := []DossierListItem{}
	tot := DashboardTotals{ByStatus: map[domain.DossierStatus]int{}}
	now := time.Now()
	for _, d := range snap.Dossiers {
		if len(statusSet) > 0 && !statusSet[d.Status] {
			continue
		}
		if f.Venue != "" && !strings.Contains(strings.ToLower(d.Venue), strings.ToLower(f.Venue)) {
			continue
		}
		if f.Keyword != "" && !strings.Contains(strings.ToLower(d.ShowName+" "+d.Venue+" "+d.ID), strings.ToLower(f.Keyword)) {
			continue
		}
		if f.From != nil && d.ScheduledAt.Before(*f.From) {
			continue
		}
		if f.To != nil && d.ScheduledAt.After(*f.To) {
			continue
		}
		permitStatus := "未签发"
		for _, p := range snap.Permits {
			if p.DossierID == d.ID {
				permitStatus = "已签发"
				break
			}
		}
		if f.Unissued && permitStatus == "已签发" {
			continue
		}
		m, _ := MatrixFor(snap, d.ID)
		item := DossierListItem{Dossier: d, InspectionRatio: m.CompletionRatio, UnresolvedBySeverity: map[domain.IssueSeverity]int{}, PermitStatus: permitStatus}
		for _, x := range snap.Issues {
			if x.DossierID != d.ID || x.ReviewDecision == domain.ReviewPassed {
				continue
			}
			item.UnresolvedBySeverity[x.Severity]++
			if x.ReviewDecision == domain.ReviewPending {
				item.PendingReview++
			}
			if x.Severity == domain.SeverityCritical {
				tot.CriticalIssues++
			}
		}
		items = append(items, item)
		tot.ByStatus[d.Status]++
		tot.Total++
		if permitStatus != "已签发" && d.ScheduledAt.After(now) && d.ScheduledAt.Before(now.Add(24*time.Hour)) {
			tot.Next24HoursUnissued++
		}
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i].Dossier, items[j].Dossier
		if a.ScheduledAt.Equal(b.ScheduledAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ScheduledAt.Before(b.ScheduledAt)
	})
	return items, tot, nil
}

func sortIssues(items []domain.SafetyIssue) {
	rank := map[domain.IssueSeverity]int{domain.SeverityCritical: 0, domain.SeverityHigh: 1, domain.SeverityMedium: 2, domain.SeverityLow: 3}
	decisionRank := map[domain.ReviewDecision]int{domain.ReviewPending: 0, domain.ReviewReturned: 1, domain.ReviewPassed: 2}
	sort.Slice(items, func(i, j int) bool {
		if rank[items[i].Severity] != rank[items[j].Severity] {
			return rank[items[i].Severity] < rank[items[j].Severity]
		}
		if decisionRank[items[i].ReviewDecision] != decisionRank[items[j].ReviewDecision] {
			return decisionRank[items[i].ReviewDecision] < decisionRank[items[j].ReviewDecision]
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

func (s *Service) IssuesForDossier(dossierID string) ([]domain.SafetyIssue, error) {
	snap := s.store.Snapshot()
	dossier, ok := snap.Dossiers[dossierID]
	if !ok {
		return nil, errors.New("档案不存在")
	}
	s.issueCacheMu.RLock()
	cached, found := s.issueCache[dossierID]
	s.issueCacheMu.RUnlock()
	if found && cached.version == dossier.Version {
		return cached.items, nil
	}
	items := []domain.SafetyIssue{}
	for _, issue := range snap.Issues {
		if issue.DossierID == dossierID {
			items = append(items, issue)
		}
	}
	sortIssues(items)
	s.issueCacheMu.Lock()
	s.issueCache[dossierID] = issueQueryCacheEntry{version: dossier.Version, items: items}
	s.issueCacheMu.Unlock()
	return items, nil
}

func (s *Service) AuditPage(dossierID, eventType string, page, pageSize int) ([]domain.AuditEvent, int, error) {
	if page < 1 {
		return nil, 0, errors.New("页码必须大于等于1")
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, 0, errors.New("每页条数必须在1到100之间")
	}
	events := []domain.AuditEvent{}
	for _, e := range s.store.Snapshot().Events {
		if (dossierID == "" || e.DossierID == dossierID) && (eventType == "" || e.Type == eventType) {
			events = append(events, e)
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].ID > events[j].ID
		}
		return events[i].At.After(events[j].At)
	})
	total := len(events)
	start := (page - 1) * pageSize
	if start >= total {
		return []domain.AuditEvent{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return events[start:end], total, nil
}
