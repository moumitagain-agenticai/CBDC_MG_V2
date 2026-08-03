package domain

import "testing"

func TestCanTransition_Legal(t *testing.T) {
	legal := [][2]TransferStatus{
		{StatusInitiated, StatusLocked},
		{StatusInitiated, StatusFailed},
		{StatusLocked, StatusReleased},
		{StatusLocked, StatusCompensating},
		{StatusReleased, StatusBurned},
		{StatusCompensating, StatusCompensated},
	}
	for _, e := range legal {
		if !CanTransition(e[0], e[1]) {
			t.Errorf("expected %s -> %s to be legal", e[0], e[1])
		}
	}
}

func TestCanTransition_Illegal(t *testing.T) {
	illegal := [][2]TransferStatus{
		{StatusReleased, StatusCompensating}, // no rollback after release
		{StatusReleased, StatusCompensated},
		{StatusBurned, StatusReleased},
		{StatusInitiated, StatusReleased}, // must lock first
		{StatusCompensated, StatusBurned},
		{StatusFailed, StatusLocked},
	}
	for _, e := range illegal {
		if CanTransition(e[0], e[1]) {
			t.Errorf("expected %s -> %s to be illegal", e[0], e[1])
		}
	}
}

func TestTransition_RejectsIllegal(t *testing.T) {
	tr := &Transfer{Status: StatusReleased}
	if err := tr.Transition(StatusCompensating); err == nil {
		t.Fatal("expected illegal transition to error")
	}
	if tr.Status != StatusReleased {
		t.Fatalf("status must be unchanged after illegal transition, got %s", tr.Status)
	}
}

func TestTransition_IdempotentNoOp(t *testing.T) {
	tr := &Transfer{Status: StatusLocked}
	if err := tr.Transition(StatusLocked); err != nil {
		t.Fatalf("same-state transition should be a no-op, got %v", err)
	}
}

func TestStatusHelpers(t *testing.T) {
	if !StatusBurned.IsTerminal() || !StatusCompensated.IsTerminal() || !StatusFailed.IsTerminal() {
		t.Error("terminal states misclassified")
	}
	if StatusLocked.IsTerminal() {
		t.Error("LOCKED should not be terminal")
	}
	if !StatusReleased.InFlight() || !StatusLocked.InFlight() {
		t.Error("in-flight states misclassified")
	}
	if StatusBurned.InFlight() {
		t.Error("BURNED should not be in-flight")
	}
}
