package domain

import "fmt"

func CanTransition(from, to DossierStatus) bool {
	switch from {
	case StatusDraft:
		return to == StatusInspecting
	case StatusInspecting:
		return to == StatusRemediation || to == StatusReview
	case StatusRemediation:
		return to == StatusReview || to == StatusInspecting
	case StatusReview:
		return to == StatusRemediation || to == StatusFrozen
	case StatusFrozen:
		return to == StatusIssued
	}
	return false
}

func (d SafetyDossier) Transition(to DossierStatus) error {
	if !CanTransition(d.Status, to) {
		return fmt.Errorf("状态不能从%s转为%s", d.Status, to)
	}
	return nil
}
