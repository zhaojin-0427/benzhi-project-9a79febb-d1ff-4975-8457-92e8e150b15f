package workflow

import (
	"errors"
	"sync"

	"stageguard/internal/domain"
)

// permitLookupCache caches permits resolved by PermitCode for a single Service.
// The cache is scoped to the Service (and therefore to the store/ledger it
// wraps) so permits from one ledger can never leak into a query served by a
// different Service instance sharing the same process.
type permitLookupCache struct {
	mu sync.Mutex
	m  map[string]domain.ActivationPermit
}

func newPermitLookupCache() *permitLookupCache {
	return &permitLookupCache{m: map[string]domain.ActivationPermit{}}
}

type PermitVerification struct {
	Valid            bool                    `json:"valid"`
	ReasonCode       string                  `json:"reasonCode"`
	Message          string                  `json:"message"`
	Permit           domain.ActivationPermit `json:"permit"`
	Dossier          domain.SafetyDossier    `json:"dossier"`
	RecalculatedHash string                  `json:"recalculatedHash"`
}

func (s *Service) PermitByCode(code string) (domain.ActivationPermit, error) {
	if cached, ok := s.permitCache.lookup(code); ok {
		return cached, nil
	}
	for _, p := range s.store.Snapshot().Permits {
		if p.PermitCode == code {
			s.permitCache.store(code, p)
			return p, nil
		}
	}
	return domain.ActivationPermit{}, errors.New("许可不存在")
}

func (s *Service) VerifyPermit(code string) (PermitVerification, error) {
	snap := s.store.Snapshot()
	p, err := s.PermitByCode(code)
	if err != nil {
		return PermitVerification{}, err
	}
	d, ok := snap.Dossiers[p.DossierID]
	if !ok {
		return PermitVerification{ReasonCode: "FREEZE_VERSION_MISMATCH", Message: "冻结版本失配", Permit: p}, nil
	}
	out := PermitVerification{Permit: p, Dossier: d}
	if d.Version != p.FrozenVersion+1 || d.Status != domain.StatusIssued {
		out.ReasonCode = "FREEZE_VERSION_MISMATCH"
		out.Message = "冻结版本失配"
		return out, nil
	}
	events := map[string]domain.AuditEvent{}
	for _, e := range snap.Events {
		events[e.ID] = e
	}
	for _, id := range p.AuditEventIDs {
		e, found := events[id]
		if !found || e.DossierID != p.DossierID {
			out.ReasonCode = "AUDIT_REFERENCE_MISSING"
			out.Message = "审计引用缺失"
			return out, nil
		}
	}
	out.RecalculatedHash = domain.HashDossierContent(snap, p.DossierID, p.FrozenVersion)
	if out.RecalculatedHash != p.ContentHash {
		out.ReasonCode = "CONTENT_HASH_MISMATCH"
		out.Message = "内容哈希失配"
		return out, nil
	}
	out.Valid = true
	out.ReasonCode = "VALID"
	out.Message = "许可有效"
	return out, nil
}

func (c *permitLookupCache) lookup(code string) (domain.ActivationPermit, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.m[code]
	return p, ok
}

func (c *permitLookupCache) store(code string, p domain.ActivationPermit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[code] = p
}
