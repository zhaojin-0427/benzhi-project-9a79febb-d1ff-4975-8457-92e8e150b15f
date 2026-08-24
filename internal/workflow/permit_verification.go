package workflow

import (
	"errors"

	"stageguard/internal/domain"
)

type PermitVerification struct {
	Valid            bool                    `json:"valid"`
	ReasonCode       string                  `json:"reasonCode"`
	Message          string                  `json:"message"`
	Permit           domain.ActivationPermit `json:"permit"`
	Dossier          domain.SafetyDossier    `json:"dossier"`
	RecalculatedHash string                  `json:"recalculatedHash"`
}

func (s *Service) PermitByCode(code string) (domain.ActivationPermit, error) {
	for _, p := range s.store.Snapshot().Permits {
		if p.PermitCode == code {
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
