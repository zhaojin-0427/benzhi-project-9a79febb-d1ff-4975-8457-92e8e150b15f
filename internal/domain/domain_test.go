package domain

import "testing"

func TestStateTransitions(t *testing.T) {
	cases := []struct {
		from, to DossierStatus
		ok       bool
	}{{StatusDraft, StatusInspecting, true}, {StatusInspecting, StatusRemediation, true}, {StatusReview, StatusFrozen, true}, {StatusFrozen, StatusIssued, true}, {StatusDraft, StatusIssued, false}, {StatusIssued, StatusDraft, false}}
	for _, tc := range cases {
		if got := CanTransition(tc.from, tc.to); got != tc.ok {
			t.Fatalf("%s -> %s = %v", tc.from, tc.to, got)
		}
	}
}
func TestEquipmentValidation(t *testing.T) {
	if err := ValidateEquipment(Equipment{ID: "lift", Name: "升降台", RatedLoadKg: 1000, IsolationBoundary: "东侧"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEquipment(Equipment{ID: "lift", Name: "升降台", RatedLoadKg: 0, IsolationBoundary: "东侧"}); err == nil {
		t.Fatal("应拒绝零额定载荷")
	}
}
