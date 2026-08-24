package workflow

import (
	"errors"
	"sort"

	"stageguard/internal/domain"
)

type RevisionDiff struct {
	FromRevision       int      `json:"fromRevision"`
	ToRevision         int      `json:"toRevision"`
	RemediationChanged bool     `json:"remediationChanged"`
	RetestBefore       string   `json:"retestBefore"`
	RetestAfter        string   `json:"retestAfter"`
	AddedEvidence      []string `json:"addedEvidence"`
	RemovedEvidence    []string `json:"removedEvidence"`
}

func (s *Service) IssueRevision(issueID string, revision int) (domain.RemediationRevision, *RevisionDiff, error) {
	snap := s.store.Snapshot()
	x, ok := snap.Issues[issueID]
	if !ok {
		return domain.RemediationRevision{}, nil, errors.New("问题不存在")
	}
	if revision < 1 || revision > len(x.Revisions) {
		return domain.RemediationRevision{}, nil, errors.New("整改修订不存在")
	}
	current := x.Revisions[revision-1]
	if revision == 1 {
		return current, nil, nil
	}
	previous := x.Revisions[revision-2]
	diff := &RevisionDiff{FromRevision: previous.Revision, ToRevision: current.Revision, RemediationChanged: previous.Remediation != current.Remediation, RetestBefore: previous.RetestData, RetestAfter: current.RetestData}
	before := map[string]bool{}
	after := map[string]bool{}
	for _, e := range previous.Evidence {
		before[e.Reference] = true
	}
	for _, e := range current.Evidence {
		after[e.Reference] = true
	}
	for ref := range after {
		if !before[ref] {
			diff.AddedEvidence = append(diff.AddedEvidence, ref)
		}
	}
	for ref := range before {
		if !after[ref] {
			diff.RemovedEvidence = append(diff.RemovedEvidence, ref)
		}
	}
	sort.Strings(diff.AddedEvidence)
	sort.Strings(diff.RemovedEvidence)
	return current, diff, nil
}
